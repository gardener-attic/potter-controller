package helm

import (
	"context"
	"testing"

	"github.com/go-logr/zapr"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/gardener/potter-controller/pkg/util"
)

const (
	expectedToken         = "thisisAToken"
	expectedDecodedCaData = "thisIsACaData"
	expectedServer        = "https://api.hub.ein.hub.cluster.shoot.ondemand.com"
)

// nolint
var testKubeconfig = `apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: dGhpc0lzQUNhRGF0YQ==
    server: https://api.hub.ein.hub.cluster.shoot.ondemand.com
  name: shoot--a--hub
contexts:
- context:
    cluster: shoot--a--hub
    namespace: hub
    user: shoot--a--hub-token
  name: shoot--a--hub
current-context: shoot--a--hub
kind: Config
preferences: {}
users:
- name: shoot--a--hub-token
  user:
    token: thisisAToken`

const (
	expectedNs = "test-namespace"
)

func TestGetActionConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	NewGomegaWithT(t)

	ctx := addLoggingContext(context.Background())

	config, err := initActionConfig(ctx, testKubeconfig, expectedNs)
	Expect(err).To(BeNil())
	Expect(config).ToNot(BeNil())
}

func TestGetClientSet(t *testing.T) {
	RegisterFailHandler(Fail)
	NewGomegaWithT(t)

	ctx := addLoggingContext(context.Background())

	clientSet, err := getClientSet(ctx, testKubeconfig, "test")
	Expect(err).To(BeNil())
	Expect(clientSet).ToNot(BeNil())
}

func TestClientGetter(t *testing.T) {
	RegisterFailHandler(Fail)
	NewGomegaWithT(t)

	ctx := addLoggingContext(context.Background())

	config, err := initActionConfig(ctx, testKubeconfig, expectedNs)
	Expect(err).To(BeNil())

	remoteRESTClientGetter := newRemoteRESTClientGetter([]byte(testKubeconfig), expectedNs)
	ns, forceNs, err := remoteRESTClientGetter.ToRawKubeConfigLoader().Namespace()

	Expect(err).To(BeNil())
	Expect(forceNs).To(BeFalse())
	Expect(ns).To(Equal(expectedNs))

	clientConfig := remoteRESTClientGetter.ToRawKubeConfigLoader()

	restConfig, err := clientConfig.ClientConfig()
	Expect(err).To(BeNil())
	Expect(restConfig.BearerToken).To(Equal(expectedToken))
	Expect(string(restConfig.CAData)).To(Equal(expectedDecodedCaData))
	Expect(restConfig.Host).To(Equal(expectedServer))

	discoveryClient, err := config.RESTClientGetter.ToDiscoveryClient()
	Expect(err).To(BeNil())
	Expect(discoveryClient).ToNot(BeNil())

	restMapper, err := config.RESTClientGetter.ToRESTMapper()
	Expect(err).To(BeNil())
	Expect(restMapper).ToNot(BeNil())
}

func addLoggingContext(ctx context.Context) context.Context {
	log := zapr.NewLogger(zap.NewNop())
	return context.WithValue(ctx, util.LoggerKey{}, log)
}
