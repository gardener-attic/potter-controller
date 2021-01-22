package util

import (
	"testing"

	hubv1 "github.com/gardener/potter-controller/api/v1"

	"github.com/arschles/assert"
	"github.com/gardener/landscaper/pkg/apis/core/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestGetClusterBomKey(t *testing.T) {
	clusterBom, _, _ := newTestObjects()
	key := GetKey(clusterBom)
	assert.Equal(t, key.Name, clusterBom.Name, "clusterbom name")
	assert.Equal(t, key.Namespace, clusterBom.Namespace, "clusterbom namespace")
}

func TestGetClusterBomKey_FromBC(t *testing.T) {
	clusterBom, _, _ := newTestObjects()
	key := GetClusterBomKeyFromClusterBomOrDeployItem(clusterBom, nil)
	assert.Equal(t, key.Name, clusterBom.Name, "clusterbom name")
	assert.Equal(t, key.Namespace, clusterBom.Namespace, "clusterbom namespace")
}

func TestGetClusterBomKey_FromHDC(t *testing.T) {
	clusterBom, deployItem, _ := newTestObjects()
	key := GetClusterBomKeyFromClusterBomOrDeployItem(clusterBom, deployItem)
	assert.Equal(t, key.Name, clusterBom.Name, "clusterbom name")
	assert.Equal(t, key.Namespace, clusterBom.Namespace, "clusterbom namespace")
}

func TestGetHDCKey(t *testing.T) {
	_, hdc, _ := newTestObjects()
	key := GetKey(hdc)
	assert.Equal(t, key.Name, hdc.Name, "hdc name")
	assert.Equal(t, key.Namespace, hdc.Namespace, "hdc namespace")
}

func TestGetSecretKeyFromClusterBom(t *testing.T) {
	clusterBom, _, _ := newTestObjects()
	secretKey := GetSecretKeyFromClusterBom(clusterBom)
	assert.Equal(t, secretKey.Name, clusterBom.Spec.SecretRef, "secret name")
	assert.Equal(t, secretKey.Namespace, clusterBom.Namespace, "secret namespace")
}

func newTestObjects() (*hubv1.ClusterBom, *v1alpha1.DeployItem, *types.NamespacedName) { // nolint
	clusterBom := &hubv1.ClusterBom{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bom",
			Namespace: "ns",
		},
		Spec: hubv1.ClusterBomSpec{
			ApplicationConfigs: []hubv1.ApplicationConfig{
				{
					ID: "app",
				},
			},
			SecretRef: "sec",
		},
	}

	deployItem := &v1alpha1.DeployItem{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterBom.Name + Separator + clusterBom.Spec.ApplicationConfigs[0].ID,
			Namespace: clusterBom.Namespace,
		},
	}

	deployItemKey := &types.NamespacedName{
		Name:      deployItem.Name,
		Namespace: deployItem.Namespace,
	}

	return clusterBom, deployItem, deployItemKey
}
