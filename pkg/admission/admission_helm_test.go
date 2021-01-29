package admission

import (
	"testing"

	"github.wdf.sap.corp/kubernetes/hub-controller/api/apitypes"

	"github.com/arschles/assert"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestInvalidHelmSpecificData(t *testing.T) {
	report := newReport(nil)

	typeSpecificData := &runtime.RawExtension{
		Raw: []byte{4},
	}

	newHelmReviewer().reviewTypeSpecificData(report, typeSpecificData, nil)
	assert.Equal(t, report.denied(), true, "denied")
}

func TestInvalidOldHelmSpecificData(t *testing.T) {
	report := newReport(nil)

	typeSpecificData, err := raw(&apitypes.HelmSpecificData{
		InstallName: "test",
		Namespace:   "test",
		TarballAccess: &apitypes.TarballAccess{
			URL: "test",
		},
	})
	assert.Nil(t, err, "error building type specific data")

	oldTypeSpecificData := &runtime.RawExtension{
		Raw: []byte{4},
	}

	newHelmReviewer().reviewTypeSpecificData(report, typeSpecificData, oldTypeSpecificData)
	assert.Equal(t, report.denied(), true, "denied")
}

func TestReviewHelmSpecificData(t *testing.T) {
	var timeout int64 = 2
	var negativeTimeout int64 = -2

	tests := []struct {
		name           string
		helmData       *apitypes.HelmSpecificData
		expectedDenied bool
	}{
		{
			name: "allow valid helm data",
			helmData: &apitypes.HelmSpecificData{
				InstallName: "test",
				Namespace:   "test",
				CatalogAccess: &apitypes.CatalogAccess{
					Repo:         "test",
					ChartName:    "test",
					ChartVersion: "1.2.3",
				},
				InstallTimeout: &timeout,
				UpgradeTimeout: &timeout,
			},
			expectedDenied: false,
		},
		{
			name: "reject empty install name",
			helmData: &apitypes.HelmSpecificData{
				InstallName: "",
				Namespace:   "test",
				TarballAccess: &apitypes.TarballAccess{
					URL: "test",
				},
			},
			expectedDenied: true,
		},
		{
			name: "reject empty namespace",
			helmData: &apitypes.HelmSpecificData{
				InstallName: "test",
				Namespace:   "",
				TarballAccess: &apitypes.TarballAccess{
					URL: "test",
				},
			},
			expectedDenied: true,
		},
		{
			name: "reject neither catalog nor tarball access",
			helmData: &apitypes.HelmSpecificData{
				InstallName: "test",
				Namespace:   "test",
			},
			expectedDenied: true,
		},
		{
			name: "reject catalog and tarball access",
			helmData: &apitypes.HelmSpecificData{
				InstallName: "test",
				Namespace:   "test",
				CatalogAccess: &apitypes.CatalogAccess{
					Repo: "test",
				},
				TarballAccess: &apitypes.TarballAccess{
					URL: "test",
				},
			},
			expectedDenied: true,
		},
		{
			name: "reject missing tarball url",
			helmData: &apitypes.HelmSpecificData{
				InstallName:   "test",
				Namespace:     "test",
				TarballAccess: &apitypes.TarballAccess{},
			},
			expectedDenied: true,
		},
		{
			name: "reject missing repo",
			helmData: &apitypes.HelmSpecificData{
				InstallName: "test",
				Namespace:   "test",
				CatalogAccess: &apitypes.CatalogAccess{
					Repo:         "",
					ChartName:    "test",
					ChartVersion: "1.2.3",
				},
			},
			expectedDenied: true,
		},
		{
			name: "reject missing chart name",
			helmData: &apitypes.HelmSpecificData{
				InstallName: "test",
				Namespace:   "test",
				CatalogAccess: &apitypes.CatalogAccess{
					Repo:         "test",
					ChartName:    "",
					ChartVersion: "1.2.3",
				},
			},
			expectedDenied: true,
		},
		{
			name: "reject missing chart version",
			helmData: &apitypes.HelmSpecificData{
				InstallName: "test",
				Namespace:   "test",
				CatalogAccess: &apitypes.CatalogAccess{
					Repo:         "test",
					ChartName:    "test",
					ChartVersion: "",
				},
			},
			expectedDenied: true,
		},
		{
			name: "reject invalid chart version",
			helmData: &apitypes.HelmSpecificData{
				InstallName: "test",
				Namespace:   "test",
				CatalogAccess: &apitypes.CatalogAccess{
					Repo:         "test",
					ChartName:    "test",
					ChartVersion: "invalid",
				},
			},
			expectedDenied: true,
		},
		{
			name: "reject negative timeout",
			helmData: &apitypes.HelmSpecificData{
				InstallName: "test",
				Namespace:   "test",
				CatalogAccess: &apitypes.CatalogAccess{
					Repo:         "test",
					ChartName:    "test",
					ChartVersion: "1.2.3",
				},
				UpgradeTimeout: &negativeTimeout,
			},
			expectedDenied: true,
		},
		{
			name: "allow install argument atomic",
			helmData: &apitypes.HelmSpecificData{
				InstallName: "test",
				Namespace:   "test",
				CatalogAccess: &apitypes.CatalogAccess{
					Repo:         "test",
					ChartName:    "test",
					ChartVersion: "1.2.3",
				},
				InstallArguments: []string{"atomic"}, // supported argument "atomic"
			},
			expectedDenied: false,
		},
		{
			name: "reject invalid install argument",
			helmData: &apitypes.HelmSpecificData{
				InstallName: "test",
				Namespace:   "test",
				CatalogAccess: &apitypes.CatalogAccess{
					Repo:         "test",
					ChartName:    "test",
					ChartVersion: "1.2.3",
				},
				InstallArguments: []string{"x"}, // unsupported argument "x"
			},
			expectedDenied: true,
		},
		{
			name: "allow normal auth header",
			helmData: &apitypes.HelmSpecificData{
				InstallName: "test",
				Namespace:   "test",
				TarballAccess: &apitypes.TarballAccess{
					URL:        "test",
					AuthHeader: "Basic abcdefgh",
					SecretRef:  apitypes.SecretRef{},
				},
			},
			expectedDenied: false,
		},
		{
			name: "allow auth header in secret",
			helmData: &apitypes.HelmSpecificData{
				InstallName: "test",
				Namespace:   "test",
				TarballAccess: &apitypes.TarballAccess{
					URL: "test",
					SecretRef: apitypes.SecretRef{
						Name: "test",
					},
				},
			},
			expectedDenied: false,
		},
		{
			name: "reject auth header in secret and normal",
			helmData: &apitypes.HelmSpecificData{
				InstallName: "test",
				Namespace:   "test",
				TarballAccess: &apitypes.TarballAccess{
					URL:        "test",
					AuthHeader: "test",
					SecretRef: apitypes.SecretRef{
						Name: "test",
					},
				},
			},
			expectedDenied: true,
		},
	}

	for i := range tests {
		test := &tests[i]
		t.Run(test.name, func(t *testing.T) {
			report := newReport(nil)

			typeSpecificData, err := raw(test.helmData)
			assert.Nil(t, err, "error building type specific data")

			newHelmReviewer().reviewTypeSpecificData(report, typeSpecificData, nil)
			assert.Equal(t, report.denied(), test.expectedDenied, "denied")
		})
	}
}

func TestReviewHelmSpecificDataUpdate(t *testing.T) {
	tests := []struct {
		name           string
		helmData       *apitypes.HelmSpecificData
		oldHelmData    *apitypes.HelmSpecificData
		expectedDenied bool
	}{
		{
			name: "reject update of install name",
			helmData: &apitypes.HelmSpecificData{
				InstallName: "newInstallName",
				Namespace:   "test",
				CatalogAccess: &apitypes.CatalogAccess{
					Repo:         "test",
					ChartName:    "test",
					ChartVersion: "1.2.3",
				},
			},
			oldHelmData: &apitypes.HelmSpecificData{
				InstallName: "oldInstallName",
				Namespace:   "test",
				CatalogAccess: &apitypes.CatalogAccess{
					Repo:         "test",
					ChartName:    "test",
					ChartVersion: "1.2.3",
				},
			},
			expectedDenied: true,
		},
		{
			name: "reject update of namespace",
			helmData: &apitypes.HelmSpecificData{
				InstallName: "test",
				Namespace:   "newNamespace",
				CatalogAccess: &apitypes.CatalogAccess{
					Repo:         "test",
					ChartName:    "test",
					ChartVersion: "1.2.3",
				},
			},
			oldHelmData: &apitypes.HelmSpecificData{
				InstallName: "test",
				Namespace:   "oldNamespace",
				CatalogAccess: &apitypes.CatalogAccess{
					Repo:         "test",
					ChartName:    "test",
					ChartVersion: "1.2.3",
				},
			},
			expectedDenied: true,
		},
		{
			name: "reject update of chart repo",
			helmData: &apitypes.HelmSpecificData{
				InstallName: "test",
				Namespace:   "test",
				CatalogAccess: &apitypes.CatalogAccess{
					Repo:         "newRepo",
					ChartName:    "test",
					ChartVersion: "1.2.3",
				},
			},
			oldHelmData: &apitypes.HelmSpecificData{
				InstallName: "test",
				Namespace:   "test",
				CatalogAccess: &apitypes.CatalogAccess{
					Repo:         "oldRepo",
					ChartName:    "test",
					ChartVersion: "1.2.3",
				},
			},
			expectedDenied: true,
		},
		{
			name: "reject update of chart name",
			helmData: &apitypes.HelmSpecificData{
				InstallName: "test",
				Namespace:   "test",
				CatalogAccess: &apitypes.CatalogAccess{
					Repo:         "test",
					ChartName:    "newChartName",
					ChartVersion: "1.2.3",
				},
			},
			oldHelmData: &apitypes.HelmSpecificData{
				InstallName: "test",
				Namespace:   "test",
				CatalogAccess: &apitypes.CatalogAccess{
					Repo:         "test",
					ChartName:    "oldChartName",
					ChartVersion: "1.2.3",
				},
			},
			expectedDenied: true,
		},
		{
			name: "reject switch to tarball access",
			helmData: &apitypes.HelmSpecificData{
				InstallName: "test",
				Namespace:   "test",
				TarballAccess: &apitypes.TarballAccess{
					URL: "test",
				},
			},
			oldHelmData: &apitypes.HelmSpecificData{
				InstallName: "test",
				Namespace:   "test",
				CatalogAccess: &apitypes.CatalogAccess{
					Repo:         "test",
					ChartName:    "test",
					ChartVersion: "1.2.3",
				},
			},
			expectedDenied: true,
		},
		{
			name: "reject switch to catalog access",
			helmData: &apitypes.HelmSpecificData{
				InstallName: "test",
				Namespace:   "test",
				CatalogAccess: &apitypes.CatalogAccess{
					Repo:         "test",
					ChartName:    "test",
					ChartVersion: "1.2.3",
				},
			},
			oldHelmData: &apitypes.HelmSpecificData{
				InstallName: "test",
				Namespace:   "test",
				TarballAccess: &apitypes.TarballAccess{
					URL: "test",
				},
			},
			expectedDenied: true,
		},
		{
			name: "allow valid update",
			helmData: &apitypes.HelmSpecificData{
				InstallName: "test",
				Namespace:   "test",
				CatalogAccess: &apitypes.CatalogAccess{
					Repo:         "test",
					ChartName:    "test",
					ChartVersion: "1.2.3",
				},
			},
			oldHelmData: &apitypes.HelmSpecificData{
				InstallName: "test",
				Namespace:   "test",
				CatalogAccess: &apitypes.CatalogAccess{
					Repo:         "test",
					ChartName:    "test",
					ChartVersion: "1.0.0",
				},
			},
			expectedDenied: false,
		},
	}

	for i := range tests {
		test := &tests[i]
		t.Run(test.name, func(t *testing.T) {
			report := newReport(nil)

			typeSpecificData, err := raw(test.helmData)
			assert.Nil(t, err, "error building type specific data")

			oldTypeSpecificData, err := raw(test.oldHelmData)
			assert.Nil(t, err, "error building old type specific data")

			newHelmReviewer().reviewTypeSpecificData(report, typeSpecificData, oldTypeSpecificData)
			assert.Equal(t, report.denied(), test.expectedDenied, "denied")
		})
	}
}
