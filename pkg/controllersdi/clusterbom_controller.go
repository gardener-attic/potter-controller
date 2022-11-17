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
	"strings"
	"time"

	hubv1 "github.com/gardener/potter-controller/api/v1"
	"github.com/gardener/potter-controller/pkg/auditlog"
	"github.com/gardener/potter-controller/pkg/avcheck"
	"github.com/gardener/potter-controller/pkg/synchronize"
	"github.com/gardener/potter-controller/pkg/util"

	landscaper "github.com/gardener/landscaper/apis/core/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

// todo clarify name of our domain. gardener is using stuff like landscaper.gardener.cloud. Change this also for our internal api version of deploy items config and status.
// todo clarify with landscaper team stability of deploy item crd
// todo only watch deploy items which really belong to us
// todo add unit tests from controllers package for all files in this package
// todo check if we could reduce number of unmarshal operations of deployitems
// todo add deployment of deploy item crd to our build pipeline and adopt rbac rules accordingly

// ClusterBomReconciler reconciles a ClusterBom object
type ClusterBomReconciler struct {
	client.Client
	Log                 logr.Logger
	Scheme              *runtime.Scheme
	auditLogger         auditlog.AuditLogger
	blockObject         synchronize.BlockObject
	AVCheck             *avcheck.AVCheck
	AVCheckConfig       *avcheck.Configuration
	uncachedClient      synchronize.UncachedClient
	hubControllerClient synchronize.UncachedClient
}

func NewClusterBomReconciler(cli client.Client, log logr.Logger, scheme *runtime.Scheme, auditLog bool,
	blockObject *synchronize.BlockObject, av *avcheck.AVCheck, uncachedClient, hubControllerClient synchronize.UncachedClient,
	avCheckConfig *avcheck.Configuration) (*ClusterBomReconciler, error) {
	var auditLogger auditlog.AuditLogger
	var err error

	if auditLog {
		log.Info("Audit logging is enabled, start audit logging")
		auditLogger, err = auditlog.NewAuditLogger(log)
		if err != nil {
			return nil, err
		}
	} else {
		log.Info("Audit logging is disabled")
		auditLogger = nil
	}
	cbr := ClusterBomReconciler{
		Client:              cli,
		Log:                 log,
		Scheme:              scheme,
		auditLogger:         auditLogger,
		blockObject:         *blockObject,
		AVCheck:             av,
		AVCheckConfig:       avCheckConfig,
		uncachedClient:      uncachedClient,
		hubControllerClient: hubControllerClient,
	}
	return &cbr, nil
}

// SetupWithManager is used to create a new instance of the ClusterBomController.
func (r *ClusterBomReconciler) SetupWithManager(mgr ctrl.Manager) error {
	maxThreads := util.GetEnvInteger("MAX_THREADS_CLUSTER_BOM_CONTROLLER", 15, r.Log)

	options := controller.Options{
		MaxConcurrentReconciles: maxThreads,
		Reconciler:              r,
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&hubv1.ClusterBom{}).
		Named("ClusterBomReconciler").
		WithOptions(options).
		Complete(r)
}

func (r *ClusterBomReconciler) Close() error {
	return r.auditLogger.Close()
}

// +kubebuilder:rbac:groups=hub.k8s.sap.com,resources=clusterboms,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=hub.k8s.sap.com,resources=clusterboms/status,verbs=get;update;patch

func (r *ClusterBomReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) { // nolint
	if r.AVCheck != nil {
		r.AVCheck.ReconcileCalled()
	}

	ctx, log := util.NewContextAndLogger(r.Log,
		util.LogKeyClusterBomName, req.NamespacedName,
		util.LogKeyCorrelationID, uuid.New().String())

	ctx = context.WithValue(ctx, util.AuditLogKey{}, r.auditLogger)

	log.V(util.LogLevelDebug).Info("Reconciling ClusterBom")

	ok, requeueDuration, err := r.blockObject.Block(ctx, &req.NamespacedName, r.uncachedClient, 5*time.Minute, false)
	if err != nil {
		return r.returnFailure(err)
	} else if !ok {
		return ctrl.Result{RequeueAfter: requeueDuration}, nil
	}
	defer r.blockObject.Release(ctx, &req.NamespacedName, false)

	log.V(util.LogLevelDebug).Info("Processing ClusterBom")

	associatedObjects, err := r.readAssociatedObjects(ctx, req.NamespacedName)
	if err != nil {
		return r.returnFailure(err)
	}

	if associatedObjects.clusterbomExists {
		d := &clusterbomDeactivator{}
		stopReconcile, actionProgressing, err := d.handleDeactivationOrReactivation(ctx, associatedObjects, r.Client)
		if err != nil {
			log.Error(err, "error handling deactivation/reactivation of clusterbom")
			return r.returnFailure(err)
		} else if actionProgressing {
			return ctrl.Result{
				Requeue:      true,
				RequeueAfter: time.Second * 3,
			}, nil
		} else if stopReconcile {
			if err = d.deleteIfRequired(ctx, associatedObjects, r.Client); err != nil {
				return r.returnFailure(err)
			}
			return r.returnSuccess()
		}
	}

	if !associatedObjects.clusterbomExists {
		return r.handleClusterBomDoesNotExist(ctx, associatedObjects, req.NamespacedName)
	}

	if associatedObjects.clusterbom.ObjectMeta.DeletionTimestamp != nil {
		return r.handleClusterBomMarkedForDeletion(ctx, associatedObjects)
	}

	r.cleanupSecrets(ctx, associatedObjects)

	return r.handleNormalClusterBom(ctx, associatedObjects)
}

type AssociatedObjects struct {
	deployItemList   landscaper.DeployItemList
	installationList landscaper.InstallationList
	clusterbom       hubv1.ClusterBom
	clusterbomExists bool
	secretKey        *types.NamespacedName
	secret           corev1.Secret
	secretExists     bool
	secretList       corev1.SecretList
}

// readAssociatedObjects reads the clusterbom, the associated hubdeplyomentconfigs, and the secret for the target cluster
func (r *ClusterBomReconciler) readAssociatedObjects(ctx context.Context, clusterbomKey types.NamespacedName) (*AssociatedObjects, error) {
	log := util.GetLoggerFromContext(ctx)

	a := AssociatedObjects{}

	// read all deploy items associated with the given clusterbom
	err := r.List(ctx, &a.deployItemList, client.InNamespace(clusterbomKey.Namespace), client.MatchingLabels{hubv1.LabelClusterBomName: clusterbomKey.Name})
	if err != nil {
		log.Error(err, "Error listing deploy items")
		return nil, err
	}

	// read installations
	err = r.List(ctx, &a.installationList, client.InNamespace(clusterbomKey.Namespace), client.MatchingLabels{hubv1.LabelClusterBomName: clusterbomKey.Name})
	if err != nil {
		log.Error(err, "Error listing installation items")
		return nil, err
	}

	// read secrets for secret values
	err = r.List(ctx, &a.secretList, client.InNamespace(clusterbomKey.Namespace),
		client.MatchingLabels{hubv1.LabelClusterBomName: clusterbomKey.Name, hubv1.LabelPurpose: util.PurposeSecretValues})
	if err != nil {
		log.Error(err, "Error fetching secret list")
		return nil, err
	}

	// read clusterbom
	err = r.Get(ctx, clusterbomKey, &a.clusterbom)
	if err != nil {
		if apierrors.IsNotFound(err) {
			a.clusterbomExists = false
			return &a, nil
		} else { // nolint
			log.Error(err, "Error fetching clusterbom")
			return nil, err
		}
	} else {
		a.clusterbomExists = true
	}

	// determine secretKey for the target cluster; will be nil if neither clusterbom nor hdcs exist
	a.secretKey = util.GetSecretKeyFromClusterBom(&a.clusterbom)

	// read the secret containing the kubeconfig for the target cluster
	if a.secretKey != nil {
		err = r.Get(ctx, *a.secretKey, &a.secret)
		if err != nil {
			if apierrors.IsNotFound(err) {
				a.secretExists = false
			} else {
				log.Error(err, "error fetching secret", "secretKey", a.secretKey)
				return nil, err
			}
		} else {
			a.secretExists = true
		}
	}

	return &a, nil
}

// handleNormalClusterBom handles a clusterbom which exists and is not marked for deletion.
func (r *ClusterBomReconciler) handleNormalClusterBom(ctx context.Context, a *AssociatedObjects) (ctrl.Result, error) {
	var auditMessage *auditlog.AuditMessageInfo = nil

	log := util.GetLoggerFromContext(ctx)

	if !a.secretExists {
		// The target cluster does not exist. Therefore we delete the associated deploy items, adjust the
		// status of the clusterbom (overall state: pending, description: cluster does not exist, no application states).
		// Finally, we requeue the clusterbom, so that in certain intervals it will be rechecked whether the cluster does
		// then exist.
		log.V(util.LogLevelWarning).Info("Deleting deploy items associated with clusterboms because the secret of the target cluster does not exist")

		if isLandscaperManagedClusterBom(&a.clusterbom) {
			err := deleteInstallations(ctx, r.Client, &a.installationList)
			if err != nil {
				r.auditLogResult(ctx, auditMessage, false)
				return r.returnFailure(err)
			}
		} else {
			err := deleteDeployItems(ctx, r.Client, &a.deployItemList)
			if err != nil {
				r.auditLogResult(ctx, auditMessage, false)
				return r.returnFailure(err)
			}
		}

		err := adjustClusterBomStatusForNotExistingTargetCluster(ctx, r.Client, &a.clusterbom, a.secretKey, r.AVCheckConfig)
		if err != nil {
			r.auditLogResult(ctx, auditMessage, false)
			return r.returnFailure(err)
		}

		// Requeue the clusterbom without error
		log.V(util.LogLevelWarning).Info("Requeueing clusterbom, because the secret to access the target cluster does not exist", "secretKey", a.secretKey)
		r.auditLogResult(ctx, auditMessage, true)
		return ctrl.Result{Requeue: true}, nil
	}

	// For all deploy items or installations that are not in the clusterbom anymore, set operation "remove"
	var err error
	if isLandscaperManagedClusterBom(&a.clusterbom) {
		auditMessage, err = r.deleteOrphanedInstallations(ctx, a, auditMessage)
		if err != nil {
			return r.returnFailure(err)
		}
	} else {
		auditMessage, err = r.deleteOrphanedDeployItems(ctx, a, auditMessage)
		if err != nil {
			return r.returnFailure(err)
		}
	}

	// For all applicationconfigs of the clusterbom, create or update the corresponding installations or deploy items
	if isLandscaperManagedClusterBom(&a.clusterbom) {
		err = r.handleAppConfigsForInstallations(ctx, a, auditMessage)
	} else {
		err = r.handleAppConfigsForDeployItems(ctx, a, auditMessage)
	}

	if err != nil {
		return r.returnFailure(err)
	}

	r.auditLogResult(ctx, auditMessage, true)

	err = r.handleReconcileAnnotation(ctx, &a.clusterbom)
	if err != nil {
		return r.returnFailure(err)
	}

	return r.returnSuccess()
}

func (r *ClusterBomReconciler) handleReconcileAnnotation(ctx context.Context, clusterbom *hubv1.ClusterBom) error {
	if util.HasAnnotation(clusterbom, util.AnnotationKeyReconcile, util.AnnotationValueReconcile) {
		log := util.GetLoggerFromContext(ctx)

		deployItemList := landscaper.DeployItemList{}
		err := r.List(ctx, &deployItemList, client.InNamespace(clusterbom.Namespace), client.MatchingLabels{hubv1.LabelClusterBomName: clusterbom.Name})
		if err != nil {
			log.Error(err, "Error listing deploy items")
			return err
		}

		for i := range deployItemList.Items {
			deployItem := &deployItemList.Items[i]
			util.AddAnnotation(deployItem, util.AnnotationKeyReconcile, util.AnnotationValueReconcile)
			err := r.Client.Update(ctx, deployItem)
			if err != nil {
				if !apierrors.IsConflict(err) {
					log.Error(err, "Error updating deploy item")
				}
				return err
			}
		}

		tmpKey := util.GetKey(clusterbom)
		return r.removeReconcileAnnotation(ctx, tmpKey)
	}

	return nil
}

func (r *ClusterBomReconciler) handleAppConfigsForInstallations(ctx context.Context, a *AssociatedObjects,
	auditMessage *auditlog.AuditMessageInfo) error {
	log := util.GetLoggerFromContext(ctx)

	for i := range a.clusterbom.Spec.ApplicationConfigs {
		appconfig := &a.clusterbom.Spec.ApplicationConfigs[i]

		// Search the DeployItem corresponding to the the ApplicationConfig
		var installation *landscaper.Installation
		installation = findInstallationInList(&a.installationList, appconfig.ID)

		// Create or update installation.
		if installation == nil {
			if auditMessage == nil {
				auditMessage = r.auditLog(ctx, auditlog.CreateOrUpdate, a)
			}

			installation = &landscaper.Installation{}

			installationFactory := InstallationFactory{}
			if err2 := installationFactory.copyAppConfigToInstallation(appconfig, installation, &a.clusterbom); err2 != nil {
				log.Error(err2, "error copying appconfig to new installation", util.LogKeyInstallationName, installation.Name)
				return err2
			}

			err2 := r.createInstallation(ctx, installation)
			if err2 != nil {
				r.auditLogResult(ctx, auditMessage, false)
				return err2
			}
		} else {
			isEqual, err := isEqualConfigForInstallation(&a.clusterbom, appconfig, installation)
			if err != nil {
				log.Error(err, "error comparing appconfig with installation", util.LogKeyInstallationName, installation.Name)
				return err
			}

			if !isEqual {
				log.V(util.LogLevelDebug).Info("Updating installation item", util.LogKeyInstallationName, installation)

				if auditMessage == nil {
					auditMessage = r.auditLog(ctx, auditlog.CreateOrUpdate, a)
				}

				installationFactory := InstallationFactory{}
				if err2 := installationFactory.copyAppConfigToInstallation(appconfig, installation, &a.clusterbom); err2 != nil {
					log.Error(err2, "error copying appconfig to installation", util.LogKeyInstallationName, installation.Name)
					return err2
				}

				if err2 := r.updateInstallation(ctx, installation, &a.clusterbom); err2 != nil {
					r.auditLogResult(ctx, auditMessage, false)
					return err2
				}
			} else {
				log.V(util.LogLevelDebug).Info("No update of the installation required (unchanged)",
					util.LogKeyInstallationName, installation.Name)
			}
		}
	}

	return nil
}

func (r *ClusterBomReconciler) handleAppConfigsForDeployItems(ctx context.Context, a *AssociatedObjects,
	auditMessage *auditlog.AuditMessageInfo) error {
	log := util.GetLoggerFromContext(ctx)

	for i := range a.clusterbom.Spec.ApplicationConfigs {
		appconfig := &a.clusterbom.Spec.ApplicationConfigs[i]

		// Search the DeployItem corresponding to the the ApplicationConfig
		var deployItem *landscaper.DeployItem
		deployItem = findDeployItemInList(&a.deployItemList, appconfig.ID)

		// Create or update DeployItem.
		if deployItem == nil {
			if auditMessage == nil {
				auditMessage = r.auditLog(ctx, auditlog.CreateOrUpdate, a)
			}

			deployItem = &landscaper.DeployItem{}

			if err2 := r.copyAppConfigToDeployItem(appconfig, deployItem, &a.clusterbom); err2 != nil {
				log.Error(err2, "error copying appconfig to new deployitem", util.LogKeyDeployItemName, deployItem.Name)
				return err2
			}

			err2 := r.createDeployItem(ctx, deployItem)
			if err2 != nil {
				r.auditLogResult(ctx, auditMessage, false)
				return err2
			}
		} else {
			isEqual, err := isEqualConfig(appconfig, deployItem)
			if err != nil {
				log.Error(err, "error comparing appconfig with deploy item", util.LogKeyDeployItemName, deployItem.Name)
				return err
			}

			if isEqual {
				log.V(util.LogLevelDebug).Info("No update of the deploy item required (unchanged)",
					util.LogKeyDeployItemName, deployItem.Name)
			} else {
				log.V(util.LogLevelDebug).Info("Updating deploy item", util.LogKeyDeployItemName, deployItem.Name)

				if auditMessage == nil {
					auditMessage = r.auditLog(ctx, auditlog.CreateOrUpdate, a)
				}

				if err2 := r.copyAppConfigToDeployItem(appconfig, deployItem, &a.clusterbom); err2 != nil {
					log.Error(err2, "error copying appconfig to deployitem", util.LogKeyDeployItemName, deployItem.Name)
					return err2
				}

				if err2 := r.updateDeployItem(ctx, deployItem, &a.clusterbom); err2 != nil {
					r.auditLogResult(ctx, auditMessage, false)
					return err2
				}
			}
		}
	}

	return nil
}

// handleMarkedForDeletion handles a clusterbom that exists and is marked for deletion; this means metadata.deletionTimestamp is set.
func (r *ClusterBomReconciler) handleClusterBomMarkedForDeletion(ctx context.Context, a *AssociatedObjects) (ctrl.Result, error) {
	log := util.GetLoggerFromContext(ctx)
	log.V(util.LogLevelWarning).Info("Handling clusterbom that is marked for deletion")

	if len(a.clusterbom.Finalizers) == 0 {
		log.V(util.LogLevelWarning).Info("Clusterbom has no finalizer and will be deleted")
		return r.returnSuccess()
	}

	if len(a.deployItemList.Items) == 0 && len(a.installationList.Items) == 0 {
		// The purpose of the finalizer in a clusterbom is to postpone the deletion until all hdcs have been removed.
		// This goal is reached here. We remove the finalizer, so that the system will delete the clusterbom.
		err := r.removeFinalizerFromClusterbom(ctx, a)
		if err != nil {
			return r.returnFailure(err)
		}
		return r.returnSuccess()
	}

	// Delete installation
	err := deleteInstallations(ctx, r.Client, &a.installationList)
	if err != nil {
		return r.returnFailure(err)
	}

	// Delete deploy items
	auditMessage := r.auditLog(ctx, auditlog.Delete, a)
	err = deleteDeployItems(ctx, r.Client, &a.deployItemList)
	r.auditLogResult(ctx, auditMessage, err == nil)
	if err != nil {
		return r.returnFailure(err)
	}

	if isLandscaperManagedClusterBom(&a.clusterbom) {
		// we need to recheck because we are not informed about installation deletion
		return r.returnRetry()
	}

	// retry will be invoked by the state controller
	return r.returnSuccess()
}

func (r *ClusterBomReconciler) removeFinalizerFromClusterbom(ctx context.Context, a *AssociatedObjects) error {
	log := util.GetLoggerFromContext(ctx)
	log.V(util.LogLevelWarning).Info("Removing finalizer from clusterbom")

	if a.clusterbom.Finalizers == nil || len(a.clusterbom.Finalizers) == 0 {
		return nil
	}

	a.clusterbom.Finalizers = nil

	err := r.cleanupAllSecrets(ctx, a)
	if err != nil {
		return err
	}

	err = r.Client.Update(ctx, &a.clusterbom)
	if err != nil {
		if util.IsConcurrentModificationErr(err) {
			log.V(util.LogLevelWarning).Info("Warning removing finalizer from clusterbom due to parallel modification: " + err.Error())
		} else {
			log.Error(err, "Error removing finalizer from clusterbom")
		}

		return err
	}

	return nil
}

func (r *ClusterBomReconciler) handleClusterBomDoesNotExist(ctx context.Context, a *AssociatedObjects,
	cbName types.NamespacedName) (ctrl.Result, error) { // nolint
	// this method is not really needed only in cases where the user removes the finalizers by hand
	log := util.GetLoggerFromContext(ctx)
	log.V(util.LogLevelWarning).Info("Handling clusterbom that does not exist")

	if len(a.deployItemList.Items) == 0 && len(a.installationList.Items) == 0 {
		// No clusterbom, no deploy items. Nothing to do.
		return r.returnSuccess()
	}

	// Cluster exists
	log.V(util.LogLevelWarning).Info("Clusterbom was deleted. Marking all deploy items or installations for deletion")
	err := deleteDeployItems(ctx, r.Client, &a.deployItemList)
	if err != nil {
		return r.returnFailure(err)
	}

	err = deleteInstallations(ctx, r.Client, &a.installationList)
	if err != nil {
		return r.returnFailure(err)
	}

	return r.returnSuccess()
}

// copyAppConfigToHubConfig Copies all values of the given struct-pointer *appconfig* to the given struct-pointer *DeployItem*
// In the DeployItem, the labels matching the DeployItem with the clusterBom are set automatically.
func (r *ClusterBomReconciler) copyAppConfigToDeployItem(appconfig *hubv1.ApplicationConfig, deployItem *landscaper.DeployItem,
	clusterbom *hubv1.ClusterBom) error {
	deployItem.ObjectMeta.Labels = make(map[string]string)
	deployItem.ObjectMeta.Labels[hubv1.LabelClusterBomName] = clusterbom.Name
	deployItem.ObjectMeta.Labels[hubv1.LabelApplicationConfigID] = appconfig.ID
	deployItem.ObjectMeta.Labels[hubv1.LabelConfigType] = appconfig.ConfigType
	deployItem.Namespace = clusterbom.Namespace
	deployItem.Name = util.CreateDeployItemName(clusterbom.GetName(), appconfig.ID)
	deployItem.Spec.Type = landscaper.DeployItemType(appconfig.ConfigType)

	config := hubv1.HubDeployItemConfiguration{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HubDeployItemConfiguration",
			APIVersion: util.DeployItemConfigVersion,
		},
		LocalSecretRef: clusterbom.Spec.SecretRef,
		DeploymentConfig: hubv1.DeploymentConfig{
			ID:                appconfig.ID,
			TypeSpecificData:  appconfig.TypeSpecificData,
			NoReconcile:       appconfig.NoReconcile,
			ReadyRequirements: appconfig.ReadyRequirements,
		},
	}

	appconfig.TypeSpecificData.DeepCopyInto(&config.DeploymentConfig.TypeSpecificData)

	// As values is an optional field, we have to check if there is a source to copy,
	// and if there is a target structure we are supposed to copy to.
	if appconfig.Values == nil {
		config.DeploymentConfig.Values = nil
	} else if appconfig.Values != nil {
		if config.DeploymentConfig.Values == nil {
			config.DeploymentConfig.Values = &runtime.RawExtension{}
		}
		appconfig.Values.DeepCopyInto(config.DeploymentConfig.Values)
	}

	if appconfig.SecretValues == nil {
		config.DeploymentConfig.InternalSecretName = ""
	} else {
		config.DeploymentConfig.InternalSecretName = appconfig.SecretValues.InternalSecretName
	}

	if len(appconfig.NamedSecretValues) == 0 {
		config.DeploymentConfig.NamedInternalSecretNames = nil
	} else {
		config.DeploymentConfig.NamedInternalSecretNames = make(map[string]string)
		for k, v := range appconfig.NamedSecretValues {
			config.DeploymentConfig.NamedInternalSecretNames[k] = v.InternalSecretName
		}
	}

	encodedConfig, err := json.Marshal(config)
	if err != nil {
		return err
	}

	deployItem.Spec.Configuration = &runtime.RawExtension{
		Raw: encodedConfig,
	}

	return nil
}

func (r *ClusterBomReconciler) updateInstallation(ctx context.Context, installation *landscaper.Installation,
	clusterBom *hubv1.ClusterBom) error {
	log := util.GetLoggerFromContext(ctx)

	util.AddAnnotation(installation, landscaper.OperationAnnotation, string(landscaper.ForceReconcileOperation))

	log.V(util.LogLevelDebug).Info("Updating existing installation", util.LogKeyInstallationName, installation.Name, "installation", installation)
	err := r.Update(ctx, installation)
	if err != nil {
		if util.IsConcurrentModificationErr(err) {
			if r.AVCheckConfig != nil && clusterBom.Name == r.AVCheckConfig.BomName && clusterBom.Namespace == r.AVCheckConfig.Namespace {
				log.V(util.LogLevelDebug).Info("Problem updating status of av check hdc")
				return nil
			}

			log.V(util.LogLevelWarning).Info("Warning updating status of installation due to parallel modification: " + err.Error())
			return err
		}

		log.Error(err, "Error updating existing installation", util.LogKeyInstallationName, installation.Name)
		return err
	}

	return nil
}

// Updates the given *DeployItem* and writes the result to the cluster.
// Automatically increases the CurrentOperation.Number and .Timestamp.
func (r *ClusterBomReconciler) updateDeployItem(ctx context.Context, deployItem *landscaper.DeployItem,
	clusterBom *hubv1.ClusterBom) error {
	log := util.GetLoggerFromContext(ctx)

	log.V(util.LogLevelDebug).Info("Updating existing deploy item", util.LogKeyDeployItemName, deployItem.Name, "deployitem", deployItem)
	err := r.Update(ctx, deployItem)
	if err != nil {
		if util.IsConcurrentModificationErr(err) {
			if r.AVCheckConfig != nil && clusterBom.Name == r.AVCheckConfig.BomName && clusterBom.Namespace == r.AVCheckConfig.Namespace {
				log.V(util.LogLevelDebug).Info("Problem updating status of av check hdc")
				return nil
			}

			log.V(util.LogLevelWarning).Info("Warning updating status of deploy item due to parallel modification: " + err.Error())
			return err
		}

		log.Error(err, "Error updating existing deploy item", util.LogKeyDeployItemName, deployItem.Name)
		return err
	}

	return nil
}

// Creates the given *DeployItem* and writes the result to the cluster.
// Automatically initializes the CurrentOperation with: Operation = install, Number = 1, Time = <NOW>.
func (r *ClusterBomReconciler) createDeployItem(ctx context.Context, deployItem *landscaper.DeployItem) error { // nolint
	log := util.GetLoggerFromContext(ctx)

	log.V(util.LogLevelDebug).Info("Creating deploy item", util.LogKeyDeployItemName, deployItem.Name, "deployitem", deployItem)
	err := r.Create(ctx, deployItem)
	if err != nil {
		log.Error(err, "Error creating deploy item", util.LogKeyDeployItemName, deployItem.Name)
		return err
	}
	return nil
}

func (r *ClusterBomReconciler) createInstallation(ctx context.Context, installation *landscaper.Installation) error { // nolint
	log := util.GetLoggerFromContext(ctx)

	log.V(util.LogLevelDebug).Info("Creating deploy item", util.LogKeyInstallationName, installation.Name, "installation", installation)
	err := r.Create(ctx, installation)
	if err != nil {
		log.Error(err, "Error creating installation", util.LogKeyInstallationName, installation.Name)
		return err
	}
	return nil
}

func (r *ClusterBomReconciler) deleteOrphanedDeployItems(ctx context.Context, a *AssociatedObjects, auditMsg *auditlog.AuditMessageInfo) (*auditlog.AuditMessageInfo, error) {
	for i := range a.deployItemList.Items {
		deployItem := &a.deployItemList.Items[i]
		appConfig := findAppDeploymentConfigInList(a.clusterbom.Spec.ApplicationConfigs, util.GetAppConfigIDFromDeployItem(deployItem))
		if appConfig == nil {
			if auditMsg == nil {
				auditMsg = r.auditLog(ctx, auditlog.CreateOrUpdate, a)
			}

			err := deleteDeployItem(ctx, r.Client, deployItem)
			r.auditLogResult(ctx, auditMsg, err == nil)
			if err != nil {
				return auditMsg, err
			}
		}
	}
	return auditMsg, nil
}

func (r *ClusterBomReconciler) deleteOrphanedInstallations(ctx context.Context, a *AssociatedObjects,
	auditMsg *auditlog.AuditMessageInfo) (*auditlog.AuditMessageInfo, error) {
	for i := range a.installationList.Items {
		installation := &a.installationList.Items[i]
		appConfig := findAppDeploymentConfigInList(a.clusterbom.Spec.ApplicationConfigs, util.GetAppConfigIDFromInstallationKey(util.GetKey(installation)))
		if appConfig == nil {
			if auditMsg == nil {
				auditMsg = r.auditLog(ctx, auditlog.CreateOrUpdate, a)
			}

			err := deleteInstallation(ctx, r.Client, installation)
			r.auditLogResult(ctx, auditMsg, err == nil)
			if err != nil {
				return auditMsg, err
			}
		}
	}
	return auditMsg, nil
}

func (r *ClusterBomReconciler) auditLog(ctx context.Context, action auditlog.Action, a *AssociatedObjects) *auditlog.AuditMessageInfo {
	log := util.GetLoggerFromContext(ctx)
	if r.auditLogger == nil {
		return nil
	}
	clusterName := ""
	userID := ""
	clusterURL := ""
	bomName := ""
	projectName := ""
	const maxCBLen int = 100000

	if a.clusterbomExists {
		bomName = a.clusterbom.Name
		projectName = a.clusterbom.Namespace
		if len(projectName) > 0 {
			projectName = projectName[len("garden-"):]
		}
	}
	if len(a.clusterbom.Spec.SecretRef) > 0 {
		secretRef := a.clusterbom.Spec.SecretRef
		index := strings.LastIndex(secretRef, ".")
		if index > 0 {
			clusterName = secretRef[0:index]
		}
	}
	if a.secretExists {
		userID = string(a.secret.UID)
		clusterURL = a.secret.Annotations["url"]
	}
	bomAsYaml, err := json.Marshal(a.clusterbom)
	var bomAsString string
	if err != nil {
		log.Error(err, "Failed to get ClusterBoM as JSON")
		bomAsString = ""
	} else {
		bomAsString = string(bomAsYaml)
		if len(bomAsString) > maxCBLen {
			bomAsString = bomAsString[:maxCBLen] + "... (shortened)"
		}
	}
	bomAsYaml, err = json.Marshal(a.clusterbom.Annotations["kubectl.kubernetes.io/last-applied-configuration"])
	var oldBomAsString string
	if err != nil {
		log.Error(err, "Failed to get old ClusterBoM as JSON")
		oldBomAsString = ""
	} else {
		oldBomAsString = string(bomAsYaml)
		if len(bomAsString) > maxCBLen {
			bomAsString = bomAsString[:maxCBLen] + "... (shortened)"
		}
	}

	auditMsg := auditlog.NewAuditMessage(action, bomName, projectName, clusterName, userID, clusterURL, bomAsString,
		oldBomAsString, nil)
	_, err = r.auditLogger.Log(auditMsg)
	if err != nil {
		log.Error(err, "Failed to write audit log message")
		return nil
	}
	return auditMsg
}

func (r *ClusterBomReconciler) auditLogResult(ctx context.Context, auditMessage *auditlog.AuditMessageInfo, success bool) {
	log := util.GetLoggerFromContext(ctx)
	if auditMessage == nil || r.auditLogger == nil {
		return
	}

	// if success code already has been set ignore this call and return. Do not log success code multiple times
	if auditMessage.Success != nil {
		return
	}
	auditMessage.Success = &success
	_, err := r.auditLogger.Log(auditMessage)
	if err != nil {
		log.Error(err, "Failed to write audit log message")
		return
	}
}

func (r *ClusterBomReconciler) GetName() string {
	return "ClusterBomReconciler"
}

func (r *ClusterBomReconciler) GetLastAVCheckReconcileTime() time.Time {
	if r.AVCheck == nil {
		return time.Time{}
	}
	return r.AVCheck.GetLastReconcileTime()
}

func (r *ClusterBomReconciler) cleanupSecrets(ctx context.Context, objects *AssociatedObjects) {
	for i := range objects.secretList.Items {
		secret := &objects.secretList.Items[i]

		found := false
		for j := range objects.clusterbom.Spec.ApplicationConfigs {
			appConfig := &objects.clusterbom.Spec.ApplicationConfigs[j]

			if appConfig.SecretValues != nil && appConfig.SecretValues.InternalSecretName == secret.Name {
				found = true
				break
			}

			if r.isContainedInNamedSecrets(appConfig, secret) {
				found = true
				break
			}
		}

		if !found {
			if secret.ObjectMeta.CreationTimestamp.Time.Add(time.Hour).Before(time.Now()) {
				// error is not checked because it is not so relevant here
				r.deleteSecret(ctx, secret) // nolint
			}
		}
	}
}

func (r *ClusterBomReconciler) isContainedInNamedSecrets(appConfig *hubv1.ApplicationConfig, secret *corev1.Secret) bool {
	for _, v := range appConfig.NamedSecretValues {
		if v.InternalSecretName == secret.Name {
			return true
		}
	}
	return false
}

func (r *ClusterBomReconciler) cleanupAllSecrets(ctx context.Context, objects *AssociatedObjects) error {
	for i := range objects.secretList.Items {
		secret := &objects.secretList.Items[i]
		err := r.deleteSecret(ctx, secret)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *ClusterBomReconciler) deleteSecret(ctx context.Context, secret *corev1.Secret) error {
	return deleteSecret(ctx, secret, r.Client, r.hubControllerClient)
}

func (r *ClusterBomReconciler) removeReconcileAnnotation(ctx context.Context, clusterBomKey *types.NamespacedName) error {
	log := util.GetLoggerFromContext(ctx)

	var err error

	util.Repeat(func() bool {
		err = r.removeReconcileAnnotationOnce(ctx, clusterBomKey)
		done := (err == nil) || !apierrors.IsConflict(err)
		return done
	}, 10, time.Second)

	if err != nil {
		log.Error(err, "error removing reconcile annotation")
		return err
	}

	return nil
}

func (r *ClusterBomReconciler) removeReconcileAnnotationOnce(ctx context.Context, clusterBomKey *types.NamespacedName) error {
	log := util.GetLoggerFromContext(ctx)

	clusterBom := hubv1.ClusterBom{}
	err := r.Get(ctx, *clusterBomKey, &clusterBom)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.V(util.LogLevelWarning).Info("no clusterbom found for removing reconcile annotation", "error", err.Error())
			return nil
		}
		log.Error(err, "error fetching cluster bom for removing reconcile annotation")
		return err
	}

	util.RemoveAnnotation(&clusterBom, util.AnnotationKeyReconcile)

	return r.Update(ctx, &clusterBom)
}

// Returns a failed reconcile.
func (r *ClusterBomReconciler) returnFailure(err error) (ctrl.Result, error) { // nolint
	return ctrl.Result{
		Requeue: true,
	}, nil
}

func (r *ClusterBomReconciler) returnRetry() (ctrl.Result, error) { // nolint
	return ctrl.Result{
		Requeue: true,
	}, nil
}

func (r *ClusterBomReconciler) returnSuccess() (ctrl.Result, error) {
	return ctrl.Result{}, nil
}
