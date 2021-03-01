package controllersdi

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"

	hubv1 "github.com/gardener/potter-controller/api/v1"
	"github.com/gardener/potter-controller/pkg/avcheck"
	"github.com/gardener/potter-controller/pkg/secrets"
	"github.com/gardener/potter-controller/pkg/synchronize"
	"github.com/gardener/potter-controller/pkg/util"

	landscaper "github.com/gardener/landscaper/apis/core/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func deleteInstallations(ctx context.Context, cli client.Client, installationList *landscaper.InstallationList) error {
	for i := range installationList.Items {
		installation := &installationList.Items[i]

		ctx, _ = util.EnrichContextAndLogger(ctx, util.LogKeyInstallationName, util.GetKey(installation))

		err := deleteInstallation(ctx, cli, installation)
		if err != nil {
			return err
		}
	}

	return nil
}

func deleteInstallation(ctx context.Context, cli client.Client, installation *landscaper.Installation) error {
	logger := util.GetLoggerFromContext(ctx)

	if !installation.ObjectMeta.DeletionTimestamp.IsZero() {
		return nil
	}

	logger.V(util.LogLevelDebug).Info("Deleting installation")
	err := cli.Delete(ctx, installation)
	if err != nil {
		logger.Error(err, "Error deleting installation")
		return err
	}

	return nil
}

func deleteDeployItems(ctx context.Context, cli client.Client, deployItemList *landscaper.DeployItemList) error {
	for i := range deployItemList.Items {
		deploymentItem := &deployItemList.Items[i]

		newLogger := util.GetLoggerFromContext(ctx).WithValues(util.LogKeyDeployItemName, util.GetKey(deploymentItem))
		newContext := context.WithValue(ctx, util.LoggerKey{}, newLogger)

		err := deleteDeployItem(newContext, cli, deploymentItem)
		if err != nil {
			return err
		}
	}

	return nil
}

// Deletes a deploy item.
// Exception: does not delete the deploy item if it is landscaper managed.
func deleteDeployItem(ctx context.Context, cli client.Client, deployItem *landscaper.DeployItem) error {
	logger := util.GetLoggerFromContext(ctx)

	if !deployItem.ObjectMeta.DeletionTimestamp.IsZero() {
		return nil
	}

	if isLandscaperManagedDeployItem(deployItem) {
		return nil
	}

	logger.V(util.LogLevelDebug).Info("Deleting deployItem")
	err := cli.Delete(ctx, deployItem)
	if err != nil {
		logger.Error(err, "Error deleting deployItem")
		return err
	}

	return nil
}

func adjustClusterBomStatusForNotExistingTargetCluster(ctx context.Context, cli client.StatusClient,
	clusterBom *hubv1.ClusterBom, secretKey *types.NamespacedName, avCheckConfig *avcheck.Configuration) error {
	if clusterBom == nil {
		return nil
	}

	logger := util.GetLoggerFromContext(ctx)
	logger.V(util.LogLevelDebug).Info("Adjust status of clusterbom for not existing target cluster",
		"secretKey", secretKey)

	var newStatus hubv1.ClusterBomStatus
	now := metav1.Now()
	newStatus.ObservedGeneration = clusterBom.ObjectMeta.Generation
	newStatus.OverallState = util.StatePending
	newStatus.OverallTime = now
	newStatus.OverallNumOfDeployments = 0
	newStatus.OverallNumOfReadyDeployments = 0
	newStatus.OverallProgress = 0
	newStatus.ApplicationStates = nil
	newStatus.Description = util.TextShootNotExisting
	newStatus.Conditions = []hubv1.ClusterBomCondition{
		{
			Type:               hubv1.ClusterBomReady,
			Status:             corev1.ConditionUnknown,
			LastUpdateTime:     now,
			LastTransitionTime: now,
			Reason:             hubv1.ReasonTargetClusterDoesNotExist,
			Message:            "Target cluster does not exist",
		},
		{
			Type:               hubv1.ClusterReachable,
			Status:             corev1.ConditionFalse,
			LastUpdateTime:     now,
			LastTransitionTime: now,
			Reason:             hubv1.ReasonClusterDoesNotExist,
			Message:            "Cluster does not exist",
		},
	}

	return updateClusterBomStatus(ctx, cli, clusterBom, &newStatus, avCheckConfig)
}

func updateClusterBomStatus(ctx context.Context, cli client.StatusClient,
	clusterBom *hubv1.ClusterBom, newStatus *hubv1.ClusterBomStatus, avCheckConfig *avcheck.Configuration) error {
	log := util.GetLoggerFromContext(ctx)

	// We update the status only if it has changed to avoid unnecessary events for the controller.
	if !hasClusterBomStatusChanged(&clusterBom.Status, newStatus) {
		log.V(util.LogLevelDebug).Info("Status of clusterbom has not changed; no update necessary")
		return nil
	}

	log.V(util.LogLevelDebug).Info("Updating status of clusterbom")

	clusterBom.Status = *newStatus
	err := cli.Status().Update(ctx, clusterBom)
	if err != nil {
		if util.IsConcurrentModificationErr(err) {
			if avCheckConfig != nil && clusterBom.Name == avCheckConfig.BomName && clusterBom.Namespace == avCheckConfig.Namespace {
				log.V(util.LogLevelDebug).Info("Problem updating status of av check clusterbom")
				return nil
			}

			log.V(util.LogLevelWarning).Info("Warning updating status of clusterbom due to parallel modification: " + err.Error())
			return err
		}

		log.Error(err, "Error updating status of clusterbom")
		return err
	}

	log.V(util.LogLevelDebug).Info("Status of clusterbom updated")

	return nil
}

func hasClusterBomStatusChanged(oldStatus, newStatus *hubv1.ClusterBomStatus) bool {
	return oldStatus.ObservedGeneration != newStatus.ObservedGeneration ||
		oldStatus.OverallState != newStatus.OverallState ||
		oldStatus.Description != newStatus.Description ||
		!isEqualDetailStates(oldStatus.ApplicationStates, newStatus.ApplicationStates) ||
		!util.IsEqualClusterBomConditionList(oldStatus.Conditions, newStatus.Conditions)
}

func isEqualDetailStates(oldList, newList []hubv1.ApplicationState) bool {
	if len(oldList) != len(newList) {
		return false
	}

	for oldIndex := range oldList {
		oldState := &oldList[oldIndex]

		found := false
		for newIndex := range newList {
			newState := &newList[newIndex]

			if oldState.ID == newState.ID {
				found = true

				if !isEqualDetailState(&oldState.DetailedState, &newState.DetailedState) {
					return false
				}
			}
		}

		if !found {
			return false
		}
	}

	return true
}

func isEqualDetailState(oldApplicationState, newApplicationState *hubv1.DetailedState) bool {
	test1 := oldApplicationState.LastOperation == newApplicationState.LastOperation
	test2 := oldApplicationState.CurrentOperation == newApplicationState.CurrentOperation

	test3 := oldApplicationState.Reachability == nil && newApplicationState.Reachability == nil
	test4 := oldApplicationState.Reachability != nil && newApplicationState.Reachability != nil && *oldApplicationState.Reachability == *newApplicationState.Reachability

	test5 := oldApplicationState.Readiness == nil && newApplicationState.Readiness == nil
	test6 := oldApplicationState.Readiness != nil && newApplicationState.Readiness != nil && *oldApplicationState.Readiness == *newApplicationState.Readiness

	test7 := oldApplicationState.TypeSpecificStatus == nil && newApplicationState.TypeSpecificStatus == nil
	test8 := oldApplicationState.TypeSpecificStatus != nil && newApplicationState.TypeSpecificStatus != nil &&
		bytes.Equal(oldApplicationState.TypeSpecificStatus.Raw, newApplicationState.TypeSpecificStatus.Raw)

	test9 := util.IsEqualHdcConditionList(oldApplicationState.HdcConditions, newApplicationState.HdcConditions)

	test10 := oldApplicationState.Generation == newApplicationState.Generation

	test11 := oldApplicationState.DeletionTimestamp == nil && newApplicationState.DeletionTimestamp == nil
	test12 := oldApplicationState.DeletionTimestamp != nil && newApplicationState.DeletionTimestamp != nil &&
		*oldApplicationState.DeletionTimestamp == *newApplicationState.DeletionTimestamp

	return test1 && test2 && (test3 || test4) && (test5 || test6) && (test7 || test8) && test9 &&
		test10 && (test11 || test12)
}

func findDeployItemInList(deployItems *landscaper.DeployItemList, id string) *landscaper.DeployItem {
	for i := range deployItems.Items {
		deployItem := &deployItems.Items[i]
		if id == util.GetAppConfigIDFromDeployItem(deployItem) {
			return deployItem
		}
	}
	return nil
}

func findInstallationInList(installations *landscaper.InstallationList, appID string) *landscaper.Installation {
	for i := range installations.Items {
		installation := &installations.Items[i]
		if util.HasLabel(installation, hubv1.LabelApplicationConfigID, appID) {
			return installation
		}
	}
	return nil
}

func findAppDeploymentConfigInList(applicationConfigs []hubv1.ApplicationConfig, id string) *hubv1.ApplicationConfig {
	for i := range applicationConfigs {
		appConfig := applicationConfigs[i]
		if appConfig.ID == id {
			return &appConfig
		}
	}
	return nil
}

func isEqualConfigForInstallation(clusterBom *hubv1.ClusterBom, appConfig *hubv1.ApplicationConfig, installation *landscaper.Installation) (bool, error) {
	installationFactory := InstallationFactory{}

	newInstallation := &landscaper.Installation{}
	err := installationFactory.copyAppConfigToInstallation(appConfig, newInstallation, clusterBom)
	if err != nil {
		return false, err
	}

	oldHash, _ := util.GetAnnotation(installation, util.AnnotationKeyInstallationHash)
	newHash, _ := util.GetAnnotation(newInstallation, util.AnnotationKeyInstallationHash)

	return oldHash == newHash, nil
}

func isEqualConfig(appConfig *hubv1.ApplicationConfig, deployItem *landscaper.DeployItem) (bool, error) {
	deployItemConfig := &hubv1.HubDeployItemConfiguration{}

	if err := json.Unmarshal(deployItem.Spec.Configuration.Raw, deployItemConfig); err != nil {
		return false, err
	}

	isEqual := appConfig.ID == deployItemConfig.DeploymentConfig.ID &&
		appConfig.ConfigType == string(deployItem.Spec.Type) &&
		appConfig.NoReconcile == deployItemConfig.DeploymentConfig.NoReconcile &&
		reflect.DeepEqual(appConfig.ReadyRequirements, deployItemConfig.DeploymentConfig.ReadyRequirements) &&
		isEqualRawJSON(appConfig.Values, deployItemConfig.DeploymentConfig.Values) &&
		isEqualRawJSON(&appConfig.TypeSpecificData, &deployItemConfig.DeploymentConfig.TypeSpecificData) &&
		isEqualSecretValues(appConfig.SecretValues, deployItemConfig.DeploymentConfig.InternalSecretName) &&
		isEqualNamedSecretValues(appConfig.NamedSecretValues, deployItemConfig.DeploymentConfig.NamedInternalSecretNames)

	return isEqual, nil
}

func isEqualNamedSecretValues(values map[string]hubv1.NamedSecretValues, names map[string]string) bool {
	if len(values) == 0 && len(names) == 0 {
		return true
	} else if len(values) != len(names) {
		return false
	}

	for k, v := range values {
		if v.InternalSecretName != names[k] {
			return false
		}
	}

	return true
}

func isEqualSecretValues(values *hubv1.SecretValues, name string) bool {
	equal := false
	if values == nil && name == "" {
		equal = true
	} else if values != nil && values.InternalSecretName == name {
		equal = true
	}

	return equal
}

// isEqualRawJson compares two given runtime.RawExtensions and checks for structural equality.
// The method does not simple compare the contained []bytes of the RawExtensions, but marshals and unmarshals it's
// content and then compares the byte arrays reflect.DeepEqual. With this, two RawExtensions with different order but
// structurally equal content are equal.
func isEqualRawJSON(value1, value2 *runtime.RawExtension) bool {
	var f1 interface{}
	var f2 interface{}

	if value1 == nil && value2 == nil {
		return true
	}
	if value1 == nil || value2 == nil {
		return false
	}
	_ = json.Unmarshal(value1.Raw, &f1)
	_ = json.Unmarshal(value2.Raw, &f2)

	marshaled1, _ := json.Marshal(f1)
	marshaled2, _ := json.Marshal(f2)

	return reflect.DeepEqual(marshaled1, marshaled2)
}

func deleteSecret(ctx context.Context, secret *corev1.Secret, secretClient client.Client,
	hubControllerClient synchronize.UncachedClient) error {
	log := util.GetLoggerFromContext(ctx)

	secretDeletionKey, err := secrets.GetSecretDeletionKey(ctx, hubControllerClient, true)
	if err != nil {
		log.Error(err, "Fetching secret deletion key failed")
		return err
	}

	token, err := util.ComputeSecretDeletionToken(secretDeletionKey, secret.Name)
	if err != nil {
		log.Error(err, "Failed to compute secret deletion token")
		return err
	}

	if secret.Data == nil {
		secret.Data = map[string][]byte{}
	}
	secret.Data[util.KeyDeletionToken] = token

	err = secretClient.Update(ctx, secret)
	if err != nil {
		log.Error(err, "Failed to update secret with deletion token")

		secretDeletionKey, err = secrets.GetSecretDeletionKey(ctx, hubControllerClient, false)
		if err != nil {
			log.Error(err, "Fetching secret deletion key failed")
			return err
		}

		token, err = util.ComputeSecretDeletionToken(secretDeletionKey, secret.Name)
		if err != nil {
			log.Error(err, "Failed to compute secret deletion token")
			return err
		}

		secret.Data[util.KeyDeletionToken] = token

		err = secretClient.Update(ctx, secret)
		if err != nil {
			log.Error(err, "Failed to update secret with deletion token (retry)")
			return err
		}
	}

	updatedSecret := corev1.Secret{}
	err = secretClient.Get(ctx, types.NamespacedName{Namespace: secret.Namespace, Name: secret.Name}, &updatedSecret)
	if err != nil {
		log.Error(err, "Failed to fetch secret after update with deletion token")
		return err
	}

	log.V(util.LogLevelWarning).Info("delete secret for secret values", util.LogKeySecretName, secret.Name)
	err = secretClient.Delete(ctx, &updatedSecret)
	if err != nil {
		log.Error(err, "Failed to delete secret")
		return err
	}

	return nil
}
