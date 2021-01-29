package testing

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/gardener/landscaper/pkg/apis/core/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	errors2 "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hubv1 "github.com/gardener/potter-controller/api/v1"
)

type TestClientDi struct {
}

func (t TestClientDi) Status() client.StatusWriter {
	return nil
}

func (t TestClientDi) Create(context.Context, runtime.Object, ...client.CreateOption) error {
	return nil
}

func (t TestClientDi) Delete(context.Context, runtime.Object, ...client.DeleteOption) error {
	return nil
}

func (t TestClientDi) Update(context.Context, runtime.Object, ...client.UpdateOption) error {
	return nil
}

func (t TestClientDi) Patch(context.Context, runtime.Object, client.Patch, ...client.PatchOption) error {
	return nil
}

func (t TestClientDi) DeleteAllOf(context.Context, runtime.Object, ...client.DeleteAllOfOption) error {
	return nil
}

func (t TestClientDi) Get(context.Context, client.ObjectKey, runtime.Object) error {
	return nil
}

func (t TestClientDi) List(context.Context, runtime.Object, ...client.ListOption) error {
	return nil
}

// Begin Unit Test Client that holds object in memory to be used as client mock
type UnitTestClientDi struct {
	TestClientDi
	ClusterBoms     map[string]*hubv1.ClusterBom
	DeployItems     map[string]*v1alpha1.DeployItem
	Installations   map[string]*v1alpha1.Installation
	Secrets         map[string]*corev1.Secret
	ClusterBomSyncs map[string]*hubv1.ClusterBomSync
}

func NewUnitTestClientDi() *UnitTestClientDi {
	cli := new(UnitTestClientDi)
	cli.ClusterBoms = make(map[string]*hubv1.ClusterBom)
	cli.DeployItems = make(map[string]*v1alpha1.DeployItem)
	cli.Installations = make(map[string]*v1alpha1.Installation)
	cli.Secrets = make(map[string]*corev1.Secret)
	cli.ClusterBomSyncs = make(map[string]*hubv1.ClusterBomSync)
	return cli
}

func NewUnitTestClientWithCBDi(bom *hubv1.ClusterBom) *UnitTestClientDi {
	cli := NewUnitTestClientDi()
	cli.AddClusterBom(bom)
	return cli
}

func NewUnitTestClientWithCBandDI(bom *hubv1.ClusterBom, deployItem *v1alpha1.DeployItem) *UnitTestClientDi {
	cli := NewUnitTestClientDi()
	cli.AddClusterBom(bom)
	cli.AddDeployItem(deployItem)
	return cli
}

func NewUnitTestClientWithCBandDis(bom *hubv1.ClusterBom, deployItems ...*v1alpha1.DeployItem) *UnitTestClientDi {
	cli := NewUnitTestClientDi()
	cli.AddClusterBom(bom)
	for i := range deployItems {
		cli.AddDeployItem(deployItems[i])
	}
	return cli
}

func NewUnitTestClientWithDi(deployItem *v1alpha1.DeployItem) *UnitTestClientDi {
	cli := NewUnitTestClientDi()
	cli.AddDeployItem(deployItem)
	return cli
}

func (t UnitTestClientDi) AddClusterBom(bom *hubv1.ClusterBom) {
	if bom != nil {
		t.ClusterBoms[bom.Name] = bom
	}
}

func (t UnitTestClientDi) AddDeployItem(deployItem *v1alpha1.DeployItem) {
	if deployItem != nil {
		t.DeployItems[deployItem.Name] = deployItem
	}
}

func (t UnitTestClientDi) AddSecret(secret *corev1.Secret) {
	if secret != nil {
		t.Secrets[secret.Name] = secret
	}
}

func (t UnitTestClientDi) AddClusterBomSync(sync *hubv1.ClusterBomSync) {
	if sync != nil {
		t.ClusterBomSyncs[sync.Name] = sync
	}
}

func (t UnitTestClientDi) ListUncached(ctx context.Context, list runtime.Object, opts ...client.ListOption) error {
	return t.List(ctx, list, opts...)
}

func (t UnitTestClientDi) List(ctx context.Context, list runtime.Object, opts ...client.ListOption) error {
	switch typedList := list.(type) {
	case *v1alpha1.DeployItemList:
		for _, hd := range t.DeployItems {
			typedList.Items = append(typedList.Items, *hd)
		}
		return nil

	case *v1alpha1.InstallationList:
		for _, in := range t.Installations {
			typedList.Items = append(typedList.Items, *in)
		}
		return nil

	case *hubv1.ClusterBomList:
		for _, cb := range t.ClusterBoms {
			typedList.Items = append(typedList.Items, *cb)
		}
		return nil

	case *corev1.SecretList:
		for _, s := range t.Secrets {
			typedList.Items = append(typedList.Items, *s)
		}
		return nil

	default:
		return errors2.NewNotFound(schema.GroupResource{}, "CLIENT")
	}
}

func (t UnitTestClientDi) GetUncached(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
	return t.Get(ctx, key, obj)
}

func (t UnitTestClientDi) Get(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
	switch typedObj := obj.(type) {
	case *hubv1.ClusterBom:
		clusterBom := t.ClusterBoms[key.Name]
		if clusterBom == nil {
			return errors2.NewNotFound(schema.GroupResource{}, "CLIENT")
		}
		clusterBom.DeepCopyInto(typedObj)
		return nil

	case *hubv1.ClusterBomSync:
		clusterBomSync := t.ClusterBomSyncs[key.Name]
		if clusterBomSync == nil {
			return errors2.NewNotFound(schema.GroupResource{}, "CLIENT")
		}
		clusterBomSync.DeepCopyInto(typedObj)
		return nil

	case *v1alpha1.DeployItem:
		deployItem := t.DeployItems[key.Name]
		if deployItem == nil {
			return errors2.NewNotFound(schema.GroupResource{}, "CLIENT")
		}
		deployItem.DeepCopyInto(typedObj)
		return nil

	case *v1alpha1.Installation:
		installation := t.Installations[key.Name]
		if installation == nil {
			return errors2.NewNotFound(schema.GroupResource{}, "CLIENT")
		}
		installation.DeepCopyInto(typedObj)
		return nil

	case *corev1.Secret:
		secret := t.Secrets[key.Name]
		if secret == nil {
			return errors2.NewNotFound(schema.GroupResource{}, "CLIENT")
		}
		secret.DeepCopyInto(typedObj)
		return nil

	default:
		return errors2.NewNotFound(schema.GroupResource{}, "CLIENT")
	}
}

func (t UnitTestClientDi) Create(ctx context.Context, obj runtime.Object, opts ...client.CreateOption) error {
	switch typedObj := obj.(type) {
	case *hubv1.ClusterBom:
		key := typedObj.Name

		if _, ok := t.ClusterBoms[key]; ok {
			return errors.New("FAKE CLIENT - could not create object")
		}

		typedObj.Status = hubv1.ClusterBomStatus{}

		t.ClusterBoms[key] = typedObj
		return nil

	case *hubv1.ClusterBomSync:
		key := typedObj.Name

		if _, ok := t.ClusterBomSyncs[key]; ok {
			return errors.New("FAKE CLIENT - could not create object")
		}

		typedObj.Status = hubv1.ClusterBomSyncStatus{}

		t.ClusterBomSyncs[key] = typedObj
		return nil

	case *v1alpha1.DeployItem:
		key := typedObj.Name

		if _, ok := t.DeployItems[key]; ok {
			return errors.New("FAKE CLIENT - could not create object")
		}

		typedObj.Status = v1alpha1.DeployItemStatus{}

		t.DeployItems[key] = typedObj
		return nil

	case *v1alpha1.Installation:
		key := typedObj.Name

		if _, ok := t.DeployItems[key]; ok {
			return errors.New("FAKE CLIENT - could not create object")
		}

		typedObj.Status = v1alpha1.InstallationStatus{}

		t.Installations[key] = typedObj
		return nil

	default:
		return errors.New("FAKE CLIENT - could not create object")
	}
}

func (t UnitTestClientDi) Delete(ctx context.Context, obj runtime.Object, opts ...client.DeleteOption) error {
	switch typedObj := obj.(type) {
	case *hubv1.ClusterBom:
		delete(t.ClusterBoms, typedObj.Name)
		return nil

	case *hubv1.ClusterBomSync:
		delete(t.ClusterBomSyncs, typedObj.Name)
		return nil

	case *v1alpha1.DeployItem:
		delete(t.DeployItems, typedObj.Name)
		return nil

	case *v1alpha1.Installation:
		delete(t.Installations, typedObj.Name)
		return nil

	default:
		return errors.New("FAKE CLIENT - could not delete object")
	}
}

func (t UnitTestClientDi) Update(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
	switch typedObj := obj.(type) {
	case *hubv1.ClusterBom:
		key := typedObj.Name

		oldClusterBom, ok := t.ClusterBoms[key]
		if !ok {
			return errors.New("FAKE CLIENT - could not update object")
		}

		typedObj.Status = oldClusterBom.Status

		t.ClusterBoms[key] = typedObj
		return nil

	case *hubv1.ClusterBomSync:
		key := typedObj.Name

		oldClusterBomSync, ok := t.ClusterBomSyncs[key]
		if !ok {
			return errors.New("FAKE CLIENT - could not update object")
		}

		typedObj.Status = oldClusterBomSync.Status

		t.ClusterBomSyncs[key] = typedObj
		return nil

	case *v1alpha1.DeployItem:
		key := typedObj.Name

		oldDeploymentConfig, ok := t.DeployItems[key]
		if !ok {
			return errors.New("FAKE CLIENT - could not update object")
		}

		typedObj.Status = oldDeploymentConfig.Status

		t.DeployItems[key] = typedObj
		return nil

	case *v1alpha1.Installation:
		key := typedObj.Name

		oldDeploymentConfig, ok := t.Installations[key]
		if !ok {
			return errors.New("FAKE CLIENT - could not update object")
		}

		typedObj.Status = oldDeploymentConfig.Status

		t.Installations[key] = typedObj
		return nil

	default:
		return errors.New("FAKE CLIENT - could not update object")
	}
}

func (t UnitTestClientDi) Status() client.StatusWriter {
	return &UnitTestStatusWriterDi{t}
}

// End Unit Test Client

type UnitTestStatusWriterDi struct {
	unitTestClient UnitTestClientDi
}

func (w *UnitTestStatusWriterDi) Update(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
	switch typedObj := obj.(type) {
	case *hubv1.ClusterBom:
		key := typedObj.Name

		oldClusterBom, ok := w.unitTestClient.ClusterBoms[key]
		if !ok {
			return errors.New("FAKE CLIENT - could not update status")
		}

		oldClusterBom.Status = typedObj.Status
		return nil

	case *v1alpha1.DeployItem:
		key := typedObj.Name

		oldDeploymentConfig, ok := w.unitTestClient.DeployItems[key]
		if !ok {
			return errors.New("FAKE CLIENT - could not update status")
		}

		oldDeploymentConfig.Status = typedObj.Status
		return nil

	case *v1alpha1.Installation:
		key := typedObj.Name

		oldDeploymentConfig, ok := w.unitTestClient.Installations[key]
		if !ok {
			return errors.New("FAKE CLIENT - could not update status")
		}

		oldDeploymentConfig.Status = typedObj.Status
		return nil

	default:
		return errors.New("FAKE CLIENT - could not update status")
	}
}

func (w *UnitTestStatusWriterDi) Patch(ctx context.Context, obj runtime.Object, patch client.Patch, opts ...client.PatchOption) error {
	return nil
}

type fakeTypeSpecificDataDi struct {
	FakeString string `json:"fakeString"`
}

func FakeRawExtensionSampleDi() *runtime.RawExtension {
	return FakeRawExtensionWithProperty("test-type-specific-1")
}

func FakeRawExtensionWithPropertyDi(name string) *runtime.RawExtension {
	fake := fakeTypeSpecificDataDi{FakeString: name}
	rawData, err := json.Marshal(fake)
	if err != nil {
		return nil
	}

	object := runtime.RawExtension{Raw: rawData}
	return &object
}

// UnitTestListErrorClient is a UnitTestClient whose List method fails.
type UnitTestListErrorClientDi struct {
	UnitTestClientDi
}

func NewUnitTestListErrorClientDi() *UnitTestListErrorClientDi {
	cli := NewUnitTestClientDi()
	return &UnitTestListErrorClientDi{*cli}
}

func (t UnitTestListErrorClientDi) List(ctx context.Context, list runtime.Object, opts ...client.ListOption) error {
	return errors.New("dummy error")
}

// UnitTestGetErrorClient is a UnitTestClient whose Get method fails.
type UnitTestGetErrorClientDi struct {
	UnitTestClientDi
}

func NewUnitTestGetErrorClientDi() *UnitTestGetErrorClientDi {
	cli := NewUnitTestClientDi()
	return &UnitTestGetErrorClientDi{*cli}
}

func (t UnitTestGetErrorClientDi) Get(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
	return errors.New("dummy error")
}

// UnitTestDeleteErrorClient is a UnitTestClient whose Delete method fails.
type UnitTestDeleteErrorClientDi struct {
	UnitTestClientDi
}

func NewUnitTestDeleteErrorClientDi() *UnitTestDeleteErrorClientDi {
	cli := NewUnitTestClientDi()
	return &UnitTestDeleteErrorClientDi{*cli}
}

func (t UnitTestDeleteErrorClientDi) Delete(context.Context, runtime.Object, ...client.DeleteOption) error {
	return errors.New("dummy error")
}

// UnitTestStatusErrorWriter is a StatusWriter whose Update method fails.
type UnitTestStatusErrorWriterDi struct {
}

func (w *UnitTestStatusErrorWriterDi) Update(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
	return errors.New("dummy error")
}

func (w *UnitTestStatusErrorWriterDi) Patch(ctx context.Context, obj runtime.Object, patch client.Patch, opts ...client.PatchOption) error {
	return nil
}

// UnitTestStatusErrorClient is a UnitTestClient whose Status method return a UnitTestStatusErrorWriter.
type UnitTestStatusErrorClientDi struct {
	UnitTestClientDi
}

func NewUnitTestStatusErrorClientDi() *UnitTestStatusErrorClientDi {
	cli := NewUnitTestClientDi()
	return &UnitTestStatusErrorClientDi{*cli}
}

func (t UnitTestStatusErrorClientDi) Status() client.StatusWriter {
	return &UnitTestStatusErrorWriterDi{}
}

type HubControllerTestClientDi struct {
	TestClientDi
	ReconcileMap *corev1.ConfigMap
}

func (t HubControllerTestClientDi) Create(ctx context.Context, obj runtime.Object, options ...client.CreateOption) error {
	switch typedObj := obj.(type) {
	case *corev1.ConfigMap:
		if t.ReconcileMap != nil {
			return errors.New("FAKE CLIENT - reconcilemap does already exist")
		}

		t.ReconcileMap = typedObj
	default:
		return errors.New("FAKE CLIENT - unsupported type")
	}
	return nil
}

func (t HubControllerTestClientDi) Update(ctx context.Context, obj runtime.Object, options ...client.UpdateOption) error {
	switch typedObj := obj.(type) {
	case *corev1.ConfigMap:
		if t.ReconcileMap == nil {
			return errors.New("FAKE CLIENT - reconcilemap does not exist")
		}

		t.ReconcileMap = typedObj
	default:
		return errors.New("FAKE CLIENT - unsupported type")
	}
	return nil
}

func (t HubControllerTestClientDi) GetUncached(ctx context.Context, cli client.ObjectKey, obj runtime.Object) error {
	return t.Get(ctx, cli, obj)
}

func (t HubControllerTestClientDi) ListUncached(ctx context.Context, list runtime.Object, opts ...client.ListOption) error {
	return t.List(ctx, list, opts...)
}

func (t HubControllerTestClientDi) Get(ctx context.Context, cli client.ObjectKey, obj runtime.Object) error {
	switch typedObj := obj.(type) {
	case *corev1.ConfigMap:
		t.ReconcileMap.DeepCopyInto(typedObj)
	default:
		return errors.New("FAKE CLIENT - unsupported type")
	}
	return nil
}

type FakeReconcileClockDi struct {
	Time time.Time
}

func (c *FakeReconcileClockDi) Now() time.Time {
	return c.Time
}

func (c *FakeReconcileClockDi) Sleep(d time.Duration) {
}

type fakeTypeSpecificData struct {
	FakeString string `json:"fakeString"`
}

func FakeRawExtensionWithProperty(name string) *runtime.RawExtension {
	fake := fakeTypeSpecificData{FakeString: name}
	rawData, err := json.Marshal(fake)
	if err != nil {
		return nil
	}

	object := runtime.RawExtension{Raw: rawData}
	return &object
}
