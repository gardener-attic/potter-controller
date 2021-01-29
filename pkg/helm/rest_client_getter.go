package helm

import (
	"context"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/kube"
	"helm.sh/helm/v3/pkg/storage"
	"helm.sh/helm/v3/pkg/storage/driver"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/gardener/potter-controller/pkg/util"
)

type remoteRESTClientGetter struct {
	kubeconfig []byte
	namespace  string
}

func newRemoteRESTClientGetter(kubeconfig []byte, namespace string) *remoteRESTClientGetter {
	return &remoteRESTClientGetter{
		kubeconfig: kubeconfig,
		namespace:  namespace,
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

func createLogFunc(ctx context.Context) func(format string, v ...interface{}) {
	log := ctx.Value(util.LoggerKey{}).(logr.Logger)

	return func(format string, v ...interface{}) {
		log.V(util.LogLevelDebug).Info(fmt.Sprintf(format, v))
	}
}

func getClientSet(ctx context.Context, kubeconfig, namespace string) (*kubernetes.Clientset, error) {
	logf := createLogFunc(ctx)

	restClientGetter := newRemoteRESTClientGetter([]byte(kubeconfig), namespace)

	kc := kube.New(restClientGetter)

	kc.Log = logf

	return kc.Factory.KubernetesClientSet()
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

func (k *remoteRESTClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	var config *rest.Config
	var err error
	if string(k.kubeconfig) != "" {
		config, err = clientcmd.RESTConfigFromKubeConfig(k.kubeconfig)
		if err != nil {
			panic(err.Error())
		}
	} else {
		panic("kubeconfig is empty")
	}

	return &ClientConfigGetter{
		config:    config,
		namespace: k.namespace,
	}
}

// ToRESTConfig returns restconfig
func (k *remoteRESTClientGetter) ToRESTConfig() (*rest.Config, error) {
	return k.ToRawKubeConfigLoader().ClientConfig()
}

// ToDiscoveryClient returns discovery client
func (k *remoteRESTClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	restConfig, err := k.ToRESTConfig()
	if err != nil {
		return nil, err
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return nil, err
	}

	client := cachedDiscoveryClient{discoveryClient}

	return client, err
}

// ToRESTMapper returns a restmapper
func (k *remoteRESTClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	discoveryClient, err := k.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}

	mapper := restmapper.NewDeferredDiscoveryRESTMapper(discoveryClient)
	expander := restmapper.NewShortcutExpander(mapper, discoveryClient)
	return expander, nil
}
