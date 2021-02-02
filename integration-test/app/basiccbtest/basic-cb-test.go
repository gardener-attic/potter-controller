package basiccbtest

import (
	"context"

	hubv1 "github.com/gardener/potter-controller/api/v1"
	"github.com/gardener/potter-controller/integration-test/app/builder"
	"github.com/gardener/potter-controller/integration-test/app/catalog"
	"github.com/gardener/potter-controller/integration-test/app/util"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Run(ctx context.Context, config *util.IntegrationTestConfig, gardenClient client.Client) {
	runBasicBomTest(ctx, config, gardenClient)
	runFailureBomTest(ctx, config, gardenClient)
	runEmptyBomTest(ctx, config, gardenClient)
	runJobReadyRequirementsTest(ctx, config, gardenClient)
	runResourceReadyRequirementsTest(ctx, config, gardenClient)
	runReconcileTest(ctx, config, gardenClient)
}

func runBasicBomTest(ctx context.Context, config *util.IntegrationTestConfig, gardenClient client.Client) {
	util.WriteSectionHeader("Basic ClusterBom Test", config.TestLandscaper)

	clusterbomKey := config.GetTestClusterBomKey("basic")

	cl := util.NewClusterBomClient(gardenClient)

	util.WriteSubSectionHeader("Cleanup")
	cl.DeleteClusterBom(ctx, clusterbomKey)

	util.WriteSubSectionHeader("Create ClusterBom")
	util.Write("Build ClusterBom " + clusterbomKey.String())
	clusterBom := builder.NewClusterBom(clusterbomKey, config.TargetClusterName, config.TestLandscaper)

	// Add first app
	const appConfigID1 = "echo"
	helmdata := builder.NewHelmDataWithCatalogAccess(util.CreateEchoServerInstallationName(config), config.TargetClusterNamespace1, &catalog.ChartEcho2)
	var timeout int64 = 6
	helmdata.UpgradeTimeout = &timeout
	appConfig := builder.NewAppConfigForHelm(appConfigID1, helmdata)
	util.WriteAddAppConfig(appConfig.ID, helmdata)
	builder.AddAppConfig(clusterBom, appConfig)

	cl.CreateClusterBom(ctx, clusterBom)

	util.CheckClusterBomCondition(ctx, clusterBom, cl)

	// Add second app
	const appConfigID2 = "nginx"
	util.WriteSubSectionHeader("Add Application")
	helmdata = builder.NewHelmDataWithTarballAccess("nginx", config.TargetClusterNamespace2, &catalog.TarballNginx1)
	appConfig = builder.NewAppConfigForHelm(appConfigID2, helmdata)

	util.WriteAddAppConfig(appConfig.ID, helmdata)
	builder.AddAppConfig(clusterBom, appConfig)

	cl.UpdateAppConfigsInClusterBom(ctx, clusterBom)

	util.CheckClusterBomCondition(ctx, clusterBom, cl)

	// Upgrade second app
	util.WriteSubSectionHeader("Upgrade Application")
	helmdata = builder.NewHelmDataWithTarballAccess("nginx", config.TargetClusterNamespace2, &catalog.TarballNginx2)
	util.WriteUpgradeAppConfig(appConfigID2, helmdata)
	builder.ReplaceAppInClusterBom(clusterBom, appConfigID2, helmdata)

	cl.UpdateAppConfigsInClusterBom(ctx, clusterBom)

	util.CheckClusterBomCondition(ctx, clusterBom, cl)

	// Remove second app
	util.WriteSubSectionHeader("Remove Application")
	util.Write("Delete App " + clusterbomKey.String())
	builder.RemoveAppConfig(clusterBom, appConfigID2)

	cl.UpdateAppConfigsInClusterBom(ctx, clusterBom)

	util.CheckClusterBomCondition(ctx, clusterBom, cl)

	// Delete ClusterBom
	util.WriteSubSectionHeader("Delete ClusterBom")
	cl.DeleteClusterBom(ctx, clusterbomKey)

	util.Write("BasicBomTest successfully finished")
}

func runFailureBomTest(ctx context.Context, config *util.IntegrationTestConfig, gardenClient client.Client) {
	util.WriteSectionHeader("Failure ClusterBom Test", config.TestLandscaper)

	clusterbomKey := config.GetTestClusterBomKey("fail")

	cl := util.NewClusterBomClient(gardenClient)

	util.WriteSubSectionHeader("Cleanup")
	cl.DeleteClusterBom(ctx, clusterbomKey)

	util.WriteSubSectionHeader("Create ClusterBom")
	util.Write("Build ClusterBom " + clusterbomKey.String())
	clusterBom := builder.NewClusterBom(clusterbomKey, config.TargetClusterName, config.TestLandscaper)

	const appConfigID = "echo"
	const installName = "echo05"
	helmdata := builder.NewHelmDataWithCatalogAccess(installName, config.TargetClusterNamespace1, &catalog.ChartEcho1)
	appConfig := builder.NewAppConfigForHelm(appConfigID, helmdata)
	util.WriteAddAppConfig(appConfig.ID, helmdata)
	builder.AddAppConfig(clusterBom, appConfig)

	cl.CreateClusterBom(ctx, clusterBom)

	util.CheckClusterBomCondition(ctx, clusterBom, cl)

	util.Write("ClusterBom before upgrading to not existing version 5.0.1111111111")
	util.WriteClusterBom(cl.GetClusterBom(ctx, clusterbomKey))

	util.WriteSubSectionHeader("Upgrade Application, Expecting Error")
	util.Write("Upgrade echo server to not existing version " + catalog.ChartEchoInvalid.ChartVersion)
	helmdata = builder.NewHelmDataWithCatalogAccess(installName, config.TargetClusterNamespace1, &catalog.ChartEchoInvalid)
	builder.ReplaceAppInClusterBom(clusterBom, appConfigID, helmdata)

	cl.UpdateAppConfigsInClusterBom(ctx, clusterBom)

	util.Write("Check failure of ClusterBom " + clusterbomKey.String())
	util.CheckFailedApplication(ctx, clusterBom, appConfigID, gardenClient)

	util.WriteSubSectionHeader("Upgrade Application")
	helmdata = builder.NewHelmDataWithCatalogAccess(installName, config.TargetClusterNamespace1, &catalog.ChartEcho2)
	builder.ReplaceAppInClusterBom(clusterBom, appConfigID, helmdata)
	util.WriteAddAppConfig(appConfigID, helmdata)

	cl.UpdateAppConfigsInClusterBom(ctx, clusterBom)

	util.CheckClusterBomCondition(ctx, clusterBom, cl)

	util.WriteSubSectionHeader("Delete ClusterBom")
	cl.DeleteClusterBom(ctx, clusterbomKey)

	util.Write("FailureBomTest successfully finished")
}

func runEmptyBomTest(ctx context.Context, config *util.IntegrationTestConfig, gardenClient client.Client) {
	util.WriteSectionHeader("Empty ClusterBom Test", config.TestLandscaper)

	clusterbomKey := config.GetTestClusterBomKey("empty")

	cl := util.NewClusterBomClient(gardenClient)

	util.WriteSubSectionHeader("Cleanup")
	cl.DeleteClusterBom(ctx, clusterbomKey)

	util.WriteSubSectionHeader("Create ClusterBom")
	util.Write("Build ClusterBom " + clusterbomKey.String())
	clusterBom := builder.NewClusterBom(clusterbomKey, config.TargetClusterName, config.TestLandscaper)

	const appConfigID1 = "echo"
	installName1 := util.CreateEchoServerInstallationNameWithSuffix(config, "empty")
	helmdata1 := builder.NewHelmDataWithCatalogAccess(installName1, config.TargetClusterNamespace1, &catalog.ChartEcho2)
	appConfig1 := builder.NewAppConfigForHelm(appConfigID1, helmdata1)
	util.WriteAddAppConfig(appConfig1.ID, helmdata1)
	builder.AddAppConfig(clusterBom, appConfig1)

	const appConfigID2 = "nginx"
	helmdata2 := builder.NewHelmDataWithTarballAccess("nginx02", config.TargetClusterNamespace2, &catalog.TarballNginx1)
	appConfig2 := builder.NewAppConfigForHelm(appConfigID2, helmdata2)
	util.WriteAddAppConfig(appConfig2.ID, helmdata2)
	builder.AddAppConfig(clusterBom, appConfig2)

	cl.CreateClusterBom(ctx, clusterBom)

	util.CheckClusterBomCondition(ctx, clusterBom, cl)

	util.WriteSubSectionHeader("Delete All Applications")
	util.Write("Delete App " + clusterbomKey.String())
	builder.RemoveAppConfig(clusterBom, appConfig1.ID)

	util.Write("Delete App " + clusterbomKey.String())
	builder.RemoveAppConfig(clusterBom, appConfig2.ID)

	cl.UpdateAppConfigsInClusterBom(ctx, clusterBom)

	util.CheckClusterBomCondition(ctx, clusterBom, cl)

	util.WriteSubSectionHeader("Delete ClusterBom")
	cl.DeleteClusterBom(ctx, clusterbomKey)

	util.Write("EmptyBomTest successfully finished")
}

func runJobReadyRequirementsTest(ctx context.Context, config *util.IntegrationTestConfig, gardenClient client.Client) {
	util.WriteSectionHeader("Job Ready Requirements Test", config.TestLandscaper)

	clusterbomKey := config.GetTestClusterBomKey("ready.jobs")

	cl := util.NewClusterBomClient(gardenClient)

	util.WriteSubSectionHeader("Cleanup")
	cl.DeleteClusterBom(ctx, clusterbomKey)

	util.WriteSubSectionHeader("Create ClusterBom")
	util.Write("Build ClusterBom " + clusterbomKey.String())
	clusterBom := builder.NewClusterBom(clusterbomKey, config.TargetClusterName, config.TestLandscaper)

	jobName1 := util.CreateJobNameWithSuffix(config, "1")
	helmdata1 := builder.NewHelmDataWithTarballAccess(jobName1, config.TargetClusterNamespace1, &catalog.TarballJob2)
	appConfig1 := builder.NewAppConfigForHelm("job1", helmdata1)
	builder.SetValues(appConfig1, map[string]interface{}{
		"number": 2,
		"name":   jobName1,
	})
	builder.AddJobReadyRequirement(appConfig1, &hubv1.Job{
		Name:      jobName1,
		Namespace: config.TargetClusterNamespace1,
	})
	util.WriteAddAppConfig(appConfig1.ID, helmdata1)
	builder.AddAppConfig(clusterBom, appConfig1)

	cl.CreateClusterBom(ctx, clusterBom)

	util.CheckClusterBomCondition(ctx, clusterBom, cl)

	// Add App
	jobName2 := util.CreateJobNameWithSuffix(config, "2")
	util.WriteSubSectionHeader("Add Application")
	helmdata2 := builder.NewHelmDataWithTarballAccess(jobName2, config.TargetClusterNamespace2, &catalog.TarballJob2)
	appConfig2 := builder.NewAppConfigForHelm("job2", helmdata2)
	builder.SetValues(appConfig2, map[string]interface{}{
		"number": -2,
		"name":   jobName2,
	})
	builder.AddJobReadyRequirement(appConfig2, &hubv1.Job{
		Name:      jobName2,
		Namespace: config.TargetClusterNamespace2,
	})
	util.WriteAddAppConfig(appConfig2.ID, helmdata2)
	builder.AddAppConfig(clusterBom, appConfig2)

	cl.UpdateAppConfigsInClusterBom(ctx, clusterBom)

	util.CheckFailedClusterBom(ctx, clusterBom, gardenClient)

	// Delete ClusterBom
	util.WriteSubSectionHeader("Delete ClusterBom")
	cl.DeleteClusterBom(ctx, clusterbomKey)

	util.Write("JobReadyRequirementsTest successfully finished")
}

func runResourceReadyRequirementsTest(ctx context.Context, config *util.IntegrationTestConfig, gardenClient client.Client) {
	util.WriteSectionHeader("Resource Ready Requirements Test", config.TestLandscaper)

	clusterbomKey := config.GetTestClusterBomKey("ready.resouces")

	cl := util.NewClusterBomClient(gardenClient)

	util.WriteSubSectionHeader("Cleanup")
	cl.DeleteClusterBom(ctx, clusterbomKey)

	util.WriteSubSectionHeader("Create ClusterBom")
	util.Write("Build ClusterBom " + clusterbomKey.String())
	clusterBom := builder.NewClusterBom(clusterbomKey, config.TargetClusterName, config.TestLandscaper)

	jobName3 := util.CreateJobNameWithSuffix(config, "3")
	helmdata1 := builder.NewHelmDataWithTarballAccess(jobName3, config.TargetClusterNamespace1, &catalog.TarballJob2)
	appConfig1 := builder.NewAppConfigForHelm("job3", helmdata1)
	builder.SetValues(appConfig1, map[string]interface{}{
		"number": 2,
		"name":   jobName3,
	})

	jobResource1 := hubv1.Resource{
		Name:       jobName3,
		Namespace:  config.TargetClusterNamespace1,
		Resource:   "jobs",
		APIVersion: "batch/v1",
		FieldPath:  `{ .status.conditions[?(@.type == "Complete")].status }`,
		SuccessValues: []runtime.RawExtension{
			*builder.Raw(map[string]interface{}{
				"value": "True",
			}),
		},
	}

	builder.AddResourceReadyRequirement(appConfig1, &jobResource1)
	util.WriteAddAppConfig(appConfig1.ID, helmdata1)
	builder.AddAppConfig(clusterBom, appConfig1)

	cl.CreateClusterBom(ctx, clusterBom)

	util.CheckClusterBomCondition(ctx, clusterBom, cl)

	// Delete ClusterBom
	util.WriteSubSectionHeader("Delete ClusterBom")
	cl.DeleteClusterBom(ctx, clusterbomKey)

	util.Write("ResourceReadyRequirementsTest successfully finished")
}

func runReconcileTest(ctx context.Context, config *util.IntegrationTestConfig, gardenClient client.Client) {
	util.WriteSectionHeader("Reconcile Test", config.TestLandscaper)

	clusterbomKey := config.GetTestClusterBomKey("reconcile")

	cl := util.NewClusterBomClient(gardenClient)

	util.WriteSubSectionHeader("Cleanup")
	cl.DeleteClusterBom(ctx, clusterbomKey)

	util.WriteSubSectionHeader("Create ClusterBom")
	util.Write("Build ClusterBom " + clusterbomKey.String())
	clusterBom := builder.NewClusterBom(clusterbomKey, config.TargetClusterName, config.TestLandscaper)

	const appConfigID1 = "echo"
	helmdata1 := builder.NewHelmDataWithCatalogAccess(util.CreateEchoServerInstallationName(config), config.TargetClusterNamespace1, &catalog.ChartEcho2)
	appConfig1 := builder.NewAppConfigForHelm(appConfigID1, helmdata1)
	util.WriteAddAppConfig(appConfig1.ID, helmdata1)
	builder.AddAppConfig(clusterBom, appConfig1)

	const appConfigID2 = "nginx"
	helmdata2 := builder.NewHelmDataWithTarballAccess("nginx", config.TargetClusterNamespace2, &catalog.TarballNginx1)
	appConfig2 := builder.NewAppConfigForHelm(appConfigID2, helmdata2)
	util.WriteAddAppConfig(appConfig2.ID, helmdata2)
	builder.AddAppConfig(clusterBom, appConfig2)

	cl.CreateClusterBom(ctx, clusterBom)

	util.CheckClusterBomCondition(ctx, clusterBom, cl)

	diListBefore := cl.ListDIs(ctx, &clusterbomKey)

	// trigger reconcile
	cl.AddReconcileAnnotationInClusterBom(ctx, clusterBom)

	util.CheckClusterBomCondition(ctx, clusterBom, cl)
	util.CheckClusterReconcileAnnotation(ctx, clusterBom, cl)
	util.CheckLastOperationTimeIncreased(ctx, clusterbomKey, diListBefore, cl)

	// Delete ClusterBom
	util.WriteSubSectionHeader("Delete ClusterBom")
	cl.DeleteClusterBom(ctx, clusterbomKey)

	util.Write("BasicBomTest successfully finished")
}
