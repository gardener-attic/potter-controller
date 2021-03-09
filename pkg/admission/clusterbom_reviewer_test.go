package admission

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gardener/potter-controller/api/apitypes"
	hubv1 "github.com/gardener/potter-controller/api/v1"
	"github.com/gardener/potter-controller/pkg/util"

	"github.com/arschles/assert"
	"k8s.io/api/admission/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TestValidClusterBom tests that the reviewer accepts a valid clusterbom.
func TestValidClusterBom(t *testing.T) {
	clusterBom := clusterBom01(t)
	reviewer := buildReviewerFromClusterBom(t, &clusterBom)
	responseReview := reviewer.review()
	if !responseReview.Response.Allowed {
		t.Error("clusterbom was rejected although it is valid: " + responseReview.Response.Result.Message)
	}
}

// TestNameEmpty tests that the reviewer rejects a clusterbom if its name is empty.
func TestNameEmpty(t *testing.T) {
	clusterBom := clusterBom01(t)
	clusterBom.ObjectMeta.Name = ""
	reviewer := buildReviewerFromClusterBom(t, &clusterBom)
	responseReview := reviewer.review()
	if responseReview.Response.Allowed {
		t.Error("clusterbom was accepted although its name is empty")
	}
}

// TestNameTooLong tests that the reviewer rejects a clusterbom if its name is too long, i.e. longer than 63 characters.
func TestNameTooLong(t *testing.T) {
	clusterBom := clusterBom01(t)
	clusterBom.ObjectMeta.Name = "testclusterbom567890testclusterbom567890testclusterbom5678901234"
	reviewer := buildReviewerFromClusterBom(t, &clusterBom)
	responseReview := reviewer.review()
	if responseReview.Response.Allowed {
		t.Error("clusterbom was accepted although its name is too long")
	}
}

// TestNameOk tests that the reviewer accepts a clusterbom with valid name
func TestNameOk(t *testing.T) {
	clusterBom := clusterBom01(t)
	clusterBom.ObjectMeta.Name = "testclusterbom................123"
	reviewer := buildReviewerFromClusterBom(t, &clusterBom)
	responseReview := reviewer.review()
	if !responseReview.Response.Allowed {
		t.Error("clusterbom was denied although its name is is valid")
	}
}

// TestNamePatternViolated tests that the reviewer rejects a clusterbom if its name violates a certain pattern.
func TestNamePatternViolated(t *testing.T) {
	clusterBom := clusterBom01(t)
	clusterBom.ObjectMeta.Name = "test&cluster-bom-01"
	reviewer := buildReviewerFromClusterBom(t, &clusterBom)
	responseReview := reviewer.review()
	if responseReview.Response.Allowed {
		t.Error("clusterbom was accepted although the name pattern is violated")
	}
}

// TestSecretRefEmpty tests that the reviewer rejects a clusterbom if its secretRef is empty
func TestSecretRefEmpty(t *testing.T) {
	clusterBom := clusterBom01(t)
	clusterBom.Spec.SecretRef = ""
	reviewer := buildReviewerFromClusterBom(t, &clusterBom)
	responseReview := reviewer.review()
	if responseReview.Response.Allowed {
		t.Error("clusterbom was accepted although its secretRef is empty")
	}
}

// TestApplConfigIDEmpty tests that the reviewer rejects a clusterbom if the ID of an applconfig is empty.
func TestApplConfigIDEmpty(t *testing.T) {
	clusterBom := clusterBom01(t)
	clusterBom.Spec.ApplicationConfigs[0].ID = ""
	reviewer := buildReviewerFromClusterBom(t, &clusterBom)
	responseReview := reviewer.review()
	if responseReview.Response.Allowed {
		t.Error("clusterbom was accepted although the id of an applconfig is empty")
	}
}

// TestApplConfigTypeEmpty tests that the reviewer rejects a clusterbom if the configtype of an applconfig is empty.
func TestApplConfigTypeEmpty(t *testing.T) {
	clusterBom := clusterBom01(t)
	clusterBom.Spec.ApplicationConfigs[0].ConfigType = ""
	reviewer := buildReviewerFromClusterBom(t, &clusterBom)
	responseReview := reviewer.review()
	if responseReview.Response.Allowed {
		t.Error("clusterbom was accepted although the configtype of an applconfig is empty")
	}
}

// TestApplConfigTypeNotSupported tests that the reviewer rejects a clusterbom if the configtype is not supported.
func TestApplConfigTypeNotSupported(t *testing.T) {
	clusterBom := clusterBom01(t)
	clusterBom.Spec.ApplicationConfigs[0].ConfigType = "x"
	reviewer := buildReviewerFromClusterBom(t, &clusterBom)
	responseReview := reviewer.review()
	if responseReview.Response.Allowed {
		t.Error("clusterbom was accepted although the configtype of an applconfig is not supported")
	}
}

// TestApplConfigDuplicateID tests that the reviewer rejects a clusterbom if two applconfigs have the same ID.
func TestApplConfigDuplicateID(t *testing.T) {
	clusterBom := clusterBom01(t)
	clusterBom.Spec.ApplicationConfigs[1].ID = clusterBom.Spec.ApplicationConfigs[0].ID
	reviewer := buildReviewerFromClusterBom(t, &clusterBom)
	responseReview := reviewer.review()
	if responseReview.Response.Allowed {
		t.Error("clusterbom was accepted although two applconfigs have the same id")
	}
}

// TestHelmWithNeitherCatalogNorTarballAccess tests that the reviewer rejects a clusterbom if the helm specific data
// contain neither catalog nor tarball access.
func TestHelmWithNeitherCatalogNorTarballAccess(t *testing.T) {
	clusterBom := clusterBom01(t)
	clusterBom.Spec.ApplicationConfigs[0].TypeSpecificData = buildRawExtension(t, apitypes.HelmSpecificData{
		Namespace:   "testnamespace01",
		InstallName: "testinstallname01",
	})
	reviewer := buildReviewerFromClusterBom(t, &clusterBom)
	responseReview := reviewer.review()
	if responseReview.Response.Allowed {
		t.Error("clusterbom was accepted although it contains helm specific data with neither catalog not tarball access")
	}
}

// TestHelmWithCatalogAndTarballAccess tests that the reviewer rejects a clusterbom if the helm specific data contain
// catalog as well as tarball access.
func TestHelmWithCatalogAndTarballAccess(t *testing.T) {
	clusterBom := clusterBom01(t)
	clusterBom.Spec.ApplicationConfigs[0].TypeSpecificData = buildRawExtension(t, apitypes.HelmSpecificData{
		Namespace:   "testnamespace01",
		InstallName: "testinstallname01",
		CatalogAccess: &apitypes.CatalogAccess{
			ChartName: "testchart01",
		},
		TarballAccess: &apitypes.TarballAccess{
			URL: "testurl01",
		},
	})
	reviewer := buildReviewerFromClusterBom(t, &clusterBom)
	responseReview := reviewer.review()
	if responseReview.Response.Allowed {
		t.Error("clusterbom was accepted although it contains helm specific data with catalog and tarball access")
	}
}

// TestHelmNamespaceEmpty tests that the reviewer rejects a clusterbom if the helm specific data contain no namespace.
func TestHelmNamespaceEmpty(t *testing.T) {
	clusterBom := clusterBom01(t)
	clusterBom.Spec.ApplicationConfigs[0].TypeSpecificData = buildRawExtension(t, apitypes.HelmSpecificData{
		Namespace:   "",
		InstallName: "testinstallname01",
		CatalogAccess: &apitypes.CatalogAccess{
			Repo:         "testrepo01",
			ChartName:    "testchart01",
			ChartVersion: "1.2.3",
		},
	})
	reviewer := buildReviewerFromClusterBom(t, &clusterBom)
	responseReview := reviewer.review()
	if responseReview.Response.Allowed {
		t.Error("clusterbom was accepted although it contains helm specific data with empty namespace")
	}
}

// TestHelmInstallNameEmpty tests that the reviewer rejects a clusterbom if the helm specific data contain no
// install name.
func TestHelmInstallNameEmpty(t *testing.T) {
	clusterBom := clusterBom01(t)
	clusterBom.Spec.ApplicationConfigs[0].TypeSpecificData = buildRawExtension(t, apitypes.HelmSpecificData{
		Namespace:   "testnamespace01",
		InstallName: "",
		CatalogAccess: &apitypes.CatalogAccess{
			Repo:         "testrepo01",
			ChartName:    "testchart01",
			ChartVersion: "1.2.3",
		},
	})
	reviewer := buildReviewerFromClusterBom(t, &clusterBom)
	responseReview := reviewer.review()
	if responseReview.Response.Allowed {
		t.Error("clusterbom was accepted although it contains helm specific data with empty namespace")
	}
}

func TestResourceReadyRequirements(t *testing.T) {
	tests := []struct {
		name                      string
		resourceReadyRequirements []hubv1.Resource
		errorMsg                  string
	}{
		{
			name: "everything valid",
			resourceReadyRequirements: []hubv1.Resource{
				{
					Name:       "my-resource",
					Namespace:  "my-namespace",
					APIVersion: "v1",
					Resource:   "secrets",
					FieldPath:  "{ .apiVersion }",
					SuccessValues: []runtime.RawExtension{
						buildRawExtension(t, map[string]interface{}{
							"value": "v1",
						}),
					},
				},
			},
		},
		{
			name: "name is empty",
			resourceReadyRequirements: []hubv1.Resource{
				{
					Namespace:  "my-namespace",
					APIVersion: "v1",
					Resource:   "secrets",
					FieldPath:  "{ .apiVersion }",
					SuccessValues: []runtime.RawExtension{
						buildRawExtension(t, map[string]interface{}{
							"value": "v1",
						}),
					},
				},
			},
			errorMsg: "id01.readyRequirements.resources[0].name is empty",
		},
		{
			name: "namespace is empty",
			resourceReadyRequirements: []hubv1.Resource{
				{
					Name:       "my-resource",
					APIVersion: "v1",
					Resource:   "secrets",
					FieldPath:  "{ .apiVersion }",
					SuccessValues: []runtime.RawExtension{
						buildRawExtension(t, map[string]interface{}{
							"value": "v1",
						}),
					},
				},
			},
			errorMsg: "id01.readyRequirements.resources[0].namespace is empty",
		},
		{
			name: "apiVersion is empty",
			resourceReadyRequirements: []hubv1.Resource{
				{
					Name:      "my-resource",
					Namespace: "my-namespace",
					Resource:  "secrets",
					FieldPath: "{ .apiVersion }",
					SuccessValues: []runtime.RawExtension{
						buildRawExtension(t, map[string]interface{}{
							"value": "v1",
						}),
					},
				},
			},
			errorMsg: "id01.readyRequirements.resources[0].apiVersion is empty",
		},
		{
			name: "resource is empty",
			resourceReadyRequirements: []hubv1.Resource{
				{
					Name:       "my-resource",
					Namespace:  "my-namespace",
					APIVersion: "v1",
					FieldPath:  "{ .apiVersion }",
					SuccessValues: []runtime.RawExtension{
						buildRawExtension(t, map[string]interface{}{
							"value": "v1",
						}),
					},
				},
			},
			errorMsg: "id01.readyRequirements.resources[0].resource is empty",
		},
		{
			name: "fieldPath is empty",
			resourceReadyRequirements: []hubv1.Resource{
				{
					Name:       "my-resource",
					Namespace:  "my-namespace",
					APIVersion: "v1",
					Resource:   "secrets",
					SuccessValues: []runtime.RawExtension{
						buildRawExtension(t, map[string]interface{}{
							"value": "v1",
						}),
					},
				},
			},
			errorMsg: "id01.readyRequirements.resources[0].fieldPath is empty",
		},
		{
			name: "invalid fieldPath",
			resourceReadyRequirements: []hubv1.Resource{
				{
					Name:       "my-resource",
					Namespace:  "my-namespace",
					APIVersion: "v1",
					Resource:   "secrets",
					FieldPath:  `{ \12345 }`,
					SuccessValues: []runtime.RawExtension{
						buildRawExtension(t, map[string]interface{}{
							"value": "v1",
						}),
					},
				},
			},
			errorMsg: "id01.readyRequirements.resources[0].fieldPath cannot be parsed:",
		},
		{
			name: "successValues is empty",
			resourceReadyRequirements: []hubv1.Resource{
				{
					Name:          "my-resource",
					Namespace:     "my-namespace",
					APIVersion:    "v1",
					Resource:      "secrets",
					FieldPath:     "{ .apiVersion }",
					SuccessValues: []runtime.RawExtension{},
				},
			},
			errorMsg: "id01.readyRequirements.resources[0].successValues is empty",
		},
		{
			name: "successValue is not an object",
			resourceReadyRequirements: []hubv1.Resource{
				{
					Name:       "my-resource",
					Namespace:  "my-namespace",
					APIVersion: "v1",
					Resource:   "secrets",
					FieldPath:  "{ .apiVersion }",
					SuccessValues: []runtime.RawExtension{
						buildRawExtension(t, "this is just a simple string"),
					},
				},
			},
			errorMsg: "id01.readyRequirements.resources[0].successValues cannot be parsed:",
		},
		{
			name: "successValue does not contain value key",
			resourceReadyRequirements: []hubv1.Resource{
				{
					Name:       "my-resource",
					Namespace:  "my-namespace",
					APIVersion: "v1",
					Resource:   "secrets",
					FieldPath:  "{ .apiVersion }",
					SuccessValues: []runtime.RawExtension{
						buildRawExtension(t, map[string]interface{}{
							"invalid-key": "v1",
						}),
					},
				},
			},
			errorMsg: "id01.readyRequirements.resources[0].successValues cannot be parsed:",
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			clusterBom := clusterBom01(t)
			clusterBom.Spec.ApplicationConfigs[0].ReadyRequirements.Resources = tt.resourceReadyRequirements
			reviewer := buildReviewerFromClusterBom(t, &clusterBom)
			responseReview := reviewer.review()

			assert.Equal(t, responseReview.Response.Allowed, tt.errorMsg == "", "isValid")
			if tt.errorMsg != "" {
				assert.True(t, strings.Contains(responseReview.Response.Result.Message, tt.errorMsg), "errorMsg")
			}
		})
	}
}

// TestLandscaperEnabled checks that a clusterbom with landscaper-managed annotation is only accepted if the config
// parameter landscaperEnabled is true.
func TestLandscaperEnabled(t *testing.T) {
	tests := []struct {
		name              string
		landscaperManaged bool
		landscaperEnabled bool
		allowed           bool
	}{
		{
			name:              "landscaper-managed-tt",
			landscaperManaged: true,
			landscaperEnabled: true,
			allowed:           true,
		},
		{
			name:              "landscaper-managed-tf",
			landscaperManaged: true,
			landscaperEnabled: false,
			allowed:           false,
		},
		{
			name:              "landscaper-managed-ft",
			landscaperManaged: false,
			landscaperEnabled: true,
			allowed:           true,
		},
		{
			name:              "landscaper-managed-ff",
			landscaperManaged: false,
			landscaperEnabled: false,
			allowed:           true,
		},
	}

	for i := range tests {
		test := &tests[i]
		t.Run(test.name, func(t *testing.T) {
			clusterBom := clusterBom01(t)
			if test.landscaperManaged {
				util.AddAnnotation(&clusterBom, hubv1.AnnotationKeyLandscaperManaged, hubv1.AnnotationValueLandscaperManaged)
			}
			reviewer := buildReviewerFromClusterBom(t, &clusterBom)
			reviewer.landscaperEnabled = test.landscaperEnabled
			responseReview := reviewer.review()
			if responseReview.Response.Allowed && !test.allowed {
				t.Error("clusterbom was accepted although it should have been rejected according to the landscaper-enabled setting")
			} else if !responseReview.Response.Allowed && test.allowed {
				t.Error("clusterbom was rejected although it should have been accepted according to the landscaper-enabled setting")
			}
		})
	}
}

func TestSwitchLandscaperManaged(t *testing.T) {
	tests := []struct {
		name                 string
		isLandscaperManaged  bool
		wasLandscaperManaged bool
		isUpdate             bool
		allowed              bool
	}{
		{
			name:                 "switch-landscaper-managed-t",
			isLandscaperManaged:  true,
			wasLandscaperManaged: false,
			isUpdate:             false,
			allowed:              true,
		},
		{
			name:                 "switch-landscaper-managed-f",
			isLandscaperManaged:  false,
			wasLandscaperManaged: false,
			isUpdate:             false,
			allowed:              true,
		},
		{
			name:                 "switch-landscaper-managed-tt",
			isLandscaperManaged:  true,
			wasLandscaperManaged: true,
			isUpdate:             true,
			allowed:              true,
		},
		{
			name:                 "switch-landscaper-managed-tf",
			isLandscaperManaged:  true,
			wasLandscaperManaged: false,
			isUpdate:             true,
			allowed:              false,
		},
		{
			name:                 "switch-landscaper-managed-ft",
			isLandscaperManaged:  false,
			wasLandscaperManaged: true,
			isUpdate:             true,
			allowed:              false,
		},
		{
			name:                 "switch-landscaper-managed-ff",
			isLandscaperManaged:  false,
			wasLandscaperManaged: false,
			isUpdate:             true,
			allowed:              true,
		},
	}

	for i := range tests {
		test := &tests[i]
		t.Run(test.name, func(t *testing.T) {
			clusterBom := clusterBom01(t)
			if test.isLandscaperManaged {
				util.AddAnnotation(&clusterBom, hubv1.AnnotationKeyLandscaperManaged, hubv1.AnnotationValueLandscaperManaged)
			}

			var reviewer *clusterBomReviewer
			if test.isUpdate {
				oldClusterBom := clusterBom01(t)
				if test.wasLandscaperManaged {
					util.AddAnnotation(&oldClusterBom, hubv1.AnnotationKeyLandscaperManaged, hubv1.AnnotationValueLandscaperManaged)
				}
				reviewer = buildReviewerForClusterBomUpdate(t, &clusterBom, &oldClusterBom)
				reviewer.landscaperEnabled = true
			} else {
				reviewer = buildReviewerFromClusterBom(t, &clusterBom)
				reviewer.landscaperEnabled = true
			}

			responseReview := reviewer.review()
			if responseReview.Response.Allowed && !test.allowed {
				t.Error("clusterbom was accepted although it should have been rejected, because the landscaper-managed annotation was switched")
			} else if !responseReview.Response.Allowed && test.allowed {
				t.Error("clusterbom was rejected although it should have been accepted")
			}
		})
	}
}

type readerMock struct {
	existingKeys []client.ObjectKey
}

func (r *readerMock) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	return nil
}

func (r *readerMock) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	return nil
}

func (r *readerMock) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	return nil
}

func (r *readerMock) Get(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	for _, existingKey := range r.existingKeys {
		if existingKey == key {
			return nil
		}
	}
	return errors.NewNotFound(schema.GroupResource{}, "")
}

func (r *readerMock) GetUncached(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	for _, existingKey := range r.existingKeys {
		if existingKey == key {
			return nil
		}
	}
	return errors.NewNotFound(schema.GroupResource{}, "")
}

func (r *readerMock) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return nil
}

func (r *readerMock) ListUncached(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return nil
}

func buildReviewerFromClusterBom(t *testing.T, clusterBom *hubv1.ClusterBom) *clusterBomReviewer {
	return &clusterBomReviewer{
		log:    ctrl.Log.WithName("ClusterBom Admission Hook Test"),
		reader: &readerMock{},
		requestReview: &v1beta1.AdmissionReview{
			Request: &v1beta1.AdmissionRequest{
				Object: buildRawExtension(t, *clusterBom),
			},
		},
		configTypes: []string{util.ConfigTypeHelm},
	}
}

func buildReviewerForClusterBomUpdate(t *testing.T, clusterBom, oldClusterBom *hubv1.ClusterBom) *clusterBomReviewer {
	return &clusterBomReviewer{
		log:    ctrl.Log.WithName("ClusterBom Admission Hook Test"),
		reader: &readerMock{},
		requestReview: &v1beta1.AdmissionReview{
			Request: &v1beta1.AdmissionRequest{
				Operation: v1beta1.Update,
				Object:    buildRawExtension(t, *clusterBom),
				OldObject: buildRawExtension(t, *oldClusterBom),
			},
		},
		configTypes: []string{util.ConfigTypeHelm},
	}
}

func buildRawExtension(t *testing.T, data interface{}) runtime.RawExtension {
	rawData, err := json.Marshal(data)
	if err != nil {
		t.Error(err, "marshaling of data to raw extension failed")
	}
	return runtime.RawExtension{
		Raw: rawData,
	}
}

func clusterBom01(t *testing.T) hubv1.ClusterBom {
	return hubv1.ClusterBom{
		ObjectMeta: metav1.ObjectMeta{
			Name: "testclusterbom01",
		},
		Spec: hubv1.ClusterBomSpec{
			SecretRef: "testsecret01",
			ApplicationConfigs: []hubv1.ApplicationConfig{
				{
					ID:         "id01",
					ConfigType: util.ConfigTypeHelm,
					TypeSpecificData: buildRawExtension(t, apitypes.HelmSpecificData{
						Namespace:   "testnamespace01",
						InstallName: "testinstallname01",
						CatalogAccess: &apitypes.CatalogAccess{
							Repo:         "testrepo01",
							ChartName:    "testchartname01",
							ChartVersion: "1.2.3",
						},
					}),
				},
				{
					ID:         "id02",
					ConfigType: util.ConfigTypeHelm,
					TypeSpecificData: buildRawExtension(t, apitypes.HelmSpecificData{
						Namespace:   "testnamespace02",
						InstallName: "testinstallname02",
						TarballAccess: &apitypes.TarballAccess{
							URL: "testurl02",
						},
					}),
				},
			},
		},
	}
}
