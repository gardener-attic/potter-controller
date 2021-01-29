package kapp

import (
	"context"
	"encoding/json"
	"reflect"
	"time"

	"github.com/gardener/potter-controller/api/apitypes"
	hubv1 "github.com/gardener/potter-controller/api/v1"
	"github.com/gardener/potter-controller/pkg/deployutil"
	"github.com/gardener/potter-controller/pkg/synchronize"
	"github.com/gardener/potter-controller/pkg/util"

	landscaper "github.com/gardener/landscaper/pkg/apis/core/v1alpha1"
	"github.com/pkg/errors"
	"github.com/vmware-tanzu/carvel-kapp-controller/pkg/apis/kappctrl/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const couldNotParse = "could not parse typeSpecificData"

type kappDeployerDI struct {
	crAndSecretClient        client.Client
	uncachedClient           synchronize.UncachedClient
	blockObject              *synchronize.BlockObject
	reconcileIntervalMinutes int64
}

func NewKappDeployerDI(crAndSecretClient client.Client, uncachedClient synchronize.UncachedClient,
	blockObject *synchronize.BlockObject, reconcileIntervalMinutes int64) deployutil.DeployItemDeployer {
	return &kappDeployerDI{
		crAndSecretClient:        crAndSecretClient,
		uncachedClient:           uncachedClient,
		blockObject:              blockObject,
		reconcileIntervalMinutes: reconcileIntervalMinutes,
	}
}

func (r *kappDeployerDI) ProcessNewOperation(ctx context.Context, deployData *deployutil.DeployData) {
	configID := deployData.Configuration.DeploymentConfig.ID

	err := r.processItem(ctx, deployData)

	now := metav1.Now()
	if err != nil {
		switch err.(type) {
		case *deployutil.ClusterUnreachableError:
			deployutil.LogHubFailure(ctx, deployutil.ReasonFailedClusterUnreachable,
				"Deployment failed for application "+configID+", because cluster is unreachable", err)
			deployData.SetStatusForUnreachableCluster()
		default:
			deployutil.LogHubFailure(ctx, deployutil.ReasonFailedDeployment,
				"Deployment failed for application "+configID, err)
			deployData.SetStatus(util.StateFailed, err.Error(), 1, now)
		}
	} else {
		deployutil.LogSuccess(ctx, deployutil.ReasonSuccessDeployment,
			"Deployment done for application "+configID)
		deployData.SetStatus(util.StateOk, r.successDescription(deployData), 1, now)
	}

	r.computeReadinessAndExport(ctx, deployData, now)
}

func (r *kappDeployerDI) ReconcileOperation(ctx context.Context, deployData *deployutil.DeployData) {
	log := util.GetLoggerFromContext(ctx)
	configID := deployData.Configuration.DeploymentConfig.ID
	log.V(util.LogLevelDebug).Info("reconcile", "observedGeneration", deployData.GetObservedGeneration(),
		"generation", deployData.GetGeneration())

	err := r.processItem(ctx, deployData)
	now := metav1.Now()
	if err != nil {
		switch err.(type) {
		case *deployutil.ClusterUnreachableError:
			deployutil.LogHubFailure(ctx, deployutil.ReasonFailedClusterUnreachable,
				"Reconcile failed for application "+configID+", because cluster is unreachable", err)
			deployData.SetStatusForUnreachableCluster()
		default:
			deployutil.LogHubFailure(ctx, deployutil.ReasonFailedDeployment, "Reconcile failed for application "+configID, err)

			deployData.SetStatus(util.StateFailed, err.Error(), 1, now)
		}
	} else {
		deployutil.LogSuccess(ctx, deployutil.ReasonSuccessDeployment, "Reconcile done for application "+configID)

		deployData.SetStatus(util.StateOk, r.successDescription(deployData), 1, now)
	}

	r.computeReadinessAndExport(ctx, deployData, metav1.Now())
}

func (r *kappDeployerDI) RetryFailedOperation(ctx context.Context, deployData *deployutil.DeployData) {
	configID := deployData.Configuration.DeploymentConfig.ID
	lastOp := deployData.ProviderStatus.LastOperation

	err := r.processItem(ctx, deployData)
	now := metav1.Now()
	if err != nil {
		switch err.(type) {
		case *deployutil.ClusterUnreachableError:
			deployutil.LogHubFailure(ctx, deployutil.ReasonFailedClusterUnreachable,
				"Retry of deployment failed for application "+configID+", because cluster is unreachable", err)
			deployData.SetStatusForUnreachableCluster()
		default:
			deployutil.LogHubFailure(ctx, deployutil.ReasonFailedDeployment,
				"Retry of deployment failed for application "+configID, err)
			deployData.SetStatus(util.StateFailed, err.Error(), lastOp.NumberOfTries+1, now)
		}
	} else {
		deployutil.LogSuccess(ctx, deployutil.ReasonSuccessDeployment, "Retry of deployment done for application "+configID)
		deployData.SetStatus(util.StateOk, r.successDescription(deployData),
			1, now)
	}

	r.computeReadinessAndExport(ctx, deployData, now)
}

func (r *kappDeployerDI) ProcessPendingOperation(ctx context.Context, deployData *deployutil.DeployData) {
	configID := deployData.Configuration.DeploymentConfig.ID

	// check reachability of target cluster
	secretKey := deployData.GetSecretKey()
	_, err := deployutil.GetTargetClient(ctx, r.crAndSecretClient, *secretKey)
	if err != nil {
		switch err.(type) {
		case *deployutil.ClusterUnreachableError:
			deployutil.LogHubFailure(ctx, deployutil.ReasonFailedClusterUnreachable,
				"Process pending failed for application "+configID+", because cluster is unreachable", err)
		default:
			deployutil.LogHubFailure(ctx, deployutil.ReasonFailedDeployment, "Reconcile failed for application "+configID, err)
		}
		deployData.SetStatusForUnreachableCluster()
	} else {
		deployData.SetStatusForReachableCluster()
	}

	r.computeReadinessAndExport(ctx, deployData, metav1.Now())
}

func (r *kappDeployerDI) processItem(ctx context.Context, deployData *deployutil.DeployData) error {
	log := util.GetLoggerFromContext(ctx)

	appKey := r.getAppKey(deployData)
	log = log.WithValues(util.LogKeyKappAppNamespacedName, appKey)
	ctx = context.WithValue(ctx, util.LoggerKey{}, log)

	isRemoveOperation := deployData.IsDeleteOperation()

	if isRemoveOperation {
		return r.Cleanup(ctx, deployData, true)
	} else { // nolint
		rawAppSpec := deployData.Configuration.DeploymentConfig.TypeSpecificData.Raw
		rawAppSpec, err := r.replaceSecretNames(ctx, rawAppSpec, deployData.Configuration.DeploymentConfig.NamedInternalSecretNames)
		if err != nil {
			return err
		}

		kappSpecificData, err := apitypes.NewKappSpecificData(rawAppSpec)
		if err != nil {
			log.Error(err, "error unmarshaling kapp specific data")
			return err
		}

		if kappSpecificData.AppSpec.Cluster == nil {
			kappSpecificData.AppSpec.Cluster = &v1alpha1.AppCluster{}
		}

		if kappSpecificData.AppSpec.Cluster.Namespace == "" {
			kappSpecificData.AppSpec.Cluster.Namespace = "default"
		}

		if kappSpecificData.AppSpec.Cluster.KubeconfigSecretRef == nil {
			kappSpecificData.AppSpec.Cluster.KubeconfigSecretRef = &v1alpha1.AppClusterKubeconfigSecretRef{}
		}

		if kappSpecificData.AppSpec.Cluster.KubeconfigSecretRef.Name == "" {
			kappSpecificData.AppSpec.Cluster.KubeconfigSecretRef.Name = deployData.Configuration.LocalSecretRef
		}

		if kappSpecificData.AppSpec.Cluster.KubeconfigSecretRef.Key == "" {
			kappSpecificData.AppSpec.Cluster.KubeconfigSecretRef.Key = kubeconfigSecretKey
		}

		if kappSpecificData.AppSpec.Cluster.KubeconfigSecretRef.Name != deployData.Configuration.LocalSecretRef {
			err = errors.New("target cluster of kapp app differs from localSecretRef")
			log.V(util.LogLevelWarning).Info(err.Error(), util.LogKeyKappAppNamespacedName, appKey)
			return err
		}

		if kappSpecificData.AppSpec.Cluster.KubeconfigSecretRef.Key != kubeconfigSecretKey {
			err = errors.New("the value of field cluster.kubeconfigSecretRef.key must be kubeconfig")
			log.V(util.LogLevelWarning).Info(err.Error(), util.LogKeyKappAppNamespacedName, appKey, "kubeconfigKey",
				kappSpecificData.AppSpec.Cluster.KubeconfigSecretRef.Key)
			return err
		}

		if kappSpecificData.AppSpec.SyncPeriod == nil {
			kappSpecificData.AppSpec.SyncPeriod = &metav1.Duration{
				Duration: time.Duration(r.reconcileIntervalMinutes) * time.Minute,
			}
		}

		err = r.createStateNamespace(ctx, deployData, kappSpecificData.AppSpec, appKey)
		if err != nil {
			return err
		}

		return r.installOrUpdate(ctx, deployData, kappSpecificData.AppSpec)
	}
}

// Creates the kapp state namespace on the target cluster, if it does not yet exist.
// Assumes that appSpec and appSpec.Cluster are not nil.
func (r *kappDeployerDI) createStateNamespace(ctx context.Context, deployData *deployutil.DeployData,
	appSpec *v1alpha1.AppSpec, appKey *types.NamespacedName) error {
	log := util.GetLoggerFromContext(ctx)

	secretKey := deployData.GetSecretKey()
	targetClient, err := deployutil.GetTargetClient(ctx, r.crAndSecretClient, *secretKey)
	if err != nil {
		return err
	}

	if appSpec.Cluster.Namespace == "" || appSpec.Cluster.Namespace == "default" {
		return nil
	}

	namespaceKey := types.NamespacedName{
		Name: appSpec.Cluster.Namespace,
	}
	namespaceObject := corev1.Namespace{}
	err = targetClient.Get(ctx, namespaceKey, &namespaceObject)
	if err != nil {
		if apierrors.IsNotFound(err) {
			namespaceObject = corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: appSpec.Cluster.Namespace,
				},
			}

			err = targetClient.Create(ctx, &namespaceObject)
			if err != nil {
				log.Error(err, "error creating kapp state namespace on target cluster", util.LogKeyKappAppNamespacedName, appKey)
				return err
			}
		} else {
			log.Error(err, "error fetching kapp state namespace of target cluster", util.LogKeyKappAppNamespacedName, appKey)
			return err
		}
	}

	return nil
}

func (r *kappDeployerDI) installOrUpdate(ctx context.Context, deployData *deployutil.DeployData, appSpec *v1alpha1.AppSpec) error {
	log := util.GetLoggerFromContext(ctx)

	appKey := r.getAppKey(deployData)

	app := v1alpha1.App{}
	err := r.crAndSecretClient.Get(ctx, *appKey, &app)
	if err != nil {
		if apierrors.IsNotFound(err) {
			app = v1alpha1.App{
				ObjectMeta: metav1.ObjectMeta{
					Name:      appKey.Name,
					Namespace: appKey.Namespace,
				},
				Spec: *appSpec,
			}

			err = r.crAndSecretClient.Create(ctx, &app)
			if err != nil {
				log.Error(err, "error creating kapp app", util.LogKeyKappAppNamespacedName, appKey)
				return err
			}

			return err
		}

		log.Error(err, "error fetching kapp app", util.LogKeyKappAppNamespacedName, appKey)
		return err
	}

	app.Spec = *appSpec
	err = r.crAndSecretClient.Update(ctx, &app)
	if err != nil {
		log.Error(err, "error updating kapp app", util.LogKeyKappAppNamespacedName, appKey)
		return err
	}

	return nil
}

func (r *kappDeployerDI) Cleanup(ctx context.Context, deployData *deployutil.DeployData, clusterExist bool) error {
	log := util.GetLoggerFromContext(ctx)

	appKey := r.getAppKey(deployData)

	app := v1alpha1.App{}
	err := r.crAndSecretClient.Get(ctx, *appKey, &app)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}

		log.Error(err, "error fetching kapp app for deletion", util.LogKeyKappAppNamespacedName, appKey)
		return err
	}

	if !clusterExist {
		app.SetFinalizers([]string{})

		err = r.crAndSecretClient.Update(ctx, &app)
		if err != nil {
			if !util.IsConcurrentModificationErr(err) {
				log.Error(err, "error removing finalizers from kapp app", util.LogKeyKappAppNamespacedName, appKey)
			}

			return err
		}

		app = v1alpha1.App{}
		err = r.crAndSecretClient.Get(ctx, *appKey, &app)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}

			log.Error(err, "error fetching kapp app for deletion", util.LogKeyKappAppNamespacedName, appKey)
			return err
		}
	}

	if app.ObjectMeta.DeletionTimestamp == nil {
		err = r.crAndSecretClient.Delete(ctx, &app)
		if err != nil {
			log.Error(err, "error deleting kapp app", util.LogKeyKappAppNamespacedName, appKey)
			return err
		}
	}

	for i := 0; i < 5; i++ {
		// wait and see if deleted
		time.Sleep(3 * time.Second)

		err = r.crAndSecretClient.Get(ctx, *appKey, &app)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}

			log.Error(err, "error fetching kapp app for deletion", util.LogKeyKappAppNamespacedName, appKey)
			return err
		}
	}

	err = errors.New("kapp controller app resource not removed within sleep period")
	log.Error(err, "could not remove kapp controller app", util.LogKeyKappAppNamespacedName, appKey)
	return err
}

func (r *kappDeployerDI) Preprocess(ctx context.Context, deployData *deployutil.DeployData) {
	log := util.GetLoggerFromContext(ctx)

	appKey := r.getAppKey(deployData)
	log = log.WithValues(util.LogKeyKappAppNamespacedName, appKey)
	ctx = context.WithValue(ctx, util.LoggerKey{}, log)

	app := &v1alpha1.App{}

	err := r.crAndSecretClient.Get(ctx, *appKey, app)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "Error fetching app")
		}

		return
	}

	// now the app exists
	oldPauseStatus, err := GetOldOrInitialPauseStatus(ctx, app)
	if err != nil {
		return
	}

	if app.GetGeneration() != app.Status.ObservedGeneration && !oldPauseStatus.Paused {
		// no need to store something
		return
	}

	// from now on app.GetGeneration() == app.Status.ObservedGeneration || oldPauseStatus.Paused
	if !r.hasProblem(app) {
		r.updateAppPausedStatus(ctx, app, oldPauseStatus, &PauseStatus{})
		return
	}

	// from now on app has problem
	if !oldPauseStatus.Problem {
		newPauseStatus := PauseStatus{
			Paused:       false,
			PausedSince:  time.Time{},
			Problem:      true,
			ProblemSince: time.Now(),
		}
		r.updateAppPausedStatus(ctx, app, oldPauseStatus, &newPauseStatus)
		return
	}

	// oldPauseStatus.Problem is now true

	if !oldPauseStatus.Paused {
		if oldPauseStatus.ProblemSince.Add(15 * time.Minute).After(time.Now()) {
			newPauseStatus := PauseStatus{
				Paused:       false,
				PausedSince:  time.Time{},
				Problem:      true,
				ProblemSince: oldPauseStatus.ProblemSince,
			}
			r.updateAppPausedStatus(ctx, app, oldPauseStatus, &newPauseStatus)
			return
		}

		newPauseStatus := PauseStatus{
			Paused:       true,
			PausedSince:  time.Now(),
			Problem:      true,
			ProblemSince: oldPauseStatus.ProblemSince,
		}

		r.updateAppPausedStatus(ctx, app, oldPauseStatus, &newPauseStatus)
		return
	}

	// oldPauseStatus.Paused is now true

	if oldPauseStatus.PausedSince.Add(5 * time.Minute).After(time.Now()) {
		return
	}

	newPauseStatus := PauseStatus{
		Paused:       false,
		PausedSince:  time.Now(),
		Problem:      true,
		ProblemSince: oldPauseStatus.ProblemSince,
	}

	r.updateAppPausedStatus(ctx, app, oldPauseStatus, &newPauseStatus)
}

func (r *kappDeployerDI) updateAppPausedStatus(ctx context.Context, app *v1alpha1.App, oldStatus, newStatus *PauseStatus) {
	if !reflect.DeepEqual(oldStatus, newStatus) {
		log := util.GetLoggerFromContext(ctx)

		SetPauseStatus(app, newStatus)

		err := r.crAndSecretClient.Update(ctx, app)
		if err != nil && !util.IsConcurrentModificationErr(err) {
			log.Error(err, "error updating kapp app")
		}
	}
}

func (r *kappDeployerDI) hasProblem(app *v1alpha1.App) bool {
	deployProblem := app.Status.Deploy != nil && app.Status.Deploy.ExitCode != 0
	fetchProblem := app.Status.Fetch != nil && app.Status.Fetch.ExitCode != 0
	inspectProblem := app.Status.Inspect != nil && app.Status.Inspect.ExitCode != 0
	templateProblem := app.Status.Template != nil && app.Status.Template.ExitCode != 0

	deployConditionFailed := util.GetAppConditionStatus(app, v1alpha1.DeleteFailed) == corev1.ConditionFalse
	reconcileConditionFailed := util.GetAppConditionStatus(app, v1alpha1.ReconcileFailed) == corev1.ConditionFalse

	return deployProblem || fetchProblem || inspectProblem || templateProblem || deployConditionFailed || reconcileConditionFailed
}

func (r *kappDeployerDI) computeReadinessAndExport(ctx context.Context, deployData *deployutil.DeployData, now metav1.Time) {
	r.computeReadiness(ctx, deployData, now)

	readyCondition := deployData.GetDeployItemCondition(hubv1.HubDeploymentReady)
	if readyCondition != nil && readyCondition.Status == landscaper.ConditionTrue {
		err := r.computeExports(ctx, deployData)
		if err != nil {
			deployData.ReplaceDeployItemCondition(hubv1.HubDeploymentReady, corev1.ConditionUnknown, now,
				hubv1.ReasonCouldNotGetExport, "Could not get export data")

			deployData.SetPhase(landscaper.ExecutionPhaseProgressing)
		}
	}
}

func (r *kappDeployerDI) computeExports(ctx context.Context, deployData *deployutil.DeployData) error {
	log := util.GetLoggerFromContext(ctx)

	kappSpecificData, err := apitypes.NewKappSpecificData(deployData.Configuration.DeploymentConfig.TypeSpecificData.Raw)
	if err != nil {
		msg := couldNotParse
		log.Error(err, msg)
		return err
	}

	if len(kappSpecificData.InternalExport) > 0 {
		secretKey := deployData.GetSecretKey()
		dynamicTargetClient, err := deployutil.NewDynamicTargetClient(ctx, r.crAndSecretClient, *secretKey)
		if err != nil {
			log.Error(err, "Error fetching dynamic target client")
			return err
		}

		newExportData := make(map[string]interface{})

		for key, exportEntry := range kappSpecificData.InternalExport {
			exportData, err := dynamicTargetClient.GetResourceData(exportEntry.APIVersion, exportEntry.Resource,
				exportEntry.Namespace, exportEntry.Name, exportEntry.FieldPath)

			if err != nil {
				log.Error(err, "Could not fetch resource: "+exportEntry.String())
				return err
			}

			newExportData[key] = exportData
		}

		if len(newExportData) > 0 {
			deployData.ExportValues = newExportData
		}
	}

	return nil
}

func (r *kappDeployerDI) computeReadiness(ctx context.Context, deployData *deployutil.DeployData, now metav1.Time) {
	log := util.GetLoggerFromContext(ctx)

	appKey := r.getAppKey(deployData)
	app := &v1alpha1.App{}

	switch deployData.ProviderStatus.LastOperation.Operation {
	case util.OperationInstall:
		// fetch app resource
		err := r.crAndSecretClient.Get(ctx, *appKey, app)
		if err != nil {
			app = nil
			log.Error(err, "error fetching kapp app after install", util.LogKeyKappAppNamespacedName, appKey)
			r.setReadiness(deployData, util.StateUnknown, now)
			r.setTypeSpecificStatus(ctx, nil, deployData)
			break
		}

		// derive readiness from condition of app
		if r.hasAppCondition(app, v1alpha1.ReconcileSucceeded) {
			targetClusterSecretKey := types.NamespacedName{
				Name:      deployData.Configuration.LocalSecretRef,
				Namespace: deployData.GetNamespace(),
			}
			dynamicTargetClient, err := deployutil.NewDynamicTargetClient(ctx, r.crAndSecretClient, targetClusterSecretKey)
			if err != nil {
				log.Error(err, "Error fetching dynamic target client")
				r.setReadiness(deployData, util.StateUnknown, now)
			} else {
				readiness := util.StateOk
				readiness = deployutil.ComputeReadinessForResourceReadyRequirements(deployData.Configuration.DeploymentConfig.ReadyRequirements.Resources,
					readiness, dynamicTargetClient, log)
				r.setReadiness(deployData, readiness, now)
			}
		} else if r.hasAppCondition(app, v1alpha1.ReconcileFailed) {
			r.setReadiness(deployData, util.StateFailed, now)
		} else if r.hasAppCondition(app, v1alpha1.Reconciling) {
			r.setReadiness(deployData, util.StatePending, now)
		} else {
			r.setReadiness(deployData, util.StateUnknown, now)
		}

		r.setTypeSpecificStatus(ctx, app, deployData)
	case util.OperationRemove:
		err := r.crAndSecretClient.Get(ctx, *appKey, app)
		if err != nil {
			app = nil
			if !apierrors.IsNotFound(err) {
				log.Error(err, "error fetching kapp app after delete", util.LogKeyKappAppNamespacedName, appKey)
			}
		}
		r.setReadiness(deployData, util.StateNotRelevant, now)
		r.setTypeSpecificStatus(ctx, app, deployData)
	default:
		log.Error(nil, "Unexpected operation", "operation", deployData.ProviderStatus.LastOperation.Operation)
		r.setReadiness(deployData, util.StateUnknown, now)
		r.setTypeSpecificStatus(ctx, nil, deployData)
	}

	// compute condition
	readinessState := deployData.ProviderStatus.Readiness.State

	if deployData.ProviderStatus.Reachability != nil && !deployData.ProviderStatus.Reachability.Reachable {
		// special case for unreachable cluster - just replace condition to unknown state
		deployData.ReplaceDeployItemCondition(hubv1.HubDeploymentReady, corev1.ConditionUnknown, now, hubv1.ReasonClusterUnreachable, "Cluster is unreachable")
		return
	} else if deployData.GetObservedGeneration() == 0 {
		deployData.ReplaceDeployItemCondition(hubv1.HubDeploymentReady, corev1.ConditionUnknown, now, hubv1.ReasonInitialState, "No deployment executed until now")
		deployData.SetPhase(landscaper.ExecutionPhaseProgressing)
	} else if deployData.IsInstallOperation() {
		if deployData.GetGeneration() != deployData.ProviderStatus.LastOperation.SuccessGeneration {
			deployData.ReplaceDeployItemCondition(hubv1.HubDeploymentReady, corev1.ConditionUnknown, now, hubv1.ReasonUpgradePending, "Upgrade pending")
			deployData.SetPhase(landscaper.ExecutionPhaseProgressing)
		} else if readinessState == util.StateOk && app.ObjectMeta.Generation == app.Status.ObservedGeneration {
			deployData.ReplaceDeployItemCondition(hubv1.HubDeploymentReady, corev1.ConditionTrue, now, hubv1.ReasonRunning, "Running")
			deployData.SetPhase(landscaper.ExecutionPhaseSucceeded)
		} else if readinessState == util.StateOk && app.ObjectMeta.Generation != app.Status.ObservedGeneration {
			deployData.ReplaceDeployItemCondition(hubv1.HubDeploymentReady, corev1.ConditionUnknown, now, hubv1.ReasonNotCurrentGeneration, "Not latest version running")
			deployData.SetPhase(landscaper.ExecutionPhaseProgressing)
		} else if readinessState == util.StateFinallyFailed {
			deployData.ReplaceDeployItemCondition(hubv1.HubDeploymentReady, corev1.ConditionFalse, now, hubv1.ReasonFinallyFailed, "Finally Failed")
			deployData.SetPhase(landscaper.ExecutionPhaseFailed)
		} else {
			deployData.ReplaceDeployItemCondition(hubv1.HubDeploymentReady, corev1.ConditionUnknown, now, hubv1.ReasonNotRunning, "Readiness is "+readinessState)
			deployData.SetPhase(landscaper.ExecutionPhaseProgressing)
		}
	} else if deployData.IsDeleteOperation() {
		if deployData.ProviderStatus.LastOperation.Operation == util.OperationInstall {
			// should never happen
			deployData.ReplaceDeployItemCondition(hubv1.HubDeploymentReady, corev1.ConditionUnknown, now, hubv1.ReasonRemovePending, "Remove pending")
			deployData.SetPhase(landscaper.ExecutionPhaseProgressing)
		} else if deployData.ProviderStatus.LastOperation.Operation == util.OperationRemove {
			if deployData.ProviderStatus.LastOperation.State == util.StateOk {
				deployData.ReplaceDeployItemCondition(hubv1.HubDeploymentReady, corev1.ConditionTrue, now, hubv1.ReasonRemoved, "Removed")
				deployData.SetPhase(landscaper.ExecutionPhaseSucceeded)
			} else {
				deployData.ReplaceDeployItemCondition(hubv1.HubDeploymentReady, corev1.ConditionUnknown, now, hubv1.ReasonRemovePending,
					"Last try to remove is pending")
				deployData.SetPhase(landscaper.ExecutionPhaseDeleting)
			}
		}
	}
}

func (r *kappDeployerDI) setReadiness(deployData *deployutil.DeployData, readinessState string, now metav1.Time) {
	deployData.ProviderStatus.Readiness = &hubv1.Readiness{
		State: readinessState,
		Time:  now,
	}
}

func (r *kappDeployerDI) setTypeSpecificStatus(ctx context.Context, app *v1alpha1.App, deployData *deployutil.DeployData) {
	log := util.GetLoggerFromContext(ctx)

	if app == nil {
		deployData.ProviderStatus.TypeSpecificStatus = nil
		return
	}

	appStatusJSON, err := json.Marshal(app.Status)
	if err != nil {
		log.Error(err, "error marshaling status of kapp app")
		return
	}

	deployData.ProviderStatus.TypeSpecificStatus = &runtime.RawExtension{
		Raw: appStatusJSON,
	}
}

func (r *kappDeployerDI) getAppKey(deployData *deployutil.DeployData) *types.NamespacedName {
	return deployData.GetDeployItemKey()
}

func (r *kappDeployerDI) hasAppCondition(app *v1alpha1.App, appConditionType v1alpha1.AppConditionType) bool {
	if app == nil {
		return false
	}

	for _, condition := range app.Status.Conditions {
		if condition.Type == appConditionType {
			return true
		}
	}
	return false
}

func (r *kappDeployerDI) successDescription(deployData *deployutil.DeployData) string {
	if deployData.IsInstallOperation() {
		return "install successful"
	}

	return "remove successful"
}

// Replaces logical secret names by the corresponding internal secret names.
// Input: kapp specific data as []byte, and the mapping from logical to internal secret names.
func (r *kappDeployerDI) replaceSecretNames(ctx context.Context, raw []byte, mapping map[string]string) ([]byte, error) {
	log := util.GetLoggerFromContext(ctx)

	if len(mapping) == 0 {
		log.Info("No replacement of secret names necessary")
		return raw, nil
	}

	log.Info("Replacing secret names")

	var obj interface{}
	err := json.Unmarshal(raw, &obj)
	if err != nil {
		log.Error(err, "error unmarshaling kapp specific data before replacement of secret names")
		return nil, err
	}

	r.replace(obj, mapping, "")

	result, err := json.Marshal(obj)
	if err != nil {
		log.Error(err, "error marshaling kapp specific data after replacement of secret names")
		return nil, err
	}

	return result, nil
}
