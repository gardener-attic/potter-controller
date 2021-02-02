package builder

import (
	"encoding/json"
	"os"

	"github.com/gardener/potter-controller/api/apitypes"
	hubv1 "github.com/gardener/potter-controller/api/v1"
	"github.com/gardener/potter-controller/integration-test/app/util"

	"github.com/vmware-tanzu/carvel-kapp-controller/pkg/apis/kappctrl/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

func NewClusterBom(clusterBomKey types.NamespacedName, targetClusterName string, testLandscaper bool) *hubv1.ClusterBom {
	annotations := map[string]string{}
	if testLandscaper {
		annotations[hubv1.AnnotationKeyLandscaperManaged] = hubv1.AnnotationValueLandscaperManaged
	}

	return &hubv1.ClusterBom{
		ObjectMeta: v1.ObjectMeta{
			Name:        clusterBomKey.Name,
			Namespace:   clusterBomKey.Namespace,
			Annotations: annotations,
		},
		Spec: hubv1.ClusterBomSpec{
			SecretRef: targetClusterName + ".kubeconfig",
		},
	}
}

func AddAppConfig(b *hubv1.ClusterBom, a *hubv1.ApplicationConfig) {
	b.Spec.ApplicationConfigs = append(b.Spec.ApplicationConfigs, *a)
}

func RemoveAppConfig(clusterBom *hubv1.ClusterBom, appConfigID string) {
	for i := range clusterBom.Spec.ApplicationConfigs {
		appConfig := &clusterBom.Spec.ApplicationConfigs[i]

		if appConfig.ID == appConfigID {
			clusterBom.Spec.ApplicationConfigs = remove(clusterBom.Spec.ApplicationConfigs, i)
			return
		}
	}

	util.Write("Unable to find appconfig to be removed: " + appConfigID)
	os.Exit(1)
}

func remove(s []hubv1.ApplicationConfig, i int) []hubv1.ApplicationConfig {
	s[i] = s[len(s)-1]
	// We do not need to put s[i] at the end, as it will be discarded anyway
	return s[:len(s)-1]
}

func NewAppConfigForHelm(configID string, helmData *apitypes.HelmSpecificData) *hubv1.ApplicationConfig {
	return newAppConfig(configID, util.ConfigTypeHelm, helmData)
}

func NewAppConfigForKapp(configID string, appSpec *v1alpha1.AppSpec) *hubv1.ApplicationConfig {
	return newAppConfig(configID, util.ConfigTypeKapp, appSpec)
}

func newAppConfig(configID, configType string, structuredTypeSpecificData interface{}) *hubv1.ApplicationConfig {
	return &hubv1.ApplicationConfig{
		ID:               configID,
		ConfigType:       configType,
		TypeSpecificData: *Raw(structuredTypeSpecificData),
	}
}

func SetValues(a *hubv1.ApplicationConfig, values map[string]interface{}) {
	a.Values = Raw(values)
}

func SetSecretValues(a *hubv1.ApplicationConfig, operation string, values map[string]interface{}) {
	if len(values) == 0 {
		a.SecretValues = &hubv1.SecretValues{
			Operation: operation,
			Data:      nil,
		}
	} else {
		a.SecretValues = &hubv1.SecretValues{
			Operation: operation,
			Data:      Raw(values),
		}
	}
}

func AddJobReadyRequirement(a *hubv1.ApplicationConfig, job *hubv1.Job) {
	a.ReadyRequirements.Jobs = append(a.ReadyRequirements.Jobs, *job)
}

func AddResourceReadyRequirement(a *hubv1.ApplicationConfig, resource *hubv1.Resource) {
	a.ReadyRequirements.Resources = append(a.ReadyRequirements.Resources, *resource)
}

func Raw(structuredData interface{}) *runtime.RawExtension {
	rawData, err := json.Marshal(structuredData)
	if err != nil {
		util.Write(err, "Unable to marshal data")
		os.Exit(1)
	}

	return &runtime.RawExtension{Raw: rawData}
}

func RawMsg(structuredData interface{}) json.RawMessage {
	rawData, err := json.Marshal(structuredData)
	if err != nil {
		util.Write(err, "Unable to marshal data")
		os.Exit(1)
	}

	return json.RawMessage(rawData)
}

func BuildClusterBom(clusterBomKey types.NamespacedName, targetClusterName, appconfigID string, helmdata *apitypes.HelmSpecificData, testLandscaper bool) *hubv1.ClusterBom {
	annotations := map[string]string{}
	if testLandscaper {
		annotations[hubv1.AnnotationKeyLandscaperManaged] = hubv1.AnnotationValueLandscaperManaged
	}

	var typeSpecificData runtime.RawExtension

	marshal, err := json.Marshal(helmdata)
	if err != nil {
		util.Write(err, "Unable to marshal helmdata")
		os.Exit(1)
	}

	typeSpecificData.Raw = marshal

	testClusterBom := &hubv1.ClusterBom{
		ObjectMeta: v1.ObjectMeta{
			Name:        clusterBomKey.Name,
			Namespace:   clusterBomKey.Namespace,
			Annotations: annotations,
		},
		Spec: hubv1.ClusterBomSpec{
			SecretRef: targetClusterName + ".kubeconfig",
			ApplicationConfigs: []hubv1.ApplicationConfig{
				{
					ID:               appconfigID,
					ConfigType:       "helm",
					TypeSpecificData: typeSpecificData,
				},
			},
		},
	}

	return testClusterBom
}

func NewHelmDataWithCatalogAccess(installName, targetClusterNamespace string, chart *apitypes.CatalogAccess) *apitypes.HelmSpecificData {
	return &apitypes.HelmSpecificData{
		InstallName:   installName,
		Namespace:     targetClusterNamespace,
		CatalogAccess: chart,
	}
}

func NewHelmDataWithCatalogAccessOLD(installName, targetClusterNamespace, catalog, chartName, chartVersion string) *apitypes.HelmSpecificData {
	helmdata := &apitypes.HelmSpecificData{
		InstallName: installName,
		Namespace:   targetClusterNamespace,
		CatalogAccess: &apitypes.CatalogAccess{
			Repo:         catalog,
			ChartName:    chartName,
			ChartVersion: chartVersion,
		},
	}
	return helmdata
}

func NewHelmDataWithTarballAccess(installName, targetClusterNamespace string, tarballAccess *apitypes.TarballAccess) *apitypes.HelmSpecificData {
	return &apitypes.HelmSpecificData{
		InstallName:   installName,
		Namespace:     targetClusterNamespace,
		TarballAccess: tarballAccess,
	}
}

func SetInternalExportsToEchoServer(helmdata *apitypes.HelmSpecificData, internalExportParameterName string) {
	deploymentName := helmdata.InstallName + "-echo-server"
	if len(deploymentName) > 24 {
		deploymentName = deploymentName[:24]
	}
	helmdata.InternalExport = map[string]apitypes.InternalExportEntry{
		internalExportParameterName: apitypes.InternalExportEntry{
			Name:       deploymentName,
			Namespace:  helmdata.Namespace,
			APIVersion: "apps/v1",
			Resource:   "deployments",
			FieldPath:  ".spec.template.spec.containers[0].resources.limits.cpu",
		},
	}
}

func SetExports(appConfig *hubv1.ApplicationConfig, exportParameterName, internalExportParameterName string) {
	appConfig.ExportParameters = hubv1.ExportParameters{
		Parameters: map[string]json.RawMessage{
			exportParameterName: RawMsg(map[string]string{
				"value": "(( internalExport." + internalExportParameterName + " ))",
			}),
		},
	}
}

func ReplaceAppInClusterBom(clusterBom *hubv1.ClusterBom, appconfigID string, helmdata *apitypes.HelmSpecificData) {
	var typeSpecificData runtime.RawExtension

	marshal, err := json.Marshal(helmdata)
	if err != nil {
		util.Write(err, "Unable to marshal helmdata")
		os.Exit(1)
	}

	typeSpecificData.Raw = marshal

	for i := range clusterBom.Spec.ApplicationConfigs {
		nextAppConfig := &clusterBom.Spec.ApplicationConfigs[i]

		if nextAppConfig.ID == appconfigID {
			nextAppConfig.TypeSpecificData = typeSpecificData
			return
		}
	}

	util.Write("Unable to find appconfig to be replaced: " + appconfigID)
	os.Exit(1)
}
