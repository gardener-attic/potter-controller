package kapptestsuite

import (
	"context"
	"os"
	"time"

	hubv1 "github.com/gardener/potter-controller/api/v1"
	"github.com/gardener/potter-controller/integration-test/app/builder"
	"github.com/gardener/potter-controller/integration-test/app/util"

	landscaper "github.com/gardener/landscaper/apis/core/v1alpha1"
	"github.com/vmware-tanzu/carvel-kapp-controller/pkg/apis/kappctrl/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Run(ctx context.Context, config *util.IntegrationTestConfig, gardenClient client.Client) {
	// cleanup before needed
	configIDKappTest := "kapp1"
	clusterbomKeyKappTest := config.GetTestClusterBomKey("kapp.git")
	cl := util.NewClusterBomClient(gardenClient)
	util.WriteSubSectionHeader("Cleanup " + clusterbomKeyKappTest.String())
	cl.DeleteClusterBom(ctx, clusterbomKeyKappTest)

	configIDKappHTTPTest := "kapp"
	clusterbomKeyKappHTTPTest := config.GetTestClusterBomKey("kapp.http")
	util.WriteSubSectionHeader("Cleanup " + clusterbomKeyKappHTTPTest.String())
	cl.DeleteClusterBom(ctx, clusterbomKeyKappHTTPTest)

	configIDKappTestFailing1 := "kappfail1"
	clusterbomKeyTestFailing1 := config.GetTestClusterBomKey(configIDKappTestFailing1)
	util.WriteSubSectionHeader("Cleanup " + clusterbomKeyTestFailing1.String())
	cl.DeleteClusterBom(ctx, clusterbomKeyTestFailing1)

	// run tests
	runKappTest(ctx, config, gardenClient, configIDKappTest, clusterbomKeyKappTest)
	runKappHTTPTest(ctx, config, gardenClient, configIDKappHTTPTest, clusterbomKeyKappHTTPTest)
	runKappTestFailing1(ctx, config, gardenClient, configIDKappTestFailing1, clusterbomKeyTestFailing1)
}

func runKappTestFailing1(ctx context.Context, config *util.IntegrationTestConfig, gardenClient client.Client,
	configIDKapp string, clusterbomKey types.NamespacedName) {
	message := "Kapp Test Failing 1 "
	util.WriteSectionHeader(message+"started", config.TestLandscaper)

	util.WriteSubSectionHeader("Create ClusterBom with Kapp Application")
	util.Write("Build ClusterBom " + clusterbomKey.String())

	clusterBom := builder.NewClusterBom(clusterbomKey, config.TargetClusterName, config.TestLandscaper)

	appConfig1 := builder.NewAppConfigForKapp(configIDKapp, defineFailingTestApp1(config))
	builder.AddAppConfig(clusterBom, appConfig1)

	cl := util.NewClusterBomClient(gardenClient)
	cl.CreateClusterBom(ctx, clusterBom)

	// get app key
	deployItemList := util.GetDeployItems(ctx, gardenClient, clusterbomKey, 1)

	appKey := types.NamespacedName{
		Namespace: config.GardenNamespace,
		Name:      deployItemList.Items[0].Name,
	}

	// check pause status with problem but not paused
	util.Write("Get app with problem")
	util.GetAppWithPaused(ctx, gardenClient, appKey, false)
	app := util.GetAppWithProblem(ctx, gardenClient, appKey, true)

	pauseStatus := util.GetPauseStatus(ctx, app)
	util.CheckPauseStatus(false, pauseStatus.PausedSince, true, pauseStatus.ProblemSince, pauseStatus)

	// let is pause
	util.Write("Get paused app")
	newProblemSince := pauseStatus.ProblemSince.Add(-16 * time.Minute)
	util.Write("NewProblemSince: " + newProblemSince.Format(time.RFC3339))
	util.UpdatePauseStatus(ctx, gardenClient, appKey,
		pauseStatus.Paused, pauseStatus.PausedSince, pauseStatus.Problem, newProblemSince)

	app = util.GetAppWithPaused(ctx, gardenClient, appKey, true)
	pauseStatus = util.GetPauseStatus(ctx, app)
	util.CheckPauseStatus(true, pauseStatus.PausedSince, true, newProblemSince, pauseStatus)

	// unpause
	util.Write("Wake up paused app")
	newPausedSince := time.Now().Add(-5 * time.Minute)
	util.UpdatePauseStatus(ctx, gardenClient, appKey,
		pauseStatus.Paused, newPausedSince, pauseStatus.Problem, pauseStatus.ProblemSince)

	app = util.GetAppWithNewPausedSince(ctx, gardenClient, appKey, newPausedSince)
	pauseStatus = util.GetPauseStatus(ctx, app)
	util.CheckPauseStatus(true, pauseStatus.PausedSince, true, newProblemSince, pauseStatus)

	// repair
	util.Write("Repair clusterbom")
	appConfig2 := builder.NewAppConfigForKapp(configIDKapp, defineTestApp1(config))
	builder.RemoveAppConfig(clusterBom, configIDKapp)
	builder.AddAppConfig(clusterBom, appConfig2)
	cl.UpdateAppConfigsInClusterBom(ctx, clusterBom)
	util.CheckClusterBomCondition(ctx, clusterBom, cl)

	app = util.GetAppWithPaused(ctx, gardenClient, appKey, false)
	pauseStatus = util.GetPauseStatus(ctx, app)
	util.CheckPauseStatus(false, time.Time{}, false, time.Time{}, pauseStatus)

	// Delete ClusterBom
	util.WriteSubSectionHeader("Delete ClusterBom with Kapp Application")
	cl.DeleteClusterBom(ctx, clusterbomKey)

	util.Write(message + "successfully finished")
}

func runKappTest(ctx context.Context, config *util.IntegrationTestConfig, gardenClient client.Client,
	configIDKapp string, clusterbomKey types.NamespacedName) {
	util.WriteSectionHeader("Kapp Test", config.TestLandscaper)

	util.WriteSubSectionHeader("Create ClusterBom with Kapp Application")
	util.Write("Build ClusterBom " + clusterbomKey.String())

	clusterBom := builder.NewClusterBom(clusterbomKey, config.TargetClusterName, config.TestLandscaper)

	appConfig1 := builder.NewAppConfigForKapp(configIDKapp, defineTestApp1(config))
	builder.AddAppConfig(clusterBom, appConfig1)

	cl := util.NewClusterBomClient(gardenClient)
	cl.CreateClusterBom(ctx, clusterBom)
	util.CheckClusterBomCondition(ctx, clusterBom, cl)

	deployItemList := landscaper.DeployItemList{}

	err := gardenClient.List(ctx, &deployItemList, client.InNamespace(config.GardenNamespace),
		client.MatchingLabels{hubv1.LabelClusterBomName: clusterbomKey.Name})

	if err != nil {
		util.Write(err, "Unable to read deploy items")
		os.Exit(1)
	}

	if len(deployItemList.Items) != 1 {
		util.Write(err, "not exactly one deploy item")
		os.Exit(1)
	}

	appKey := types.NamespacedName{
		Namespace: config.GardenNamespace,
		Name:      deployItemList.Items[0].Name,
	}
	var app v1alpha1.App
	err = gardenClient.Get(ctx, appKey, &app)
	if err != nil {
		util.Write(err, "Unable to read app "+appKey.String())
		os.Exit(1)
	}

	if app.Spec.SyncPeriod == nil || app.Spec.SyncPeriod.Duration != time.Minute {
		util.Write(err, "Wrong SyncPeriod")
		os.Exit(1)
	}

	util.WriteSubSectionHeader("Update Kapp Application")

	appConfig2 := builder.NewAppConfigForKapp(configIDKapp, defineTestApp2(config))
	builder.RemoveAppConfig(clusterBom, configIDKapp)
	builder.AddAppConfig(clusterBom, appConfig2)

	cl.UpdateAppConfigsInClusterBom(ctx, clusterBom)
	util.CheckClusterBomCondition(ctx, clusterBom, cl)

	// Delete ClusterBom
	util.WriteSubSectionHeader("Delete ClusterBom with Kapp Application")
	cl.DeleteClusterBom(ctx, clusterbomKey)

	util.Write("KappTest successfully finished")
}

func runKappHTTPTest(ctx context.Context, config *util.IntegrationTestConfig, gardenClient client.Client,
	configIDKapp string, clusterbomKey types.NamespacedName) {
	util.WriteSectionHeader("Kapp HTTP Test", config.TestLandscaper)

	util.WriteSubSectionHeader("Create ClusterBom with Kapp Application")
	util.Write("Build ClusterBom " + clusterbomKey.String())

	clusterBom := builder.NewClusterBom(clusterbomKey, config.TargetClusterName, config.TestLandscaper)

	appConfig1 := builder.NewAppConfigForKapp(configIDKapp, defineTestAppHTTP(config))
	builder.AddAppConfig(clusterBom, appConfig1)

	util.WriteClusterBom(clusterBom)

	cl := util.NewClusterBomClient(gardenClient)
	cl.CreateClusterBom(ctx, clusterBom)
	util.CheckClusterBomCondition(ctx, clusterBom, cl)

	// Delete ClusterBom
	util.WriteSubSectionHeader("Delete ClusterBom with Kapp Application")
	cl.DeleteClusterBom(ctx, clusterbomKey)

	util.Write("KappHTTPTest successfully finished")
}

func defineFailingTestApp1(config *util.IntegrationTestConfig) *v1alpha1.AppSpec {
	return &v1alpha1.AppSpec{
		SyncPeriod: &metav1.Duration{
			Duration: time.Minute,
		},
		Cluster: &v1alpha1.AppCluster{
			Namespace: config.TargetClusterNamespace1,
		},
		Fetch: []v1alpha1.AppFetch{
			{
				Git: &v1alpha1.AppFetchGit{
					URL: "https://github.com/k14s/k8s-simple-app-example-wrong",
					Ref: "origin/develop",
				},
			},
		},
		Template: []v1alpha1.AppTemplate{
			{
				Ytt: &v1alpha1.AppTemplateYtt{
					Paths: []string{
						"config-step-2-template",
					},
				},
			},
		},
		Deploy: []v1alpha1.AppDeploy{
			{
				Kapp: &v1alpha1.AppDeployKapp{
					IntoNs: config.TargetClusterNamespace1,
				},
			},
		},
	}
}

func defineTestApp1(config *util.IntegrationTestConfig) *v1alpha1.AppSpec {
	return &v1alpha1.AppSpec{
		SyncPeriod: &metav1.Duration{
			Duration: time.Minute,
		},
		Cluster: &v1alpha1.AppCluster{
			Namespace: config.TargetClusterNamespace1,
		},
		Fetch: []v1alpha1.AppFetch{
			{
				Git: &v1alpha1.AppFetchGit{
					URL: "https://github.com/k14s/k8s-simple-app-example",
					Ref: "origin/develop",
				},
			},
		},
		Template: []v1alpha1.AppTemplate{
			{
				Ytt: &v1alpha1.AppTemplateYtt{
					Paths: []string{
						"config-step-2-template",
					},
				},
			},
		},
		Deploy: []v1alpha1.AppDeploy{
			{
				Kapp: &v1alpha1.AppDeployKapp{
					IntoNs: config.TargetClusterNamespace1,
				},
			},
		},
	}
}

func defineTestApp2(config *util.IntegrationTestConfig) *v1alpha1.AppSpec {
	app := defineTestApp1(config)
	app.Template[0].Ytt.Paths = []string{
		"config-step-2-template",
		"config-step-2b-multiple-data-values",
	}
	return app
}

func defineTestAppHTTP(config *util.IntegrationTestConfig) *v1alpha1.AppSpec {
	return &v1alpha1.AppSpec{
		Fetch: []v1alpha1.AppFetch{
			{
				HTTP: &v1alpha1.AppFetchHTTP{
					URL: "https://storage.googleapis.com/hub-tarballs/kapp/success/http-zip-yml/config/simple.zip",
				},
			},
		},
		Template: []v1alpha1.AppTemplate{
			{
				Ytt: &v1alpha1.AppTemplateYtt{},
			},
		},
		Deploy: []v1alpha1.AppDeploy{
			{
				Kapp: &v1alpha1.AppDeployKapp{
					IntoNs: config.TargetClusterNamespace1,
				},
			},
		},
	}
}
