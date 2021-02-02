package admissionhooktestsuite

import (
	"context"
	"fmt"
	"os"

	"github.wdf.sap.corp/kubernetes/hub-controller/integration-test/app/builder"
	"github.wdf.sap.corp/kubernetes/hub-controller/integration-test/app/catalog"
	"github.wdf.sap.corp/kubernetes/hub-controller/integration-test/app/util"

	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Run(ctx context.Context, config *util.IntegrationTestConfig, gardenClient client.Client) {
	runValidatingAdmissionHookTest(ctx, config, gardenClient)
	runMutatingAdmissionHookTest(ctx, config, gardenClient)
}

// runValidatingAdmissionHookTest checks that the admission hook denies the creation of an invalid clusterbom with empty appConfigID and helmData.
func runValidatingAdmissionHookTest(ctx context.Context, config *util.IntegrationTestConfig, gardenClient client.Client) {
	util.WriteSectionHeader("Validating Admission Hook Test", config.TestLandscaper)

	clusterbomKey := config.GetTestClusterBomKey("hook.validating")

	cl := util.NewClusterBomClient(gardenClient)

	util.WriteSubSectionHeader("Cleanup")
	cl.DeleteClusterBom(ctx, clusterbomKey)

	util.WriteSubSectionHeader("Rejection Of Invalid ClusterBom")
	clusterbom := builder.BuildClusterBom(clusterbomKey, config.TargetClusterName, "", nil, config.TestLandscaper)

	util.Write("Trying to create invalid clusterbom " + clusterbomKey.String())
	err := gardenClient.Create(ctx, clusterbom)

	if err == nil {
		util.Write("Error: admission hook did not deny creation of an invalid clusterbom")
		os.Exit(1)
	}

	switch typedErr := err.(type) {
	case *errors.StatusError:
		code := typedErr.ErrStatus.Code
		if code != 200 {
			util.Write("Success: admission hook denied the creation of an invalid clusterbom")
		} else {
			log.Println(typedErr, "Error: creation of invalid clusterbom failed with status error; code: "+fmt.Sprint(code))
			os.Exit(1)
		}
	default:
		log.Println(err, "Creation of invalid clusterbom failed")
		os.Exit(1)
	}

	util.Write("ValidatingAdmissionHookTest successfully finished")
}

// runMutatingAdmissionHookTest checks that the admission hook adds a finalizer to a clusterbom.
func runMutatingAdmissionHookTest(ctx context.Context, config *util.IntegrationTestConfig, gardenClient client.Client) {
	util.WriteSectionHeader("Mutating Admission Hook Test", config.TestLandscaper)

	configID := "inttest04id"

	clusterbomKey := config.GetTestClusterBomKey("hook.mutating")

	cl := util.NewClusterBomClient(gardenClient)

	util.WriteSubSectionHeader("Cleanup")
	cl.DeleteClusterBom(ctx, clusterbomKey)

	util.WriteSubSectionHeader("Finalizer Check")
	util.Write("Build ClusterBom " + clusterbomKey.String())
	installName := util.CreateEchoServerInstallationNameWithSuffix(config, "hook")
	helmdata := builder.NewHelmDataWithCatalogAccess(installName, config.TargetClusterNamespace1, &catalog.ChartEcho2)
	util.WriteAddAppConfig(configID, helmdata)
	clusterBom := builder.BuildClusterBom(clusterbomKey, config.TargetClusterName, configID, helmdata, config.TestLandscaper)

	cl.CreateClusterBom(ctx, clusterBom)

	util.CheckClusterBomCondition(ctx, clusterBom, cl)

	storedClusterBom := cl.GetClusterBom(ctx, clusterbomKey)

	if len(storedClusterBom.ObjectMeta.Finalizers) > 0 {
		util.Write("Success: clusterbom has finalizer")
	} else {
		util.Write("Error: clusterbom has no finalizer")
	}

	util.WriteSubSectionHeader("Delete ClusterBom")
	cl.DeleteClusterBom(ctx, clusterbomKey)

	util.Write("MutatingAdmissionHookTest successfully finished")
}
