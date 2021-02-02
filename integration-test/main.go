package main

import (
	"context"
	"flag"
	"github.com/go-logr/zapr"
	"github.wdf.sap.corp/kubernetes/hub-controller/integration-test/app/admissionhooktestsuite"
	"github.wdf.sap.corp/kubernetes/hub-controller/integration-test/app/basiccbtest"
	"github.wdf.sap.corp/kubernetes/hub-controller/integration-test/app/namedsecretvaluestestsuite"
	"github.wdf.sap.corp/kubernetes/hub-controller/integration-test/app/secretvaluestestsuite"
	"go.uber.org/zap"
	"os"

	hubv1 "github.wdf.sap.corp/kubernetes/hub-controller/api/v1"
	"github.wdf.sap.corp/kubernetes/hub-controller/integration-test/app/exportimporttestsuite"
	"github.wdf.sap.corp/kubernetes/hub-controller/integration-test/app/kapptestsuite"
	mainutil "github.wdf.sap.corp/kubernetes/hub-controller/pkg/util"

	"github.wdf.sap.corp/kubernetes/hub-controller/integration-test/app/builder"
	"github.wdf.sap.corp/kubernetes/hub-controller/integration-test/app/util"

	landscaper "github.com/gardener/landscaper/pkg/apis/core/v1alpha1"
	log "github.com/sirupsen/logrus"
	kappcrtl "github.com/vmware-tanzu/carvel-kapp-controller/pkg/apis/kappctrl/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = hubv1.AddToScheme(scheme)
	_ = kappcrtl.AddToScheme(scheme)
	_ = landscaper.AddToScheme(scheme)
}

const (
	typeLandscaper   = "landscaper"
	typeNoLandscaper = "nolandscaper"
	typeAll          = "all"
)

func main() {
	log.Println("Integration Tests for hub-controller started")

	config, testType := readIntegrationTestConfig()

	gardenClient, err := getClient(config.GardenKubeconfigPath)
	if err != nil || gardenClient == nil {
		log.Println(err, "Unable to read gardenKubeconfigPath kubeconfig?")
		os.Exit(1)
	}

	ctx := context.Background()
	ctx = context.WithValue(ctx, mainutil.LoggerKey{}, zapr.NewLogger(zap.NewNop()))

	if testType == typeAll || testType == typeNoLandscaper {
		runTests(ctx, config, gardenClient, false)
	}

	if (testType == typeAll || testType == typeLandscaper) && isLandscaperEnabled(ctx, config, gardenClient) {
		runTests(ctx, config, gardenClient, true)
	}

	log.Println("\nIntegration Tests for hub-controller successfully finished")
}

func runTests(ctx context.Context, config *util.IntegrationTestConfig, gardenClient client.Client, landscaperTest bool) {
	config.TestLandscaper = landscaperTest

	basiccbtest.Run(ctx, config, gardenClient)

	admissionhooktestsuite.Run(ctx, config, gardenClient)

	secretvaluestestsuite.Run(ctx, config, gardenClient)

	namedsecretvaluestestsuite.Run(ctx, config, gardenClient)

	kapptestsuite.Run(ctx, config, gardenClient)

	if landscaperTest {
		exportimporttestsuite.Run(ctx, config, gardenClient)
	}
}

func readIntegrationTestConfig() (*util.IntegrationTestConfig, string) {
	log.Println("\nReading integration test configuration")

	var gardenKubeconfigPath string
	var gardenNamespace string
	var targetClusterName string
	var targetClusterNamespace1 string
	var targetClusterNamespace2 string
	var testPrefix string
	var testType string

	flag.StringVar(&gardenKubeconfigPath, "garden-kubeconfig", "", "kubeconfig of garden cluster for accessing ClusterBoms and HDCs")
	flag.StringVar(&gardenNamespace, "garden-namespace", "", "garden namespace to store ClusterBoms")
	flag.StringVar(&targetClusterName, "target-clustername", "", "name of the target shoot cluster")
	flag.StringVar(&targetClusterNamespace1, "target-cluster-namespace1", "", "namespace in target cluster for deployments")
	flag.StringVar(&targetClusterNamespace2, "target-cluster-namespace2", "", "namespace in target cluster for deployments")
	flag.StringVar(&testPrefix, "test-prefix", "", "prefix used to differentiate between parallel tests")
	flag.StringVar(&testType, "test-type", typeAll, "tests to execute: "+typeLandscaper+", "+typeNoLandscaper+", "+typeAll)

	flag.Parse()

	log.Println("Argument garden-kubeconfig: " + gardenKubeconfigPath)
	log.Println("Argument garden-namespace: " + gardenNamespace)
	log.Println("Argument target-clustername: " + targetClusterName)
	log.Println("Argument target-cluster-namespace1: " + targetClusterNamespace1)
	log.Println("Argument target-cluster-namespace2: " + targetClusterNamespace2)
	log.Println("Argument test-prefix: " + testPrefix)

	config := util.IntegrationTestConfig{
		GardenKubeconfigPath:    gardenKubeconfigPath,
		GardenNamespace:         gardenNamespace,
		TargetClusterName:       targetClusterName,
		TargetClusterNamespace1: targetClusterNamespace1,
		TargetClusterNamespace2: targetClusterNamespace2,
		TestPrefix:              testPrefix,
	}

	return &config, testType
}

func getClient(kubeconfigPath string) (client.Client, error) {
	kubeconfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, err
	}
	client, err := client.New(kubeconfig, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return nil, err
	}
	return client, nil
}

func isLandscaperEnabled(ctx context.Context, config *util.IntegrationTestConfig, gardenClient client.Client) bool {
	clusterbomKey := types.NamespacedName{
		Namespace: config.GardenNamespace,
		Name:      config.TestPrefix + "test-landcaper-enablement",
	}

	cl := util.NewClusterBomClient(gardenClient)

	util.WriteSectionHeader("Check Landscaper Enabled", true)
	cl.DeleteClusterBom(ctx, clusterbomKey)

	util.WriteSubSectionHeader("Create ClusterBom")
	util.Write("Build ClusterBom " + clusterbomKey.String())
	clusterBom := builder.NewClusterBom(clusterbomKey, config.TargetClusterName, true)

	creationErr := gardenClient.Create(ctx, clusterBom)

	cl.DeleteClusterBom(ctx, clusterbomKey)

	if creationErr != nil {
		util.Write(creationErr, "Landscaper not enabled")
		return false
	}

	util.Write("Landscaper enabled")
	return true
}
