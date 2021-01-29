package deployutil

import (
	"testing"

	"github.com/go-logr/zapr"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	hubv1 "github.wdf.sap.corp/kubernetes/hub-controller/api/v1"
	"github.wdf.sap.corp/kubernetes/hub-controller/pkg/util"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
)

func TestComputeReadinessForResourceReadyRequirements(t *testing.T) {
	tests := []struct {
		name              string
		resourceReadyReqs []hubv1.Resource
		expectedReadiness string
	}{
		{
			name: "everything valid",
			resourceReadyReqs: []hubv1.Resource{
				{
					Name:       "my-cm",
					Namespace:  "ns-1",
					APIVersion: "v1",
					Resource:   "configmaps",
					FieldPath:  "{ .data.my-key }",
					SuccessValues: []runtime.RawExtension{
						*util.CreateRawExtensionOrPanic(map[string]interface{}{
							"value": "my-val",
						}),
					},
				},
				{
					Name:       "my-job",
					Namespace:  "ns-2",
					APIVersion: "batch/v1",
					Resource:   "jobs",
					FieldPath:  "{ .status.conditions[?(@.type == 'Complete')].status }",
					SuccessValues: []runtime.RawExtension{
						*util.CreateRawExtensionOrPanic(map[string]interface{}{
							"value": "True",
						}),
					},
				},
			},
			expectedReadiness: util.StateOk,
		},
		{
			name: "value from fieldPath doesn't match with successValues",
			resourceReadyReqs: []hubv1.Resource{
				{
					Name:       "my-cm",
					Namespace:  "ns-1",
					APIVersion: "v1",
					Resource:   "configmaps",
					FieldPath:  "{ .data.my-key }",
					SuccessValues: []runtime.RawExtension{
						*util.CreateRawExtensionOrPanic(map[string]interface{}{
							"value": "different-val",
						}),
					},
				},
			},
			expectedReadiness: util.StateUnknown,
		},
		{
			name: "resource doesn't exist",
			resourceReadyReqs: []hubv1.Resource{
				{
					Name:       "non-existent-configmap",
					Namespace:  "ns-1",
					APIVersion: "v1",
					Resource:   "configmaps",
					FieldPath:  "{ .data.my-key }",
					SuccessValues: []runtime.RawExtension{
						*util.CreateRawExtensionOrPanic(map[string]interface{}{
							"value": "my-val",
						}),
					},
				},
			},
			expectedReadiness: util.StateUnknown,
		},
		{
			name: "fieldPath doesn't exist",
			resourceReadyReqs: []hubv1.Resource{
				{
					Name:       "my-cm",
					Namespace:  "ns-1",
					APIVersion: "v1",
					Resource:   "configmaps",
					FieldPath:  "{ .data.invalid-key }",
					SuccessValues: []runtime.RawExtension{
						*util.CreateRawExtensionOrPanic(map[string]interface{}{
							"value": "my-val",
						}),
					},
				},
			},
			expectedReadiness: util.StateUnknown,
		},
		{
			name: "invalid key in successValues",
			resourceReadyReqs: []hubv1.Resource{
				{
					Name:       "my-cm",
					Namespace:  "ns-1",
					APIVersion: "v1",
					Resource:   "configmaps",
					FieldPath:  "{ .data.my-key }",
					SuccessValues: []runtime.RawExtension{
						*util.CreateRawExtensionOrPanic(map[string]interface{}{
							"invalid-success-value-key": "my-val",
						}),
					},
				},
			},
			expectedReadiness: util.StateUnknown,
		},
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Namespace: "ns-1",
			Name:      "my-cm",
		},
		Data: map[string]string{
			"my-key": "my-val",
		},
	}
	job := &batchv1.Job{
		ObjectMeta: v1.ObjectMeta{
			Namespace: "ns-2",
			Name:      "my-job",
		},
		Status: batchv1.JobStatus{
			Conditions: []batchv1.JobCondition{
				{
					Type:   "Complete",
					Status: "True",
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)

	fakeClient := fake.NewSimpleDynamicClient(scheme, cm, job)
	dynamicTargetClient := DynamicTargetClient{fakeClient}
	nopLogger := zapr.NewLogger(zap.NewNop())

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			readiness := ComputeReadinessForResourceReadyRequirements(tt.resourceReadyReqs, util.StateOk, &dynamicTargetClient, nopLogger)
			assert.Equal(t, tt.expectedReadiness, readiness, "readiness")
		})
	}
}
