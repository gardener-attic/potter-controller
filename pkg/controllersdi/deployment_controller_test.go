package controllersdi

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	hubv1 "github.com/gardener/potter-controller/api/v1"
	"github.com/gardener/potter-controller/pkg/deployutil"
	"github.com/gardener/potter-controller/pkg/helm"
	"github.com/gardener/potter-controller/pkg/synchronize"
	testUtils "github.com/gardener/potter-controller/pkg/testing"
	"github.com/gardener/potter-controller/pkg/util"

	. "github.com/arschles/assert"
	"github.com/gardener/landscaper/apis/core/v1alpha1"
	"github.com/go-logr/zapr"
	"github.com/stretchr/testify/assert"
	"helm.sh/helm/v3/pkg/release"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const testNS = "test-ns"

const testHDCName = "testbom-dummy"
const testHDCKey = testNS + "/" + testHDCName

func TestReconcileWithGetHDCErr(t *testing.T) {
	tests := []struct {
		name                string
		expectedRequeueTime time.Duration
		expectErr           bool
		provokedErr         error
	}{
		{
			name:        "test Get HDC call returns with NotFoundErr",
			expectErr:   false,
			provokedErr: k8sErrors.NewNotFound(schema.GroupResource{}, testHDCKey),
		},
		{
			name:        "test Get HDC call returns with InternalServerError",
			expectErr:   true,
			provokedErr: k8sErrors.NewInternalError(errors.New("omg omg omg! omg")),
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			reactorFuncs := map[string]func() error{}

			if tt.provokedErr != nil {
				reactorFuncs[testHDCKey] = func() error {
					return tt.provokedErr
				}
			}

			fakeClient := testUtils.NewReactiveMockClient(reactorFuncs)
			controller := newDeploymentReconciler(&fakeClient, &helmFacadeMock{})

			result, err := controller.Reconcile(ctrl.Request{
				NamespacedName: types.NamespacedName{
					Namespace: testNS,
					Name:      testHDCName,
				},
			})

			Nil(t, err, "error")
			NotNil(t, result, "result")
			if tt.expectErr {
				Equal(t, result.Requeue, true, "result.Requeue")
			} else {
				Equal(t, result.Requeue, false, "result.Requeue")
			}
			Equal(t, result.RequeueAfter, time.Duration(0), "result.RequeueAfter")
		})
	}
}

func TestReconcileWithInvalidSecretRef(t *testing.T) {
	const operation = "install"
	const number = int32(1)

	typeSpecificData := map[string]interface{}{
		"installName": "der-gute-alte-broker",
		"namespace":   "broker-ns",
		"tarballAccess": map[string]interface{}{
			"url":        "https://myrepo.io/service-broker-0.5.0.tgz",
			"authHeader": "<Insert-correct-auth-here>",
		},
	}

	deployItemConfig := hubv1.HubDeployItemConfiguration{
		LocalSecretRef: "test.secret",
		DeploymentConfig: hubv1.DeploymentConfig{
			ID:               "1",
			TypeSpecificData: *util.CreateRawExtensionOrPanic(typeSpecificData),
		},
	}

	encodedConfig, _ := json.Marshal(deployItemConfig)

	deployItemStatus := hubv1.HubDeployItemProviderStatus{
		LastOperation: hubv1.LastOperation{},
	}

	encodedStatus, _ := json.Marshal(deployItemStatus)

	newDeployItem := v1alpha1.DeployItem{
		ObjectMeta: metav1.ObjectMeta{
			Name:       testHDCName,
			Namespace:  testNS,
			Generation: int64(number),
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
		},
	}

	fakeClient := testUtils.NewReactiveMockClient(map[string]func() error{}, &newDeployItem)
	controller := newDeploymentReconciler(&fakeClient, &helmFacadeMock{})

	result, err := controller.Reconcile(ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNS,
			Name:      testHDCName,
		},
	})

	Nil(t, err, "unexpected error returned from reconcile run")
	NotNil(t, result, "result")
	Equal(t, result.RequeueAfter, time.Second*0, "result.RequeueAfter")

	hdcKey := types.NamespacedName{
		Namespace: testNS,
		Name:      testHDCName,
	}

	err = fakeClient.Get(context.TODO(), hdcKey, &newDeployItem)
	Nil(t, err, "unexpected error when getting updated HDC")

	actualDeployItemStatus := &hubv1.HubDeployItemProviderStatus{}
	err = json.Unmarshal(newDeployItem.Status.ProviderStatus.Raw, actualDeployItemStatus)
	assert.Nil(t, err, "unmarshal error")

	lastOperation := actualDeployItemStatus.LastOperation

	Equal(t, lastOperation.State, util.StateFailed, "lastOperation.State must be failed")
	NotNil(t, lastOperation.Description, "lastOperation.Description must be set")
	Equal(t, lastOperation.NumberOfTries, int32(1), "lastOperation.NumberOfTries")
	Equal(t, lastOperation.Operation, operation, "lastOperation.Operation")
}

func TestReconcileWithDesiredStateEqualToActualState(t *testing.T) {
	const operation = "install"

	typeSpecificData := map[string]interface{}{
		"installName": "der-gute-alte-broker",
		"namespace":   "broker-ns",
		"tarballAccess": map[string]interface{}{
			"url":        "https://myrepo.io/service-broker-0.5.0.tgz",
			"authHeader": "<Insert-correct-auth-here>",
		},
	}

	deployItemConfig := hubv1.HubDeployItemConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testHDCName,
			Namespace: testNS,
		},
		LocalSecretRef: "test.secret",
		DeploymentConfig: hubv1.DeploymentConfig{
			ID:               "1",
			TypeSpecificData: *util.CreateRawExtensionOrPanic(typeSpecificData),
		},
	}

	encodedConfig, _ := json.Marshal(deployItemConfig)

	deployItemStatus := hubv1.HubDeployItemProviderStatus{
		LastOperation: hubv1.LastOperation{
			Operation:     operation,
			Time:          metav1.Now(),
			NumberOfTries: 1,
			State:         util.StateOk,
			Description:   "helm controller is working perfectly fine",
		},

		Readiness: &hubv1.Readiness{
			State: util.StateOk,
		},
	}

	encodedStatus, _ := json.Marshal(deployItemStatus)

	newDeployItem := v1alpha1.DeployItem{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testHDCName,
			Namespace: testNS,
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
		},
	}

	fakeClient := testUtils.NewReactiveMockClient(map[string]func() error{}, &newDeployItem)
	controller := newDeploymentReconciler(&fakeClient, &helmFacadeMock{})

	result, err := controller.Reconcile(ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNS,
			Name:      testHDCName,
		},
	})

	Nil(t, err, "unexpected error returned from reconcile run")
	False(t, result.Requeue, "result.Requeue")
	Equal(t, result.RequeueAfter, time.Second*0, "result.RequeueAfter")
}

type helmFacadeMock struct {
	// configure the return values
	iouReturn error
	remReturn error

	// save InstallOrUpdate() parameters
	iouChartData        *helm.ChartData
	iouNamespace        string
	iouTargetKubeconfig string
	iouReleaseMetadata  *helm.ReleaseMetadata

	// save Remove() parameters
	remInstallName      string
	remNamespace        string
	remTargetKubeconfig string
}

func (h *helmFacadeMock) GetRelease(ctx context.Context, chartData *helm.ChartData, namespace, targetKubeconfig string) (*release.Release, error) {
	return nil, nil
}

func (h *helmFacadeMock) InstallOrUpdate(ctx context.Context, chartData *helm.ChartData, namespace, targetKubeconfig string, metadata *helm.ReleaseMetadata) (*release.Release, error) {
	h.iouChartData = chartData
	h.iouNamespace = namespace
	h.iouTargetKubeconfig = targetKubeconfig
	h.iouReleaseMetadata = metadata
	return nil, h.iouReturn
}

func (h *helmFacadeMock) Remove(ctx context.Context, chartData *helm.ChartData, namespace, targetKubeconfig string) error {
	h.remInstallName = chartData.InstallName
	h.remNamespace = namespace
	h.remTargetKubeconfig = targetKubeconfig
	return h.remReturn
}

func TestInstallOrUpdate_Successful(t *testing.T) {
	const (
		operation        = "install"
		installName      = "der-gute-alte-broker"
		namespace        = "broker-ns"
		secretName       = "test.secret"
		targetKubeconfig = "123xyz"
	)

	typeSpecificData := map[string]interface{}{
		"installName": installName,
		"namespace":   namespace,
		"tarballAccess": map[string]interface{}{
			"url":        "https://myrepo.io/service-broker-0.5.0.tgz",
			"authHeader": "<Insert-correct-auth-here>",
		},
	}

	deployItemConfig := hubv1.HubDeployItemConfiguration{
		LocalSecretRef: secretName,
		DeploymentConfig: hubv1.DeploymentConfig{
			ID:               "1",
			TypeSpecificData: *util.CreateRawExtensionOrPanic(typeSpecificData),
		},
	}

	encodedConfig, _ := json.Marshal(deployItemConfig)

	deployItemStatus := hubv1.HubDeployItemProviderStatus{
		LastOperation: hubv1.LastOperation{
			Operation: operation,
			// ObservedGeneration: 1,
			Time:          metav1.Now(),
			NumberOfTries: 1,
			State:         util.StateOk,
			Description:   "helm controller is working perfectly fine",
		},
	}

	encodedStatus, _ := json.Marshal(deployItemStatus)

	newDeployItem := v1alpha1.DeployItem{
		ObjectMeta: metav1.ObjectMeta{
			Name:       testHDCName,
			Namespace:  testNS,
			Generation: 2,
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
		},
	}

	expectedReleaseMetadata := helm.ReleaseMetadata{
		BomName: "testbom",
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: testNS,
		},
		Data: map[string][]byte{
			"kubeconfig": []byte(targetKubeconfig),
		},
		Type: corev1.SecretTypeOpaque,
	}

	fakeClient := testUtils.NewReactiveMockClient(map[string]func() error{}, &newDeployItem, secret)
	hFacadeMock := &helmFacadeMock{}
	controller := newDeploymentReconciler(&fakeClient, hFacadeMock)

	result, err := controller.Reconcile(ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNS,
			Name:      testHDCName,
		},
	})

	Nil(t, err, "unexpected error returned from reconcile run")
	False(t, result.Requeue, "result.Requeue")
	Equal(t, result.RequeueAfter, time.Second*0, "result.RequeueAfter")
	Equal(t, hFacadeMock.iouChartData.InstallName, installName, "installation name")
	Equal(t, hFacadeMock.iouNamespace, namespace, "installation namespace")
	Equal(t, hFacadeMock.iouTargetKubeconfig, targetKubeconfig, "target kubeconfig")
	Equal(t, *hFacadeMock.iouReleaseMetadata, expectedReleaseMetadata, "release metadata")

	key := client.ObjectKey{
		Namespace: newDeployItem.Namespace,
		Name:      newDeployItem.Name,
	}

	err = fakeClient.Get(context.TODO(), key, &newDeployItem)
	NoErr(t, err)

	actualDeployItemStatus := &hubv1.HubDeployItemProviderStatus{}
	err = json.Unmarshal(newDeployItem.Status.ProviderStatus.Raw, actualDeployItemStatus)
	assert.Nil(t, err, "unmarshal error")

	testState(t, &actualDeployItemStatus.LastOperation, "ok", operation+" successful", operation, int32(2), int32(1),
		"TestInstallOrUpdate_Successful")
}

func TestInstallOrUpdate_WithInvalidTypeSpecificData(t *testing.T) {
	const (
		operation                = "install"
		namespace                = "broker-ns"
		secretName               = "test.secret"
		targetKubeconfig         = "123xyz"
		expectedStateDescription = "could not parse typeSpecificData: property \"installName\" not found"
	)

	// installName is missing in typeSpecificData
	typeSpecificData := map[string]interface{}{
		"namespace": namespace,
		"tarballAccess": map[string]interface{}{
			"url": "https://myrepo.io/service-broker-0.5.0.tgz",
		},
	}

	deployItemConfig := hubv1.HubDeployItemConfiguration{
		LocalSecretRef: secretName,
		DeploymentConfig: hubv1.DeploymentConfig{
			ID:               "1",
			TypeSpecificData: *util.CreateRawExtensionOrPanic(typeSpecificData),
		},
	}

	encodedConfig, _ := json.Marshal(deployItemConfig)

	deployItemStatus := hubv1.HubDeployItemProviderStatus{
		LastOperation: hubv1.LastOperation{
			Operation: operation,
			//ObservedGeneration: 1,
			Time:          metav1.Now(),
			NumberOfTries: 1,
			State:         util.StateOk,
			Description:   "helm controller is working perfectly fine",
		},
	}

	encodedStatus, _ := json.Marshal(deployItemStatus)

	newDeployItem := v1alpha1.DeployItem{
		ObjectMeta: metav1.ObjectMeta{
			Name:       testHDCName,
			Namespace:  testNS,
			Generation: 2,
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
		},
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: testNS,
		},
		Data: map[string][]byte{
			"kubeconfig": []byte(targetKubeconfig),
		},
		Type: corev1.SecretTypeOpaque,
	}

	fakeClient := testUtils.NewReactiveMockClient(map[string]func() error{}, &newDeployItem, secret)
	hFacadeMock := &helmFacadeMock{}
	controller := newDeploymentReconciler(&fakeClient, hFacadeMock)

	result, err := controller.Reconcile(ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNS,
			Name:      testHDCName,
		},
	})

	NoErr(t, err)
	False(t, result.Requeue, "result.Requeue")
	Equal(t, result.RequeueAfter, time.Second*0, "result.RequeueAfter")

	key := client.ObjectKey{
		Namespace: newDeployItem.Namespace,
		Name:      newDeployItem.Name,
	}

	err = fakeClient.Get(context.TODO(), key, &newDeployItem)
	NoErr(t, err)

	actualDeployItemStatus := &hubv1.HubDeployItemProviderStatus{}
	err = json.Unmarshal(newDeployItem.Status.ProviderStatus.Raw, actualDeployItemStatus)
	assert.Nil(t, err, "unmarshal error")

	testState(t, &actualDeployItemStatus.LastOperation, "failed", expectedStateDescription, operation, int32(2), int32(1),
		"TestInstallOrUpdate_WithInvalidTypeSpecificData")
}

func TestRemove_Successful(t *testing.T) {
	const (
		operation        = util.OperationRemove
		installName      = "der-gute-alte-broker"
		namespace        = "broker-ns"
		secretName       = "test.secret"
		targetKubeconfig = "123xyz"
	)

	typeSpecificData := map[string]interface{}{
		"installName": installName,
		"namespace":   namespace,
		"tarballAccess": map[string]interface{}{
			"url":        "https://myrepo.io/service-broker-0.5.0.tgz",
			"authHeader": "<Insert-correct-auth-here>",
		},
	}

	deletionTimestamp := metav1.Now()

	deployItemConfig := hubv1.HubDeployItemConfiguration{
		LocalSecretRef: secretName,
		DeploymentConfig: hubv1.DeploymentConfig{
			ID:               "1",
			TypeSpecificData: *util.CreateRawExtensionOrPanic(typeSpecificData),
		},
	}

	encodedConfig, _ := json.Marshal(deployItemConfig)

	deployItemStatus := hubv1.HubDeployItemProviderStatus{
		LastOperation: hubv1.LastOperation{
			Operation: operation,
			//ObservedGeneration: 1,
			Time:          metav1.Time{Time: time.Now().Add(time.Duration(-1) * time.Minute)},
			NumberOfTries: 1,
			State:         util.StateFailed,
			Description:   "helm controller is working perfectly fine",
		},
	}

	encodedStatus, _ := json.Marshal(deployItemStatus)

	newDeployItem := v1alpha1.DeployItem{
		ObjectMeta: metav1.ObjectMeta{
			Name:              testHDCName,
			Namespace:         testNS,
			Generation:        2,
			DeletionTimestamp: &deletionTimestamp,
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
		},
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: testNS,
		},
		Data: map[string][]byte{
			"kubeconfig": []byte(targetKubeconfig),
		},
		Type: corev1.SecretTypeOpaque,
	}

	fakeClient := testUtils.NewReactiveMockClient(map[string]func() error{}, &newDeployItem, secret)
	hFacadeMock := &helmFacadeMock{}
	controller := newDeploymentReconciler(&fakeClient, hFacadeMock)

	result, err := controller.Reconcile(ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNS,
			Name:      testHDCName,
		},
	})

	Nil(t, err, "unexpected error returned from reconcile run")
	False(t, result.Requeue, "result.Requeue")
	Equal(t, result.RequeueAfter, time.Second*0, "result.RequeueAfter")
	Equal(t, hFacadeMock.remInstallName, installName, "installation name")
	Equal(t, hFacadeMock.remNamespace, namespace, "namespace")
	Equal(t, hFacadeMock.remTargetKubeconfig, targetKubeconfig, "target kubeconfig")

	key := client.ObjectKey{
		Namespace: newDeployItem.Namespace,
		Name:      newDeployItem.Name,
	}

	err = fakeClient.Get(context.TODO(), key, &newDeployItem)
	NoErr(t, err)

	actualDeployItemStatus := &hubv1.HubDeployItemProviderStatus{}
	err = json.Unmarshal(newDeployItem.Status.ProviderStatus.Raw, actualDeployItemStatus)
	assert.Nil(t, err, "unmarshal error")

	testState(t, &actualDeployItemStatus.LastOperation, "ok", operation+" successful", operation, int32(2), int32(1),
		"TestRemove_Successful")
}

func Test_TooEarly_Requeue_After_State_Failed(t *testing.T) {
	const (
		operation             = "install"
		installName           = "der-gute-alte-broker"
		namespace             = "broker-ns"
		secretName            = "test.secret"
		actualOperationNumber = 2
		actualNumberOfTries   = 3
	)

	typeSpecificData := map[string]interface{}{
		"installName": installName,
		"namespace":   namespace,
		"tarballAccess": map[string]interface{}{
			"url":        "https://myrepo.io/service-broker-0.5.0.tgz",
			"authHeader": "<Insert-correct-auth-here>",
		},
	}

	deployItemConfig := hubv1.HubDeployItemConfiguration{
		LocalSecretRef: secretName,
		DeploymentConfig: hubv1.DeploymentConfig{
			ID:               "1",
			TypeSpecificData: *util.CreateRawExtensionOrPanic(typeSpecificData),
		},
	}

	encodedConfig, _ := json.Marshal(deployItemConfig)

	deployItemStatus := hubv1.HubDeployItemProviderStatus{
		LastOperation: hubv1.LastOperation{
			Operation:     operation,
			Time:          metav1.Time{Time: time.Now().Add(time.Second * -70)},
			NumberOfTries: actualNumberOfTries,
			State:         util.StateFailed,
			Description:   "helm controller is working perfectly fine",
		},
	}

	encodedStatus, _ := json.Marshal(deployItemStatus)

	newDeployItem := v1alpha1.DeployItem{
		ObjectMeta: metav1.ObjectMeta{
			Name:       testHDCName,
			Namespace:  testNS,
			Generation: actualOperationNumber,
		},
		Spec: v1alpha1.DeployItemSpec{
			Type: util.ConfigTypeHelm,
			Configuration: &runtime.RawExtension{
				Raw: encodedConfig,
			},
		},
		Status: v1alpha1.DeployItemStatus{
			ObservedGeneration: actualOperationNumber,
			ProviderStatus: &runtime.RawExtension{
				Raw: encodedStatus,
			},
		},
	}

	fakeClient := testUtils.NewReactiveMockClient(map[string]func() error{}, &newDeployItem)
	hFacadeMock := &helmFacadeMock{}
	controller := newDeploymentReconciler(&fakeClient, hFacadeMock)

	result, err := controller.Reconcile(ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNS,
			Name:      testHDCName,
		},
	})

	Nil(t, err, "unexpected error returned from reconcile run")
	True(t, result.RequeueAfter > time.Second*9, "result.RequeueAfter")
	True(t, result.RequeueAfter < time.Second*11, "result.RequeueAfter")
}

func Test_Install_After_State_Failed(t *testing.T) {
	const (
		operation             = "install"
		installName           = "der-gute-alte-broker"
		namespace             = "broker-ns"
		secretName            = "test.secret"
		targetKubeconfig      = "123xyz"
		actualOperationNumber = 4
		actualNumberOfTries   = 3
	)

	typeSpecificData := map[string]interface{}{
		"installName": installName,
		"namespace":   namespace,
		"tarballAccess": map[string]interface{}{
			"url":        "https://myrepo.io/service-broker-0.5.0.tgz",
			"authHeader": "<Insert-correct-auth-here>",
		},
	}

	deployItemConfig := hubv1.HubDeployItemConfiguration{
		LocalSecretRef: secretName,
		DeploymentConfig: hubv1.DeploymentConfig{
			ID:               "1",
			TypeSpecificData: *util.CreateRawExtensionOrPanic(typeSpecificData),
		},
	}

	encodedConfig, _ := json.Marshal(deployItemConfig)

	deployItemStatus := hubv1.HubDeployItemProviderStatus{
		LastOperation: hubv1.LastOperation{
			Operation: operation,
			//ObservedGeneration: actualOperationNumber,
			Time:          metav1.Time{Time: time.Now().Add(time.Second * -400)},
			NumberOfTries: actualNumberOfTries,
			State:         util.StateFailed,
			Description:   "helm controller is working perfectly fine",
		},
	}

	encodedStatus, _ := json.Marshal(deployItemStatus)

	newDeployItem := v1alpha1.DeployItem{
		ObjectMeta: metav1.ObjectMeta{
			Name:       testHDCName,
			Namespace:  testNS,
			Generation: actualOperationNumber,
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
		},
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: testNS,
		},
		Data: map[string][]byte{
			"kubeconfig": []byte(targetKubeconfig),
		},
		Type: corev1.SecretTypeOpaque,
	}

	fakeClient := testUtils.NewReactiveMockClient(map[string]func() error{}, &newDeployItem, secret)
	hFacadeMock := &helmFacadeMock{}
	controller := newDeploymentReconciler(&fakeClient, hFacadeMock)

	result, err := controller.Reconcile(ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNS,
			Name:      testHDCName,
		},
	})

	Nil(t, err, "unexpected error returned from reconcile run")
	False(t, result.Requeue, "result.Requeue")
	Equal(t, result.RequeueAfter, time.Second*0, "result.RequeueAfter")
	Equal(t, hFacadeMock.iouChartData.InstallName, installName, "installation name")
	NotNil(t, hFacadeMock.iouChartData.Load, "loaderFunc")
	Equal(t, hFacadeMock.iouNamespace, namespace, "namespace")
	Equal(t, hFacadeMock.iouTargetKubeconfig, targetKubeconfig, "target kubeconfig")

	key := client.ObjectKey{
		Namespace: newDeployItem.Namespace,
		Name:      newDeployItem.Name,
	}

	err = fakeClient.Get(context.TODO(), key, &newDeployItem)
	NoErr(t, err)

	actualDeployItemStatus := &hubv1.HubDeployItemProviderStatus{}
	err = json.Unmarshal(newDeployItem.Status.ProviderStatus.Raw, actualDeployItemStatus)
	assert.Nil(t, err, "unmarshal error")

	testState(t, &actualDeployItemStatus.LastOperation, util.StateOk, operation+" successful", operation, int32(actualOperationNumber), int32(1),
		"Test_Install_After_State_Failed")
}

func Test_Install_Fails_After_State_Failed(t *testing.T) {
	const (
		operation             = "install"
		installName           = "der-gute-alte-broker"
		namespace             = "broker-ns"
		secretName            = "test.secret"
		targetKubeconfig      = "123xyz"
		actualOperationNumber = 4
		actualNumberOfTries   = 3
		errorString           = "omg omg omg omg!"
	)

	typeSpecificData := map[string]interface{}{
		"installName": installName,
		"namespace":   namespace,
		"tarballAccess": map[string]interface{}{
			"url":        "https://myrepo.io/service-broker-0.5.0.tgz",
			"authHeader": "<Insert-correct-auth-here>",
		},
	}

	deployItemConfig := hubv1.HubDeployItemConfiguration{
		LocalSecretRef: secretName,
		DeploymentConfig: hubv1.DeploymentConfig{
			ID:               "1",
			TypeSpecificData: *util.CreateRawExtensionOrPanic(typeSpecificData),
		},
	}

	encodedConfig, _ := json.Marshal(deployItemConfig)

	deployItemStatus := hubv1.HubDeployItemProviderStatus{
		LastOperation: hubv1.LastOperation{
			Operation:     operation,
			Time:          metav1.Time{Time: time.Now().Add(time.Second * -400)},
			NumberOfTries: actualNumberOfTries,
			State:         util.StateFailed,
			Description:   "helm controller is working perfectly fine",
		},
	}

	encodedStatus, _ := json.Marshal(deployItemStatus)

	newDeployItem := v1alpha1.DeployItem{
		ObjectMeta: metav1.ObjectMeta{
			Name:       testHDCName,
			Namespace:  testNS,
			Generation: actualOperationNumber,
		},
		Spec: v1alpha1.DeployItemSpec{
			Type: util.ConfigTypeHelm,
			Configuration: &runtime.RawExtension{
				Raw: encodedConfig,
			},
		},
		Status: v1alpha1.DeployItemStatus{
			ObservedGeneration: actualOperationNumber,
			ProviderStatus: &runtime.RawExtension{
				Raw: encodedStatus,
			},
		},
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: testNS,
		},
		Data: map[string][]byte{
			"kubeconfig": []byte(targetKubeconfig),
		},
		Type: corev1.SecretTypeOpaque,
	}

	fakeClient := testUtils.NewReactiveMockClient(map[string]func() error{}, &newDeployItem, secret)
	hFacadeMock := &helmFacadeMock{iouReturn: errors.New(errorString)}
	controller := newDeploymentReconciler(&fakeClient, hFacadeMock)

	result, err := controller.Reconcile(ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNS,
			Name:      testHDCName,
		},
	})

	Nil(t, err, "unexpected error returned from reconcile run")
	NotNil(t, hFacadeMock.iouChartData.InstallName, "loaderFunc")
	False(t, result.Requeue, "result.Requeue")
	Equal(t, result.RequeueAfter, time.Second*0, "result.RequeueAfter")

	key := client.ObjectKey{
		Namespace: newDeployItem.Namespace,
		Name:      newDeployItem.Name,
	}

	err = fakeClient.Get(context.TODO(), key, &newDeployItem)
	NoErr(t, err)

	actualDeployItemStatus := &hubv1.HubDeployItemProviderStatus{}
	err = json.Unmarshal(newDeployItem.Status.ProviderStatus.Raw, actualDeployItemStatus)
	assert.Nil(t, err, "unmarshal error")

	testState(t, &actualDeployItemStatus.LastOperation, util.StateFailed, errorString, operation, int32(actualOperationNumber),
		int32(actualNumberOfTries+1), "Test_Install_Fails_After_State_Failed")
}

func Test_Update_HDC_State_Fails(t *testing.T) {
	const (
		operation             = "install"
		installName           = "der-gute-alte-broker"
		namespace             = "broker-ns"
		secretName            = "test.secret"
		targetKubeconfig      = "123xyz"
		actualOperationNumber = 4
		actualNumberOfTries   = 3
		errorString           = "omg omg omg omg!"
	)
	typeSpecificData := map[string]interface{}{
		"installName": installName,
		"namespace":   namespace,
		"tarballAccess": map[string]interface{}{
			"url":        "https://myrepo.io/service-broker-0.5.0.tgz",
			"authHeader": "<Insert-correct-auth-here>",
		},
	}

	deployItemConfig := hubv1.HubDeployItemConfiguration{
		LocalSecretRef: secretName,
		DeploymentConfig: hubv1.DeploymentConfig{
			ID:               "1",
			TypeSpecificData: *util.CreateRawExtensionOrPanic(typeSpecificData),
		},
	}

	encodedConfig, _ := json.Marshal(deployItemConfig)

	deployItemStatus := hubv1.HubDeployItemProviderStatus{
		LastOperation: hubv1.LastOperation{
			Operation: operation,
			//ObservedGeneration: actualOperationNumber,
			Time:          metav1.Time{Time: time.Now().Add(time.Second * -35)},
			NumberOfTries: actualNumberOfTries,
			State:         util.StateFailed,
			Description:   "helm controller is working perfectly fine",
		},
	}

	encodedStatus, _ := json.Marshal(deployItemStatus)

	newDeployItem := v1alpha1.DeployItem{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testHDCName,
			Namespace: testNS,
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
		},
	}

	fun := map[string]func() error{
		testHDCKey: func() error {
			return k8sErrors.NewInternalError(errors.New("something went wrong"))
		},
	}

	fakeClient := testUtils.NewReactiveMockClient(fun, &newDeployItem)
	controller := newDeploymentReconciler(&fakeClient, &helmFacadeMock{})

	ctx := context.Background()
	ctx = context.WithValue(ctx, util.CRAndSecretClientKey{}, &fakeClient)
	ctx = context.WithValue(ctx, util.LoggerKey{}, zapr.NewLogger(zap.NewNop()))

	deployData, _ := deployutil.NewDeployData(&newDeployItem)
	result, err := controller.updateStatus(ctx, deployData)

	Nil(t, err, "error")
	NotNil(t, result, "result")
	Equal(t, result.Requeue, true, "result.Requeue")
	Equal(t, result.RequeueAfter, time.Duration(0), "result.RequeueAfter")
}

func TestMergeMap(t *testing.T) {
	m1 := map[string]interface{}{
		"k1": "1",
		"k2": "2",
		"k4": []string{"4", "5", "6"},
		"k5": map[string]interface{}{
			"k51": "7",
			"k52": []string{"71", "72"},
			"k53": "8",
		},
	}
	m2 := map[string]interface{}{
		"k2": "200",
		"k3": "300",
		"k4": []string{"400", "500"},
		"k5": map[string]interface{}{
			"k52": "700",
			"k53": map[string]interface{}{
				"k531": "800",
			},
		},
	}
	mergedMap := deployutil.MergeMaps(m1, m2)
	Equal(t, len(mergedMap), 5, "length of merged map")
	Equal(t, mergedMap["k1"], m1["k1"], "value 1")
	Equal(t, mergedMap["k2"], m2["k2"], "value 2")
	Equal(t, mergedMap["k3"], m2["k3"], "value 3")
	Equal(t, mergedMap["k4"], m2["k4"], "value 4")

	NotNil(t, mergedMap["k5"], "value 5")
	m5, ok := mergedMap["k5"].(map[string]interface{})
	True(t, ok, "value 5 is a map")
	Equal(t, len(m5), 3, "length of m5")
	Equal(t, m5["k51"], "7", "value 51")
	Equal(t, m5["k52"], "700", "value 52")

	NotNil(t, m5["k53"], "value 53")
	m53, ok := m5["k53"].(map[string]interface{})
	True(t, ok, "value 53 is a map")
	Equal(t, len(m53), 1, "length of m53")
	Equal(t, m53["k531"], "800", "value 531")
}

func testState(t *testing.T, lastOperation *hubv1.LastOperation, state, description, operation string, number, numberOfTries int32, test string) { // nolint
	Equal(t, lastOperation.Operation, operation, test+" - operation")
	Equal(t, lastOperation.State, state, test+" - state")
	Equal(t, lastOperation.Description, description, test+" - description")
	Equal(t, lastOperation.NumberOfTries, numberOfTries, test+" - number of tries")
	NotNil(t, lastOperation.Time, test+" - time")
}

func newDeploymentReconciler(fakeClient client.Client, helmFacade helm.Facade) *DeploymentReconciler {
	uncachedClient := testUtils.NewUnitTestClientDi()
	blockObject := synchronize.NewBlockObject(nil, false)

	deployerFactory := unitTestDeployerFactory{
		crAndSecretClient: fakeClient,
		uncachedClient:    uncachedClient,
		helmFacade:        helmFacade,
		blockObject:       blockObject,
	}

	return &DeploymentReconciler{
		deployerFactory:   &deployerFactory,
		crAndSecretClient: fakeClient,
		log:               ctrl.Log.WithName("controllers").WithName("DeploymentReconciler"),
		scheme:            &runtime.Scheme{},
		blockObject:       blockObject,
		uncachedClient:    uncachedClient,
	}
}

type unitTestDeployerFactory struct {
	crAndSecretClient client.Client
	uncachedClient    synchronize.UncachedClient
	helmFacade        helm.Facade
	blockObject       *synchronize.BlockObject
}

func (r *unitTestDeployerFactory) GetDeployer(configType string) (deployutil.DeployItemDeployer, error) {
	deployer := helm.NewHelmDeployerDIWithFacade(r.crAndSecretClient, r.uncachedClient, r.helmFacade, nil, r.blockObject)
	return deployer, nil
}
