package exportimporttestsuite

import (
	"context"
	"encoding/json"

	hubv1 "github.wdf.sap.corp/kubernetes/hub-controller/api/v1"
	"github.wdf.sap.corp/kubernetes/hub-controller/integration-test/app/builder"
	"github.wdf.sap.corp/kubernetes/hub-controller/integration-test/app/catalog"
	"github.wdf.sap.corp/kubernetes/hub-controller/integration-test/app/helm"
	"github.wdf.sap.corp/kubernetes/hub-controller/integration-test/app/util"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Run(ctx context.Context, config *util.IntegrationTestConfig, gardenClient client.Client) {
	runSimpleExportImportTest(ctx, config, gardenClient)
}

func runSimpleExportImportTest(ctx context.Context, config *util.IntegrationTestConfig, gardenClient client.Client) {
	util.WriteSectionHeader("Export Import Test", config.TestLandscaper)

	const (
		configIDEchoServerExport = "echoexport"
		configIDEchoServerImport = "echoimport"
	)

	exportClusterbomKey := config.GetTestClusterBomKey("export.simple")
	importClusterbomKey := config.GetTestClusterBomKey("import.simple")

	cl := util.NewClusterBomClient(gardenClient)

	// Delete ClusterBoms
	util.WriteSubSectionHeader("Cleanup")
	util.Parallel(func() {
		cl.DeleteClusterBom(ctx, importClusterbomKey)
	}, func() {
		cl.DeleteClusterBom(ctx, exportClusterbomKey)
	})

	// Exporting clusterbom
	util.WriteSubSectionHeader("Create Exporting ClusterBom")
	util.Write("Build ClusterBom " + exportClusterbomKey.String())
	exportClusterBom := builder.NewClusterBom(exportClusterbomKey, config.TargetClusterName, config.TestLandscaper)

	util.Write("Add appconfig " + configIDEchoServerExport + " (echo-server-1.0.1.tgz)")
	helmdata := builder.NewHelmDataWithTarballAccess(util.CreateEchoServerInstallationNameWithSuffix(config, "ex"), config.TargetClusterNamespace1, &catalog.TarballEcho)
	internalExportParameterName := "resourcelimitscpuInternal"
	builder.SetInternalExportsToEchoServer(helmdata, internalExportParameterName)
	exportAppConfig := builder.NewAppConfigForHelm(configIDEchoServerExport, helmdata)
	builder.SetValues(exportAppConfig, map[string]interface{}{
		"resources": map[string]interface{}{
			"limits": map[string]interface{}{
				"cpu": "110m",
			},
		},
	})
	exportParameterName := "resourcelimitscpu"
	builder.SetExports(exportAppConfig, exportParameterName, internalExportParameterName)
	builder.AddAppConfig(exportClusterBom, exportAppConfig)

	cl.CreateClusterBom(ctx, exportClusterBom)

	// Importing clusterbom
	util.WriteSubSectionHeader("Create Importing ClusterBom")
	util.Write("Build ClusterBom " + importClusterbomKey.String())
	importClusterBom := builder.NewClusterBom(importClusterbomKey, config.TargetClusterName, config.TestLandscaper)

	util.Write("Add appconfig " + configIDEchoServerImport + " (echo-server-1.0.1.tgz)")
	importInstallationName := util.CreateEchoServerInstallationNameWithSuffix(config, "im")
	importNamespace := config.TargetClusterNamespace1
	importHelmData := builder.NewHelmDataWithTarballAccess(importInstallationName, importNamespace, &catalog.TarballEcho)
	importAppConfig := builder.NewAppConfigForHelm(configIDEchoServerImport, importHelmData)

	importParameterName := "importedCpu"
	internalImportParameterName := "internalImportedCpu"
	importAppConfig.ImportParameters = []hubv1.ImportParameter{
		{
			Name:            importParameterName,
			ClusterBomName:  exportClusterbomKey.Name,
			AppID:           configIDEchoServerExport,
			ExportParamName: exportParameterName,
		},
	}
	importAppConfig.InternalImportParameters = hubv1.InternalImportParameters{
		Parameters: map[string]json.RawMessage{
			internalImportParameterName: builder.RawMsg(map[string]interface{}{
				"value": map[string]interface{}{
					"resources": map[string]interface{}{
						"limits": map[string]interface{}{
							"cpu": "(( imports." + importParameterName + " ))",
						},
					},
				},
			}),
		},
	}

	builder.AddAppConfig(importClusterBom, importAppConfig)

	cl.CreateClusterBom(ctx, importClusterBom)

	// Perform checks for both clusterboms
	util.CheckClusterBomCondition(ctx, exportClusterBom, cl)
	util.CheckClusterBomCondition(ctx, importClusterBom, cl)

	helmClient := helm.NewHelmClient(ctx, config, gardenClient)
	deployedValues := helmClient.GetDeployedValues(ctx, importInstallationName, importNamespace)
	util.CheckDeployedValue(deployedValues, map[string]interface{}{
		"resources": map[string]interface{}{
			"limits": map[string]interface{}{
				"cpu": "110m",
			},
		},
	})

	// Update
	util.WriteSubSectionHeader("Update Exporting ClusterBom")
	exportAppConfig = &exportClusterBom.Spec.ApplicationConfigs[0]
	builder.SetValues(exportAppConfig, map[string]interface{}{
		"resources": map[string]interface{}{
			"limits": map[string]interface{}{
				"cpu": "120m",
			},
		},
	})
	cl.UpdateAppConfigsInClusterBom(ctx, exportClusterBom)

	// Perform checks for both clusterboms
	util.CheckClusterBomCondition(ctx, exportClusterBom, cl)
	util.CheckClusterBomCondition(ctx, importClusterBom, cl)

	deployedValues = helmClient.GetDeployedValues(ctx, importInstallationName, importNamespace)
	util.CheckDeployedValue(deployedValues, map[string]interface{}{
		"resources": map[string]interface{}{
			"limits": map[string]interface{}{
				"cpu": "120m",
			},
		},
	})

	// Delete ClusterBom
	util.WriteSubSectionHeader("Delete ClusterBoms")
	util.Parallel(func() {
		cl.DeleteClusterBom(ctx, importClusterbomKey)
	}, func() {
		cl.DeleteClusterBom(ctx, exportClusterbomKey)
	})

	util.Write("Export Import Test successfully finished")
}
