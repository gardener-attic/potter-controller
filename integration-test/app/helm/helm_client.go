package helm

import (
	"context"
	"fmt"
	"os"

	"github.wdf.sap.corp/kubernetes/hub-controller/integration-test/app/util"

	"github.com/pkg/errors"
	grpcStatus "google.golang.org/grpc/status"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/kube"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage"
	"helm.sh/helm/v3/pkg/storage/driver"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type HelmClient struct { // nolint
	targetKubeConfig string
}

func NewHelmClient(ctx context.Context, config *util.IntegrationTestConfig, gardenClient client.Client) *HelmClient {
	return &HelmClient{
		targetKubeConfig: util.GetTargetKubeConfig(ctx, gardenClient, config),
	}
}

func (p *HelmClient) GetDeployedValues(ctx context.Context, installName, namespace string) map[string]interface{} {
	helmrelease, err := p.getRelease(ctx, installName, namespace, p.targetKubeConfig)
	if err != nil {
		util.Write(err, "Unable to get release")
		os.Exit(1)
	}
	return helmrelease.Config
}

func (p *HelmClient) getRelease(ctx context.Context, name, namespace, kubeconfig string) (*release.Release, error) {
	config, err := initActionConfig(ctx, kubeconfig, namespace)
	if err != nil {
		return nil, err
	}

	rls, err := action.NewGet(config).Run(name)

	if err != nil {
		return nil, errors.New(prettyError(err).Error())
	}

	// We check that the release found is from the provided namespace.
	// If `namespace` is an empty string we do not do that check
	// This check check is to prevent users of for example updating releases that might be
	// in namespaces that they do not have access to.
	if namespace != "" && rls.Namespace != namespace {
		return nil, errors.Errorf("release %q not found in namespace %q", name, namespace)
	}

	return rls, err
}

func initActionConfig(ctx context.Context, kubeconfig, namespace string) (*action.Configuration, error) {
	logf := createLogFunc(ctx)

	restClientGetter := newRemoteRESTClientGetter([]byte(kubeconfig), namespace)
	kc := kube.New(restClientGetter)
	kc.Log = logf

	clientset, err := kc.Factory.KubernetesClientSet()
	if err != nil {
		return nil, err
	}

	store := getStorageType(ctx, clientset, namespace)

	actionConfig := action.Configuration{
		RESTClientGetter: restClientGetter,
		Releases:         store,
		KubeClient:       kc,
		Log:              logf,
	}

	return &actionConfig, nil
}

func prettyError(err error) error {
	// Add this check can prevent the object creation if err is nil.
	if err == nil {
		return nil
	}
	// If it's grpc's error, make it more user-friendly.
	if s, ok := grpcStatus.FromError(err); ok {
		return fmt.Errorf(s.Message())
	}
	// Else return the original error.
	return err
}

func createLogFunc(ctx context.Context) func(format string, v ...interface{}) {
	return func(format string, v ...interface{}) {
	}
}

func getStorageType(ctx context.Context, clientset *kubernetes.Clientset, namespace string) *storage.Storage {
	logf := createLogFunc(ctx)

	var store *storage.Storage
	switch os.Getenv("HELM_DRIVER") {
	case "secret", "secrets", "":
		d := driver.NewSecrets(clientset.CoreV1().Secrets(namespace))
		d.Log = logf
		store = storage.Init(d)
	case "configmap", "configmaps":
		d := driver.NewConfigMaps(clientset.CoreV1().ConfigMaps(namespace))
		d.Log = logf
		store = storage.Init(d)
	case "memory":
		d := driver.NewMemory()
		store = storage.Init(d)
	default:
		// Not sure what to do here.
		panic("Unknown driver in HELM_DRIVER: " + os.Getenv("HELM_DRIVER"))
	}
	return store
}
