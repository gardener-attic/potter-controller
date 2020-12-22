package admission

import (
	"encoding/json"
	"testing"

	"github.com/arschles/assert"
	"github.com/vmware-tanzu/carvel-kapp-controller/pkg/apis/kappctrl/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
)

func TestInvalidKappSpecificData(t *testing.T) {
	var log = ctrl.Log.WithName("ClusterBom Admission Hook Unit Tests")
	report := newReport(nil)

	typeSpecificData := &runtime.RawExtension{
		Raw: []byte{4},
	}

	newKappReviewer().reviewTypeSpecificData(log, report, typeSpecificData, "a.kubeconfig")
	assert.Equal(t, report.denied(), true, "denied")
}

func TestReviewKappSpecificData(t *testing.T) {
	var log = ctrl.Log.WithName("ClusterBom Admission Hook Unit Tests")

	tests := []struct {
		name           string
		cluster        *v1alpha1.AppCluster
		secretRef      string
		expectedDenied bool
	}{
		{
			name: "allow equal secrets",
			cluster: &v1alpha1.AppCluster{
				KubeconfigSecretRef: &v1alpha1.AppClusterKubeconfigSecretRef{
					Name: "a.kubeconfig",
				},
			},
			secretRef:      "a.kubeconfig",
			expectedDenied: false,
		},
		{
			name: "allow initial secret name",
			cluster: &v1alpha1.AppCluster{
				KubeconfigSecretRef: &v1alpha1.AppClusterKubeconfigSecretRef{},
			},
			secretRef:      "a.kubeconfig",
			expectedDenied: false,
		},
		{
			name:           "allow initial secret ref",
			cluster:        &v1alpha1.AppCluster{},
			secretRef:      "a.kubeconfig",
			expectedDenied: false,
		},
		{
			name:           "allow initial cluster",
			cluster:        nil,
			secretRef:      "a.kubeconfig",
			expectedDenied: false,
		},
		{
			name: "reject different secrets",
			cluster: &v1alpha1.AppCluster{
				KubeconfigSecretRef: &v1alpha1.AppClusterKubeconfigSecretRef{
					Name: "a.kubeconfig",
				},
			},
			secretRef:      "b.kubeconfig",
			expectedDenied: true,
		},
	}

	for i := range tests {
		test := &tests[i]
		t.Run(test.name, func(t *testing.T) {
			report := newReport(nil)

			typeSpecificData, err := raw(&v1alpha1.AppSpec{
				Cluster: test.cluster,
			})
			assert.Nil(t, err, "error building type specific data")

			newKappReviewer().reviewTypeSpecificData(log, report, typeSpecificData, test.secretRef)

			assert.Equal(t, report.denied(), test.expectedDenied, "denied")
		})
	}
}

func raw(structuredData interface{}) (*runtime.RawExtension, error) {
	rawData, err := json.Marshal(structuredData)
	if err != nil {
		return nil, err
	}
	return &runtime.RawExtension{Raw: rawData}, nil
}
