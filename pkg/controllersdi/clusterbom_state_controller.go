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
	"encoding/json"
	"time"

	hubv1 "github.com/gardener/potter-controller/api/v1"
	"github.com/gardener/potter-controller/pkg/avcheck"
	"github.com/gardener/potter-controller/pkg/deployutil"
	"github.com/gardener/potter-controller/pkg/synchronize"
	"github.com/gardener/potter-controller/pkg/util"
	"github.com/gardener/potter-controller/pkg/util/predicate"

	"github.com/gardener/landscaper/pkg/apis/core/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

// ClusterBomCleaner contains the information whether a clusterbom cleanup was done, and if it has failed when the
// cleanup was last tried. The clusterbom cleanup is performed once. It updates the status of empty clusterboms.
type ClusterBomCleaner struct {
	LastCheck int64
	Succeeded bool
}

// ClusterBomStateReconciler reconciles a DeployItem object
type ClusterBomStateReconciler struct {
	client.Client
	Log            logr.Logger
	Scheme         *runtime.Scheme
	Cleaner        ClusterBomCleaner
	BlockObject    *synchronize.BlockObject
	AVCheck        *avcheck.AVCheck
	AvCheckConfig  *avcheck.Configuration
	UncachedClient synchronize.UncachedClient
}

func (r *ClusterBomStateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	maxThreads := util.GetEnvInteger("MAX_THREADS_CLUSTER_BOM_STATE_CONTROLLER", 35, r.Log)

	options := controller.Options{
		MaxConcurrentReconciles: maxThreads,
		Reconciler:              r,
	}

	notLandscaperManaged := builder.WithPredicates(predicate.Not(predicate.LandscaperManaged()))

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.DeployItem{}, notLandscaperManaged).
		Named("ClusterBomStateReconciler").
		WithOptions(options).
		Complete(r)
}

func (r *ClusterBomStateReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	if r.AVCheck != nil {
		r.AVCheck.ReconcileCalled()
	}

	ctx, logger := util.NewContextAndLogger(r.Log,
		util.LogKeyDeployItemName, req.NamespacedName,
		util.LogKeyCorrelationID, uuid.New().String())

	logger.V(util.LogLevelDebug).Info("Reconciling status of deployitem")

	r.cleanupStatusOfEmptyClusterBoms(ctx)

	// block
	clusterBomKey := util.GetClusterBomKeyFromDeployItemKey(&req.NamespacedName)
	ok, requeueDuration, err := r.BlockObject.Block(ctx, clusterBomKey, r.UncachedClient, 5*time.Minute, false)
	if err != nil {
		return r.returnFailure(err)
	} else if !ok {
		return ctrl.Result{RequeueAfter: requeueDuration}, nil
	}
	defer r.BlockObject.Release(ctx, &req.NamespacedName, false)

	logger.V(util.LogLevelDebug).Info("Processing status of deployitem")

	// main reconcile function
	return r.reconcileDeployItem(ctx, req.NamespacedName)
}

func (r *ClusterBomStateReconciler) cleanupStatusOfEmptyClusterBoms(ctx context.Context) {
	if !r.Cleaner.Succeeded && time.Now().Unix()-r.Cleaner.LastCheck > 60*60 {
		logger := util.GetLoggerFromContext(ctx)
		logger.V(util.LogLevelDebug).Info("Cleaning up of empty clusterboms")

		err := r.checkEmptyClusterBoms(ctx)

		if err != nil {
			r.Cleaner.Succeeded = false
			r.Cleaner.LastCheck = time.Now().Unix()
			logger.Error(err, "Error cleaning up empty clusterboms")
		} else {
			r.Cleaner.Succeeded = true
			logger.V(util.LogLevelDebug).Info("Clean up of empty clusterboms finished")
		}
	}
}

func (r *ClusterBomStateReconciler) checkEmptyClusterBoms(ctx context.Context) error {
	var clusterBomList hubv1.ClusterBomList
	err := r.Client.List(ctx, &clusterBomList)

	if err != nil {
		return err
	}

	for i := range clusterBomList.Items {
		clusterBom := &clusterBomList.Items[i]

		if len(clusterBom.Spec.ApplicationConfigs) == 0 {
			var deployItemList v1alpha1.DeployItemList
			err = r.List(ctx, &deployItemList, client.InNamespace(clusterBom.Namespace), client.MatchingLabels{hubv1.LabelClusterBomName: clusterBom.Name})
			if err != nil {
				return err
			}

			if len(deployItemList.Items) == 0 {
				err := r.cleanupEmptyClusterBom(ctx, clusterBom)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (r *ClusterBomStateReconciler) cleanupEmptyClusterBom(ctx context.Context, clusterBom *hubv1.ClusterBom) error {
	logger := util.GetLoggerFromContext(ctx)

	clusterBomKey := util.GetKey(clusterBom)

	ok, _, err := r.BlockObject.Block(ctx, clusterBomKey, r.UncachedClient, 5*time.Minute, false)
	if err != nil {
		return err
	} else if !ok {
		return nil
	}
	defer r.BlockObject.Release(ctx, clusterBomKey, false)

	secretKey := util.GetSecretKeyFromClusterBom(clusterBom)
	secretExists, err := r.existsSecret(ctx, secretKey)
	if err != nil {
		logger.Error(err, "Error fetching secret", "secret", *secretKey)
		return err
	}

	if !secretExists {
		err = adjustClusterBomStatusForNotExistingTargetCluster(ctx, r.Client, clusterBom, secretKey, r.AvCheckConfig)
		if err != nil {
			return err
		}
	} else {
		err = r.adjustClusterBomStatusForEmptyClusterBom(ctx, clusterBom, r.AvCheckConfig)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *ClusterBomStateReconciler) adjustClusterBomStatusForEmptyClusterBom(ctx context.Context,
	clusterBom *hubv1.ClusterBom, avCheckConfig *avcheck.Configuration) error {
	logger := util.GetLoggerFromContext(ctx)
	logger.V(util.LogLevelDebug).Info("Adjust status of empty clusterbom", util.LogKeyClusterBomName, util.GetKey(clusterBom))

	var newStatus hubv1.ClusterBomStatus
	now := metav1.Now()
	newStatus.ObservedGeneration = clusterBom.ObjectMeta.Generation
	newStatus.OverallState = util.StateOk
	newStatus.OverallTime = now
	newStatus.OverallNumOfDeployments = 0
	newStatus.OverallNumOfReadyDeployments = 0
	newStatus.OverallProgress = 100
	newStatus.ApplicationStates = nil
	newStatus.Description = ""
	newStatus.Conditions = []hubv1.ClusterBomCondition{
		{
			Type:               hubv1.ClusterBomReady,
			Status:             corev1.ConditionTrue,
			LastUpdateTime:     now,
			LastTransitionTime: now,
			Reason:             hubv1.ReasonEmptyClusterBom,
			Message:            "Empty ClusterBom",
		},
	}

	return updateClusterBomStatus(ctx, r.Client, clusterBom, &newStatus, avCheckConfig)
}

func (r *ClusterBomStateReconciler) reconcileDeployItem(ctx context.Context, deployItemKey types.NamespacedName) (ctrl.Result, error) {
	logger := util.GetLoggerFromContext(ctx)

	clusterBomKey := util.GetClusterBomKeyFromDeployItemKey(&deployItemKey)
	clusterBom, clusterBomExists, err := r.getClusterBom(ctx, clusterBomKey)
	if err != nil {
		logger.Error(err, "error fetching clusterbom for deployitem", "clusterbom", *clusterBomKey)
		return r.returnFailure(err)
	}

	if !clusterBomExists {
		return r.reconcileDeployItemWithoutClusterBom(ctx, deployItemKey)
	}

	return r.reconcileDeployItemWithClusterBom(ctx, deployItemKey, clusterBom)
}

func (r *ClusterBomStateReconciler) reconcileDeployItemWithClusterBom(ctx context.Context,
	deployItemKey types.NamespacedName, clusterBom *hubv1.ClusterBom) (ctrl.Result, error) {
	logger := util.GetLoggerFromContext(ctx)

	deployItem, _, _, err := r.getDeployItem(ctx, &deployItemKey)
	if err != nil {
		return r.returnFailure(err)
	}

	// Check whether target cluster exists
	secretKey := util.GetSecretKeyFromClusterBom(clusterBom)
	secretExists, err := r.existsSecret(ctx, secretKey)
	if err != nil {
		logger.Error(err, "error fetching secret", "secret", *secretKey)
		return r.returnFailure(err)
	}
	if !secretExists {
		return r.handleNotExistingTargetCluster(ctx, clusterBom, deployItem, secretKey, r.AvCheckConfig)
	}

	return r.adjustClusterBomStatus(ctx, clusterBom)
}

func (r *ClusterBomStateReconciler) reconcileDeployItemWithoutClusterBom(ctx context.Context,
	deployItemKey types.NamespacedName) (ctrl.Result, error) {
	logger := util.GetLoggerFromContext(ctx)

	deployItem, deployItemConfig, deployItemExists, err := r.getDeployItem(ctx, &deployItemKey)
	if err != nil {
		return r.returnFailure(err)
	}

	if !deployItemExists {
		logger.V(util.LogLevelWarning).Info("no status update necessary, because clusterbom and deployitem do not exist any more")
		return r.returnSuccess()
	}

	secretKey := &types.NamespacedName{
		Namespace: deployItemKey.Namespace,
		Name:      deployItemConfig.LocalSecretRef,
	}
	secretExists, err := r.existsSecret(ctx, secretKey)
	if err != nil {
		logger.Error(err, "error fetching secret", "secret", *secretKey)
		return r.returnFailure(err)
	}
	if !secretExists {
		return r.handleNotExistingTargetCluster(ctx, nil, deployItem, secretKey, r.AvCheckConfig)
	}

	err = deleteDeployItem(ctx, r.Client, deployItem)
	if err != nil {
		return r.returnFailure(err)
	}

	return r.returnSuccess()
}

func (r *ClusterBomStateReconciler) handleNotExistingTargetCluster(ctx context.Context, clusterBom *hubv1.ClusterBom,
	deployItem *v1alpha1.DeployItem, secretKey *types.NamespacedName, avCheckConfig *avcheck.Configuration) (ctrl.Result, error) {
	clusterBomKey := util.GetClusterBomKeyFromClusterBomOrDeployItem(clusterBom, deployItem)

	err := r.deleteDeployItemsForNotExistingTargetCluster(ctx, clusterBomKey)
	if err != nil {
		return r.returnFailure(err)
	}

	err = adjustClusterBomStatusForNotExistingTargetCluster(ctx, r.Client, clusterBom, secretKey, avCheckConfig)
	if err != nil {
		return r.returnFailure(err)
	}

	return r.returnSuccess()
}

func (r *ClusterBomStateReconciler) deleteDeployItemsForNotExistingTargetCluster(ctx context.Context, clusterBomKey *types.NamespacedName) error {
	logger := util.GetLoggerFromContext(ctx).WithValues(util.LogKeyClusterBomName, clusterBomKey)

	deployItemList := &v1alpha1.DeployItemList{}
	err := r.listDeployItemsForClusterBom(ctx, clusterBomKey, deployItemList)
	if err != nil {
		logger.Error(err, "error fetching deployitems for clusterbom")
		return err
	}

	logger.V(util.LogLevelWarning).Info("Deleting deploy items associated with clusterboms because the secret of the target cluster does not exist")
	err = deleteDeployItems(ctx, r.Client, deployItemList)
	if err != nil {
		return err
	}

	return nil
}

func (r *ClusterBomStateReconciler) adjustClusterBomStatus(ctx context.Context, clusterBom *hubv1.ClusterBom) (ctrl.Result, error) {
	clusterBomKey := util.GetKey(clusterBom)
	logger := util.GetLoggerFromContext(ctx).WithValues(util.LogKeyClusterBomName, clusterBomKey)

	var deployItemList v1alpha1.DeployItemList
	if err := r.listDeployItemsForClusterBom(ctx, clusterBomKey, &deployItemList); err != nil {
		logger.Error(err, "error fetching deployitems for clusterbom")
		return r.returnFailure(err)
	}

	logger.V(util.LogLevelDebug).Info("adjusting status of clusterbom", "length", len(deployItemList.Items))

	var newStatus hubv1.ClusterBomStatus
	var err error
	var stat *statistics
	newStatus.Conditions, stat, err = r.computeClusterBomConditions(ctx, &deployItemList, clusterBom)
	if err != nil {
		return r.returnFailure(err)
	}

	newStatus.ObservedGeneration = clusterBom.ObjectMeta.Generation

	if newStatus.ApplicationStates, err = r.computeApplicationStates(ctx, deployItemList.Items); err != nil {
		logger.Error(err, "error computing application states for clusterbom")
		return r.returnFailure(err)
	}

	newStatus.OverallState = r.computeOverallState(newStatus.ApplicationStates)
	newStatus.OverallNumOfDeployments = stat.getOverallNumOfDeployments()
	newStatus.OverallNumOfReadyDeployments = stat.getOverallNumOfReadyDeployments()
	newStatus.OverallProgress = stat.getOverallProgress()
	newStatus.OverallTime = metav1.Now()

	err = updateClusterBomStatus(ctx, r.Client, clusterBom, &newStatus, r.AvCheckConfig)
	if err != nil {
		return r.returnFailure(err)
	}

	return ctrl.Result{}, nil
}

func (r *ClusterBomStateReconciler) computeApplicationStates(ctx context.Context,
	deployItems []v1alpha1.DeployItem) ([]hubv1.ApplicationState, error) {
	logger := util.GetLoggerFromContext(ctx)

	applicationStates := make([]hubv1.ApplicationState, len(deployItems))

	if len(deployItems) == 0 {
		return nil, nil
	}

	for i := range deployItems {
		deployItem := &deployItems[i]

		providerStatus := &hubv1.HubDeployItemProviderStatus{}
		if deployItem.Status.ProviderStatus != nil && len(deployItem.Status.ProviderStatus.Raw) > 0 {
			if err := json.Unmarshal(deployItem.Status.ProviderStatus.Raw, providerStatus); err != nil {
				logger.Error(err, "error unmarshaling provider status of deployitems", util.LogKeyDeployItemName, util.GetKey(deployItem))
				return nil, err
			}
		}

		if providerStatus.LastOperation.State == "" {
			providerStatus.LastOperation = *util.CreateInitialLastOperation()
		}

		state, err := r.computeState(ctx, deployItem)
		if err != nil {
			logger.Error(err, "error computing states of deploy item", util.LogKeyDeployItemName, util.GetKey(deployItem))
			return nil, err
		}

		applicationStates[i] = hubv1.ApplicationState{
			ID:    util.GetAppConfigIDFromDeployItem(deployItem),
			State: state,
			DetailedState: hubv1.DetailedState{
				CurrentOperation:   hubv1.CurrentOperation{Time: metav1.Now()},
				LastOperation:      providerStatus.LastOperation,
				Reachability:       providerStatus.Reachability,
				Readiness:          providerStatus.Readiness,
				HdcConditions:      util.MapToHdcCondition(deployItem.Status.Conditions),
				TypeSpecificStatus: providerStatus.TypeSpecificStatus,
				Generation:         deployItem.GetGeneration(),
				ObservedGeneration: deployItem.Status.ObservedGeneration,
				Phase:              deployItem.Status.Phase,
				DeletionTimestamp:  deployItem.GetDeletionTimestamp(),
			},
		}
	}

	return applicationStates, nil
}

func (r *ClusterBomStateReconciler) computeOverallState(newApplicationStates []hubv1.ApplicationState) string {
	overallState := util.StateOk

	for i := range newApplicationStates {
		newApplicationState := &newApplicationStates[i]
		overallState = deployutil.WorseState(overallState, newApplicationState.State)
	}

	return overallState
}

func (r *ClusterBomStateReconciler) computeState(ctx context.Context, deployItem *v1alpha1.DeployItem) (string, error) {
	log := util.GetLoggerFromContext(ctx)

	var state string

	deployItemStatus := &hubv1.HubDeployItemProviderStatus{}
	if deployItem.Status.ProviderStatus != nil && len(deployItem.Status.ProviderStatus.Raw) > 0 {
		if err := json.Unmarshal(deployItem.Status.ProviderStatus.Raw, deployItemStatus); err != nil {
			return "", err
		}
	}

	if !deployItem.ObjectMeta.DeletionTimestamp.IsZero() {
		if deployItemStatus.LastOperation.Operation != util.OperationRemove {
			state = util.StatePending
		} else if deployItemStatus.LastOperation.State == util.StateOk {
			state = util.StateOk
		} else {
			state = util.StateFailed
		}
	} else {
		if deployItem.ObjectMeta.Generation != deployItem.Status.ObservedGeneration {
			state = util.StatePending
		} else if deployItem.ObjectMeta.Generation == deployItemStatus.LastOperation.SuccessGeneration {
			if deployItemStatus.Readiness == nil {
				log.Error(nil, "readiness is nil")
				state = util.StateUnknown
			} else if deployItemStatus.Readiness.State == util.StateFinallyFailed {
				state = util.StateFailed
			} else {
				state = deployItemStatus.Readiness.State
			}
		} else {
			state = util.StateFailed
		}
	}

	return state, nil
}

func (r *ClusterBomStateReconciler) listDeployItemsForClusterBom(ctx context.Context,
	clusterBomKey *types.NamespacedName, deployItemList *v1alpha1.DeployItemList) error {
	return r.Client.List(
		ctx,
		deployItemList,
		client.InNamespace(clusterBomKey.Namespace),
		client.MatchingLabels{hubv1.LabelClusterBomName: clusterBomKey.Name},
	)
}

func (r *ClusterBomStateReconciler) getDeployItem(ctx context.Context, deployItemKey *types.NamespacedName) (*v1alpha1.DeployItem, *hubv1.HubDeployItemConfiguration, bool, error) {
	logger := util.GetLoggerFromContext(ctx)

	deployItem := &v1alpha1.DeployItem{}
	if err := r.Get(ctx, *deployItemKey, deployItem); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil, false, nil
		}

		logger.Error(err, "error fetching deployitem")
		return nil, nil, false, err
	}

	deployItemConfig := &hubv1.HubDeployItemConfiguration{}
	if err := json.Unmarshal(deployItem.Spec.Configuration.Raw, deployItemConfig); err != nil {
		logger.Error(err, "error unmashalling deployitem configuration")
		return deployItem, nil, true, err
	}

	return deployItem, deployItemConfig, true, nil
}

func (r *ClusterBomStateReconciler) getClusterBom(ctx context.Context, clusterBomKey *types.NamespacedName) (*hubv1.ClusterBom, bool, error) {
	var clusterBom hubv1.ClusterBom
	err := r.Get(ctx, *clusterBomKey, &clusterBom)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return &clusterBom, true, nil
}

func (r *ClusterBomStateReconciler) existsSecret(ctx context.Context, secretKey *types.NamespacedName) (bool, error) {
	var secret corev1.Secret
	err := r.Get(ctx, *secretKey, &secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *ClusterBomStateReconciler) GetName() string {
	return "ClusterBomStateReconciler"
}

func (r *ClusterBomStateReconciler) GetLastAVCheckReconcileTime() time.Time {
	if r.AVCheck == nil {
		return time.Time{}
	}
	return r.AVCheck.GetLastReconcileTime()
}

func (r *ClusterBomStateReconciler) computeClusterBomConditions(ctx context.Context, deployItemList *v1alpha1.DeployItemList,
	clusterbom *hubv1.ClusterBom) ([]hubv1.ClusterBomCondition, *statistics, error) {
	conditionReady, stat, err := r.computeClusterBomReadyCondition(ctx, deployItemList, clusterbom)
	if err != nil {
		return nil, nil, err
	}

	conditionReachable, err := r.computeClusterReachableCondition(ctx, deployItemList, clusterbom)
	if err != nil {
		return nil, nil, err
	}

	conditions := []hubv1.ClusterBomCondition{
		*conditionReady,
		*conditionReachable,
	}

	return conditions, stat, nil
}

func (r *ClusterBomStateReconciler) computeClusterReachableCondition(ctx context.Context, deployItemList *v1alpha1.DeployItemList,
	clusterbom *hubv1.ClusterBom) (*hubv1.ClusterBomCondition, error) { // nolint
	logger := util.GetLoggerFromContext(ctx)

	resultCondition := hubv1.ClusterBomCondition{Type: hubv1.ClusterReachable}

	var latestDate time.Time
	isReachable := true

	someInfoFound := false

	if deployItemList != nil {
		for i := range deployItemList.Items {
			deployItem := &deployItemList.Items[i]

			deployItemStatus := &hubv1.HubDeployItemProviderStatus{}
			if deployItem.Status.ProviderStatus != nil && len(deployItem.Status.ProviderStatus.Raw) > 0 {
				if err := json.Unmarshal(deployItem.Status.ProviderStatus.Raw, deployItemStatus); err != nil {
					logger.Error(err, "error unmashalling deployitem status", util.LogKeyDeployItemName, util.GetKey(deployItem))
					return nil, err
				}
			}

			if deployItemStatus.Reachability != nil {
				if deployItemStatus.Reachability.Time.After(latestDate) {
					latestDate = deployItemStatus.Reachability.Time.Time
					isReachable = deployItemStatus.Reachability.Reachable
					someInfoFound = true
				}
			}
		}
	}

	if !someInfoFound {
		resultCondition.Reason = hubv1.ReasonClusterReachabilityUnknown
		resultCondition.Message = "No info about cluster reachability"
		resultCondition.Status = corev1.ConditionUnknown
	} else if isReachable {
		resultCondition.Reason = hubv1.ReasonClusterReachable
		resultCondition.Message = "Cluster reachable"
		resultCondition.Status = corev1.ConditionTrue
	} else {
		resultCondition.Reason = hubv1.ReasonClusterNotReachable
		resultCondition.Message = "Cluster not reachable"
		resultCondition.Status = corev1.ConditionFalse
	}

	// Determine LastUpdateTime and LastTransitionTime
	now := metav1.Now()
	resultCondition.LastUpdateTime = now

	clusterbomCondition := util.GetClusterBomCondition(clusterbom, hubv1.ClusterReachable)
	if clusterbomCondition != nil && clusterbomCondition.Status == resultCondition.Status {
		resultCondition.LastTransitionTime = clusterbomCondition.LastTransitionTime
	} else {
		resultCondition.LastTransitionTime = now
	}

	return &resultCondition, nil
}

func (r *ClusterBomStateReconciler) computeClusterBomReadyCondition(ctx context.Context, deployItemList *v1alpha1.DeployItemList,
	clusterbom *hubv1.ClusterBom) (*hubv1.ClusterBomCondition, *statistics, error) { // nolint

	resultCondition := hubv1.ClusterBomCondition{Type: hubv1.ClusterBomReady}

	resultCondition.Status = corev1.ConditionTrue

	stat := statistics{}

	var err error

	for i := range clusterbom.Spec.ApplicationConfigs {
		applConfig := &clusterbom.Spec.ApplicationConfigs[i]
		deployItem := findDeployItemInList(deployItemList, applConfig.ID)

		err = r.adaptConditionStatus(ctx, applConfig, deployItem, &resultCondition, &stat)
		if err != nil {
			return nil, &stat, err
		}
	}

	for j := range deployItemList.Items {
		deployItem := &deployItemList.Items[j]
		appConfigID := util.GetAppConfigIDFromDeployItem(deployItem)
		appconfig := findAppDeploymentConfigInList(clusterbom.Spec.ApplicationConfigs, appConfigID)

		if appconfig == nil {
			deployItemStatus := &hubv1.HubDeployItemProviderStatus{}
			if deployItem.Status.ProviderStatus != nil && len(deployItem.Status.ProviderStatus.Raw) > 0 {
				if err := json.Unmarshal(deployItem.Status.ProviderStatus.Raw, deployItemStatus); err != nil {
					return nil, &stat, err
				}
			}

			if deployItemStatus.LastOperation.Operation != util.OperationRemove {
				resultCondition.Status = util.WorseConditionStatus(resultCondition.Status, corev1.ConditionUnknown)
				stat.addPendingApp(appConfigID)
			} else {
				condition := util.GetDeployItemCondition(deployItem, hubv1.HubDeploymentReady)
				status := util.GetDeployItemConditionStatus(condition)
				resultCondition.Status = util.WorseConditionStatus(resultCondition.Status, status)

				if status == corev1.ConditionUnknown {
					stat.addPendingApp(appConfigID)
				} else if status == corev1.ConditionFalse {
					stat.addFailedApp(appConfigID)
				} else {
					stat.addSuccessfulApp(appConfigID)
				}
			}
		}
	}

	reason, message := stat.getReasonAndMessageForReadyCondition()
	resultCondition.Reason = reason
	resultCondition.Message = message

	// Determine LastUpdateTime and LastTransitionTime
	now := metav1.Now()
	resultCondition.LastUpdateTime = now

	clusterbomCondition := util.GetClusterBomCondition(clusterbom, hubv1.ClusterBomReady)
	if clusterbomCondition != nil && clusterbomCondition.Status == resultCondition.Status {
		resultCondition.LastTransitionTime = clusterbomCondition.LastTransitionTime
	} else {
		resultCondition.LastTransitionTime = now
	}

	return &resultCondition, &stat, nil
}

func (r *ClusterBomStateReconciler) adaptConditionStatus(ctx context.Context, appConfig *hubv1.ApplicationConfig, deployItem *v1alpha1.DeployItem,
	resultCondition *hubv1.ClusterBomCondition, stat *statistics) error {
	log := util.GetLoggerFromContext(ctx)

	if deployItem == nil {
		resultCondition.Status = util.WorseConditionStatus(resultCondition.Status, corev1.ConditionUnknown)
		stat.addPendingApp(appConfig.ID)
		return nil
	}

	isEqualConfig, err := isEqualConfig(appConfig, deployItem)
	if err != nil {
		log.Error(err, "error comparing appconfig with deployitem", util.LogKeyDeployItemName, util.GetKey(deployItem))
	}

	if !isEqualConfig {
		resultCondition.Status = util.WorseConditionStatus(resultCondition.Status, corev1.ConditionUnknown)
		stat.addPendingApp(appConfig.ID)
		return nil
	}

	deployItemStatus := &hubv1.HubDeployItemProviderStatus{}
	if deployItem.Status.ProviderStatus != nil && len(deployItem.Status.ProviderStatus.Raw) > 0 {
		if err := json.Unmarshal(deployItem.Status.ProviderStatus.Raw, deployItemStatus); err != nil {
			log.Error(err, "error unmarshaling status of deploy item", util.LogKeyDeployItemName, util.GetKey(deployItem))
			return err
		}
	}

	if deployItem.ObjectMeta.Generation != deployItem.Status.ObservedGeneration {
		resultCondition.Status = util.WorseConditionStatus(resultCondition.Status, corev1.ConditionUnknown)

		stat.addPendingApp(appConfig.ID)
		return nil
	}

	condition := util.GetDeployItemCondition(deployItem, hubv1.HubDeploymentReady)
	status := util.GetDeployItemConditionStatus(condition)
	resultCondition.Status = util.WorseConditionStatus(resultCondition.Status, status)

	if status == corev1.ConditionUnknown {
		stat.addPendingApp(appConfig.ID)
	} else if status == corev1.ConditionFalse {
		stat.addFailedApp(appConfig.ID)
	} else {
		stat.addSuccessfulApp(appConfig.ID)
	}

	return nil
}

func (r *ClusterBomStateReconciler) returnFailure(err error) (ctrl.Result, error) { // nolint
	return ctrl.Result{
		Requeue: true,
	}, nil
}

func (r *ClusterBomStateReconciler) returnSuccess() (ctrl.Result, error) {
	return ctrl.Result{}, nil
}
