/*

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllersdi

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	hubv1 "github.com/gardener/potter-controller/api/v1"
	"github.com/gardener/potter-controller/pkg/avcheck"
	"github.com/gardener/potter-controller/pkg/deployutil"
	"github.com/gardener/potter-controller/pkg/synchronize"
	"github.com/gardener/potter-controller/pkg/util"

	"github.com/gardener/landscaper/pkg/apis/core/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/yaml"
)

// TODO
// only one definition of helm specific data
// remove appRepoClient member from deploymentReconciler

// DeploymentReconciler reconciles a DeployItem
type DeploymentReconciler struct {
	deployerFactory   DeployerFactory
	crAndSecretClient client.Client
	log               logr.Logger
	scheme            *runtime.Scheme
	threadCounterLog  *util.ThreadCounterMap
	blockObject       *synchronize.BlockObject
	avCheck           *avcheck.AVCheck
	uncachedClient    synchronize.UncachedClient
	eventRecorder     record.EventRecorder
}

func NewDeploymentReconciler(deployerFactory DeployerFactory, crAndSecretClient client.Client, log logr.Logger,
	scheme *runtime.Scheme, threadCounterLog *util.ThreadCounterMap, blockObject *synchronize.BlockObject,
	avCheck *avcheck.AVCheck, uncachedClient synchronize.UncachedClient, eventRecorder record.EventRecorder) *DeploymentReconciler {
	return &DeploymentReconciler{
		deployerFactory:   deployerFactory,
		crAndSecretClient: crAndSecretClient,
		log:               log,
		scheme:            scheme,
		threadCounterLog:  threadCounterLog,
		blockObject:       blockObject,
		avCheck:           avCheck,
		uncachedClient:    uncachedClient,
		eventRecorder:     eventRecorder,
	}
}

func (r *DeploymentReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	if r.avCheck != nil {
		r.avCheck.ReconcileCalled()
	}

	if r.threadCounterLog != nil {
		r.threadCounterLog.IncreaseEntryAndLog(req.Namespace)
		defer r.threadCounterLog.ReduceEntry(req.Namespace)
	}

	ctx, log := util.NewContextAndLogger(r.log,
		util.LogKeyDeployItemName, req.NamespacedName,
		util.LogKeyCorrelationID, uuid.New().String())

	ctx = context.WithValue(ctx, util.CRAndSecretClientKey{}, r.crAndSecretClient)

	log.V(util.LogLevelDebug).Info("Reconciling DeployItem")

	// block
	clusterBomKey := util.GetClusterBomKeyFromDeployItemKey(&req.NamespacedName)
	ok, requeueDuration, err := r.blockObject.Block(ctx, clusterBomKey, r.uncachedClient, 5*time.Minute, true)
	if err != nil {
		return r.returnFailure()
	} else if !ok {
		return ctrl.Result{RequeueAfter: requeueDuration}, nil
	}
	defer r.blockObject.Release(ctx, clusterBomKey, true)

	log.V(util.LogLevelDebug).Info("Processing DeployItem")

	ctx = r.setupEventWriter(ctx, clusterBomKey)

	deployItem := &v1alpha1.DeployItem{}
	err = r.crAndSecretClient.Get(ctx, req.NamespacedName, deployItem)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return r.logAndReturnHubFailure(ctx, deployutil.ReasonFailedFetchingObject, "Could not read deployment config", err)
	}

	deployData, err := deployutil.NewDeployData(deployItem)
	if err != nil {
		log.Error(err, "error unmarshaling data of deployitem")
		return r.returnFailure()
	}

	deployer, err := r.deployerFactory.GetDeployer(string(deployItem.Spec.Type))
	if err != nil {
		log.Error(err, "wrong configtype")
		return r.returnSuccess()
	}

	disablePreprocess := util.GetEnvBool("DISABLE_DEPLOY_PREPROCESS", false, r.log)
	if !disablePreprocess {
		deployer.Preprocess(ctx, deployData)
	}

	if deployData.IsDeleteOperation() {
		secretKey := deployData.GetSecretKey()
		var clusterExists bool
		clusterExists, err = deployutil.DoesTargetClusterExist(ctx, r.crAndSecretClient, *secretKey)
		if err != nil {
			return r.returnFailure()
		}

		if !clusterExists {
			if err = deployer.Cleanup(ctx, deployData, false); err != nil {
				return r.returnFailure()
			}

			if err = r.removeFinalizer(ctx, deployItem); err != nil {
				return r.returnFailure()
			}

			log.V(util.LogLevelWarning).Info("removed finalizer from hdc, because target cluster does not exist")
			return r.returnSuccess()
		}
	}

	deployItem, err = r.addFinalizer(ctx, deployItem)
	if err != nil {
		return r.returnFailure()
	}

	requeue, duration := r.calculateRequeueDurationForUnreachableCluster(deployData.ProviderStatus)
	if requeue {
		// Less than 5 minutes ago, the cluster was unreachable. That is too early to process the HDC again.
		// Therefore we requeue it for a few minutes.
		log.V(util.LogLevelWarning).Info("Event for unreachable cluster requeued")
		return ctrl.Result{RequeueAfter: *duration}, nil
	}

	lastOp := deployData.ProviderStatus.LastOperation

	if deployData.IsNewOperation() {
		log.V(util.LogLevelDebug).Info("new operation number", "observedGeneration",
			deployData.GetObservedGeneration(), "generation", deployData.GetGeneration())

		deployer.ProcessNewOperation(ctx, deployData)

		return r.updateStatus(ctx, deployData)
	} else if deployData.IsFinallyFailed() {
		log.V(util.LogLevelWarning).Info("Deployment finally failed", "observedGeneration",
			deployData.GetObservedGeneration(), "generation", deployData.GetGeneration())

		return ctrl.Result{}, nil
	} else if deployData.IsLastDeployFailed() {
		log.V(util.LogLevelWarning).Info("lastOp.State == failed", "observedGeneration",
			deployData.GetObservedGeneration(), "generation", deployData.GetGeneration())

		requeue, duration := util.CalculateRequeueDurationForPrematureRetry(&lastOp)
		if requeue {
			log.V(util.LogLevelDebug).Info("Too early for retry", "requeue-duration", duration)
			return ctrl.Result{RequeueAfter: *duration}, nil
		}

		deployer.RetryFailedOperation(ctx, deployData)

		return r.updateStatus(ctx, deployData)
	} else if deployData.IsReconcile() {
		deployer.ReconcileOperation(ctx, deployData)

		return r.updateStatus(ctx, deployData)
	} else if deployData.IsInstallButNotReady() {
		// here lastOp.State == util.StateOk holds automatically because r.isLastDeployFailed(lastOp) was checked before

		log.V(util.LogLevelDebug).Info("readiness is not ok", "observedGeneration",
			deployData.GetObservedGeneration(), "generation", deployData.GetGeneration())

		requeue, duration := r.calculateRequeueDurationForNotReadyInstall(deployData.ProviderStatus)
		if requeue {
			log.V(util.LogLevelDebug).Info("event for install that is not ok requeued")
			return ctrl.Result{RequeueAfter: *duration}, nil
		}

		deployer.ProcessPendingOperation(ctx, deployData)

		return r.updateStatus(ctx, deployData)
	} else if lastOp.Operation == util.OperationRemove {
		// here lastOp.State == util.StateOk holds automatically because r.isLastDeployFailed(lastOp) was checked before

		if err := r.removeFinalizer(ctx, deployItem); err != nil {
			return r.returnFailure()
		}

		deployutil.LogSuccess(ctx, deployutil.ReasonSuccessDeployment, "Removal ok for application "+deployData.GetConfigID())
		return ctrl.Result{}, nil
	} else if lastOp.Operation == util.OperationInstall {
		// here lastOp.State == util.StateOk && deployItemStatus.Readiness != nil && deployItemStatus.Readiness.State == util.StateOk
		// holds automatically because of the checks before
		log.V(util.LogLevelDebug).Info("lastOp.State == ok", "observedGeneration",
			deployData.GetObservedGeneration(), "generation", deployData.GetGeneration())
		deployutil.LogSuccess(ctx, deployutil.ReasonSuccessDeployment, "Deployment ok for application "+deployData.GetConfigID())
		return ctrl.Result{}, nil
	} else {
		err := errors.New("this branch should never be executed")
		log.Error(err, fmt.Sprintf("lastOp=%+v", lastOp))
		return r.returnFailure()
	}
}

// Adds the HubControllerFinalizer to the DeployItem, except if the DeployItem is about to be deleted, or the finalizer
// is already there. Returns the updated DeployItem.
func (r *DeploymentReconciler) addFinalizer(ctx context.Context, deployItem *v1alpha1.DeployItem) (*v1alpha1.DeployItem, error) {
	log := util.GetLoggerFromContext(ctx)

	if !deployItem.ObjectMeta.DeletionTimestamp.IsZero() {
		return deployItem, nil
	}

	if util.HasFinalizer(deployItem, util.HubControllerFinalizer) {
		return deployItem, nil
	}

	log.V(util.LogLevelDebug).Info("adding finalizer to deployitem")

	util.AddFinalizer(deployItem, util.HubControllerFinalizer)

	if err := r.crAndSecretClient.Update(ctx, deployItem); err != nil {
		log.Error(err, "error adding finalizer to deployitem")
		return nil, err
	}

	updatedDeployItem := &v1alpha1.DeployItem{}
	if err := r.crAndSecretClient.Get(ctx, *util.GetKey(deployItem), updatedDeployItem); err != nil {
		log.Error(err, "error fetching deployitem after finalizer was added")
		return nil, err
	}

	return updatedDeployItem, nil
}

func (r *DeploymentReconciler) removeFinalizer(ctx context.Context, deployItem *v1alpha1.DeployItem) error {
	log := util.GetLoggerFromContext(ctx)
	log.V(util.LogLevelWarning).Info("removing finalizer from deployItem")

	err := r.removeUnreferencedExportSecrets(ctx, deployItem, nil)
	if err != nil {
		return err
	}

	util.RemoveFinalizer(deployItem, util.HubControllerFinalizer)

	err = r.crAndSecretClient.Update(ctx, deployItem)
	if err != nil {
		log.Error(err, "Error removing finalizer from deployitem")
		return err
	}

	return nil
}

func (r *DeploymentReconciler) updateStatus(ctx context.Context, deployData *deployutil.DeployData) (ctrl.Result, error) {
	log := util.GetLoggerFromContext(ctx)

	// if ready condition = true, and export data exists, then write export secret
	if deployData.IsConditionTrue(hubv1.HubDeploymentReady) {
		if len(deployData.ExportValues) > 0 {
			err := r.storeExportData(ctx, deployData)
			if err != nil {
				return r.returnFailure()
			}
		}
	}

	r.removeReconcileAnnotation(ctx, deployData.GetDeployItem())

	newStatus, err := deployData.GetStatus()
	if err != nil {
		log.Error(err, "error marshaling deployitem status")
		return r.returnFailure()
	}

	result, err := r.updateDeployItemStatus(ctx, deployData.GetDeployItemKey(), newStatus)

	if err != nil {
		return result, err
	}

	_ = r.removeUnreferencedExportSecrets(ctx, deployData.GetDeployItem(), newStatus.ExportReference)
	return result, err
}

func (r *DeploymentReconciler) removeUnreferencedExportSecrets(ctx context.Context, deployItem *v1alpha1.DeployItem,
	referencedSecret *v1alpha1.ObjectReference) error {
	secretHandler := deployutil.NewSecretHandler(r.crAndSecretClient)

	return secretHandler.RemoveUnreferencedExportSecrets(ctx, deployItem, referencedSecret)
}

func (r *DeploymentReconciler) storeExportData(ctx context.Context, deployData *deployutil.DeployData) error {
	log := util.GetLoggerFromContext(ctx)

	storeExportData := true

	oldExportRef := deployData.GetExportReference()
	if oldExportRef != nil {
		secret := v1.Secret{}
		err := r.crAndSecretClient.Get(ctx, types.NamespacedName{Namespace: oldExportRef.Namespace, Name: oldExportRef.Name}, &secret)
		if err != nil && !apierrors.IsNotFound(err) {
			log.Error(err, "Failed to fetch secret with export values")
			return err
		} else if err == nil {
			var oldData map[string]interface{}
			err = yaml.Unmarshal(secret.Data[v1alpha1.DataObjectSecretDataKey], oldData)
			if err != nil {
				log.Error(err, "Error unmashalling old export data")
				return err
			}

			if reflect.DeepEqual(oldData, deployData.ExportValues) {
				storeExportData = false
			}
		}
		// other the secret was removed and we create a new one
	}

	if storeExportData {
		marshaledData, err := yaml.Marshal(deployData.ExportValues)
		if err != nil {
			log.Error(err, "Error marshaling new export data")
			return err
		}

		secretHandler := deployutil.NewSecretHandler(r.crAndSecretClient)
		secretName, err := secretHandler.CreateExportSecretForDi(ctx, deployData, marshaledData)
		if err != nil {
			log.Error(err, "Error creating new export data")
			return err
		}

		deployData.SetExportSecretName(secretName)
	}

	return nil
}

// Updates the status of an hdc. Retries the update several times in case of concurrent modification.
func (r *DeploymentReconciler) updateDeployItemStatus(ctx context.Context, deployItemKey *types.NamespacedName,
	status *v1alpha1.DeployItemStatus) (ctrl.Result, error) {
	log := util.GetLoggerFromContext(ctx)

	var err error

	util.Repeat(func() bool {
		err = r.updateDeployItemStatusOnce(ctx, deployItemKey, status)
		done := !util.IsStatusErrorConflict(err)
		return done
	}, 10, time.Second)

	if err != nil {
		return r.logAndReturnHubFailure(ctx, deployutil.ReasonFailedWriteState, "error updating deployitem status", err)
	}

	log.V(util.LogLevelDebug).Info("updated deployitem status")
	return ctrl.Result{}, nil
}

// Updates the status of an hdc - without retry
func (r *DeploymentReconciler) updateDeployItemStatusOnce(ctx context.Context, deployItemKey *types.NamespacedName,
	status *v1alpha1.DeployItemStatus) error {
	deployItem := &v1alpha1.DeployItem{}
	if err := r.crAndSecretClient.Get(ctx, *deployItemKey, deployItem); err != nil {
		return err
	}

	deployItem.Status = *status

	if err := r.crAndSecretClient.Status().Update(ctx, deployItem); err != nil {
		return err
	}

	return nil
}

func (r *DeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	maxThreads := util.GetEnvInteger("MAX_THREADS_DEPLOYMENT_CONTROLLER", 35, r.log)

	options := controller.Options{
		MaxConcurrentReconciles: maxThreads,
		Reconciler:              r,
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.DeployItem{}).
		Named("DeploymentReconciler").
		WithOptions(options).
		Complete(r)
}

func (r *DeploymentReconciler) GetName() string {
	return "DeploymentReconciler"
}

func (r *DeploymentReconciler) GetLastAVCheckReconcileTime() time.Time {
	if r.avCheck == nil {
		return time.Time{}
	}
	return r.avCheck.GetLastReconcileTime()
}

// Returns a context, based on the given context, which additionally contains an EventWriter.
func (r *DeploymentReconciler) setupEventWriter(ctx context.Context, clusterBomKey *types.NamespacedName) context.Context {
	clusterBom := r.getClusterBomForEventRecording(ctx, clusterBomKey)
	eventWriter := deployutil.NewEventWriter(clusterBom, r.eventRecorder)
	return deployutil.ContextWithEventWriter(ctx, eventWriter)
}

// Does not throw an error; can return nil. Therefore the result should only be used for recording events.
func (r *DeploymentReconciler) getClusterBomForEventRecording(ctx context.Context, clusterBomKey *types.NamespacedName) *hubv1.ClusterBom {
	log := util.GetLoggerFromContext(ctx)

	clusterBom := &hubv1.ClusterBom{}
	err := r.crAndSecretClient.Get(ctx, *clusterBomKey, clusterBom)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.V(util.LogLevelDebug).Info("clusterbom for event recording not found")
		} else {
			log.Error(err, "could not fetch clusterbom for event recording")
		}
		return nil
	}

	return clusterBom
}

func (r *DeploymentReconciler) logAndReturnHubFailure(ctx context.Context, reason, message string, err error) (ctrl.Result, error) {
	deployutil.LogHubFailure(ctx, reason, message, err)
	return ctrl.Result{
		Requeue: true,
	}, nil
}

func (r *DeploymentReconciler) calculateRequeueDurationForUnreachableCluster(deployItemStatus *hubv1.HubDeployItemProviderStatus) (bool, *time.Duration) {
	if deployItemStatus.Reachability != nil && !deployItemStatus.Reachability.Reachable {
		lastTime := deployItemStatus.Reachability.Time
		currentTime := time.Now()
		nextScheduledRun := lastTime.Add(5 * time.Minute)

		if currentTime.Before(nextScheduledRun) {
			duration := nextScheduledRun.Sub(currentTime)
			return true, &duration
		}
	}

	return false, nil
}

func (r *DeploymentReconciler) calculateRequeueDurationForNotReadyInstall(deployItemStatus *hubv1.HubDeployItemProviderStatus) (bool, *time.Duration) {
	if deployItemStatus.Readiness != nil && deployItemStatus.Readiness.State != util.StateOk {
		lastTime := deployItemStatus.Readiness.Time
		currentTime := time.Now()
		nextScheduledRun := lastTime.Add(15 * time.Second)

		if currentTime.Before(nextScheduledRun) {
			duration := nextScheduledRun.Sub(currentTime)
			return true, &duration
		}
	}

	return false, nil
}

func (r *DeploymentReconciler) returnFailure() (ctrl.Result, error) {
	return ctrl.Result{
		Requeue: true,
	}, nil
}

func (r *DeploymentReconciler) returnSuccess() (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

func (r *DeploymentReconciler) removeReconcileAnnotation(ctx context.Context, deployItem *v1alpha1.DeployItem) {
	if util.HasAnnotation(deployItem, util.AnnotationKeyReconcile, util.AnnotationValueReconcile) {
		log := util.GetLoggerFromContext(ctx)
		storedDeployItem := v1alpha1.DeployItem{}
		err := r.crAndSecretClient.Get(ctx, *util.GetKey(deployItem), &storedDeployItem)

		if err != nil {
			if !apierrors.IsNotFound(err) {
				log.Error(err, "error fetching deploy item for removing annotation")
			}
			return
		}

		util.RemoveAnnotation(&storedDeployItem, util.AnnotationKeyReconcile)

		err = r.crAndSecretClient.Update(ctx, &storedDeployItem)

		if err != nil {
			if apierrors.IsConflict(err) {
				log.V(util.LogLevelDebug).Info("updating deploy item for removing annotation had a conflict")
			} else {
				log.Error(err, "error updating deploy item for removing annotation")
			}
			return
		}
	}
}
