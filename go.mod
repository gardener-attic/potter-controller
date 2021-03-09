module github.com/gardener/potter-controller

go 1.16

replace (
	github.com/docker/distribution => github.com/distribution/distribution v2.7.1+incompatible
	github.com/docker/docker => github.com/moby/moby v20.10.5+incompatible
	github.com/docker/docker@v0.7.3-0.20190327010347-be7ac8be2ae0 => github.com/moby/moby v20.10.5+incompatible
	github.com/docker/docker@v1.4.2-0.20200203170920-46ec8731fbce => github.com/moby/moby v20.10.5+incompatible
	// replace needed for hub-controller dependency
	github.com/moby/moby@v0.7.3-0.20190826074503-38ab9da00309 => github.com/moby/moby v20.10.5+incompatible
	golang.org/x/text@v0.3.0 => golang.org/x/text v0.3.5
	golang.org/x/text@v0.3.1 => golang.org/x/text v0.3.5
	golang.org/x/text@v0.3.2 => golang.org/x/text v0.3.5
	golang.org/x/text@v0.3.3 => golang.org/x/text v0.3.5
	golang.org/x/text@v0.3.4 => golang.org/x/text v0.3.5
)

require (
	github.com/arschles/assert v1.0.0
	github.com/coreos/go-oidc v2.2.1+incompatible
	github.com/docker/go-metrics v0.0.1 // indirect
	github.com/gardener/landscaper/apis v0.6.1-0.20210301094647-c077da8895ea
	github.com/ghodss/yaml v1.0.0
	github.com/go-logr/logr v0.3.0
	github.com/go-logr/zapr v0.3.0
	github.com/google/uuid v1.1.2
	github.com/gorilla/mux v1.8.0
	github.com/onsi/ginkgo v1.14.2
	github.com/onsi/gomega v1.10.4
	github.com/pkg/errors v0.9.1
	github.com/prometheus/common v0.10.0
	github.com/sirupsen/logrus v1.7.0
	github.com/stretchr/testify v1.7.0
	github.com/vmware-tanzu/carvel-kapp-controller v0.14.0
	go.uber.org/zap v1.16.0
	golang.org/x/oauth2 v0.0.0-20201109201403-9fd604954f58
	golang.org/x/text v0.3.5 // indirect
	google.golang.org/grpc v1.33.2
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b
	// If you update helm you need to update the kubernetes libs as well
	helm.sh/helm/v3 v3.5.2
	k8s.io/api v0.20.2
	k8s.io/apimachinery v0.20.2
	k8s.io/client-go v0.20.2
	sigs.k8s.io/controller-runtime v0.8.1
	sigs.k8s.io/yaml v1.2.0
)
