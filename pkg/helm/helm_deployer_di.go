package helm

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gardener/potter-controller/api/apitypes"
	hubv1 "github.com/gardener/potter-controller/api/v1"
	"github.com/gardener/potter-controller/pkg/deployutil"
	"github.com/gardener/potter-controller/pkg/synchronize"
	"github.com/gardener/potter-controller/pkg/util"

	"github.com/gardener/landscaper/apis/core/v1alpha1"
	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/release"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

const couldNotParse = "could not parse typeSpecificData"

type helmDeployerDI struct {
	crAndSecretClient client.Client
	uncachedClient    synchronize.UncachedClient
	helmFacade        Facade
	appRepoClient     client.Client
	blockObject       *synchronize.BlockObject
}

func NewHelmDeployerDI(crAndSecretClient client.Client, uncachedClient synchronize.UncachedClient, appRepoClient client.Client,
	blockObject *synchronize.BlockObject) deployutil.DeployItemDeployer {
	helmFacade := &FacadeImpl{Client: NewDefaultClient()}
	return NewHelmDeployerDIWithFacade(crAndSecretClient, uncachedClient, helmFacade, appRepoClient, blockObject)
}

func NewHelmDeployerDIWithFacade(crAndSecretClient client.Client, uncachedClient synchronize.UncachedClient, helmFacade Facade,
	appRepoClient client.Client, blockObject *synchronize.BlockObject) deployutil.DeployItemDeployer {
	return &helmDeployerDI{
		crAndSecretClient: crAndSecretClient,
		uncachedClient:    uncachedClient,
		helmFacade:        helmFacade,
		appRepoClient:     appRepoClient,
		blockObject:       blockObject,
	}
}

func (r *helmDeployerDI) ProcessNewOperation(ctx context.Context, deployData *deployutil.DeployData) {
	configID := deployData.Configuration.DeploymentConfig.ID

	rel, err := r.processItem(ctx, deployData)
	now := metav1.Now()
	if err != nil {
		switch err.(type) {
		case *deployutil.ClusterUnreachableError:
			deployutil.LogHubFailure(ctx, deployutil.ReasonFailedClusterUnreachable,
				"Deployment failed for application "+configID+", because cluster is unreachable", err)
			deployData.SetStatusForUnreachableCluster()
		default:
			deployutil.LogHubFailure(ctx, deployutil.ReasonFailedDeployment, "Deployment failed for application "+configID, err)
			deployData.SetStatus(util.StateFailed, err.Error(), 1, now)
		}
	} else {
		deployutil.LogSuccess(ctx, deployutil.ReasonSuccessDeployment, "Deployment done for application "+configID)
		deployData.SetStatus(util.StateOk, r.successDescription(deployData), 1, now)
	}

	r.computeReadinessAndExport(ctx, deployData, rel, now)
}

func (r *helmDeployerDI) ReconcileOperation(ctx context.Context, deployData *deployutil.DeployData) {
	log := util.GetLoggerFromContext(ctx)
	configID := deployData.Configuration.DeploymentConfig.ID
	log.V(util.LogLevelDebug).Info("reconcile",
		"observedGeneration", deployData.GetObservedGeneration(),
		"generation", deployData.GetGeneration())

	rel, err := r.processItem(ctx, deployData)
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

	r.computeReadinessAndExport(ctx, deployData, rel, now)
}

func (r *helmDeployerDI) RetryFailedOperation(ctx context.Context, deployData *deployutil.DeployData) {
	configID := deployData.Configuration.DeploymentConfig.ID
	lastOp := deployData.ProviderStatus.LastOperation

	rel, err := r.processItem(ctx, deployData)
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
		deployData.SetStatus(util.StateOk, r.successDescription(deployData), 1, now)
	}

	r.computeReadinessAndExport(ctx, deployData, rel, now)
}

func (r *helmDeployerDI) ProcessPendingOperation(ctx context.Context, deployData *deployutil.DeployData) {
	configID := deployData.Configuration.DeploymentConfig.ID

	rel, err := r.getRelease(ctx, deployData)
	now := metav1.Now()
	if err != nil {
		switch err.(type) {
		case *deployutil.ClusterUnreachableError:
			deployutil.LogHubFailure(ctx, deployutil.ReasonFailedClusterUnreachable,
				"Readiness check failed for application "+configID+", because cluster is unreachable", err)
		default:
			deployutil.LogHubFailure(ctx, deployutil.ReasonFailedDeployment, "Readiness check failed for application "+configID, err)
		}
		deployData.SetStatusForUnreachableCluster()
	} else {
		deployData.SetStatusForReachableCluster()
	}

	r.computeReadinessAndExport(ctx, deployData, rel, now)
}

func (r *helmDeployerDI) Cleanup(ctx context.Context, deployData *deployutil.DeployData, clusterExists bool) error {
	return nil
}

func (r *helmDeployerDI) Preprocess(ctx context.Context, deployData *deployutil.DeployData) {
}

func (r *helmDeployerDI) processItem(ctx context.Context, deployData *deployutil.DeployData) (*release.Release, error) { // nolint
	log := util.GetLoggerFromContext(ctx)

	isInstallOperation := deployData.IsInstallOperation()

	namedInternalSecretNames := deployData.Configuration.DeploymentConfig.NamedInternalSecretNames
	namedSecretResolver := apitypes.NewNamedSecretResolver(r.crAndSecretClient, deployData.GetNamespace(), namedInternalSecretNames)

	helmSpecificData, err := apitypes.NewHelmSpecificData(&deployData.Configuration.DeploymentConfig.TypeSpecificData)
	if err != nil {
		msg := couldNotParse
		log.Error(err, msg)
		return nil, errors.Wrap(err, msg)
	}

	helmChartData, namespace, err := ParseTypeSpecificData(ctx, namedSecretResolver, &deployData.Configuration.DeploymentConfig, helmSpecificData,
		isInstallOperation, r.appRepoClient)
	if err != nil {
		msg := couldNotParse
		log.Error(err, msg)
		return nil, errors.Wrap(err, msg)
	}

	secretKey := deployData.GetSecretKey()
	targetKubeconfig, err := deployutil.GetTargetConfig(ctx, r.crAndSecretClient, *secretKey)
	if err != nil {
		return nil, err
	}

	if isInstallOperation {
		err = r.mergeSecretValues(ctx, deployData, helmChartData, helmSpecificData)
		if err != nil {
			return nil, err
		}

		reblockDuration := max(helmChartData.InstallTimeout, helmChartData.UpgradeTimeout) + time.Minute
		clusterBomKey := util.GetClusterBomKeyFromDeployItemKey(deployData.GetDeployItemKey())
		_, err := r.blockObject.Reblock(ctx, clusterBomKey, r.uncachedClient, reblockDuration, true)
		if err != nil {
			return nil, err
		}

		metadata := ReleaseMetadata{
			BomName: clusterBomKey.Name,
		}

		return r.helmFacade.InstallOrUpdate(ctx, helmChartData, namespace, string(targetKubeconfig), &metadata)
	} else { // nolint
		reblockDuration := helmChartData.UninstallTimeout + time.Minute
		clusterBomKey := util.GetClusterBomKeyFromDeployItemKey(deployData.GetDeployItemKey())
		_, err := r.blockObject.Reblock(ctx, clusterBomKey, r.uncachedClient, reblockDuration, true)
		if err != nil {
			return nil, err
		}

		return nil, r.helmFacade.Remove(ctx, helmChartData, namespace, string(targetKubeconfig))
	}
}

func (r *helmDeployerDI) computeReadinessAndExport(ctx context.Context, deployData *deployutil.DeployData, rel *release.Release, now metav1.Time) {
	r.computeReadiness(ctx, deployData, rel, now)

	readyCondition := deployData.GetDeployItemCondition(hubv1.HubDeploymentReady)
	if readyCondition != nil && readyCondition.Status == v1alpha1.ConditionTrue {
		err := r.computeExports(ctx, deployData)
		if err != nil {
			deployData.ReplaceDeployItemCondition(hubv1.HubDeploymentReady, corev1.ConditionUnknown, now,
				hubv1.ReasonCouldNotGetExport, "Could not get export data")

			deployData.SetPhase(v1alpha1.ExecutionPhaseProgressing)
		}
	}
}

func (r *helmDeployerDI) computeReadiness(ctx context.Context, deployData *deployutil.DeployData,
	rel *release.Release, now metav1.Time) { // nolint
	log := util.GetLoggerFromContext(ctx)

	// special case for unreachable cluster - just replace condition to unknown state
	if deployData.ProviderStatus.Reachability != nil && !deployData.ProviderStatus.Reachability.Reachable {
		deployData.ReplaceDeployItemCondition(hubv1.HubDeploymentReady, corev1.ConditionUnknown, now,
			hubv1.ReasonClusterUnreachable, "Cluster is unreachable")
		return
	}

	// compute readiness
	var readinessState string
	if deployData.ProviderStatus.LastOperation.Operation == util.OperationInstall {
		if deployData.ProviderStatus.LastOperation.SuccessGeneration > 0 {
			// The operation has at least once succeeded; but it is possible that the last reconcile has failed.
			if rel != nil && rel.Info != nil {
				if rel.Info.Status == release.StatusDeployed {
					readinessState = r.computeReadinessOnTargetCluster(ctx, deployData, &rel.Manifest)
				} else if rel.Info.Status == release.StatusPendingInstall || rel.Info.Status == release.StatusPendingUpgrade {
					readinessState = util.StatePending
				} else if rel.Info.Status == release.StatusFailed {
					readinessState = util.StateFailed
				} else {
					log.Error(nil, "Unexpected release info", "releaseInfoStatus", rel.Info.Status)
					readinessState = util.StateUnknown
				}
			} else {
				log.V(util.LogLevelWarning).Info("No release info")
				readinessState = util.StateUnknown
			}
		} else {
			// The operation has never succeeded
			readinessState = util.StateFailed
		}
	} else if deployData.ProviderStatus.LastOperation.Operation == util.OperationRemove {
		readinessState = util.StateNotRelevant
	} else {
		log.Error(nil, "Unexpected operation", "operation", deployData.ProviderStatus.LastOperation.Operation)
		readinessState = util.StateUnknown
	}

	deployData.ProviderStatus.Readiness = &hubv1.Readiness{
		State: readinessState,
		Time:  now,
	}

	// compute condition
	if deployData.GetObservedGeneration() == 0 {
		deployData.ReplaceDeployItemCondition(hubv1.HubDeploymentReady, corev1.ConditionUnknown, now, hubv1.ReasonInitialState, "No deployment executed until now")
		deployData.SetPhase(v1alpha1.ExecutionPhaseProgressing)
	} else if deployData.IsInstallOperation() {
		// install operation
		if deployData.GetGeneration() != deployData.ProviderStatus.LastOperation.SuccessGeneration {
			deployData.ReplaceDeployItemCondition(hubv1.HubDeploymentReady, corev1.ConditionUnknown, now, hubv1.ReasonUpgradePending, "Upgrade pending")
			deployData.SetPhase(v1alpha1.ExecutionPhaseProgressing)
		} else if readinessState == util.StateOk {
			deployData.ReplaceDeployItemCondition(hubv1.HubDeploymentReady, corev1.ConditionTrue, now, hubv1.ReasonRunning, "Running")
			deployData.SetPhase(v1alpha1.ExecutionPhaseSucceeded)
		} else if readinessState == util.StateFinallyFailed {
			deployData.ReplaceDeployItemCondition(hubv1.HubDeploymentReady, corev1.ConditionFalse, now, hubv1.ReasonFinallyFailed, "Finally Failed")
			deployData.SetPhase(v1alpha1.ExecutionPhaseFailed)
		} else {
			deployData.ReplaceDeployItemCondition(hubv1.HubDeploymentReady, corev1.ConditionUnknown, now, hubv1.ReasonNotRunning, "Readiness is "+readinessState)
			deployData.SetPhase(v1alpha1.ExecutionPhaseProgressing)
		}
	} else { // nolint
		// remove operation
		if deployData.ProviderStatus.LastOperation.Operation == util.OperationInstall {
			deployData.ReplaceDeployItemCondition(hubv1.HubDeploymentReady, corev1.ConditionUnknown, now, hubv1.ReasonRemovePending, "Remove pending")
			deployData.SetPhase(v1alpha1.ExecutionPhaseProgressing)
		} else if deployData.ProviderStatus.LastOperation.Operation == util.OperationRemove {
			if deployData.ProviderStatus.LastOperation.State == util.StateOk {
				deployData.ReplaceDeployItemCondition(hubv1.HubDeploymentReady, corev1.ConditionTrue, now, hubv1.ReasonRemoved, "Removed")
				deployData.SetPhase(v1alpha1.ExecutionPhaseSucceeded)
			} else {
				deployData.ReplaceDeployItemCondition(hubv1.HubDeploymentReady, corev1.ConditionUnknown, now, hubv1.ReasonRemovePending,
					"Last try to remove has failed")
				deployData.SetPhase(v1alpha1.ExecutionPhaseDeleting)
			}
		}
	}
}

func (r *helmDeployerDI) computeExports(ctx context.Context, deployData *deployutil.DeployData) error {
	log := util.GetLoggerFromContext(ctx)

	helmSpecificData, err := apitypes.NewHelmSpecificData(&deployData.Configuration.DeploymentConfig.TypeSpecificData)
	if err != nil {
		msg := couldNotParse
		log.Error(err, msg)
		return err
	}

	if len(helmSpecificData.InternalExport) > 0 {
		secretKey := deployData.GetSecretKey()
		dynamicTargetClient, err := deployutil.NewDynamicTargetClient(ctx, r.crAndSecretClient, *secretKey)
		if err != nil {
			log.Error(err, "Error fetching dynamic target client")
			return err
		}

		newExportData := make(map[string]interface{})

		for key, exportEntry := range helmSpecificData.InternalExport {
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

func (r *helmDeployerDI) computeReadinessOnTargetCluster(ctx context.Context, deployData *deployutil.DeployData, manifest *string) string {
	log := util.GetLoggerFromContext(ctx)

	helmSpecificData, err := apitypes.NewHelmSpecificData(&deployData.Configuration.DeploymentConfig.TypeSpecificData)
	if err != nil {
		msg := couldNotParse
		log.Error(err, msg)
		return util.StateUnknown
	}

	basicKubernetesObjects, err := unmarshalManifest(manifest, deployutil.ReadinessFilter)
	if err != nil {
		log.Error(err, "Error unmarshaling manifest")
		return util.StateUnknown
	}

	secretKey := deployData.GetSecretKey()
	targetClient, err := deployutil.GetTargetClient(ctx, r.crAndSecretClient, *secretKey)
	if err != nil {
		log.Error(err, "Error fetching target client")
		return util.StateUnknown
	}

	dynamicTargetClient, err := deployutil.NewDynamicTargetClient(ctx, r.crAndSecretClient, *secretKey)
	if err != nil {
		log.Error(err, "Error fetching dynamic target client")
		return util.StateUnknown
	}

	return deployData.ComputeReadiness(ctx, basicKubernetesObjects, targetClient, dynamicTargetClient, helmSpecificData.Namespace)
}

func (r *helmDeployerDI) getRelease(ctx context.Context, deployData *deployutil.DeployData) (*release.Release, error) {
	log := util.GetLoggerFromContext(ctx)

	namedInternalSecretNames := deployData.Configuration.DeploymentConfig.NamedInternalSecretNames
	namedSecretResolver := apitypes.NewNamedSecretResolver(r.crAndSecretClient, deployData.GetNamespace(), namedInternalSecretNames)

	helmSpecificData, err := apitypes.NewHelmSpecificData(&deployData.Configuration.DeploymentConfig.TypeSpecificData)
	if err != nil {
		msg := couldNotParse
		log.Error(err, msg)
		return nil, errors.Wrap(err, msg)
	}

	helmChartData, namespace, err := ParseTypeSpecificData(ctx, namedSecretResolver, &deployData.Configuration.DeploymentConfig,
		helmSpecificData, true, r.appRepoClient)
	if err != nil {
		msg := couldNotParse
		log.Error(err, msg)
		return nil, errors.Wrap(err, msg)
	}

	secretKey := deployData.GetSecretKey()
	targetKubeconfig, err := deployutil.GetTargetConfig(ctx, r.crAndSecretClient, *secretKey)
	if err != nil {
		return nil, err
	}

	return r.helmFacade.GetRelease(ctx, helmChartData, namespace, string(targetKubeconfig))
}

func (r *helmDeployerDI) mergeSecretValues(ctx context.Context, deployData *deployutil.DeployData, helmChartData *ChartData,
	helmSpecificData *apitypes.HelmSpecificData) error {
	log := util.GetLoggerFromContext(ctx)

	// Merge secret values to values
	if deployData.Configuration.DeploymentConfig.InternalSecretName != "" {
		secretKey := types.NamespacedName{
			Name:      deployData.Configuration.DeploymentConfig.InternalSecretName,
			Namespace: deployData.GetNamespace(),
		}
		secret := corev1.Secret{}
		err := r.crAndSecretClient.Get(ctx, secretKey, &secret)
		if err != nil {
			msg := "could not read secret values"
			log.Error(err, msg, util.LogKeySecretName, deployData.Configuration.DeploymentConfig.InternalSecretName)
			return errors.Wrap(err, msg)
		}

		var secretValues map[string]interface{}
		secretBytes := secret.Data[util.SecretValuesKey]
		err = json.Unmarshal(secretBytes, &secretValues)
		if err != nil {
			msg := "could not unmarshal secret values"
			log.Error(err, msg, util.LogKeySecretName, deployData.Configuration.DeploymentConfig.InternalSecretName)
			return errors.Wrap(err, msg)
		}

		helmChartData.Values = deployutil.MergeMaps(helmChartData.Values, secretValues)
	}

	// Merge named secret values to values
	excludedSecretName := ""
	if helmSpecificData != nil && helmSpecificData.TarballAccess != nil {
		excludedSecretName = helmSpecificData.TarballAccess.SecretRef.Name
	}

	for logicalSecretName, internalSecretName := range deployData.Configuration.DeploymentConfig.NamedInternalSecretNames {
		if logicalSecretName == excludedSecretName {
			continue
		}

		secretKey := types.NamespacedName{
			Name:      internalSecretName,
			Namespace: deployData.GetNamespace(),
		}
		secret := corev1.Secret{}
		err := r.crAndSecretClient.Get(ctx, secretKey, &secret)
		if err != nil {
			msg := "could not read named secret values for " + logicalSecretName
			log.Error(err, msg, util.LogKeySecretName, deployData.Configuration.DeploymentConfig.InternalSecretName)
			return errors.Wrap(err, msg)
		}

		secretValues := make(map[string]interface{})
		for key, rawValue := range secret.Data {
			var value interface{}
			err = yaml.Unmarshal(rawValue, &value)
			if err != nil {
				msg := "could not unmarshal entry of named secret value for logicalSecretName " + logicalSecretName + ", internalSecretName " + internalSecretName
				log.Error(err, msg)
				return errors.Wrap(err, msg)
			}

			secretValues[key] = value
		}

		helmChartData.Values = deployutil.MergeMaps(helmChartData.Values, secretValues)
	}

	// Merge imported values to values
	for paramName, paramValueObject := range deployData.Configuration.DeploymentConfig.InternalImportParameters.Parameters {
		var valueObject map[string]interface{}
		err := json.Unmarshal(paramValueObject, &valueObject)
		if err != nil {
			msg := "could not unmarshal imported value " + paramName
			log.Error(err, msg)
			return errors.Wrap(err, msg)
		}

		m, ok := valueObject["value"].(map[string]interface{})
		if !ok {
			msg := "value of internal import parameter " + paramName + " is not a map"
			log.Error(err, msg)
			return errors.Wrap(err, msg)
		}

		helmChartData.Values = deployutil.MergeMaps(helmChartData.Values, m)
	}

	return nil
}

func (r *helmDeployerDI) successDescription(deployData *deployutil.DeployData) string {
	if deployData.IsInstallOperation() {
		return "install successful"
	}

	return "remove successful"
}

func max(d1, d2 time.Duration) time.Duration {
	if d1 < d2 {
		return d2
	}
	return d1
}
