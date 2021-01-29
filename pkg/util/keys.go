package util

import (
	"strings"

	hubv1 "github.wdf.sap.corp/kubernetes/hub-controller/api/v1"

	"github.com/gardener/landscaper/pkg/apis/core/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
)

const (
	Separator       = "-"
	DoubleSeparator = "--"
)

func CreateDeployItemName(clusterBomName, appID string) string {
	return clusterBomName + Separator + appID
}

func CreateInstallationName(clusterBomName, appID string) string {
	return clusterBomName + DoubleSeparator + appID
}

func CreateSecretName(clusterBomName, appConfigID string) string {
	uniqueString := string(uuid.NewUUID())
	return clusterBomName + Separator + appConfigID + Separator + uniqueString
}

func GetClusterBomKeyFromDeployItemKey(deployItemKey *types.NamespacedName) *types.NamespacedName {
	index := strings.LastIndex(deployItemKey.Name, DoubleSeparator)
	if index == -1 {
		index = strings.LastIndex(deployItemKey.Name, Separator)
	}

	return &types.NamespacedName{
		Name:      deployItemKey.Name[0:index],
		Namespace: deployItemKey.Namespace,
	}
}

func GetClusterBomKeyFromDeployItem(deployItem *v1alpha1.DeployItem) *types.NamespacedName {
	name, _ := GetLabel(deployItem, hubv1.LabelClusterBomName)
	return &types.NamespacedName{
		Name:      name,
		Namespace: deployItem.Namespace,
	}
}

func GetAppConfigIDFromDeployItem(deployItem *v1alpha1.DeployItem) string {
	name, _ := GetLabel(deployItem, hubv1.LabelApplicationConfigID)
	return name
}

func GetAppConfigIDFromInstallation(installation *v1alpha1.Installation) string {
	name, _ := GetLabel(installation, hubv1.LabelApplicationConfigID)
	return name
}

func GetAppConfigIDFromInstallationKey(installationKey *types.NamespacedName) string {
	index := strings.LastIndex(installationKey.Name, DoubleSeparator)
	return installationKey.Name[index+len(DoubleSeparator):]
}

func GetClusterBomKeyFromInstallationKey(installationKey *types.NamespacedName) *types.NamespacedName {
	index := strings.LastIndex(installationKey.Name, DoubleSeparator)
	return &types.NamespacedName{
		Namespace: installationKey.Namespace,
		Name:      installationKey.Name[:index],
	}
}

func GetClusterBomKeyFromClusterBomOrDeployItem(clusterBom *hubv1.ClusterBom, deployItem *v1alpha1.DeployItem) *types.NamespacedName {
	if clusterBom != nil {
		return GetKey(clusterBom)
	}

	if deployItem != nil {
		return GetClusterBomKeyFromDeployItemKey(GetKey(deployItem))
	}

	return nil
}

func GetSecretKeyFromClusterBom(clusterBom *hubv1.ClusterBom) *types.NamespacedName {
	return &types.NamespacedName{
		Name:      clusterBom.Spec.SecretRef,
		Namespace: clusterBom.Namespace,
	}
}

func GetKey(obj metav1.Object) *types.NamespacedName { // nolint
	return &types.NamespacedName{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}
}
