package controllersdi

import (
	"bytes"
	"encoding/json"
	"testing"

	hubv1 "github.com/gardener/potter-controller/api/v1"
	"github.com/gardener/potter-controller/pkg/synchronize"
	testing2 "github.com/gardener/potter-controller/pkg/testing"
	"github.com/gardener/potter-controller/pkg/util"

	"github.com/gardener/landscaper/pkg/apis/core/v1alpha1"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

const testBomName = "testbom1"
const appConfigID = "testappid1"

func Test_No_UpdateDi(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	g := gomega.NewGomegaWithT(t)

	deployItemName := util.CreateDeployItemName(testBomName, appConfigID)

	testClusterBom := &hubv1.ClusterBom{
		ObjectMeta: v1.ObjectMeta{
			Name: testBomName,
		},
		Spec: hubv1.ClusterBomSpec{
			SecretRef: "asdf",
			ApplicationConfigs: []hubv1.ApplicationConfig{
				{
					ID:               appConfigID,
					ConfigType:       util.ConfigTypeHelm,
					TypeSpecificData: *testing2.FakeRawExtensionWithProperty("existing-value"),
					Values:           testing2.FakeRawExtensionWithProperty("existing-value"),
				},
			},
		},
		Status: hubv1.ClusterBomStatus{},
	}

	deployItemConfig := hubv1.HubDeployItemConfiguration{
		LocalSecretRef: "asdf",
		DeploymentConfig: hubv1.DeploymentConfig{
			ID:               appConfigID,
			TypeSpecificData: *testing2.FakeRawExtensionWithProperty("existing-value"),
			Values:           testing2.FakeRawExtensionWithProperty("existing-value"),
		},
	}

	encodedConfig, err := json.Marshal(deployItemConfig)
	g.Expect(err).To(gomega.BeNil())

	testDeployItem := &v1alpha1.DeployItem{
		ObjectMeta: v1.ObjectMeta{
			Name: deployItemName,
			Labels: map[string]string{
				hubv1.LabelClusterBomName:      testBomName,
				hubv1.LabelApplicationConfigID: appConfigID,
			},
		},
		Spec: v1alpha1.DeployItemSpec{
			Type: util.ConfigTypeHelm,
			Configuration: &runtime.RawExtension{
				Raw: encodedConfig,
			},
		},
	}

	// create a client initially containing our test BOM
	unitTestClient := testing2.NewUnitTestClientWithCBandDI(testClusterBom, testDeployItem)
	unitTestClient.AddSecret(testing2.CreateSecret(testClusterBom.Spec.SecretRef))

	clusterBomCRController := ClusterBomReconciler{
		Client:              unitTestClient,
		Log:                 ctrl.Log.WithName("controllers").WithName("ClusterBom"),
		Scheme:              runtime.NewScheme(),
		blockObject:         *synchronize.NewBlockObject(nil, false),
		uncachedClient:      unitTestClient,
		hubControllerClient: unitTestClient,
	}

	// Call reconciler
	testRequest := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: testBomName,
		},
	}

	result, err := clusterBomCRController.Reconcile(testRequest)

	// Assert Reconciliation without error
	g.Expect(result).To(gomega.Equal(ctrl.Result{}))
	g.Expect(err).To(gomega.BeNil())

	// Assert Reconciliation had correct behavior
	g.Expect(len(unitTestClient.ClusterBoms)).To(gomega.Equal(1))
	g.Expect(len(unitTestClient.DeployItems)).To(gomega.Equal(1))

	// Validate that everything stayed as is
	actual := unitTestClient.DeployItems[deployItemName]

	actualDeployItemConfig := &hubv1.HubDeployItemConfiguration{}

	err = json.Unmarshal(actual.Spec.Configuration.Raw, actualDeployItemConfig)
	g.Expect(err).To(gomega.BeNil())

	g.Expect(actual).NotTo(gomega.BeNil())
	g.Expect(actualDeployItemConfig.LocalSecretRef).To(gomega.Equal("asdf"))
	g.Expect(actualDeployItemConfig.DeploymentConfig.ID).To(gomega.Equal(appConfigID))
	g.Expect(string(actual.Spec.Type)).To(gomega.Equal(util.ConfigTypeHelm))
	// TODO Maybe we can find a nicer way to compare the []byte here
	g.Expect(bytes.Equal(actualDeployItemConfig.DeploymentConfig.TypeSpecificData.Raw, testing2.FakeRawExtensionWithProperty("existing-value").Raw)).To(gomega.BeTrue())
	g.Expect(bytes.Equal(actualDeployItemConfig.DeploymentConfig.Values.Raw, testing2.FakeRawExtensionWithProperty("existing-value").Raw)).To(gomega.BeTrue())
}

func TestReconcile_Update_Existing_HDC_From_CB(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	g := gomega.NewGomegaWithT(t)

	deployItemName := util.CreateDeployItemName(testBomName, appConfigID)

	testClusterBom := &hubv1.ClusterBom{
		ObjectMeta: v1.ObjectMeta{
			Name: testBomName,
		},
		Spec: hubv1.ClusterBomSpec{
			SecretRef: "asdf",
			ApplicationConfigs: []hubv1.ApplicationConfig{
				{
					ID:               appConfigID,
					ConfigType:       util.ConfigTypeHelm,
					TypeSpecificData: *testing2.FakeRawExtensionWithProperty("new-type-value"),
					Values:           testing2.FakeRawExtensionWithProperty("new-value-value"),
				},
			},
		},
		Status: hubv1.ClusterBomStatus{},
	}

	deployItemConfig := hubv1.HubDeployItemConfiguration{
		LocalSecretRef: "asdf",
		DeploymentConfig: hubv1.DeploymentConfig{
			ID:               appConfigID,
			TypeSpecificData: *testing2.FakeRawExtensionWithProperty("old-type-value"),
			Values:           testing2.FakeRawExtensionWithProperty("old-value-value"),
		},
	}

	encodedConfig, err := json.Marshal(deployItemConfig)
	g.Expect(err).To(gomega.BeNil())

	testDeployItem := &v1alpha1.DeployItem{
		ObjectMeta: v1.ObjectMeta{
			Name: deployItemName,
			Labels: map[string]string{
				hubv1.LabelClusterBomName:      testBomName,
				hubv1.LabelApplicationConfigID: appConfigID,
			},
		},
		Spec: v1alpha1.DeployItemSpec{
			Type: util.ConfigTypeHelm,
			Configuration: &runtime.RawExtension{
				Raw: encodedConfig,
			},
		},
	}

	// create a client that has initially our test BOM
	unitTestClient := testing2.NewUnitTestClientWithCBandDI(testClusterBom, testDeployItem)
	unitTestClient.AddSecret(testing2.CreateSecret(testClusterBom.Spec.SecretRef))

	clusterBomCRController := ClusterBomReconciler{
		Client:              unitTestClient,
		Log:                 ctrl.Log.WithName("controllers").WithName("ClusterBom"),
		Scheme:              runtime.NewScheme(),
		blockObject:         *synchronize.NewBlockObject(nil, false),
		uncachedClient:      unitTestClient,
		hubControllerClient: unitTestClient,
	}

	// Call reconciler
	testRequest := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: testBomName,
		},
	}

	result, err := clusterBomCRController.Reconcile(testRequest)

	// Assert Reconciliation without error
	g.Expect(result).To(gomega.Equal(ctrl.Result{}))
	g.Expect(err).To(gomega.BeNil())

	// Assert Reconciliation had correct behavior
	g.Expect(len(unitTestClient.ClusterBoms)).To(gomega.Equal(1))
	g.Expect(len(unitTestClient.DeployItems)).To(gomega.Equal(1))

	actual := unitTestClient.DeployItems[deployItemName]

	actualDeployItemConfig := &hubv1.HubDeployItemConfiguration{}

	err = json.Unmarshal(actual.Spec.Configuration.Raw, actualDeployItemConfig)
	g.Expect(err).To(gomega.BeNil())

	g.Expect(actual).NotTo(gomega.BeNil())
	g.Expect(actualDeployItemConfig.LocalSecretRef).To(gomega.Equal("asdf"))
	g.Expect(actualDeployItemConfig.DeploymentConfig.ID).To(gomega.Equal(appConfigID))
	g.Expect(string(actual.Spec.Type)).To(gomega.Equal(util.ConfigTypeHelm))
	// TODO Maybe we can find a nicer way to compare the []byte here
	g.Expect(bytes.Equal(actualDeployItemConfig.DeploymentConfig.TypeSpecificData.Raw, testing2.FakeRawExtensionWithProperty("new-type-value").Raw)).To(gomega.BeTrue())
	g.Expect(bytes.Equal(actualDeployItemConfig.DeploymentConfig.Values.Raw, testing2.FakeRawExtensionWithProperty("new-value-value").Raw)).To(gomega.BeTrue())
	// Validate that an operation was done
}

func TestReconcile_Create_New_HDC_From_CB(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	g := gomega.NewGomegaWithT(t)

	deployItemName := util.CreateDeployItemName(testBomName, appConfigID)

	testClusterBom := &hubv1.ClusterBom{
		ObjectMeta: v1.ObjectMeta{
			Name: testBomName,
		},
		Spec: hubv1.ClusterBomSpec{
			SecretRef: "asdf",
			ApplicationConfigs: []hubv1.ApplicationConfig{
				{
					ID:               appConfigID,
					ConfigType:       util.ConfigTypeHelm,
					TypeSpecificData: *testing2.FakeRawExtensionWithProperty("test-data"),
				},
			},
		},
		Status: hubv1.ClusterBomStatus{},
	}

	// create a client that has initially our test BOM
	unitTestClient := testing2.NewUnitTestClientWithCBDi(testClusterBom)
	unitTestClient.AddSecret(testing2.CreateSecret(testClusterBom.Spec.SecretRef))

	clusterBomCRController := ClusterBomReconciler{
		Client:              unitTestClient,
		Log:                 ctrl.Log.WithName("controllers").WithName("ClusterBom"),
		Scheme:              runtime.NewScheme(),
		blockObject:         *synchronize.NewBlockObject(nil, false),
		uncachedClient:      unitTestClient,
		hubControllerClient: unitTestClient,
	}

	// Call reconciler
	testRequest := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: testBomName,
		},
	}

	result, err := clusterBomCRController.Reconcile(testRequest)

	// Assert Reconciliation without error
	g.Expect(result).To(gomega.Equal(ctrl.Result{}))
	g.Expect(err).To(gomega.BeNil())

	// Assert Reconciliation had correct behavior
	g.Expect(len(unitTestClient.ClusterBoms)).To(gomega.Equal(1))
	g.Expect(len(unitTestClient.DeployItems)).To(gomega.Equal(1))

	actual := unitTestClient.DeployItems[deployItemName]

	actualDeployItemConfig := &hubv1.HubDeployItemConfiguration{}

	err = json.Unmarshal(actual.Spec.Configuration.Raw, actualDeployItemConfig)
	g.Expect(err).To(gomega.BeNil())

	g.Expect(actual).NotTo(gomega.BeNil())
	g.Expect(actualDeployItemConfig.LocalSecretRef).To(gomega.Equal("asdf"))
	g.Expect(actualDeployItemConfig.DeploymentConfig.ID).To(gomega.Equal(appConfigID))
	g.Expect(string(actual.Spec.Type)).To(gomega.Equal(util.ConfigTypeHelm))
	g.Expect(actualDeployItemConfig.DeploymentConfig.Values).To(gomega.BeNil())
	// TODO Maybe we can find a nicer way to compare the []byte here
	g.Expect(bytes.Equal(actualDeployItemConfig.DeploymentConfig.TypeSpecificData.Raw, testing2.FakeRawExtensionWithProperty("test-data").Raw)).To(gomega.BeTrue())

	// Validate that an operation was done, and the operation is initializes correctly
}

func TestReconcile_Delete_Existing_HDC_From_CB(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	g := gomega.NewGomegaWithT(t)

	deployItemName := util.CreateDeployItemName(testBomName, appConfigID)

	testClusterBom := &hubv1.ClusterBom{
		ObjectMeta: v1.ObjectMeta{
			Name: testBomName,
		},
		Spec: hubv1.ClusterBomSpec{
			SecretRef:          "asdf",
			ApplicationConfigs: []hubv1.ApplicationConfig{},
		},
		Status: hubv1.ClusterBomStatus{},
	}

	deployItemConfig := hubv1.HubDeployItemConfiguration{
		LocalSecretRef: "asdf",
		DeploymentConfig: hubv1.DeploymentConfig{
			ID:               appConfigID,
			TypeSpecificData: *testing2.FakeRawExtensionWithProperty("old-type-value"),
			Values:           testing2.FakeRawExtensionWithProperty("old-value-value"),
		},
	}

	encodedConfig, err := json.Marshal(deployItemConfig)
	g.Expect(err).To(gomega.BeNil())

	testDeployItem := &v1alpha1.DeployItem{
		ObjectMeta: v1.ObjectMeta{
			Name: deployItemName,
		},
		Spec: v1alpha1.DeployItemSpec{
			Type: util.ConfigTypeHelm,
			Configuration: &runtime.RawExtension{
				Raw: encodedConfig,
			},
		},
	}

	// create a client that has initially our test BOM
	unitTestClient := testing2.NewUnitTestClientWithCBandDI(testClusterBom, testDeployItem)
	unitTestClient.AddSecret(testing2.CreateSecret(testClusterBom.Spec.SecretRef))

	clusterBomCRController := ClusterBomReconciler{
		Client:              unitTestClient,
		Log:                 ctrl.Log.WithName("controllers").WithName("ClusterBom"),
		Scheme:              runtime.NewScheme(),
		blockObject:         *synchronize.NewBlockObject(nil, false),
		uncachedClient:      unitTestClient,
		hubControllerClient: unitTestClient,
	}

	// Call reconciler
	testRequest := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: testBomName,
		},
	}

	result, err := clusterBomCRController.Reconcile(testRequest)

	// Assert Reconciliation without error
	g.Expect(result).To(gomega.Equal(ctrl.Result{}))
	g.Expect(err).To(gomega.BeNil())

	// Assert Reconciliation had correct behavior

	actual := unitTestClient.DeployItems[deployItemName]

	g.Expect(actual).To(gomega.BeNil())
}

func TestReconcile_Delete_Existing_CB(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	g := gomega.NewGomegaWithT(t)

	deployItemName := util.CreateDeployItemName(testBomName, appConfigID)

	deployItemConfig := hubv1.HubDeployItemConfiguration{
		LocalSecretRef: "asdf",
		DeploymentConfig: hubv1.DeploymentConfig{
			ID:               appConfigID,
			TypeSpecificData: *testing2.FakeRawExtensionWithProperty("old-type-value"),
			Values:           testing2.FakeRawExtensionWithProperty("old-value-value"),
		},
	}

	encodedConfig, err := json.Marshal(deployItemConfig)
	g.Expect(err).To(gomega.BeNil())

	testDeployItem := &v1alpha1.DeployItem{
		ObjectMeta: v1.ObjectMeta{
			Name: deployItemName,
		},
		Spec: v1alpha1.DeployItemSpec{
			Type: util.ConfigTypeHelm,
			Configuration: &runtime.RawExtension{
				Raw: encodedConfig,
			},
		},
	}

	// create a client that has initially our test BOM
	unitTestClient := testing2.NewUnitTestClientWithDi(testDeployItem)
	unitTestClient.AddSecret(testing2.CreateSecret(deployItemConfig.LocalSecretRef))

	clusterBomCRController := ClusterBomReconciler{
		Client:              unitTestClient,
		Log:                 ctrl.Log.WithName("controllers").WithName("ClusterBom"),
		Scheme:              runtime.NewScheme(),
		blockObject:         *synchronize.NewBlockObject(nil, false),
		uncachedClient:      unitTestClient,
		hubControllerClient: unitTestClient,
	}

	// Call reconciler
	testRequest := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: testBomName,
		},
	}

	result, err := clusterBomCRController.Reconcile(testRequest)

	// Assert Reconciliation without error
	g.Expect(result).To(gomega.Equal(ctrl.Result{}))
	g.Expect(err).To(gomega.BeNil())

	// Assert Reconciliation had correct behavior
	actual := unitTestClient.DeployItems[deployItemName]
	g.Expect(actual).To(gomega.BeNil())
}

func TestReconcile_Remove_Values(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	g := gomega.NewGomegaWithT(t)

	deployItemName := util.CreateDeployItemName(testBomName, appConfigID)

	testClusterBom := &hubv1.ClusterBom{
		ObjectMeta: v1.ObjectMeta{
			Name: testBomName,
		},
		Spec: hubv1.ClusterBomSpec{
			ApplicationConfigs: []hubv1.ApplicationConfig{
				{
					ID:               appConfigID,
					ConfigType:       util.ConfigTypeHelm,
					TypeSpecificData: *testing2.FakeRawExtensionWithProperty("test-data"),
					Values:           nil,
				},
			},
		},
		Status: hubv1.ClusterBomStatus{},
	}

	deployItemConfig := hubv1.HubDeployItemConfiguration{
		LocalSecretRef: "asdf",
		DeploymentConfig: hubv1.DeploymentConfig{
			ID:               appConfigID,
			TypeSpecificData: *testing2.FakeRawExtensionWithProperty("test"),
			Values:           testing2.FakeRawExtensionWithProperty("some-value"),
		},
	}

	encodedConfig, err := json.Marshal(deployItemConfig)
	g.Expect(err).To(gomega.BeNil())

	testDeployItem := &v1alpha1.DeployItem{
		ObjectMeta: v1.ObjectMeta{
			Name: deployItemName,
			Labels: map[string]string{
				hubv1.LabelClusterBomName:      testBomName,
				hubv1.LabelApplicationConfigID: appConfigID,
			},
		},
		Spec: v1alpha1.DeployItemSpec{
			Type: util.ConfigTypeHelm,
			Configuration: &runtime.RawExtension{
				Raw: encodedConfig,
			},
		},
	}

	// create a client that has initially our test BOM
	unitTestClient := testing2.NewUnitTestClientWithCBandDI(testClusterBom, testDeployItem)
	unitTestClient.AddSecret(testing2.CreateSecret(testClusterBom.Spec.SecretRef))

	clusterBomCRController := ClusterBomReconciler{
		Client:              unitTestClient,
		Log:                 ctrl.Log.WithName("controllers").WithName("ClusterBom"),
		Scheme:              runtime.NewScheme(),
		blockObject:         *synchronize.NewBlockObject(nil, false),
		uncachedClient:      unitTestClient,
		hubControllerClient: unitTestClient,
	}

	// Call reconciler
	testRequest := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: testBomName,
		},
	}

	result, err := clusterBomCRController.Reconcile(testRequest)

	// Assert Reconciliation without error
	g.Expect(result).To(gomega.Equal(ctrl.Result{}))
	g.Expect(err).To(gomega.BeNil())

	// Assert Reconciliation had correct behavior
	actual := unitTestClient.DeployItems[deployItemName]
	g.Expect(actual).NotTo(gomega.BeNil())

	actualDeployItemConfig := &hubv1.HubDeployItemConfiguration{}

	err = json.Unmarshal(actual.Spec.Configuration.Raw, actualDeployItemConfig)
	g.Expect(err).To(gomega.BeNil())
	g.Expect(actualDeployItemConfig.DeploymentConfig.Values).To(gomega.BeNil())
}

func Test_HDC_List_Error(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	g := gomega.NewGomegaWithT(t)

	// create a client that returns an error on Deploy Item List.
	unitTestClient := testing2.NewUnitTestListErrorClientDi()

	clusterBomCRController := ClusterBomReconciler{
		Client:              unitTestClient,
		Log:                 ctrl.Log.WithName("controllers").WithName("ClusterBom"),
		Scheme:              runtime.NewScheme(),
		blockObject:         *synchronize.NewBlockObject(nil, false),
		uncachedClient:      unitTestClient,
		hubControllerClient: unitTestClient,
	}

	// Call reconciler
	testRequest := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: testBomName,
		},
	}

	result, err := clusterBomCRController.Reconcile(testRequest)

	// Assert Reconciliation without error
	g.Expect(err).To(gomega.BeNil())
	g.Expect(result.Requeue).To(gomega.BeTrue())
}

func Test_CB_Get_Error(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	g := gomega.NewGomegaWithT(t)

	// create a client that returns an error on ClusterBom Get.
	unitTestClient := testing2.NewUnitTestGetErrorClientDi()

	clusterBomCRController := ClusterBomReconciler{
		Client:              unitTestClient,
		Log:                 ctrl.Log.WithName("controllers").WithName("ClusterBom"),
		Scheme:              runtime.NewScheme(),
		blockObject:         *synchronize.NewBlockObject(nil, false),
		uncachedClient:      unitTestClient,
		hubControllerClient: unitTestClient,
	}

	// Call reconciler
	testRequest := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: testBomName,
		},
	}

	result, err := clusterBomCRController.Reconcile(testRequest)

	// Assert Reconciliation without error
	g.Expect(err).To(gomega.BeNil())
	g.Expect(result.Requeue).To(gomega.BeTrue())
}

func Test_isEqualConfig(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	g := gomega.NewGomegaWithT(t)

	fake1 := "{\"user\": \"fabian\", \"password\": \"secret\"}"
	fake2 := "{\"password\": \"secret\", \"user\": \"fabian\"}"
	fake3 := "[1, 2, 3]"
	fake4 := "[3, 2, 1]"
	var ext1 runtime.RawExtension
	var ext2 runtime.RawExtension

	ext1.Raw = []byte(fake1)
	ext2.Raw = []byte(fake2)
	isEqual := isEqualRawJSON(&ext1, &ext2)
	g.Expect(isEqual).To(gomega.BeTrue())

	ext1.Raw = []byte(fake3)
	ext2.Raw = []byte(fake4)

	isEqual = isEqualRawJSON(&ext1, &ext2)
	g.Expect(isEqual).To(gomega.BeFalse())

	// test nil cases
	g.Expect(isEqualRawJSON(nil, nil)).To(gomega.BeTrue())
	g.Expect(isEqualRawJSON(&ext1, nil)).To(gomega.BeFalse())
	g.Expect(isEqualRawJSON(nil, &ext2)).To(gomega.BeFalse())
}
