module github.com/gardener/potter-controller

go 1.15

replace (
	github.com/docker/docker => github.com/moby/moby v17.12.1-ce+incompatible
	github.com/docker/docker@v0.7.3-0.20190327010347-be7ac8be2ae0 => github.com/moby/moby v17.12.1-ce+incompatible
	github.com/docker/docker@v1.4.2-0.20200203170920-46ec8731fbce => github.com/moby/moby v17.12.1-ce+incompatible
	// replace needed for hub-controller dependency
	github.com/moby/moby@v0.7.3-0.20190826074503-38ab9da00309 => github.com/moby/moby v17.12.1-ce+incompatible
	golang.org/x/text@v0.3.0 => golang.org/x/text v0.3.5
	golang.org/x/text@v0.3.1 => golang.org/x/text v0.3.5
	golang.org/x/text@v0.3.2 => golang.org/x/text v0.3.5
	golang.org/x/text@v0.3.3 => golang.org/x/text v0.3.5
	golang.org/x/text@v0.3.4 => golang.org/x/text v0.3.5
)

require (
	github.com/Nvveen/Gotty v0.0.0-20170406111628-a8b993ba6abd // indirect
	github.com/arschles/assert v1.0.0
	github.com/coreos/go-oidc v2.2.1+incompatible
	github.com/gardener/landscaper/apis v0.6.1-0.20210225105446-cc69d9c9425f
	github.com/ghodss/yaml v1.0.0
	github.com/go-logr/logr v0.3.0
	github.com/go-logr/zapr v0.3.0
	github.com/google/uuid v1.1.2
	github.com/gorilla/mux v1.8.0
	github.com/gotestyourself/gotestyourself v2.2.0+incompatible // indirect
	github.com/onsi/ginkgo v1.14.2
	github.com/onsi/gomega v1.10.4
	github.com/opencontainers/selinux v1.8.0 // indirect
	github.com/pkg/errors v0.9.1
	github.com/prometheus/common v0.10.0
	github.com/sirupsen/logrus v1.7.0
	github.com/stretchr/testify v1.6.1
	github.com/vmware-tanzu/carvel-kapp-controller v0.14.0
	go.uber.org/zap v1.16.0
	golang.org/x/oauth2 v0.0.0-20201109201403-9fd604954f58
	golang.org/x/text v0.3.5 // indirect
	google.golang.org/grpc v1.33.2
	gopkg.in/yaml.v3 v3.0.0-20200615113413-eeeca48fe776
	// If you update helm you need to update the kubernetes libs as well
	helm.sh/helm/v3 v3.4.2
	k8s.io/api v0.19.7
	k8s.io/apimachinery v0.19.7
	k8s.io/client-go v0.19.6
	rsc.io/letsencrypt v0.0.3 // indirect
	sigs.k8s.io/controller-runtime v0.6.4
	sigs.k8s.io/yaml v1.2.0
)
