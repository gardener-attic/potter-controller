package controllersdi

import (
	"context"
	"encoding/json"
	"time"

	hubv1 "github.com/gardener/potter-controller/api/v1"
	"github.com/gardener/potter-controller/pkg/deployutil"
	"github.com/gardener/potter-controller/pkg/synchronize"
	"github.com/gardener/potter-controller/pkg/util"
	"github.com/gardener/potter-controller/pkg/util/predicate"

	ls "github.com/gardener/landscaper/apis/core/v1alpha1"
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

// ClusterBomStateReconciler reconciles a DeployItem object
type InstallationStateReconciler struct {
	client.Client
	Log            logr.Logger
	Scheme         *runtime.Scheme
	BlockObject    *synchronize.BlockObject
	UncachedClient synchronize.UncachedClient
}

func (r *InstallationStateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	maxThreads := util.GetEnvInteger("MAX_THREADS_CLUSTER_BOM_STATE_CONTROLLER", 35, r.Log)

	options := controller.Options{
		MaxConcurrentReconciles: maxThreads,
		Reconciler:              r,
	}

	landscaperManaged := builder.WithPredicates(predicate.LandscaperManaged())

	return ctrl.NewControllerManagedBy(mgr).
		For(&ls.Installation{}, landscaperManaged).
		Named("InstallationStateReconciler").
		WithOptions(options).
		Complete(r)
}

func (r *InstallationStateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) { // nolint
	ctx, logger := util.NewContextAndLogger(r.Log,
		util.LogKeyInstallationName, req.NamespacedName,
		util.LogKeyCorrelationID, uuid.New().String())

	logger.V(util.LogLevelDebug).Info("Reconciling status of installation")

	// block
	clusterBomKey := util.GetClusterBomKeyFromInstallationKey(&req.NamespacedName)
	ok, requeueDuration, err := r.BlockObject.Block(ctx, clusterBomKey, r.UncachedClient, 5*time.Minute, false)
	if err != nil {
		return r.returnFailure(err)
	} else if !ok {
		return ctrl.Result{RequeueAfter: requeueDuration}, nil
	}
	defer r.BlockObject.Release(ctx, &req.NamespacedName, false)

	logger.V(util.LogLevelDebug).Info("Processing status of installation")

	// main reconcile function
	return r.reconcileInstallation(ctx, req.NamespacedName)
}

func (r *InstallationStateReconciler) reconcileInstallation(ctx context.Context, installationKey types.NamespacedName) (ctrl.Result, error) {
	logger := util.GetLoggerFromContext(ctx)

	clusterBomKey := util.GetClusterBomKeyFromInstallationKey(&installationKey)

	clusterBom, clusterBomExists, err := r.getClusterBom(ctx, clusterBomKey)
	if err != nil {
		logger.Error(err, "error fetching clusterbom for deployitem", "clusterbom", *clusterBomKey)
		return r.returnFailure(err)
	}

	if !clusterBomExists {
		return r.reconcileInstallationWithoutClusterBom(ctx, installationKey)
	}

	return r.reconcileInstallationWithClusterBom(ctx, clusterBom)
}

func (r *InstallationStateReconciler) reconcileInstallationWithClusterBom(ctx context.Context, clusterBom *hubv1.ClusterBom) (ctrl.Result, error) {
	clusterBomKey := util.GetKey(clusterBom)
	ctx, logger := util.EnrichContextAndLogger(ctx, util.LogKeyClusterBomName, clusterBomKey)

	// Fetch installations
	var installationList ls.InstallationList
	if err := r.listInstallationsForClusterBom(ctx, clusterBomKey, &installationList); err != nil {
		logger.Error(err, "error fetching installations for clusterbom")
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
		return r.handleNotExistingTargetCluster(ctx, clusterBom, &installationList, secretKey)
	}

	return r.adjustClusterBomStatus(ctx, clusterBom, &installationList)
}

func (r *InstallationStateReconciler) adjustClusterBomStatus(ctx context.Context, clusterBom *hubv1.ClusterBom,
	installationList *ls.InstallationList) (ctrl.Result, error) {
	logger := util.GetLoggerFromContext(ctx)

	clusterBomKey := util.GetKey(clusterBom)
	var deployItemList ls.DeployItemList
	if err := r.listDeployItemsForClusterBom(ctx, clusterBomKey, &deployItemList); err != nil {
		logger.Error(err, "error fetching deploy items for clusterbom")
		return r.returnFailure(err)
	}

	logger.V(util.LogLevelDebug).Info("adjusting status of clusterbom", "length", len(installationList.Items))

	var newStatus hubv1.ClusterBomStatus
	var err error
	var stat *statistics
	newStatus.Conditions, stat, err = r.computeClusterBomConditions(ctx, installationList, &deployItemList, clusterBom)
	if err != nil {
		return r.returnFailure(err)
	}

	newStatus.ObservedGeneration = clusterBom.ObjectMeta.Generation

	appMap := newApplicationMap(installationList, &deployItemList)

	if newStatus.ApplicationStates, err = r.computeApplicationStates(ctx, appMap); err != nil {
		logger.Error(err, "error computing application states for clusterbom")
		return r.returnFailure(err)
	}

	newStatus.OverallState = r.computeOverallState(newStatus.ApplicationStates)
	newStatus.OverallNumOfDeployments = stat.getOverallNumOfDeployments()
	newStatus.OverallNumOfReadyDeployments = stat.getOverallNumOfReadyDeployments()
	newStatus.OverallProgress = stat.getOverallProgress()
	newStatus.OverallTime = metav1.Now()

	err = updateClusterBomStatus(ctx, r.Client, clusterBom, &newStatus, nil)
	if err != nil {
		return r.returnFailure(err)
	}

	newReadyCondition := util.GetClusterBomConditionFromStatus(&newStatus, hubv1.ClusterBomReady)
	if newReadyCondition == nil || newReadyCondition.Status == corev1.ConditionUnknown {
		return ctrl.Result{Requeue: true, RequeueAfter: time.Second * 15}, nil
	}

	return ctrl.Result{}, nil
}

func (r *InstallationStateReconciler) computeOverallState(newApplicationStates []hubv1.ApplicationState) string {
	overallState := util.StateOk

	for i := range newApplicationStates {
		newApplicationState := &newApplicationStates[i]
		overallState = deployutil.WorseState(overallState, newApplicationState.State)
	}

	return overallState
}

func (r *InstallationStateReconciler) computeApplicationStates(ctx context.Context, appMap applicationMap) ([]hubv1.ApplicationState, error) {
	if len(appMap) == 0 {
		return nil, nil
	}

	applicationStates := []hubv1.ApplicationState{}

	for appID, item := range appMap {
		deployItem := item.deployItem

		state, err := r.computeState(ctx, deployItem)
		if err != nil {
			return nil, err
		}

		detailedState, err := r.computeDetailedState(ctx, deployItem)
		if err != nil {
			return nil, err
		}

		installationState := r.computeInstallationState(item.installation)

		applicationStates = append(applicationStates, hubv1.ApplicationState{
			ID:                appID,
			State:             state,
			DetailedState:     *detailedState,
			InstallationState: installationState,
		})
	}

	return applicationStates, nil
}

func (r *InstallationStateReconciler) computeInstallationState(installation *ls.Installation) *hubv1.InstallationState {
	if installation == nil {
		return nil
	}

	installationState := hubv1.InstallationState{
		Phase:              installation.Status.Phase,
		ObservedGeneration: installation.Status.ObservedGeneration,
		Conditions:         installation.Status.Conditions,
		LastError:          installation.Status.LastError,
		ConfigGeneration:   installation.Status.ConfigGeneration,
		Imports:            installation.Status.Imports,
	}

	return &installationState
}

func (r *InstallationStateReconciler) computeDetailedState(ctx context.Context, deployItem *ls.DeployItem) (*hubv1.DetailedState, error) {
	if deployItem == nil {
		return &hubv1.DetailedState{
			CurrentOperation: hubv1.CurrentOperation{Time: metav1.Now()},
			LastOperation:    *util.CreateInitialLastOperation(),
		}, nil
	}

	logger := util.GetLoggerFromContext(ctx)

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

	detailedState := hubv1.DetailedState{
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
	}

	return &detailedState, nil
}

func (r *InstallationStateReconciler) computeState(ctx context.Context, deployItem *ls.DeployItem) (string, error) {
	log := util.GetLoggerFromContext(ctx)

	if deployItem == nil {
		return util.StatePending, nil
	}

	var state string

	deployItemStatus := &hubv1.HubDeployItemProviderStatus{}
	if deployItem.Status.ProviderStatus != nil && len(deployItem.Status.ProviderStatus.Raw) > 0 {
		if err := json.Unmarshal(deployItem.Status.ProviderStatus.Raw, deployItemStatus); err != nil {
			log.Error(err, "error unmarshaling provider state of deploy item", util.LogKeyDeployItemName, util.GetKey(deployItem))
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

func (r *InstallationStateReconciler) computeClusterBomConditions(ctx context.Context, installationList *ls.InstallationList,
	deployItemList *ls.DeployItemList, clusterbom *hubv1.ClusterBom) ([]hubv1.ClusterBomCondition, *statistics, error) {
	conditionReady, stat, err := r.computeClusterBomReadyCondition(ctx, installationList, clusterbom)
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

func (r *InstallationStateReconciler) computeClusterReachableCondition(ctx context.Context, deployItemList *ls.DeployItemList,
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

func (r *InstallationStateReconciler) computeClusterBomReadyCondition(ctx context.Context, installationList *ls.InstallationList,
	clusterbom *hubv1.ClusterBom) (*hubv1.ClusterBomCondition, *statistics, error) { // nolint

	resultCondition := hubv1.ClusterBomCondition{Type: hubv1.ClusterBomReady}

	resultCondition.Status = corev1.ConditionTrue

	stat := statistics{}

	var err error

	for i := range clusterbom.Spec.ApplicationConfigs {
		applConfig := &clusterbom.Spec.ApplicationConfigs[i]
		installation := findInstallationInList(installationList, applConfig.ID)

		err = r.adaptConditionStatus(ctx, clusterbom, applConfig, installation, &resultCondition, &stat)
		if err != nil {
			return nil, &stat, err
		}
	}

	for j := range installationList.Items {
		installation := &installationList.Items[j]
		appConfigID := util.GetAppConfigIDFromInstallation(installation)
		appconfig := findAppDeploymentConfigInList(clusterbom.Spec.ApplicationConfigs, appConfigID)

		if appconfig == nil {
			resultCondition.Status = util.WorseConditionStatus(resultCondition.Status, corev1.ConditionUnknown)
			stat.addPendingApp(appConfigID)
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

func (r *InstallationStateReconciler) adaptConditionStatus(ctx context.Context, clusterBom *hubv1.ClusterBom,
	appConfig *hubv1.ApplicationConfig, installation *ls.Installation,
	resultCondition *hubv1.ClusterBomCondition, stat *statistics) error {
	log := util.GetLoggerFromContext(ctx)

	if installation == nil {
		resultCondition.Status = util.WorseConditionStatus(resultCondition.Status, corev1.ConditionUnknown)
		stat.addPendingApp(appConfig.ID)
		return nil
	}

	if installation.ObjectMeta.Generation != installation.Status.ObservedGeneration {
		resultCondition.Status = util.WorseConditionStatus(resultCondition.Status, corev1.ConditionUnknown)

		stat.addPendingApp(appConfig.ID)
		return nil
	}

	isEqualConfig, err := isEqualConfigForInstallation(clusterBom, appConfig, installation)
	if err != nil {
		log.Error(err, "error comparing appconfig with installation", util.LogKeyInstallationName, util.GetKey(installation))
		return err
	}

	if !isEqualConfig {
		resultCondition.Status = util.WorseConditionStatus(resultCondition.Status, corev1.ConditionUnknown)
		stat.addPendingApp(appConfig.ID)
		return nil
	}

	phase := installation.Status.Phase

	if phase == ls.ComponentPhaseSucceeded {
		stat.addSuccessfulApp(appConfig.ID)
	} else if phase == ls.ComponentPhaseFailed {
		resultCondition.Status = util.WorseConditionStatus(resultCondition.Status, corev1.ConditionFalse)
		stat.addFailedApp(appConfig.ID)
	} else {
		resultCondition.Status = util.WorseConditionStatus(resultCondition.Status, corev1.ConditionUnknown)
		stat.addPendingApp(appConfig.ID)
	}

	return nil
}

func (r *InstallationStateReconciler) reconcileInstallationWithoutClusterBom(ctx context.Context,
	installationKey types.NamespacedName) (ctrl.Result, error) {
	logger := util.GetLoggerFromContext(ctx)

	installation, installationExists, err := r.getInstallation(ctx, &installationKey)
	if err != nil {
		return r.returnFailure(err)
	}

	if !installationExists {
		logger.V(util.LogLevelWarning).Info("no status update necessary, because clusterbom and installation do not exist any more")
		return r.returnSuccess()
	}

	err = deleteInstallation(ctx, r.Client, installation)
	if err != nil {
		return r.returnFailure(err)
	}

	return r.returnSuccess()
}

func (r *InstallationStateReconciler) handleNotExistingTargetCluster(ctx context.Context, clusterBom *hubv1.ClusterBom,
	installationList *ls.InstallationList, secretKey *types.NamespacedName) (ctrl.Result, error) {
	err := r.deleteInstallationsForNotExistingTargetCluster(ctx, installationList)
	if err != nil {
		return r.returnFailure(err)
	}

	err = adjustClusterBomStatusForNotExistingTargetCluster(ctx, r.Client, clusterBom, secretKey, nil)
	if err != nil {
		return r.returnFailure(err)
	}

	return r.returnSuccess()
}

func (r *InstallationStateReconciler) deleteInstallationsForNotExistingTargetCluster(ctx context.Context,
	installationList *ls.InstallationList) error {
	logger := util.GetLoggerFromContext(ctx)

	logger.V(util.LogLevelWarning).Info("Deleting installations associated with clusterboms because the secret of the target cluster does not exist")
	err := deleteInstallations(ctx, r.Client, installationList)
	if err != nil {
		return err
	}

	return nil
}

func (r *InstallationStateReconciler) listDeployItemsForClusterBom(ctx context.Context,
	clusterBomKey *types.NamespacedName, deployItemList *ls.DeployItemList) error {
	return r.Client.List(
		ctx,
		deployItemList,
		client.InNamespace(clusterBomKey.Namespace),
		client.MatchingLabels{hubv1.LabelClusterBomName: clusterBomKey.Name},
	)
}

func (r *InstallationStateReconciler) listInstallationsForClusterBom(ctx context.Context,
	clusterBomKey *types.NamespacedName, installationList *ls.InstallationList) error {
	return r.Client.List(
		ctx,
		installationList,
		client.InNamespace(clusterBomKey.Namespace),
		client.MatchingLabels{hubv1.LabelClusterBomName: clusterBomKey.Name},
	)
}

func (r *InstallationStateReconciler) existsSecret(ctx context.Context, secretKey *types.NamespacedName) (bool, error) {
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

func (r *InstallationStateReconciler) getClusterBom(ctx context.Context, clusterBomKey *types.NamespacedName) (*hubv1.ClusterBom, bool, error) {
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

func (r *InstallationStateReconciler) getInstallation(ctx context.Context,
	installationKey *types.NamespacedName) (*ls.Installation, bool, error) {
	logger := util.GetLoggerFromContext(ctx)

	installation := &ls.Installation{}
	if err := r.Get(ctx, *installationKey, installation); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, false, nil
		}

		logger.Error(err, "error fetching installation")
		return nil, false, err
	}

	return installation, true, nil
}

func (r *InstallationStateReconciler) returnFailure(err error) (ctrl.Result, error) { // nolint
	return ctrl.Result{
		Requeue: true,
	}, nil
}

func (r *InstallationStateReconciler) returnSuccess() (ctrl.Result, error) {
	return ctrl.Result{}, nil
}
