package helm

import (
	"github.wdf.sap.corp/kubernetes/hub-controller/pkg/deployutil"

	"testing"

	"github.com/arschles/assert"
)

const testYamlServiceAccount = `---
# Source: test.yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  labels:
    app: test-app
    chart: test-chart
    release: test-release
  name: test-account-1
  namespace: test
`

const testYamlConfigMap = `---
# Source: test.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-configmap
  namespace: test
`

const testYamlDeployment = `---
# Source: test.yaml
apiVersion: v1
kind: Deployment
metadata:
  name: test-deployment
  namespace: test
`

const testYamlDaemonSet = `---
# Source: test.yaml
apiVersion: v1
kind: DaemonSet
metadata:
  name: test-daemonset
  namespace: test
`

const testYamlStatefulSet = `---
# Source: test.yaml
apiVersion: v1
kind: StatefulSet
metadata:
  name: test-statefulset
  namespace: test
`

func TestUnmarshalManifest(t *testing.T) {
	manifest := testYamlConfigMap + testYamlServiceAccount
	basicKubernetesObjects, err := unmarshalManifest(&manifest, deployutil.AcceptAllFilter)
	assert.Nil(t, err, "unmarshaling error")
	assert.NotNil(t, basicKubernetesObjects, "basicKubernetesObjects")
	assert.Equal(t, len(basicKubernetesObjects), 2, "number of basicKubernetesObjects")
}

func TestUnmarshalManifest_Empty(t *testing.T) {
	manifest := testYamlConfigMap + testYamlServiceAccount
	basicKubernetesObjects, err := unmarshalManifest(&manifest, deployutil.ReadinessFilter)
	assert.Nil(t, err, "unmarshaling error")
	assert.Nil(t, basicKubernetesObjects, "basicKubernetesObjects")
}

func TestUnmarshalManifest_Deployment(t *testing.T) {
	manifest := testYamlConfigMap + testYamlDeployment + testYamlServiceAccount
	basicKubernetesObjects, err := unmarshalManifest(&manifest, deployutil.ReadinessFilter)
	assert.Nil(t, err, "unmarshaling error")
	assert.Equal(t, len(basicKubernetesObjects), 1, "number of basicKubernetesObjects")
}

func TestUnmarshalManifest_DaemonSetAndStatefulSet(t *testing.T) {
	manifest := testYamlDaemonSet + testYamlStatefulSet + testYamlServiceAccount
	basicKubernetesObjects, err := unmarshalManifest(&manifest, deployutil.ReadinessFilter)
	assert.Nil(t, err, "unmarshaling error")
	assert.Equal(t, len(basicKubernetesObjects), 2, "number of basicKubernetesObjects")
}
