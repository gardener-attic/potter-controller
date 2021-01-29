package avcheck

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.wdf.sap.corp/kubernetes/hub-controller/pkg/util"

	"github.com/arschles/assert"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	hubv1 "github.wdf.sap.corp/kubernetes/hub-controller/api/v1"
)

func buildTestBom(namespace, name string) *hubv1.ClusterBom {
	typeSpecificData := map[string]interface{}{
		"installName": "test-app",
		"namespace":   "install-here",
		"catalogAccess": map[string]interface{}{
			"chartName":    "mongodb",
			"repo":         "stable",
			"chartVersion": "1.0.0",
		},
	}

	values := map[string]interface{}{
		switchKey: true,
	}

	bom := &hubv1.ClusterBom{
		ObjectMeta: v1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: hubv1.ClusterBomSpec{
			SecretRef: "my.kubeconfig",
			ApplicationConfigs: []hubv1.ApplicationConfig{
				{
					ID:               "helm-app",
					ConfigType:       util.ConfigTypeHelm,
					TypeSpecificData: *util.CreateRawExtensionOrPanic(typeSpecificData),
					Values:           util.CreateRawExtensionOrPanic(values),
				},
			},
		},
	}

	return bom
}

func TestInitializeBomWithNoPreexistingBom(t *testing.T) {
	const (
		bomNamespace = "test-namespace"
		bomName      = "test-bom"
	)

	scheme := runtime.NewScheme()
	_ = hubv1.AddToScheme(scheme)
	k8sClient := fake.NewFakeClientWithScheme(scheme)

	initialBom := buildTestBom(bomNamespace, bomName)

	actor := Actor{
		K8sClient:  k8sClient,
		Log:        zapr.NewLogger(zap.NewNop()),
		InitialBom: initialBom,
	}

	err := actor.initializeBom()
	assert.NoErr(t, err)

	bomKey := types.NamespacedName{
		Namespace: bomNamespace,
		Name:      bomName,
	}
	var actualBom hubv1.ClusterBom
	err = k8sClient.Get(context.Background(), bomKey, &actualBom)

	assert.NoErr(t, err)
	assert.Equal(t, actualBom.GetNamespace(), bomNamespace, "namespace")
	assert.Equal(t, actualBom.GetName(), bomName, "name")
	assert.True(t, reflect.DeepEqual(initialBom.Spec, actualBom.Spec), "spec")
}

func TestInitializeBomWithPreexistingBom(t *testing.T) {
	const (
		bomNamespace = "test-namespace"
		bomName      = "test-bom"
	)

	preexistingBom := hubv1.ClusterBom{
		ObjectMeta: v1.ObjectMeta{
			Namespace: bomNamespace,
			Name:      bomName,
		},
		Spec: hubv1.ClusterBomSpec{
			SecretRef:          "oldcluster.kubeconfig",
			ApplicationConfigs: []hubv1.ApplicationConfig{},
		},
	}

	scheme := runtime.NewScheme()
	_ = hubv1.AddToScheme(scheme)
	k8sClient := fake.NewFakeClientWithScheme(scheme, &preexistingBom)

	initialBom := buildTestBom(bomNamespace, bomName)

	actor := Actor{
		K8sClient:  k8sClient,
		Log:        zapr.NewLogger(zap.NewNop()),
		InitialBom: initialBom,
	}

	err := actor.initializeBom()
	assert.NoErr(t, err)

	bomKey := types.NamespacedName{
		Namespace: bomNamespace,
		Name:      bomName,
	}
	var actualBom hubv1.ClusterBom
	err = k8sClient.Get(context.Background(), bomKey, &actualBom)

	assert.NoErr(t, err)
	assert.Equal(t, actualBom.GetNamespace(), bomNamespace, "namespace")
	assert.Equal(t, actualBom.GetName(), bomName, "name")
	assert.True(t, reflect.DeepEqual(initialBom.Spec, actualBom.Spec), "spec")
}

func TestModifyBom(t *testing.T) {
	const (
		bomNamespace = "test-namespace"
		bomName      = "test-bom"
	)

	scheme := runtime.NewScheme()
	_ = hubv1.AddToScheme(scheme)
	k8sClient := fake.NewFakeClientWithScheme(scheme)

	initialBom := buildTestBom(bomNamespace, bomName)
	initialApplConfig := initialBom.Spec.ApplicationConfigs[0]
	var initialValues map[string]interface{}
	err := json.Unmarshal(initialApplConfig.Values.Raw, &initialValues)
	assert.NoErr(t, err)

	actor := Actor{
		K8sClient:  k8sClient,
		Log:        zapr.NewLogger(zap.NewNop()),
		InitialBom: initialBom,
	}

	err = actor.initializeBom()
	assert.NoErr(t, err)

	err = actor.modifyBom()
	assert.NoErr(t, err)

	bomKey := types.NamespacedName{
		Namespace: bomNamespace,
		Name:      bomName,
	}
	var actualBom hubv1.ClusterBom
	err = k8sClient.Get(context.Background(), bomKey, &actualBom)

	assert.NoErr(t, err)
	assert.Equal(t, actualBom.GetNamespace(), bomNamespace, "namespace")
	assert.Equal(t, actualBom.GetName(), bomName, "name")
	assert.Equal(t, actualBom.Spec.SecretRef, initialBom.Spec.SecretRef, "secretRef")

	actualApplConfig := actualBom.Spec.ApplicationConfigs[0]
	assert.Equal(t, actualApplConfig.ID, initialApplConfig.ID, "id")
	assert.Equal(t, actualApplConfig.ConfigType, initialApplConfig.ConfigType, "configType")
	assert.Equal(t, actualApplConfig.NoReconcile, initialApplConfig.NoReconcile, "noReconcile")
	assert.True(t, reflect.DeepEqual(initialApplConfig.TypeSpecificData, actualApplConfig.TypeSpecificData), "typeSpecificData")

	var actualValues map[string]interface{}
	err = json.Unmarshal(actualApplConfig.Values.Raw, &actualValues)
	assert.NoErr(t, err)

	actualSwitchStr := actualValues[switchKey]
	actualSwitch := actualSwitchStr.(bool)

	initialSwitchStr := initialValues[switchKey]
	initialSwitch := initialSwitchStr.(bool)

	assert.True(t, actualSwitch == !initialSwitch, "values[%s]", switchKey)
}
