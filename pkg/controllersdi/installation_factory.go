package controllersdi

import (
	"encoding/json"
	"strings"

	hubv1 "github.com/gardener/potter-controller/api/v1"
	"github.com/gardener/potter-controller/pkg/util"

	ls "github.com/gardener/landscaper/pkg/apis/core/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"
)

const defaultExecutionName = "default"

func isLandscaperManagedClusterBom(clusterBom *hubv1.ClusterBom) bool {
	return clusterBom != nil &&
		util.HasAnnotation(clusterBom, hubv1.AnnotationKeyLandscaperManaged, hubv1.AnnotationValueLandscaperManaged)
}

func isLandscaperManagedDeployItem(deployItem *ls.DeployItem) bool { // nolint
	return deployItem != nil &&
		util.HasLabel(deployItem, hubv1.LabelLandscaperManaged, hubv1.LabelValueLandscaperManaged)
}

type InstallationFactory struct {
}

func (f *InstallationFactory) copyAppConfigToInstallation(appConfig *hubv1.ApplicationConfig, installation *ls.Installation,
	clusterBom *hubv1.ClusterBom) error {
	blueprintFilesystem, err := f.generateBlueprintFilesystem(clusterBom, appConfig)
	if err != nil {
		return err
	}

	installation.SetName(util.CreateInstallationName(clusterBom.GetName(), appConfig.ID))
	installation.SetNamespace(clusterBom.GetNamespace())

	installation.SetLabels(map[string]string{
		hubv1.LabelLandscaperManaged:   hubv1.LabelValueLandscaperManaged,
		hubv1.LabelApplicationConfigID: appConfig.ID,
		hubv1.LabelClusterBomName:      clusterBom.GetName(),
		hubv1.LabelConfigType:          appConfig.ConfigType,
	})

	dataImports := []ls.DataImport{}
	for i := range appConfig.ImportParameters {
		importParam := appConfig.ImportParameters[i]

		dataImport := ls.DataImport{
			Name:    importParam.Name,
			DataRef: importParam.ClusterBomName + util.DoubleSeparator + importParam.AppID + util.DoubleSeparator + importParam.ExportParamName,
		}

		dataImports = append(dataImports, dataImport)
	}

	dataExports := []ls.DataExport{}
	for name := range appConfig.ExportParameters.Parameters {
		dataExport := ls.DataExport{
			Name:    name,
			DataRef: clusterBom.Name + util.DoubleSeparator + appConfig.ID + util.DoubleSeparator + name,
		}

		dataExports = append(dataExports, dataExport)
	}

	installation.Spec = ls.InstallationSpec{
		Blueprint: ls.BlueprintDefinition{
			Inline: &ls.InlineBlueprint{
				Filesystem: blueprintFilesystem,
			},
		},

		Imports: ls.InstallationImports{
			Data: dataImports,
		},

		Exports: ls.InstallationExports{
			Data: dataExports,
		},
	}

	hash, err := util.HashObject256(installation.Spec)
	if err != nil {
		return err
	}

	util.AddAnnotation(installation, util.AnnotationKeyInstallationHash, hash)

	return nil
}

func (f *InstallationFactory) generateBlueprintFilesystem(clusterBom *hubv1.ClusterBom, appConfig *hubv1.ApplicationConfig) (json.RawMessage, error) {
	rawConfig, err := f.generateDeployItemConfigurationRaw(clusterBom, appConfig)
	if err != nil {
		return nil, err
	}

	executionTemplate := ls.ExecutionSpec{
		DeployItems: []ls.DeployItemTemplate{
			ls.DeployItemTemplate{
				Name: appConfig.ID,
				Type: ls.ExecutionType(appConfig.ConfigType),
				Labels: map[string]string{
					hubv1.LabelLandscaperManaged:   hubv1.LabelValueLandscaperManaged,
					hubv1.LabelApplicationConfigID: appConfig.ID,
					hubv1.LabelClusterBomName:      clusterBom.GetName(),
					hubv1.LabelConfigType:          appConfig.ConfigType,
				},
				Configuration: &runtime.RawExtension{
					Raw: rawConfig,
				},
			},
		},
	}

	executionTemplateBytes, err := json.Marshal(executionTemplate)
	if err != nil {
		return nil, err
	}

	imports := f.generateImports(appConfig)

	exports, exportExecutions, err := f.generateExportsAndExportExecutions(appConfig)
	if err != nil {
		return nil, err
	}

	blueprint := ls.Blueprint{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Blueprint",
			APIVersion: "landscaper.gardener.cloud/v1alpha1",
		},
		Imports:          imports,
		Exports:          exports,
		ExportExecutions: exportExecutions,
		DeployExecutions: []ls.TemplateExecutor{
			ls.TemplateExecutor{
				Name:     defaultExecutionName,
				Type:     ls.SpiffTemplateType,
				Template: json.RawMessage(executionTemplateBytes),
			},
		},
	}

	rawBlueprint, err := yaml.Marshal(blueprint)
	if err != nil {
		return nil, err
	}

	blueprintFilesystem := map[string]string{
		"blueprint.yaml": string(rawBlueprint),
	}

	return json.Marshal(blueprintFilesystem)
}

// cf. clusterbomController.copyAppConfigToDeployItem()
func (f *InstallationFactory) generateDeployItemConfigurationRaw(clusterbom *hubv1.ClusterBom, appconfig *hubv1.ApplicationConfig) ([]byte, error) {
	config := f.generateDeployItemConfiguration(clusterbom, appconfig)
	return json.Marshal(config)
}

func (f *InstallationFactory) generateDeployItemConfiguration(clusterbom *hubv1.ClusterBom,
	appconfig *hubv1.ApplicationConfig) *hubv1.HubDeployItemConfiguration {
	config := hubv1.HubDeployItemConfiguration{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HubDeployItemConfiguration",
			APIVersion: util.DeployItemConfigVersion,
		},
		LocalSecretRef: clusterbom.Spec.SecretRef,
		DeploymentConfig: hubv1.DeploymentConfig{
			ID:                       appconfig.ID,
			TypeSpecificData:         appconfig.TypeSpecificData,
			NoReconcile:              appconfig.NoReconcile,
			ReadyRequirements:        appconfig.ReadyRequirements,
			InternalImportParameters: appconfig.InternalImportParameters,
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

	return &config
}

func (f *InstallationFactory) generateImports(appConfig *hubv1.ApplicationConfig) (imports []ls.ImportDefinition) {
	imports = []ls.ImportDefinition{}

	for i := range appConfig.ImportParameters {
		importParamName := appConfig.ImportParameters[i].Name

		importEntry := ls.ImportDefinition{
			FieldValueDefinition: ls.FieldValueDefinition{
				Name:   importParamName,
				Schema: ls.JSONSchemaDefinition([]byte("{}")),
			},
		}

		imports = append(imports, importEntry)
	}

	return imports
}

func (f *InstallationFactory) generateExportsAndExportExecutions(
	appConfig *hubv1.ApplicationConfig) (exports []ls.ExportDefinition, exportExecutions []ls.TemplateExecutor, err error) {
	exports = []ls.ExportDefinition{}
	exportExecutions = []ls.TemplateExecutor{}

	for name, value := range appConfig.ExportParameters.Parameters {
		export := ls.ExportDefinition{
			FieldValueDefinition: ls.FieldValueDefinition{
				Name:   name,
				Schema: ls.JSONSchemaDefinition([]byte("{}")),
			},
		}

		exportMappingTemplate, err := f.generateExportMappingTemplate(appConfig.ID, name, value)
		if err != nil {
			return nil, nil, err
		}

		exportExecution := ls.TemplateExecutor{
			Name:     name,
			Type:     ls.SpiffTemplateType,
			Template: exportMappingTemplate,
		}

		exports = append(exports, export)
		exportExecutions = append(exportExecutions, exportExecution)
	}

	return exports, exportExecutions, nil
}

func (f *InstallationFactory) generateExportMappingTemplate(appConfigID, exportParamName string, value json.RawMessage) ([]byte, error) {
	valueString := string(value)
	valueString = strings.ReplaceAll(valueString, "internalExport.", "values.deployitems."+appConfigID+".")
	value = []byte(valueString)

	valueObject := map[string]interface{}{}
	err := json.Unmarshal(value, &valueObject)
	if err != nil {
		return nil, err
	}

	val := valueObject["value"]

	template := map[string]interface{}{
		"exports": map[string]interface{}{
			exportParamName: val,
		},
	}

	return json.Marshal(template)
}
