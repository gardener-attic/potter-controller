package helm

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/gardener/potter-controller/api/apitypes"
	appRepov1 "github.com/gardener/potter-controller/api/external/apprepository/v1alpha1"
	hubv1 "github.com/gardener/potter-controller/api/v1"
	"github.com/gardener/potter-controller/pkg/util"

	. "github.com/arschles/assert"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestIsReleaseNotFoundErr(t *testing.T) {
	tests := []struct {
		name                 string
		err                  error
		isReleaseNotFoundErr bool
	}{
		{
			name:                 "test0",
			err:                  errors.New("Error: RELEASE: not FoUnD"),
			isReleaseNotFoundErr: true,
		},
		{
			name:                 "test1",
			err:                  errors.New("error message"),
			isReleaseNotFoundErr: false,
		},
		{
			name:                 "test2",
			err:                  errors.Wrap(errors.New(""), "release: not found"),
			isReleaseNotFoundErr: true,
		},
		{
			name:                 "test3",
			err:                  errors.New(""),
			isReleaseNotFoundErr: false,
		},
		{
			name:                 "test4",
			err:                  errors.Wrap(errors.Wrap(errors.New(""), "release: not found"), "random msg"),
			isReleaseNotFoundErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			actualIsReleaseNotFoundErr := IsReleaseNotFoundErr(tt.err)
			Equal(t, actualIsReleaseNotFoundErr, tt.isReleaseNotFoundErr, "isReleaseNotFoundErr")
		})
	}
}

func TestParseTypeSpecificData_TarballAccessWithAllFields(t *testing.T) {
	const expectedInstallName = "der-gute-alte-broker"
	const expectedNamespace = "broker-ns"
	const expectedCAData = "dGVzdENBRGF0YQ=="
	expectedValues := map[string]interface{}{
		"key-1": "val-1",
		"key-2": map[string]interface{}{
			"key-3": map[string]interface{}{
				// JSON numbers are automatically converted to float64 by Golang's json.Unmarshal(),
				// see also: https://golang.org/pkg/encoding/json/#Unmarshal
				"key-4": float64(4),
			},
		},
	}

	typeSpecificData := map[string]interface{}{
		"installName": expectedInstallName,
		"namespace":   expectedNamespace,
		"tarballAccess": map[string]interface{}{
			"url":          "https://myrepo.io/service-broker-0.5.0.tgz",
			"authHeader":   "<Insert-correct-auth-here>",
			"customCAData": expectedCAData,
		},
	}

	dc := &hubv1.DeploymentConfig{
		ID:               "1",
		TypeSpecificData: *util.CreateRawExtensionOrPanic(typeSpecificData),
		Values:           util.CreateRawExtensionOrPanic(expectedValues),
	}

	helmSpecificData, err := apitypes.NewHelmSpecificData(&dc.TypeSpecificData)
	Nil(t, err, "err")

	ch, namespace, err := ParseTypeSpecificData(context.TODO(), nil, dc, helmSpecificData, true, nil)

	Nil(t, err, "unexpected error")
	NotNil(t, ch, "chart data must not be nil")
	Equal(t, ch.InstallName, expectedInstallName, "ch.InstallName")
	True(t, reflect.DeepEqual(ch.Values, expectedValues), "ch.Values")
	NotNil(t, ch.Load, "ch.Load")
	Equal(t, namespace, expectedNamespace, "namespace")
}

func TestParseTypeSpecificData_TarballAccessWithUrlOnly(t *testing.T) {
	const expectedInstallName = "der-gute-alte-broker"
	const expectedNamespace = "broker-ns"

	typeSpecificData := map[string]interface{}{
		"installName": expectedInstallName,
		"namespace":   expectedNamespace,
		"tarballAccess": map[string]interface{}{
			"url": "https://myrepo.io/service-broker-0.5.0.tgz",
		},
	}

	dc := &hubv1.DeploymentConfig{
		ID:               "1",
		TypeSpecificData: *util.CreateRawExtensionOrPanic(typeSpecificData),
	}

	helmSpecificData, err := apitypes.NewHelmSpecificData(&dc.TypeSpecificData)
	Nil(t, err, "err")

	ch, namespace, err := ParseTypeSpecificData(context.TODO(), nil, dc, helmSpecificData, true, nil)

	Nil(t, err, "unexpected error")
	NotNil(t, ch, "chart data must not be nil")
	Equal(t, ch.InstallName, expectedInstallName, "ch.InstallName")
	NotNil(t, ch.Load, "ch.Load")
	Equal(t, namespace, expectedNamespace, "namespace")
}

func TestParseTypeSpecificData_CatalogAccess(t *testing.T) {
	const expectedInstallName = "der-gute-alte-broker"
	const expectedNamespace = "broker-ns"
	const catalog = "example-catalog"

	typeSpecificData := map[string]interface{}{
		"installName": expectedInstallName,
		"namespace":   expectedNamespace,
		"catalogAccess": map[string]interface{}{
			"chartName":    "grafana",
			"repo":         catalog,
			"chartVersion": "4.3.1",
		},
	}

	apprepo := &appRepov1.AppRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      catalog,
			Namespace: util.GetApprepoNamespace(),
		},
		Spec: appRepov1.AppRepositorySpec{
			Type: util.ConfigTypeHelm,
			URL:  "www.example-catalog.cloud",
		},
	}

	scheme := runtime.NewScheme()
	_ = appRepov1.AddToScheme(scheme)

	k8sClient := fake.NewFakeClientWithScheme(scheme)
	appRepoClient := fake.NewFakeClientWithScheme(scheme, apprepo)

	ctx := context.Background()
	ctx = context.WithValue(ctx, util.CRAndSecretClientKey{}, k8sClient)

	dc := &hubv1.DeploymentConfig{
		ID:               "1",
		TypeSpecificData: *util.CreateRawExtensionOrPanic(typeSpecificData),
	}

	helmSpecificData, err := apitypes.NewHelmSpecificData(&dc.TypeSpecificData)
	Nil(t, err, "err")

	ch, namespace, err := ParseTypeSpecificData(ctx, nil, dc, helmSpecificData, true, appRepoClient)

	Nil(t, err, "unexpected error")
	NotNil(t, ch, "chart data must not be nil")
	Equal(t, ch.InstallName, expectedInstallName, "ch.InstallName")
	Nil(t, ch.Values, "ch.Values")
	NotNil(t, ch.Load, "ch.Load")
	Equal(t, namespace, expectedNamespace, "namespace")
}

func TestParseTypeSpecificData_InvalidData(t *testing.T) {
	tests := []struct {
		name             string
		typeSpecificData map[string]interface{}
	}{
		{
			name: "no access data",
			typeSpecificData: map[string]interface{}{
				"installName": "der-gute-alte-broker",
				"namespace":   "broker-ns",
			},
		},
		{
			name: "no namespace",
			typeSpecificData: map[string]interface{}{
				"installName": "der-gute-alte-broker",
				"catalogAccess": map[string]interface{}{
					"chartName":    "grafana",
					"repo":         "example-catalog",
					"chartVersion": "4.3.1",
				},
			},
		},
		{
			name: "no installName",
			typeSpecificData: map[string]interface{}{
				"namespace": "broker-ns",
				"catalogAccess": map[string]interface{}{
					"chartName":    "grafana",
					"repo":         "example-catalog",
					"chartVersion": "4.3.1",
				},
			},
		},
		{
			name: "bad key in catalog access",
			typeSpecificData: map[string]interface{}{
				"installName": "der-gute-alte-broker",
				"namespace":   "broker-ns",
				"catalogAccess": map[string]interface{}{
					"chartName":    "grafana",
					"BAD-REPO-KEY": "example-catalog",
					"chartVersion": "4.3.1",
				},
			},
		},
		{
			name: "invalid chartName datatype",
			typeSpecificData: map[string]interface{}{
				"installName": "der-gute-alte-broker",
				"namespace":   "broker-ns",
				"catalogAccess": map[string]interface{}{
					"chartName":    map[string]string{},
					"repo":         "example-catalog",
					"chartVersion": "4.3.1",
				},
			},
		},
		{
			name: "invalid repo datatype",
			typeSpecificData: map[string]interface{}{
				"installName": "der-gute-alte-broker",
				"namespace":   "broker-ns",
				"catalogAccess": map[string]interface{}{
					"chartName":    "grafana",
					"repo":         42,
					"chartVersion": "4.3.1",
				},
			},
		},
		{
			name: "invalid chartVersion datatype",
			typeSpecificData: map[string]interface{}{
				"installName": "der-gute-alte-broker",
				"namespace":   "broker-ns",
				"catalogAccess": map[string]interface{}{
					"chartName":    "grafana",
					"repo":         "example-catalog",
					"chartVersion": 42,
				},
			},
		},
		{
			name: "invalid catalogAccess type",
			typeSpecificData: map[string]interface{}{
				"installName":   "broker",
				"namespace":     "broker-ns",
				"catalogAccess": 42,
			},
		},
		{
			name: "invalid tarballAccess type",
			typeSpecificData: map[string]interface{}{
				"installName":   "broker",
				"namespace":     "broker-ns",
				"tarballAccess": 42,
			},
		},
		{
			name: "invalid customCAData type",
			typeSpecificData: map[string]interface{}{
				"installName": "broker",
				"namespace":   "broker-ns",
				"tarballAccess": map[string]interface{}{
					"url":          "www.test-1234567.com/test-chart.tgz",
					"authHeader":   "<Insert-correct-auth-here>",
					"customCAData": map[string]string{},
				},
			},
		},
		{
			name: "invalid authHeader type",
			typeSpecificData: map[string]interface{}{
				"installName": "broker",
				"namespace":   "broker-ns",
				"tarballAccess": map[string]interface{}{
					"url":        "www.test-1234567.com/test-chart.tgz",
					"authHeader": 42,
				},
			},
		},
		{
			name: "missing attribute url",
			typeSpecificData: map[string]interface{}{
				"installName": "broker",
				"namespace":   "broker-ns",
				"tarballAccess": map[string]interface{}{
					"authHeader": "<Insert-correct-auth-here>",
				},
			},
		},
	}

	scheme := runtime.NewScheme()

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			k8sClient := fake.NewFakeClientWithScheme(scheme)

			ctx := context.Background()
			ctx = context.WithValue(ctx, util.CRAndSecretClientKey{}, k8sClient)

			dc := &hubv1.DeploymentConfig{
				ID:               "1",
				TypeSpecificData: *util.CreateRawExtensionOrPanic(tt.typeSpecificData),
			}

			// This code is not allowed to panic on bad input. It should return a normal error instead.
			var ch *ChartData
			var namespace string

			helmSpecificData, err := apitypes.NewHelmSpecificData(&dc.TypeSpecificData)
			if err == nil {
				ch, namespace, err = ParseTypeSpecificData(ctx, nil, dc, helmSpecificData, true, nil)
			}

			NotNil(t, err, "err")
			Nil(t, ch, "chart data must be nil")
			Equal(t, namespace, "", "namespace")
		})
	}
}

func TestGetOptionalStringSlicePropSafe(t *testing.T) {
	tests := []struct {
		name          string
		propMap       string
		propName      string
		expectSuccess bool
		expectedValue []string
	}{
		{
			name:          "property with 0 values",
			propMap:       `{"installArguments": []}`,
			propName:      "installArguments",
			expectSuccess: true,
			expectedValue: []string{},
		},
		{
			name:          "property with 1 item",
			propMap:       `{"installArguments": ["atomic"]}`,
			propName:      "installArguments",
			expectSuccess: true,
			expectedValue: []string{"atomic"},
		},
		{
			name:          "property with 2 items",
			propMap:       `{"installArguments": ["atomic", "create-namespace"]}`,
			propName:      "installArguments",
			expectSuccess: true,
			expectedValue: []string{"atomic", "create-namespace"},
		},
		{
			name:          "missing property",
			propMap:       `{}`,
			propName:      "installArguments",
			expectSuccess: true,
			expectedValue: nil,
		},
		{
			name:          "invalid property value null",
			propMap:       `{"installArguments": null}`,
			propName:      "installArguments",
			expectSuccess: false,
			expectedValue: nil,
		},
		{
			name:          "invalid property value no slice",
			propMap:       `{"installArguments": {}}`,
			propName:      "installArguments",
			expectSuccess: false,
			expectedValue: nil,
		},
		{
			name:          "invalid property value item",
			propMap:       `{"installArguments": [42]}`,
			propName:      "installArguments",
			expectSuccess: false,
			expectedValue: nil,
		},
	}

	for i := range tests {
		test := &tests[i]
		t.Run(test.name, func(t *testing.T) {
			var propertyMap map[string]interface{}
			err := json.Unmarshal([]byte(test.propMap), &propertyMap)
			Nil(t, err, "error unmarshalling property map")

			args, err := getOptionalStringSlicePropSafe(test.propName, propertyMap)

			if test.expectSuccess {
				Equal(t, args, test.expectedValue, "slice property value")
				Nil(t, err, "error getting slice property")
			} else {
				NotNil(t, err, "error getting slice property")
			}
		})
	}
}
