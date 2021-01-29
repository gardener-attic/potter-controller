package controllersdi

import (
	"context"
	"encoding/json"
	"testing"

	hubv1 "github.wdf.sap.corp/kubernetes/hub-controller/api/v1"
	"github.wdf.sap.corp/kubernetes/hub-controller/pkg/synchronize"
	hubtesting "github.wdf.sap.corp/kubernetes/hub-controller/pkg/testing"
	"github.wdf.sap.corp/kubernetes/hub-controller/pkg/util"

	"github.com/arschles/assert"
	"github.com/gardener/landscaper/pkg/apis/core/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	testClusterBomName       = "testclusterbom01"
	testAppID                = "testapp01"                  // nolint
	testAppID2               = "testapp02"                  // nolint
	testDeploymentConfigName = "testclusterbom01-testapp01" // nolint
)

// TestStatusController_NoHDC_NoBOM checks the state reconciler for the case that neither the HDC, nor the clusterbom exists.
func TestStatusController_NoHDC_NoBOM(t *testing.T) {
	unitTestClient := hubtesting.NewUnitTestClientDi()

	clusterBomStateController := ClusterBomStateReconciler{
		Client:         unitTestClient,
		Log:            ctrl.Log.WithName("controllers").WithName("ClusterBomState"),
		Scheme:         runtime.NewScheme(),
		BlockObject:    synchronize.NewBlockObject(nil, false),
		UncachedClient: unitTestClient,
	}

	// Neither the HDC "clusterbom1-id1", nor the clusterbom "clusterbom1" exists; this means the unitTestClient was
	// not prepared with such objects.
	testRequest := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "someNamespace",
			Name:      "clusterbom1-id1",
		},
	}

	result, err := clusterBomStateController.Reconcile(testRequest)

	assert.Nil(t, err, "reconcile error")
	assert.False(t, result.Requeue, "requeue")
}

// TestEmptyClusterBom checks the state reconciler for the case that neither the HDC, nor the clusterbom exists.
func TestEmptyClusterBom(t *testing.T) {
	clusterBom := hubtesting.CreateClusterBom(testClusterBomName, util.StateFailed)

	hubtesting.AddApplicationStatus(clusterBom, "testid1", util.StateFailed, &hubv1.LastOperation{})

	hubtesting.AddApplicationStatus(clusterBom, "testid2", util.StateFailed, &hubv1.LastOperation{})

	unitTestClient := hubtesting.NewUnitTestClientWithCBandDI(clusterBom, nil)
	unitTestClient.AddSecret(hubtesting.CreateSecret(clusterBom.Spec.SecretRef))

	clusterBomStateController := ClusterBomStateReconciler{
		Client:         unitTestClient,
		Log:            ctrl.Log.WithName("controllers").WithName("ClusterBomState"),
		Scheme:         runtime.NewScheme(),
		BlockObject:    synchronize.NewBlockObject(nil, false),
		UncachedClient: unitTestClient,
	}

	// Neither the HDC "clusterbom1-id1", nor the clusterbom "clusterbom1" exists
	testRequest := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "someNamespace",
			Name:      "clusterbom1-id1",
		},
	}

	result, err := clusterBomStateController.Reconcile(testRequest)

	assert.Nil(t, err, "reconcile error")
	assert.False(t, result.Requeue, "requeue")

	resultClusterBom := unitTestClient.ClusterBoms[testClusterBomName]

	assert.Equal(t, resultClusterBom.Status.OverallState, util.StateOk, "overall state")
	assert.Equal(t, len(resultClusterBom.Status.ApplicationStates), 0, "length of applicationStates")
}

func TestStatusController_Install_ReadinessPending(t *testing.T) {
	testClusterBom := hubtesting.CreateClusterBom(testClusterBomName, util.StateFailed)
	hubtesting.AddApplicationConfig(testClusterBom, testAppID)

	lastOp := hubtesting.LastOp(util.OperationInstall, 3, 3, util.StateFailed, 1)
	readiness := hubv1.Readiness{State: util.StatePending}
	testHubDeployItem := hubtesting.CreateDeployItem(testClusterBomName, testAppID, 3, 0, true, &lastOp, &readiness)

	unitTestClient := hubtesting.NewUnitTestClientWithCBandDI(testClusterBom, testHubDeployItem)
	unitTestClient.AddSecret(hubtesting.CreateSecret(testClusterBom.Spec.SecretRef))

	clusterBomStateController := ClusterBomStateReconciler{
		Client:         unitTestClient,
		Log:            ctrl.Log.WithName("controllers").WithName("ClusterBomState"),
		Scheme:         runtime.NewScheme(),
		BlockObject:    synchronize.NewBlockObject(nil, false),
		UncachedClient: unitTestClient,
	}

	testRequest := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: testDeploymentConfigName,
		},
	}

	result, err := clusterBomStateController.Reconcile(testRequest)
	assert.Nil(t, err, "reconcile error")
	assert.False(t, result.Requeue, "requeue")

	actualDeployItemStatus := &hubv1.HubDeployItemProviderStatus{}
	err = json.Unmarshal(testHubDeployItem.Status.ProviderStatus.Raw, actualDeployItemStatus)
	assert.Nil(t, err, "unmarshal error")

	resultClusterBom := unitTestClient.ClusterBoms[testClusterBomName]
	assert.NotNil(t, resultClusterBom, "resultClusterBom")
	assert.Equal(t, resultClusterBom.Status.OverallState, util.StatePending, "overallState")
	assert.Equal(t, len(resultClusterBom.Status.ApplicationStates), 1, "length of applicationStates")
	assert.Equal(t, resultClusterBom.Status.ApplicationStates[0].ID, testAppID, "id")
	assert.Equal(t, resultClusterBom.Status.ApplicationStates[0].State, util.StatePending, "state")
	assert.Equal(t, resultClusterBom.Status.ApplicationStates[0].DetailedState.LastOperation, actualDeployItemStatus.LastOperation, "lastOperation")
}

// TestStatusController_Install_ReadinessOk tests how the clusterbomStateController
// updates the clusterbom status from the HDC status. In this case, the last operation number is smaller than the
// current operation number, so that the status and overall status should be "pending".
func TestStatusController_Install_ReadinessOk(t *testing.T) {
	testClusterBom := hubtesting.CreateClusterBom(testClusterBomName, util.StateFailed)
	hubtesting.AddApplicationConfig(testClusterBom, testAppID)

	lastOp := hubtesting.LastOp(util.OperationInstall, 3, 3, util.StateFailed, 1)
	readiness := hubv1.Readiness{State: util.StateOk}
	testHubDeployItem := hubtesting.CreateDeployItem(testClusterBomName, testAppID, 3, 0, true, &lastOp, &readiness)

	unitTestClient := hubtesting.NewUnitTestClientWithCBandDI(testClusterBom, testHubDeployItem)
	unitTestClient.AddSecret(hubtesting.CreateSecret(testClusterBom.Spec.SecretRef))

	clusterBomStateController := ClusterBomStateReconciler{
		Client:         unitTestClient,
		Log:            ctrl.Log.WithName("controllers").WithName("ClusterBomState"),
		Scheme:         runtime.NewScheme(),
		BlockObject:    synchronize.NewBlockObject(nil, false),
		UncachedClient: unitTestClient,
	}

	testRequest := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: testDeploymentConfigName,
		},
	}

	result, err := clusterBomStateController.Reconcile(testRequest)

	assert.Nil(t, err, "reconcile error")
	assert.False(t, result.Requeue, "requeue")

	actualDeployItemStatus := &hubv1.HubDeployItemProviderStatus{}
	err = json.Unmarshal(testHubDeployItem.Status.ProviderStatus.Raw, actualDeployItemStatus)
	assert.Nil(t, err, "unmarshal error")

	resultClusterBom := unitTestClient.ClusterBoms[testClusterBomName]
	assert.NotNil(t, resultClusterBom, "resultClusterBom")
	assert.Equal(t, resultClusterBom.Status.OverallState, util.StatePending, "overallState")
	assert.Equal(t, len(resultClusterBom.Status.ApplicationStates), 1, "length of applicationStates")
	assert.Equal(t, resultClusterBom.Status.ApplicationStates[0].ID, testAppID, "id")
	assert.Equal(t, resultClusterBom.Status.ApplicationStates[0].State, util.StatePending, "state")
	assert.Equal(t, resultClusterBom.Status.ApplicationStates[0].DetailedState.LastOperation, actualDeployItemStatus.LastOperation, "lastOperation")
}

// TestStatusController_Install_LastNumberSmallerThanCurrentNumber tests how the clusterbomStateController
// updates the clusterbom status from the HDC status. In this case, the last operation number is smaller than the
// current operation number, so that the status and overall status should be "pending".
func TestStatusController_Install_LastNumberSmallerThanCurrentNumber(t *testing.T) {
	testClusterBom := hubtesting.CreateClusterBom(testClusterBomName, util.StateFailed)
	hubtesting.AddApplicationConfig(testClusterBom, testAppID)

	lastOp := hubtesting.LastOp(util.OperationInstall, 2, 2, util.StateOk, 1)
	readiness := hubv1.Readiness{State: util.StateOk}
	testHubDeployItem := hubtesting.CreateDeployItem(testClusterBomName, testAppID, 3, 0, true, &lastOp, &readiness)

	unitTestClient := hubtesting.NewUnitTestClientWithCBandDI(testClusterBom, testHubDeployItem)
	unitTestClient.AddSecret(hubtesting.CreateSecret(testClusterBom.Spec.SecretRef))

	clusterBomStateController := ClusterBomStateReconciler{
		Client:         unitTestClient,
		Log:            ctrl.Log.WithName("controllers").WithName("ClusterBomState"),
		Scheme:         runtime.NewScheme(),
		BlockObject:    synchronize.NewBlockObject(nil, false),
		UncachedClient: unitTestClient,
	}

	testRequest := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: testDeploymentConfigName,
		},
	}

	result, err := clusterBomStateController.Reconcile(testRequest)

	assert.Nil(t, err, "reconcile error")
	assert.False(t, result.Requeue, "requeue")

	actualDeployItemStatus := &hubv1.HubDeployItemProviderStatus{}
	err = json.Unmarshal(testHubDeployItem.Status.ProviderStatus.Raw, actualDeployItemStatus)
	assert.Nil(t, err, "unmarshal error")

	resultClusterBom := unitTestClient.ClusterBoms[testClusterBomName]
	assert.NotNil(t, resultClusterBom, "resultClusterBom")
	assert.Equal(t, resultClusterBom.Status.OverallState, util.StatePending, "overallState")
	assert.Equal(t, len(resultClusterBom.Status.ApplicationStates), 1, "length of applicationStates")
	assert.Equal(t, resultClusterBom.Status.ApplicationStates[0].ID, testAppID, "id")
	assert.Equal(t, resultClusterBom.Status.ApplicationStates[0].State, util.StatePending, "state")
	assert.Equal(t, resultClusterBom.Status.ApplicationStates[0].DetailedState.LastOperation, actualDeployItemStatus.LastOperation, "lastOperation")
}

// TestStatusController_Install_SuccessNumberSmallerThanLastNumber tests how the clusterbomStateController
// updates the clusterbom status from the HDC status. In this case, the last success number is smaller than the
// last operation number. So the current version of the app was never successfully installed. The readiness of the HDC
// is ok; but this refers to the previously installed version. Therefore, the clusterbom app state and overall state
// should be "failed".
func TestStatusController_Install_SuccessNumberSmallerThanLastNumber(t *testing.T) {
	testClusterBom := hubtesting.CreateClusterBom(testClusterBomName, util.StateFailed)
	hubtesting.AddApplicationConfig(testClusterBom, testAppID)

	lastOp := hubtesting.LastOp(util.OperationInstall, 3, 2, util.StateOk, 1)
	readiness := hubv1.Readiness{State: util.StateOk}
	testHubDeployItem := hubtesting.CreateDeployItem(testClusterBomName, testAppID, 3, 0, true, &lastOp, &readiness)

	unitTestClient := hubtesting.NewUnitTestClientWithCBandDI(testClusterBom, testHubDeployItem)
	unitTestClient.AddSecret(hubtesting.CreateSecret(testClusterBom.Spec.SecretRef))

	clusterBomStateController := ClusterBomStateReconciler{
		Client:         unitTestClient,
		Log:            ctrl.Log.WithName("controllers").WithName("ClusterBomState"),
		Scheme:         runtime.NewScheme(),
		BlockObject:    synchronize.NewBlockObject(nil, false),
		UncachedClient: unitTestClient,
	}

	testRequest := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: testDeploymentConfigName,
		},
	}

	result, err := clusterBomStateController.Reconcile(testRequest)
	assert.Nil(t, err, "reconcile error")
	assert.False(t, result.Requeue, "requeue")

	actualDeployItemStatus := &hubv1.HubDeployItemProviderStatus{}
	err = json.Unmarshal(testHubDeployItem.Status.ProviderStatus.Raw, actualDeployItemStatus)
	assert.Nil(t, err, "unmarshal error")

	resultClusterBom := unitTestClient.ClusterBoms[testClusterBomName]
	assert.NotNil(t, resultClusterBom, "resultClusterBom")
	assert.Equal(t, resultClusterBom.Status.OverallState, util.StatePending, "overallState")
	assert.Equal(t, len(resultClusterBom.Status.ApplicationStates), 1, "length of applicationStates")
	assert.Equal(t, resultClusterBom.Status.ApplicationStates[0].ID, testAppID, "id")
	assert.Equal(t, resultClusterBom.Status.ApplicationStates[0].State, util.StatePending, "state")
	assert.Equal(t, resultClusterBom.Status.ApplicationStates[0].DetailedState.LastOperation, actualDeployItemStatus.LastOperation, "lastOperation")
}

// TestStatusController_Remove_LastNumberSmallerThanCurrentNumber tests how the clusterbomStateController
// updates the clusterbom status from the HDC status. In this case, the last operation number is smaller than the
// current operation number, so that the status and overall status should be "pending".
func TestStatusController_Remove_LastNumberSmallerThanCurrentNumber(t *testing.T) {
	testClusterBom := hubtesting.CreateClusterBom(testClusterBomName, util.StateFailed)
	hubtesting.AddApplicationConfig(testClusterBom, testAppID)

	lastOp := hubtesting.LastOp(util.OperationInstall, 2, 2, util.StateOk, 1)
	readiness := hubv1.Readiness{State: util.StateOk}
	testHubDeployItem := hubtesting.CreateDeployItem(testClusterBomName, testAppID, 3, 0, false, &lastOp, &readiness)

	unitTestClient := hubtesting.NewUnitTestClientWithCBandDI(testClusterBom, testHubDeployItem)
	unitTestClient.AddSecret(hubtesting.CreateSecret(testClusterBom.Spec.SecretRef))

	clusterBomStateController := ClusterBomStateReconciler{
		Client:         unitTestClient,
		Log:            ctrl.Log.WithName("controllers").WithName("ClusterBomState"),
		Scheme:         runtime.NewScheme(),
		BlockObject:    synchronize.NewBlockObject(nil, false),
		UncachedClient: unitTestClient,
	}

	testRequest := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: testDeploymentConfigName,
		},
	}

	result, err := clusterBomStateController.Reconcile(testRequest)

	assert.Nil(t, err, "reconcile error")
	assert.False(t, result.Requeue, "requeue")

	actualDeployItemStatus := &hubv1.HubDeployItemProviderStatus{}
	err = json.Unmarshal(testHubDeployItem.Status.ProviderStatus.Raw, actualDeployItemStatus)
	assert.Nil(t, err, "unmarshal error")

	resultClusterBom := unitTestClient.ClusterBoms[testClusterBomName]
	assert.NotNil(t, resultClusterBom, "resultClusterBom")
	assert.Equal(t, resultClusterBom.Status.OverallState, util.StatePending, "overallState")
	assert.Equal(t, len(resultClusterBom.Status.ApplicationStates), 1, "length of applicationStates")
	assert.Equal(t, resultClusterBom.Status.ApplicationStates[0].ID, testAppID, "id")
	assert.Equal(t, resultClusterBom.Status.ApplicationStates[0].State, util.StatePending, "state")
	assert.Equal(t, resultClusterBom.Status.ApplicationStates[0].DetailedState.LastOperation, actualDeployItemStatus.LastOperation, "lastOperation")
}

// TestStatusController_Remove_Failed tests how the clusterbomStateController updates the clusterbom status from the
// HDC status. In this case, the last remove operation number has failed, so that the status and overall status should
// be "failed". The readiness is not relevant for a remove operation.
func TestStatusController_Remove_Failed(t *testing.T) {
	testClusterBom := hubtesting.CreateClusterBom(testClusterBomName, util.StateOk)
	hubtesting.AddApplicationConfig(testClusterBom, testAppID)

	lastOp := hubtesting.LastOp(util.OperationRemove, 3, 3, util.StateFailed, 8)
	readiness := hubv1.Readiness{State: util.StateNotRelevant}
	testHubDeployItem := hubtesting.CreateDeployItem(testClusterBomName, testAppID, 3, 0, false, &lastOp, &readiness)

	unitTestClient := hubtesting.NewUnitTestClientWithCBandDI(testClusterBom, testHubDeployItem)
	unitTestClient.AddSecret(hubtesting.CreateSecret(testClusterBom.Spec.SecretRef))

	clusterBomStateController := ClusterBomStateReconciler{
		Client:         unitTestClient,
		Log:            ctrl.Log.WithName("controllers").WithName("ClusterBomState"),
		Scheme:         runtime.NewScheme(),
		BlockObject:    synchronize.NewBlockObject(nil, false),
		UncachedClient: unitTestClient,
	}

	testRequest := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: testDeploymentConfigName,
		},
	}

	result, err := clusterBomStateController.Reconcile(testRequest)

	assert.Nil(t, err, "reconcile error")
	assert.False(t, result.Requeue, "requeue")

	actualDeployItemStatus := &hubv1.HubDeployItemProviderStatus{}
	err = json.Unmarshal(testHubDeployItem.Status.ProviderStatus.Raw, actualDeployItemStatus)
	assert.Nil(t, err, "unmarshal error")

	resultClusterBom := unitTestClient.ClusterBoms[testClusterBomName]
	assert.NotNil(t, resultClusterBom, "resultClusterBom")
	assert.Equal(t, resultClusterBom.Status.OverallState, util.StateFailed, "overallState")
	assert.Equal(t, len(resultClusterBom.Status.ApplicationStates), 1, "length of applicationStates")
	assert.Equal(t, resultClusterBom.Status.ApplicationStates[0].ID, testAppID, "id")
	assert.Equal(t, resultClusterBom.Status.ApplicationStates[0].State, util.StateFailed, "state")
	assert.Equal(t, resultClusterBom.Status.ApplicationStates[0].DetailedState.LastOperation, actualDeployItemStatus.LastOperation, "lastOperation")
}

// TestStatusController_Remove_Ok tests how the clusterbomStateController updates the clusterbom status from the
// HDC status. In the present test case, the last remove operation has succeeded, so that the HDC and the corresponding
// status section of the clusterbom should be removed. The overall status should be "ok". The readiness is not
// relevant for a remove operation.
func TestStatusController_Remove_Ok(t *testing.T) {
	testClusterBom := hubtesting.CreateClusterBom(testClusterBomName, util.StateOk)

	lastOp := hubtesting.LastOp(util.OperationRemove, 3, 3, util.StateOk, 1)
	readiness := hubv1.Readiness{State: util.StateNotRelevant}
	testHubDeployItem := hubtesting.CreateDeployItem(testClusterBomName, testAppID, 3, 0, false, &lastOp, &readiness)

	unitTestClient := hubtesting.NewUnitTestClientWithCBandDI(testClusterBom, testHubDeployItem)
	unitTestClient.AddSecret(hubtesting.CreateSecret(testClusterBom.Spec.SecretRef))

	clusterBomStateController := ClusterBomStateReconciler{
		Client:         unitTestClient,
		Log:            ctrl.Log.WithName("controllers").WithName("ClusterBomState"),
		Scheme:         runtime.NewScheme(),
		BlockObject:    synchronize.NewBlockObject(nil, false),
		UncachedClient: unitTestClient,
	}

	testRequest := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: testDeploymentConfigName,
		},
	}

	result, err := clusterBomStateController.Reconcile(testRequest)

	assert.Nil(t, err, "reconcile error")
	assert.False(t, result.Requeue, "requeue")

	resultClusterBom := unitTestClient.ClusterBoms[testClusterBomName]
	assert.NotNil(t, resultClusterBom, "resultClusterBom")
	assert.Equal(t, resultClusterBom.Status.OverallState, util.StateOk, "overallState")
	assert.Equal(t, len(resultClusterBom.Status.ApplicationStates), 1, "length of applicationStates")
}

func TestStatusController_StatusChange(t *testing.T) {
	testClusterBom := hubtesting.CreateClusterBom(testClusterBomName, util.StateFailed)
	hubtesting.AddApplicationConfig(testClusterBom, testAppID)

	lastOp := hubtesting.LastOp(util.OperationInstall, 3, 3, util.StateOk, 1)
	readiness := hubv1.Readiness{State: util.StateOk}
	testHubDeployItem := hubtesting.CreateDeployItem(testClusterBomName, testAppID, 3, 0, true, &lastOp, &readiness)

	unitTestClient := hubtesting.NewUnitTestClientWithCBandDI(testClusterBom, testHubDeployItem)
	unitTestClient.AddSecret(hubtesting.CreateSecret(testClusterBom.Spec.SecretRef))

	clusterBomStateController := ClusterBomStateReconciler{
		Client:         unitTestClient,
		Log:            ctrl.Log.WithName("controllers").WithName("ClusterBomState"),
		Scheme:         runtime.NewScheme(),
		BlockObject:    synchronize.NewBlockObject(nil, false),
		UncachedClient: unitTestClient,
	}

	testRequest := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: testDeploymentConfigName,
		},
	}

	result, err := clusterBomStateController.Reconcile(testRequest)

	assert.Nil(t, err, "reconcile error")
	assert.False(t, result.Requeue, "requeue")

	actualDeployItemStatus := &hubv1.HubDeployItemProviderStatus{}
	err = json.Unmarshal(testHubDeployItem.Status.ProviderStatus.Raw, actualDeployItemStatus)
	assert.Nil(t, err, "unmarshal error")

	resultClusterBom := unitTestClient.ClusterBoms[testClusterBomName]
	assert.NotNil(t, resultClusterBom, "resultClusterBom")
	assert.Equal(t, resultClusterBom.Status.OverallState, util.StatePending, "overallState")
	assert.Equal(t, len(resultClusterBom.Status.ApplicationStates), 1, "length of applicationStates")
	assert.Equal(t, resultClusterBom.Status.ApplicationStates[0].ID, testAppID, "id")
	assert.Equal(t, resultClusterBom.Status.ApplicationStates[0].State, util.StatePending, "state")
	assert.Equal(t, resultClusterBom.Status.ApplicationStates[0].DetailedState.LastOperation, actualDeployItemStatus.LastOperation, "lastOperation")
}

func TestStatusController_StatusChange_Complex(t *testing.T) {
	IDs := []string{"testapp0", "testapp1", "testapp2", "testapp3"}

	// prepare clusterbom with 3 applications: testapp0, testapp1, testapp2
	lastOp0B := hubtesting.LastOp(util.OperationInstall, 34, 34, util.StateOk, 38)
	lastOp1B := hubtesting.LastOp(util.OperationInstall, 32, 32, util.StateOk, 37)
	lastOp2B := hubtesting.LastOp(util.OperationInstall, 30, 30, util.StateFailed, 36)

	clusterBom := hubtesting.CreateClusterBom(testClusterBomName, util.StateFailed)

	hubtesting.AddApplicationConfig(clusterBom, IDs[0])
	hubtesting.AddApplicationStatus(clusterBom, IDs[0], util.StatePending, &lastOp0B)

	hubtesting.AddApplicationConfig(clusterBom, IDs[1])
	hubtesting.AddApplicationStatus(clusterBom, IDs[1], util.StateOk, &lastOp1B)

	hubtesting.AddApplicationConfig(clusterBom, IDs[2])
	hubtesting.AddApplicationStatus(clusterBom, IDs[2], util.StateFailed, &lastOp2B)

	// prepare deployment configs for 2 applications: testapp2, testapp3
	lastOp2 := hubtesting.LastOp(util.OperationInstall, 4, 4, util.StateOk, 6)
	readiness2 := hubv1.Readiness{State: util.StateOk}
	lastOp3 := hubtesting.LastOp(util.OperationInstall, 15, 15, util.StateFailed, 2)
	readiness3 := hubv1.Readiness{State: util.StateFailed}

	deployItem2 := hubtesting.CreateDeployItem(testClusterBomName, IDs[2], 4, 0, true, &lastOp2, &readiness2)
	deployItem3 := hubtesting.CreateDeployItem(testClusterBomName, IDs[3], 22, 0, false, &lastOp3, &readiness3)

	// prepare client, controller, and request
	client := hubtesting.NewUnitTestClientWithCBandDis(clusterBom, deployItem2, deployItem3)
	client.AddSecret(hubtesting.CreateSecret(clusterBom.Spec.SecretRef))

	controller := ClusterBomStateReconciler{
		Client:         client,
		Log:            ctrl.Log.WithName("controllers").WithName("ClusterBomState"),
		Scheme:         runtime.NewScheme(),
		BlockObject:    synchronize.NewBlockObject(nil, false),
		UncachedClient: client,
	}

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: testDeploymentConfigName,
		},
	}

	// execute reconcile logic to be tested
	result, err := controller.Reconcile(request)

	assert.Nil(t, err, "reconcile error")
	assert.False(t, result.Requeue, "requeue")
	resultClusterBom := client.ClusterBoms[testClusterBomName]
	assert.NotNil(t, resultClusterBom, "resultClusterBom")
	assert.Equal(t, resultClusterBom.Status.OverallState, util.StatePending, "overallState")
	assert.Equal(t, len(resultClusterBom.Status.ApplicationStates), 2, "length of applicationStates")

	for i := range resultClusterBom.Status.ApplicationStates {
		applicationState := &resultClusterBom.Status.ApplicationStates[i]
		if applicationState.ID == IDs[2] {
			assert.Equal(t, applicationState.State, util.StatePending, "state 2")
			assert.Equal(t, applicationState.DetailedState.LastOperation, lastOp2, "lastOperation 2")
		} else if applicationState.ID == IDs[3] {
			assert.Equal(t, applicationState.State, util.StatePending, "state 3")
			assert.Equal(t, applicationState.DetailedState.LastOperation, lastOp3, "lastOperation 3")
		} else {
			t.Fail()
		}
	}
}

func TestStatusController_StatusChange_NoDeploymentConfigs(t *testing.T) {
	lastOp := hubtesting.LastOp(util.OperationRemove, 3, 3, util.StateOk, 1)

	clusterBom := hubtesting.CreateClusterBom(testClusterBomName, util.StatePending)
	hubtesting.AddApplicationConfig(clusterBom, testAppID)
	hubtesting.AddApplicationStatus(clusterBom, testAppID, util.StatePending, &lastOp)

	client := hubtesting.NewUnitTestClientWithCBDi(clusterBom)
	client.AddSecret(hubtesting.CreateSecret(clusterBom.Spec.SecretRef))

	controller := ClusterBomStateReconciler{
		Client:         client,
		Log:            ctrl.Log.WithName("controllers").WithName("ClusterBomState"),
		Scheme:         runtime.NewScheme(),
		BlockObject:    synchronize.NewBlockObject(nil, false),
		UncachedClient: client,
	}

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: "testclusterbom01-testapp01",
		},
	}

	result, err := controller.Reconcile(request)

	assert.Nil(t, err, "reconcile error")
	assert.False(t, result.Requeue, "requeue")
	resultClusterBom := client.ClusterBoms[testClusterBomName]
	assert.NotNil(t, resultClusterBom, "resultClusterBom")
	assert.Equal(t, resultClusterBom.Status.OverallState, "ok", "overallState")
	assert.Nil(t, resultClusterBom.Status.ApplicationStates, "applicationStates")
}

func TestStatusController_StatusChange_NothingChanged(t *testing.T) {
	lastOp := hubtesting.LastOp(util.OperationInstall, 5, 5, util.StateFailed, 2)
	readiness := hubv1.Readiness{State: util.StateFailed}

	clusterBom := hubtesting.CreateClusterBom(testClusterBomName, util.StatePending)
	hubtesting.AddApplicationConfig(clusterBom, testAppID)
	hubtesting.AddApplicationStatus(clusterBom, testAppID, util.StatePending, &lastOp)

	deployItem := hubtesting.CreateDeployItem(testClusterBomName, testAppID, 8, 0, true, &lastOp, &readiness)

	client := hubtesting.NewUnitTestClientWithCBandDI(clusterBom, deployItem)
	client.AddSecret(hubtesting.CreateSecret(clusterBom.Spec.SecretRef))

	controller := ClusterBomStateReconciler{
		Client:         client,
		Log:            ctrl.Log.WithName("controllers").WithName("ClusterBomState"),
		Scheme:         runtime.NewScheme(),
		BlockObject:    synchronize.NewBlockObject(nil, false),
		UncachedClient: client,
	}

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: "testclusterbom01-testapp01",
		},
	}

	result, err := controller.Reconcile(request)

	assert.Nil(t, err, "reconcile error")
	assert.False(t, result.Requeue, "requeue")

	actualDeployItemStatus := &hubv1.HubDeployItemProviderStatus{}
	err = json.Unmarshal(deployItem.Status.ProviderStatus.Raw, actualDeployItemStatus)
	assert.Nil(t, err, "unmarshal error")

	resultClusterBom := client.ClusterBoms[testClusterBomName]
	assert.NotNil(t, resultClusterBom, "resultClusterBom")
	assert.Equal(t, resultClusterBom.Status.OverallState, util.StatePending, "overallState")
	assert.Equal(t, len(resultClusterBom.Status.ApplicationStates), 1, "length of applicationStates")
	assert.Equal(t, resultClusterBom.Status.ApplicationStates[0].ID, testAppID, "id")
	assert.Equal(t, resultClusterBom.Status.ApplicationStates[0].State, util.StatePending, "state")
	assert.Equal(t, resultClusterBom.Status.ApplicationStates[0].DetailedState.LastOperation, actualDeployItemStatus.LastOperation, "lastOperation")
}

// TestStatusController_ClusterBom_And_HDC_Removed tests the state controller, if it gets an event for a HDC and neither
// the HDC nor the clusterbom exist. The controller should change nothing and not throw an exception.
func TestStatusController_ClusterBom_And_HDC_Removed(t *testing.T) {
	unitTestClient := hubtesting.NewUnitTestClientDi()

	clusterBomStateController := ClusterBomStateReconciler{
		Client:         unitTestClient,
		Log:            ctrl.Log.WithName("controllers").WithName("ClusterBomState"),
		Scheme:         runtime.NewScheme(),
		BlockObject:    synchronize.NewBlockObject(nil, false),
		UncachedClient: unitTestClient,
	}

	testRequest := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: "testclusterbom01-testapp01",
		},
	}

	result, err := clusterBomStateController.Reconcile(testRequest)

	assert.Nil(t, err, "reconcile error")
	assert.False(t, result.Requeue, "requeue")
	assert.Equal(t, len(unitTestClient.ClusterBoms), 0, "number of clusterboms")
	assert.Equal(t, len(unitTestClient.DeployItems), 0, "number of deployitems")
}

// TestStatusController_RemoveDeploymentConfig tests the state controller, if it gets an event for a HDC with
// last operation = remove and last state = ok. The controller should remove the deploymentconfig; it should not
// change the clusterbom.
func TestStatusController_RemoveDeploymentConfig(t *testing.T) {
	testClusterBom := hubtesting.CreateClusterBom(testClusterBomName, util.StateFailed)
	hubtesting.AddApplicationConfig(testClusterBom, testAppID)

	lastOp := hubtesting.LastOp(util.OperationRemove, 10, 10, util.StateOk, 2)
	readiness := hubv1.Readiness{State: util.StateOk}
	testHubDeployItem := hubtesting.CreateDeployItem(testClusterBomName, testAppID, 10, 0, false, &lastOp, &readiness)

	unitTestClient := hubtesting.NewUnitTestClientWithCBandDI(testClusterBom, testHubDeployItem)

	clusterBomStateController := ClusterBomStateReconciler{
		Client:         unitTestClient,
		Log:            ctrl.Log.WithName("controllers").WithName("ClusterBomState"),
		Scheme:         runtime.NewScheme(),
		BlockObject:    synchronize.NewBlockObject(nil, false),
		UncachedClient: unitTestClient,
	}

	testRequest := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: testDeploymentConfigName,
		},
	}

	result, err := clusterBomStateController.Reconcile(testRequest)

	assert.Nil(t, err, "reconcile error")
	assert.False(t, result.Requeue, "requeue")
	resultClusterBom := unitTestClient.ClusterBoms[testClusterBomName]
	assert.Equal(t, resultClusterBom, testClusterBom, "clusterBom")
}

// TestStatusController_ClusterBomRemoved tests the state controller, if it gets an event for a HDC whose clusterbom was
// removed. The controller should set the currentOperation.operation of the deployitem to remove and increase
// currentOperation.number.
func TestStatusController_ClusterBomRemoved(t *testing.T) {
	lastOp := hubtesting.LastOp(util.OperationInstall, 6, 6, util.StateOk, 3)
	readiness := hubv1.Readiness{State: util.StateOk}
	testHubDeployItem := hubtesting.CreateDeployItem(testClusterBomName, testAppID, 10, 0, true, &lastOp, &readiness)

	unitTestClient := hubtesting.NewUnitTestClientWithDi(testHubDeployItem)

	actualDeployItemConfig := &hubv1.HubDeployItemConfiguration{}
	err := json.Unmarshal(testHubDeployItem.Status.ProviderStatus.Raw, actualDeployItemConfig)
	assert.Nil(t, err, "unmarshal error")

	unitTestClient.AddSecret(hubtesting.CreateSecret(actualDeployItemConfig.LocalSecretRef))

	clusterBomStateController := ClusterBomStateReconciler{
		Client:         unitTestClient,
		Log:            ctrl.Log.WithName("controllers").WithName("ClusterBomState"),
		Scheme:         runtime.NewScheme(),
		BlockObject:    synchronize.NewBlockObject(nil, false),
		UncachedClient: unitTestClient,
	}

	testRequest := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: testDeploymentConfigName,
		},
	}

	result, err := clusterBomStateController.Reconcile(testRequest)

	assert.Nil(t, err, "reconcile error")
	assert.False(t, result.Requeue, "requeue")
	resultClusterBom := unitTestClient.ClusterBoms[testClusterBomName]
	assert.Nil(t, resultClusterBom, "clusterBom")
}

// TestStatusController_DeploymentConfigWithoutStatus tests a clusterbom with an application config.
// There already exists a corresponding deployment config, but it is not yet deployed so that it does not have a status.
func TestStatusController_DeploymentConfigWithoutStatus(t *testing.T) {
	clusterBom := hubtesting.CreateClusterBom(testClusterBomName, util.StatePending)
	hubtesting.AddApplicationConfig(clusterBom, testAppID)

	lastOp := hubtesting.LastOp("", 0, 0, "", 0)
	deployItem := hubtesting.CreateDeployItem(testClusterBomName, testAppID, 1, 0, true, &lastOp, nil)

	client := hubtesting.NewUnitTestClientWithCBandDI(clusterBom, deployItem)
	client.AddSecret(hubtesting.CreateSecret(clusterBom.Spec.SecretRef))

	controller := ClusterBomStateReconciler{
		Client:         client,
		Log:            ctrl.Log.WithName("controllers").WithName("ClusterBomState"),
		Scheme:         runtime.NewScheme(),
		BlockObject:    synchronize.NewBlockObject(nil, false),
		UncachedClient: client,
	}

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: testDeploymentConfigName,
		},
	}

	result, err := controller.Reconcile(request)

	assert.Nil(t, err, "reconcile error")
	assert.False(t, result.Requeue, "requeue")
	resultClusterBom := client.ClusterBoms[testClusterBomName]
	assert.NotNil(t, resultClusterBom, "resultClusterBom")
	assert.Equal(t, resultClusterBom.Status.OverallState, util.StatePending, "overallState")
	assert.Equal(t, len(resultClusterBom.Status.ApplicationStates), 1, "length of applicationStates")
	assert.Equal(t, resultClusterBom.Status.ApplicationStates[0].ID, testAppID, "id")
	assert.Equal(t, resultClusterBom.Status.ApplicationStates[0].State, util.StatePending, "state")
	assert.Equal(t, resultClusterBom.Status.ApplicationStates[0].DetailedState.ObservedGeneration, int64(0), "lastOperation.number")
	assert.Equal(t, resultClusterBom.Status.ApplicationStates[0].DetailedState.LastOperation.NumberOfTries, int32(0), "lastOperation.numberOfTries")
	assert.Equal(t, resultClusterBom.Status.ApplicationStates[0].DetailedState.LastOperation.State, util.StateOk, "lastOperation.state")
	assert.Equal(t, resultClusterBom.Status.ApplicationStates[0].DetailedState.LastOperation.Operation, util.OperationInstall, "lastOperation.operation")
}

func TestStatusController_ErrorGet(t *testing.T) {
	unitTestClient := hubtesting.NewUnitTestGetErrorClientDi()

	clusterBomStateController := ClusterBomStateReconciler{
		Client:         unitTestClient,
		Log:            ctrl.Log.WithName("controllers").WithName("ClusterBomState"),
		Scheme:         runtime.NewScheme(),
		BlockObject:    synchronize.NewBlockObject(nil, false),
		UncachedClient: unitTestClient,
	}

	testRequest := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: testDeploymentConfigName,
		},
	}

	// Execute reconcile logic. This should fail, since the Get method of the client is prepared to fail.
	result, err := clusterBomStateController.Reconcile(testRequest)

	assert.True(t, result.Requeue, "requeue")
	assert.Nil(t, err, "reconcile error")
}

func TestStatusController_ErrorList(t *testing.T) {
	clusterBom := hubtesting.CreateClusterBom(testClusterBomName, util.StateFailed)
	hubtesting.AddApplicationConfig(clusterBom, testAppID)
	lastOp := hubtesting.LastOp(util.OperationRemove, 10, 10, util.StateOk, 2)
	hubtesting.AddApplicationStatus(clusterBom, testAppID, util.StateOk, &lastOp)

	unitTestClient := hubtesting.NewUnitTestListErrorClientDi()
	unitTestClient.AddClusterBom(clusterBom)

	clusterBomStateController := ClusterBomStateReconciler{
		Client:         unitTestClient,
		Log:            ctrl.Log.WithName("controllers").WithName("ClusterBomState"),
		Scheme:         runtime.NewScheme(),
		BlockObject:    synchronize.NewBlockObject(nil, false),
		UncachedClient: unitTestClient,
	}

	testRequest := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: testDeploymentConfigName,
		},
	}

	// Execute reconcile logic. This should fail, since the List method of the client is prepared to fail.
	result, err := clusterBomStateController.Reconcile(testRequest)

	// assert that reconcile fails

	assert.True(t, result.Requeue, "requeue")
	assert.Nil(t, err, "reconcile error")
	// assert that the clusterbom is unchanged
	assert.Equal(t, unitTestClient.ClusterBoms[testClusterBomName], clusterBom, "clusterBom")
}

func TestStatusController_ErrorStatusUpdate(t *testing.T) {
	testClusterBom := hubtesting.CreateClusterBom(testClusterBomName, util.StateFailed)
	hubtesting.AddApplicationConfig(testClusterBom, testAppID)

	lastOp := hubtesting.LastOp(util.OperationInstall, 3, 3, util.StateOk, 1)
	readiness := hubv1.Readiness{State: util.StateOk}
	testHubDeployItem := hubtesting.CreateDeployItem(testClusterBomName, testAppID, 3, 0, true, &lastOp, &readiness)

	unitTestClient := hubtesting.NewUnitTestStatusErrorClientDi()
	unitTestClient.AddClusterBom(testClusterBom)
	unitTestClient.AddDeployItem(testHubDeployItem)
	unitTestClient.AddSecret(hubtesting.CreateSecret(testClusterBom.Spec.SecretRef))

	clusterBomStateController := ClusterBomStateReconciler{
		Client:         unitTestClient,
		Log:            ctrl.Log.WithName("controllers").WithName("ClusterBomState"),
		Scheme:         runtime.NewScheme(),
		BlockObject:    synchronize.NewBlockObject(nil, true),
		UncachedClient: unitTestClient,
	}

	testRequest := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: testDeploymentConfigName,
		},
	}

	result, err := clusterBomStateController.Reconcile(testRequest)

	assert.True(t, result.Requeue, "requeue")
	assert.Nil(t, err, "reconcile error")
}

// TestStatusController_NoCluster tests the state controller, in the case that the secret of the clusterbom
// does not exist. (We do not add a secret to the untiTestClient as in other test cases.) We check that the controller
// removes the HDC, sets the overallState of the clusterbom to pending, and removes the applicationStates from the
// status section of the clusterbom.
func TestStatusController_NoCluster(t *testing.T) {
	testClusterBom := hubtesting.CreateClusterBom(testClusterBomName, util.StateFailed)
	hubtesting.AddApplicationConfig(testClusterBom, testAppID)

	lastOp := hubtesting.LastOp(util.OperationInstall, 3, 3, util.StateOk, 1)
	readiness := hubv1.Readiness{State: util.StateOk}
	testHubDeployItem := hubtesting.CreateDeployItem(testClusterBomName, testAppID, 3, 0, true, &lastOp, &readiness)

	unitTestClient := hubtesting.NewUnitTestClientWithCBandDI(testClusterBom, testHubDeployItem)

	clusterBomStateController := ClusterBomStateReconciler{
		Client:         unitTestClient,
		Log:            ctrl.Log.WithName("controllers").WithName("ClusterBomState"),
		Scheme:         runtime.NewScheme(),
		BlockObject:    synchronize.NewBlockObject(nil, false),
		UncachedClient: unitTestClient,
	}

	testRequest := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: testDeploymentConfigName,
		},
	}

	result, err := clusterBomStateController.Reconcile(testRequest)

	assert.Nil(t, err, "reconcile error")
	assert.False(t, result.Requeue, "requeue")
	resultClusterBom := unitTestClient.ClusterBoms[testClusterBomName]
	assert.NotNil(t, resultClusterBom, "resultClusterBom")
	assert.Equal(t, resultClusterBom.Status.OverallState, "pending", "overallState")
	assert.Equal(t, len(resultClusterBom.Status.ApplicationStates), 0, "length of applicationStates")
	_, resultHDCExists := unitTestClient.DeployItems[testHubDeployItem.Name]
	assert.False(t, resultHDCExists, "resultHDCExists")
}

// TestStatusController_NoCluster_OnlyCB tests the state controller, in the case that the secret of the clusterbom
// does not exist. (We do not add a secret to the untiTestClient as in other test cases.) We start with a clusterbom,
// and no HDCs. After the reconcile, the clusterbom should still exist with overall state pending; no HDCs should be created.
func TestStatusController_NoCluster_OnlyCB(t *testing.T) {
	testClusterBom := hubtesting.CreateClusterBom(testClusterBomName, util.StateFailed)
	hubtesting.AddApplicationConfig(testClusterBom, testAppID)

	unitTestClient := hubtesting.NewUnitTestClientWithCBDi(testClusterBom)

	clusterBomStateController := ClusterBomStateReconciler{
		Client:         unitTestClient,
		Log:            ctrl.Log.WithName("controllers").WithName("ClusterBomState"),
		Scheme:         runtime.NewScheme(),
		BlockObject:    synchronize.NewBlockObject(nil, false),
		UncachedClient: unitTestClient,
	}

	testRequest := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: testDeploymentConfigName,
		},
	}

	result, err := clusterBomStateController.Reconcile(testRequest)

	assert.Nil(t, err, "reconcile error")
	assert.False(t, result.Requeue, "requeue")
	resultClusterBom := unitTestClient.ClusterBoms[testClusterBomName]
	assert.NotNil(t, resultClusterBom, "resultClusterBom")
	assert.Equal(t, resultClusterBom.Status.OverallState, "pending", "overallState")
	assert.Equal(t, len(resultClusterBom.Status.ApplicationStates), 0, "length of applicationStates")
	assert.Equal(t, len(unitTestClient.DeployItems), 0, "number of HDCs")
}

// TestStatusController_NoCluster_DeleteHDC tests the state controller, in the case that the secret of the clusterbom
// does not exist. (We do not add a secret to the untiTestClient as in other test cases.) We check that the controller
// removes the HDC. In this test, we start only with a HDC, which should be deleted after the reconcile. There is no clusterbom.
func TestStatusController_NoCluster_DeleteHDC(t *testing.T) {
	lastOp := hubtesting.LastOp(util.OperationInstall, 3, 3, util.StateOk, 1)
	readiness := hubv1.Readiness{State: util.StateOk}
	testHubDeployItem := hubtesting.CreateDeployItem(testClusterBomName, testAppID, 3, 0, true, &lastOp, &readiness)

	unitTestClient := hubtesting.NewUnitTestClientWithDi(testHubDeployItem)

	clusterBomStateController := ClusterBomStateReconciler{
		Client:         unitTestClient,
		Log:            ctrl.Log.WithName("controllers").WithName("ClusterBomState"),
		Scheme:         runtime.NewScheme(),
		BlockObject:    synchronize.NewBlockObject(nil, false),
		UncachedClient: unitTestClient,
	}

	testRequest := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: testDeploymentConfigName,
		},
	}

	result, err := clusterBomStateController.Reconcile(testRequest)

	assert.Nil(t, err, "reconcile error")
	assert.False(t, result.Requeue, "requeue")
	_, resultClusterBomExists := unitTestClient.ClusterBoms[testClusterBomName]
	assert.False(t, resultClusterBomExists, "resultClusterBomExists")
	_, resultHDCExists := unitTestClient.DeployItems[testHubDeployItem.Name]
	assert.False(t, resultHDCExists, "resultHDCExists")
}

func TestHasClusterBomStatusChanged_ApplicationStates(t *testing.T) {
	oldStatus := hubv1.ClusterBomStatus{
		ApplicationStates: []hubv1.ApplicationState{
			{
				ID:            "A",
				State:         util.StatePending,
				DetailedState: hubv1.DetailedState{},
			},
		},
		OverallState: util.StatePending,
	}
	newStatus := hubv1.ClusterBomStatus{
		ApplicationStates: []hubv1.ApplicationState{
			{
				ID:            "B",
				State:         util.StatePending,
				DetailedState: hubv1.DetailedState{},
			},
		},
		OverallState: util.StatePending,
	}

	changed := hasClusterBomStatusChanged(&oldStatus, &newStatus)
	assert.Equal(t, changed, true, "hasChanged")
}

func TestHasClusterBomStatusChanged_DifferentConditionsTypes(t *testing.T) {
	oldStatus := hubv1.ClusterBomStatus{
		Conditions: []hubv1.ClusterBomCondition{
			{
				Type:   "TestType1",
				Status: corev1.ConditionUnknown,
			},
		},
	}
	newStatus := hubv1.ClusterBomStatus{
		Conditions: []hubv1.ClusterBomCondition{
			{
				Type:   "TestType2",
				Status: corev1.ConditionUnknown,
			},
		},
	}

	changed := hasClusterBomStatusChanged(&oldStatus, &newStatus)
	assert.Equal(t, changed, true, "hasChanged")
}

func TestHasClusterBomStatusChanged_EqualConditions(t *testing.T) {
	oldStatus := hubv1.ClusterBomStatus{
		Conditions: []hubv1.ClusterBomCondition{
			{
				Type:   hubv1.ClusterBomReady,
				Status: corev1.ConditionFalse,
			},
		},
	}
	newStatus := hubv1.ClusterBomStatus{
		Conditions: []hubv1.ClusterBomCondition{
			{
				Type:   hubv1.ClusterBomReady,
				Status: corev1.ConditionFalse,
			},
		},
	}

	changed := hasClusterBomStatusChanged(&oldStatus, &newStatus)
	assert.Equal(t, changed, false, "hasChanged")
}

func buildTestHDC(clusterBomName, appID string, generation, observedGeneration int32, status corev1.ConditionStatus) v1alpha1.DeployItem { //nolint
	deployItemConfig := hubv1.HubDeployItemConfiguration{
		DeploymentConfig: hubv1.DeploymentConfig{
			ID: appID,
		},
	}

	encodedConfig, _ := json.Marshal(deployItemConfig)

	deployItemProviderStatus := hubv1.HubDeployItemProviderStatus{
		LastOperation: hubv1.LastOperation{},
	}

	encodedStatus, _ := json.Marshal(deployItemProviderStatus)

	return v1alpha1.DeployItem{
		ObjectMeta: metav1.ObjectMeta{
			Generation: int64(generation),
			Name:       util.CreateDeployItemName(clusterBomName, appID),
			Labels: map[string]string{
				hubv1.LabelApplicationConfigID: appID,
				hubv1.LabelClusterBomName:      clusterBomName,
			},
		},
		Spec: v1alpha1.DeployItemSpec{
			Type: util.ConfigTypeHelm,
			Configuration: &runtime.RawExtension{
				Raw: encodedConfig,
			},
		},

		Status: v1alpha1.DeployItemStatus{
			ProviderStatus: &runtime.RawExtension{
				Raw: encodedStatus,
			},
			ObservedGeneration: int64(observedGeneration),
			Conditions: []v1alpha1.Condition{
				{
					Type:   v1alpha1.ConditionType(hubv1.HubDeploymentReady),
					Status: v1alpha1.ConditionStatus(status),
				},
			},
		},
	}
}

func TestReadyCondition_AllAppsReady(t *testing.T) {
	clusterbom := hubv1.ClusterBom{
		ObjectMeta: metav1.ObjectMeta{
			Name: testBomName,
		},
		Spec: hubv1.ClusterBomSpec{
			ApplicationConfigs: []hubv1.ApplicationConfig{
				{ID: testAppID, ConfigType: util.ConfigTypeHelm},
				{ID: testAppID2, ConfigType: util.ConfigTypeHelm},
			},
		},
	}

	var deployItems = v1alpha1.DeployItemList{
		Items: []v1alpha1.DeployItem{
			buildTestHDC(testBomName, testAppID, 4, 4, corev1.ConditionTrue),
			buildTestHDC(testBomName, testAppID2, 1, 1, corev1.ConditionTrue),
		},
	}

	clusterBomStateController := ClusterBomStateReconciler{}

	ctx := context.Background()
	ctx = context.WithValue(ctx, util.LoggerKey{}, ctrl.Log.WithName("test"))

	condition, _, err := clusterBomStateController.computeClusterBomReadyCondition(ctx, &deployItems, &clusterbom)
	assert.Nil(t, err, "computeClusterBomReadyCondition")

	assert.Equal(t, condition.Type, hubv1.ClusterBomReady, "condition type")
	assert.Equal(t, condition.Status, corev1.ConditionTrue, "condition status")
	assert.Equal(t, condition.Reason, hubv1.ReasonAllAppsReady, "condition reason")
}

func TestReadyCondition_Failed(t *testing.T) {
	clusterbom := hubv1.ClusterBom{
		ObjectMeta: metav1.ObjectMeta{
			Name: testBomName,
		},
		Spec: hubv1.ClusterBomSpec{
			ApplicationConfigs: []hubv1.ApplicationConfig{
				{ID: testAppID, ConfigType: util.ConfigTypeHelm},
				{ID: testAppID2, ConfigType: util.ConfigTypeHelm},
			},
		},
	}

	var deployItems = v1alpha1.DeployItemList{
		Items: []v1alpha1.DeployItem{
			buildTestHDC(testBomName, testAppID, 4, 4, corev1.ConditionTrue),
			buildTestHDC(testBomName, testAppID2, 1, 1, corev1.ConditionFalse),
		},
	}

	clusterBomStateController := ClusterBomStateReconciler{}

	ctx := context.Background()
	ctx = context.WithValue(ctx, util.LoggerKey{}, ctrl.Log.WithName("test"))

	condition, _, err := clusterBomStateController.computeClusterBomReadyCondition(ctx, &deployItems, &clusterbom)
	assert.Nil(t, err, "computeClusterBomReadyCondition")

	assert.Equal(t, condition.Type, hubv1.ClusterBomReady, "condition type")
	assert.Equal(t, condition.Status, corev1.ConditionFalse, "condition status")
	assert.Equal(t, condition.Reason, hubv1.ReasonFailedApps, "condition reason")
}

func TestReadyCondition_FailedAndPendingApps(t *testing.T) {
	clusterbom := hubv1.ClusterBom{
		ObjectMeta: metav1.ObjectMeta{
			Name: testBomName,
		},
		Spec: hubv1.ClusterBomSpec{
			ApplicationConfigs: []hubv1.ApplicationConfig{
				{ID: testAppID, ConfigType: util.ConfigTypeHelm},
				{ID: testAppID2, ConfigType: util.ConfigTypeHelm},
			},
		},
	}

	var deployItems = v1alpha1.DeployItemList{
		Items: []v1alpha1.DeployItem{
			buildTestHDC(testBomName, testAppID, 4, 3, corev1.ConditionTrue),
			buildTestHDC(testBomName, testAppID2, 1, 1, corev1.ConditionFalse),
		},
	}

	clusterBomStateController := ClusterBomStateReconciler{}

	ctx := context.Background()
	ctx = context.WithValue(ctx, util.LoggerKey{}, ctrl.Log.WithName("test"))

	condition, _, err := clusterBomStateController.computeClusterBomReadyCondition(ctx, &deployItems, &clusterbom)
	assert.Nil(t, err, "computeClusterBomReadyCondition")

	assert.Equal(t, condition.Type, hubv1.ClusterBomReady, "condition type")
	assert.Equal(t, condition.Status, corev1.ConditionFalse, "condition status")
	assert.Equal(t, condition.Reason, hubv1.ReasonFailedAndPendingApps, "condition reason")
}

func TestReadyCondition_PendingApps(t *testing.T) {
	clusterbom := hubv1.ClusterBom{
		ObjectMeta: metav1.ObjectMeta{
			Name: testBomName,
		},
		Spec: hubv1.ClusterBomSpec{
			ApplicationConfigs: []hubv1.ApplicationConfig{
				{ID: testAppID},
				{ID: testAppID2},
			},
		},
	}

	var deployItems = v1alpha1.DeployItemList{
		Items: []v1alpha1.DeployItem{
			buildTestHDC(testBomName, testAppID, 4, 3, corev1.ConditionFalse),
			buildTestHDC(testBomName, testAppID2, 1, 1, corev1.ConditionTrue),
		},
	}

	clusterBomStateController := ClusterBomStateReconciler{}

	ctx := context.Background()
	ctx = context.WithValue(ctx, util.LoggerKey{}, ctrl.Log.WithName("test"))

	condition, _, err := clusterBomStateController.computeClusterBomReadyCondition(ctx, &deployItems, &clusterbom)
	assert.Nil(t, err, "computeClusterBomReadyCondition")

	assert.Equal(t, condition.Type, hubv1.ClusterBomReady, "condition type")
	assert.Equal(t, condition.Status, corev1.ConditionUnknown, "condition status")
	assert.Equal(t, condition.Reason, hubv1.ReasonPendingApps, "condition reason")
}

func TestReadyCondition_MissingApp(t *testing.T) {
	clusterbom := hubv1.ClusterBom{
		ObjectMeta: metav1.ObjectMeta{
			Name: testBomName,
		},
		Spec: hubv1.ClusterBomSpec{
			ApplicationConfigs: []hubv1.ApplicationConfig{
				{ID: testAppID},
			},
		},
	}

	deployItems := v1alpha1.DeployItemList{}

	clusterBomStateController := ClusterBomStateReconciler{}

	ctx := context.Background()
	ctx = context.WithValue(ctx, util.LoggerKey{}, ctrl.Log.WithName("test"))

	condition, _, err := clusterBomStateController.computeClusterBomReadyCondition(ctx, &deployItems, &clusterbom)
	assert.Nil(t, err, "computeClusterBomReadyCondition")

	assert.Equal(t, condition.Type, hubv1.ClusterBomReady, "condition type")
	assert.Equal(t, condition.Status, corev1.ConditionUnknown, "condition status")
	assert.Equal(t, condition.Reason, hubv1.ReasonPendingApps, "condition reason")
}
