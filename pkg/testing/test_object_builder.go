package testing

import (
	"encoding/json"

	"github.com/gardener/landscaper/pkg/apis/core/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	hubv1 "github.com/gardener/potter-controller/api/v1"
	"github.com/gardener/potter-controller/pkg/util"
)

func CreateClusterBom(clusterBomName, overallState string) *hubv1.ClusterBom {
	testClusterBom := &hubv1.ClusterBom{
		ObjectMeta: v1.ObjectMeta{
			Name: clusterBomName,
		},

		Spec: hubv1.ClusterBomSpec{
			SecretRef: "testsecret01",
		},

		Status: hubv1.ClusterBomStatus{
			OverallState: overallState,
		},
	}
	return testClusterBom
}

func AddApplicationConfig(clusterBom *hubv1.ClusterBom, id string) {
	clusterBom.Spec.ApplicationConfigs = append(clusterBom.Spec.ApplicationConfigs, hubv1.ApplicationConfig{
		ID: id,
	})
}

func AddApplicationStatus(clusterBom *hubv1.ClusterBom, id, state string, lastOp *hubv1.LastOperation) {
	clusterBom.Status.ApplicationStates = append(clusterBom.Status.ApplicationStates, hubv1.ApplicationState{
		ID:    id,
		State: state,
		DetailedState: hubv1.DetailedState{
			LastOperation: *lastOp,
		},
	})
}

func CreateDeployItem(clusterBomName, id string, generation, observedGeneration int64, isInstallOperation bool, lastOp *hubv1.LastOperation, readiness *hubv1.Readiness) *v1alpha1.DeployItem {
	deployItemConfig := hubv1.HubDeployItemConfiguration{
		LocalSecretRef: "asdf",
		DeploymentConfig: hubv1.DeploymentConfig{
			ID: id,
		},
	}

	encodedConfig, _ := json.Marshal(deployItemConfig)

	deployItemProviderStatus := hubv1.HubDeployItemProviderStatus{
		LastOperation: *lastOp,
		Readiness:     readiness,
	}

	encodedStatus, _ := json.Marshal(deployItemProviderStatus)

	result := &v1alpha1.DeployItem{
		ObjectMeta: v1.ObjectMeta{
			Name: clusterBomName + util.Separator + id,
			Labels: map[string]string{
				hubv1.LabelClusterBomName:      clusterBomName,
				hubv1.LabelApplicationConfigID: id,
			},
			Generation: generation,
		},
		Spec: v1alpha1.DeployItemSpec{
			Type: util.ConfigTypeHelm,
			Configuration: &runtime.RawExtension{
				Raw: encodedConfig,
			},
		},

		Status: v1alpha1.DeployItemStatus{
			ObservedGeneration: observedGeneration,
			ProviderStatus: &runtime.RawExtension{
				Raw: encodedStatus,
			},
		},
	}

	if !isInstallOperation {
		now := v1.Now()
		result.SetDeletionTimestamp(&now)
	}

	return result
}

func LastOp(operation string, observedGeneration, successNumber int32, state string, numOfTries int32) hubv1.LastOperation {
	return hubv1.LastOperation{
		Operation:         operation,
		SuccessGeneration: int64(successNumber),
		State:             state,
		NumberOfTries:     numOfTries,
	}
}

func CreateSecret(name string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name: name,
		},
	}
}
