package helm

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
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
