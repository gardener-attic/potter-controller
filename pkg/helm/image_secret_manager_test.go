package helm

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/go-logr/zapr"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/release"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.wdf.sap.corp/kubernetes/hub-controller/pkg/util"
)

const (
	expectedHubDockerSecret        = "test-foo-kubeconfig"
	expectedUpdatedHubDockerSecret = "test-bar-kubeconfig"
)

func createMockRelease(hubSecInReleaseConfig, hubSecInChartValues bool) *release.Release {
	rel := &release.Release{
		Name:      "testrelease",
		Namespace: "testnamespace",
		Chart:     &chart.Chart{Values: map[string]interface{}{}},
		Config:    map[string]interface{}{},
	}

	if hubSecInChartValues {
		rel.Chart = &chart.Chart{Values: map[string]interface{}{
			"hubsec": map[string]interface{}{
				"enabled": true,
			},
		}}
	}
	if hubSecInReleaseConfig {
		rel.Config = map[string]interface{}{
			"hubsec": map[string]interface{}{
				"enabled": true,
			},
		}
	}

	return rel
}

func Test_CreateSecret_With_ChartValues(t *testing.T) {
	mockRelease := createMockRelease(false, true)

	secretManager := imageSecretManager{
		kubernetesClientset: fake.NewSimpleClientset(),
	}
	ctx := context.WithValue(context.TODO(), util.LoggerKey{}, zapr.NewLogger(zap.NewNop()))

	encodedHubDockerSecret := base64.StdEncoding.EncodeToString([]byte(expectedHubDockerSecret))

	err := secretManager.createOrUpdateImageSecret(ctx, mockRelease, &encodedHubDockerSecret)

	assert.Nil(t, err, "Secret should have been created without error")

	secret, err := secretManager.kubernetesClientset.CoreV1().Secrets(mockRelease.Namespace).Get(context.TODO(), sapSecretName, metav1.GetOptions{})
	assert.Nil(t, err, "Secret should have been read without error")

	assert.Equal(t, secret.StringData[".dockerconfigjson"], expectedHubDockerSecret)
}

func Test_CreateSecret_With_ReleaseConfig(t *testing.T) {
	mockRelease := createMockRelease(true, false)

	secretManager := imageSecretManager{
		kubernetesClientset: fake.NewSimpleClientset(),
	}
	ctx := context.WithValue(context.TODO(), util.LoggerKey{}, zapr.NewLogger(zap.NewNop()))

	encodedHubDockerSecret := base64.StdEncoding.EncodeToString([]byte(expectedHubDockerSecret))

	err := secretManager.createOrUpdateImageSecret(ctx, mockRelease, &encodedHubDockerSecret)
	assert.Nil(t, err, "Secret should have been updated without error")

	secret, err := secretManager.kubernetesClientset.CoreV1().Secrets(mockRelease.Namespace).Get(context.TODO(), sapSecretName, metav1.GetOptions{})
	assert.Nil(t, err, "Secret should have been read without error")

	assert.Equal(t, secret.StringData[".dockerconfigjson"], expectedHubDockerSecret)
}

func Test_UpdateSecret_With_ReleaseConfig(t *testing.T) {
	mockRelease := createMockRelease(true, false)

	secretManager := imageSecretManager{
		kubernetesClientset: fake.NewSimpleClientset(),
	}

	ctx := context.WithValue(context.TODO(), util.LoggerKey{}, zapr.NewLogger(zap.NewNop()))

	initSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sapSecretName,
			Namespace: mockRelease.Namespace,
		},
		StringData: map[string]string{
			".dockerconfigjson": expectedHubDockerSecret,
		},
		Type: corev1.SecretTypeDockerConfigJson,
	}

	_, err := secretManager.kubernetesClientset.CoreV1().Secrets(mockRelease.Namespace).Create(context.TODO(), &initSecret, metav1.CreateOptions{})
	assert.Nil(t, err, "Secret should have been created without error")

	encodedHubDockerSecret := base64.StdEncoding.EncodeToString([]byte(expectedUpdatedHubDockerSecret))

	err = secretManager.createOrUpdateImageSecret(ctx, mockRelease, &encodedHubDockerSecret)
	assert.Nil(t, err, "Secret should have been updated without error")

	secret, err := secretManager.kubernetesClientset.CoreV1().Secrets(mockRelease.Namespace).Get(context.TODO(), sapSecretName, metav1.GetOptions{})
	assert.Nil(t, err, "Secret should have been read without error")

	assert.Equal(t, secret.StringData[".dockerconfigjson"], expectedUpdatedHubDockerSecret)
}

func Test_UpdateSecret_With_ChartValues(t *testing.T) {
	mockRelease := createMockRelease(false, true)

	secretManager := imageSecretManager{
		kubernetesClientset: fake.NewSimpleClientset(),
	}

	ctx := context.WithValue(context.TODO(), util.LoggerKey{}, zapr.NewLogger(zap.NewNop()))

	initSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sapSecretName,
			Namespace: mockRelease.Namespace,
		},
		StringData: map[string]string{
			".dockerconfigjson": expectedHubDockerSecret,
		},
		Type: corev1.SecretTypeDockerConfigJson,
	}

	_, err := secretManager.kubernetesClientset.CoreV1().Secrets(mockRelease.Namespace).Create(context.TODO(), &initSecret, metav1.CreateOptions{})
	assert.Nil(t, err, "Secret should have been created without error")

	encodedHubDockerSecret := base64.StdEncoding.EncodeToString([]byte(expectedUpdatedHubDockerSecret))

	err = secretManager.createOrUpdateImageSecret(ctx, mockRelease, &encodedHubDockerSecret)
	assert.Nil(t, err, "Secret should have been updated without error")

	secret, err := secretManager.kubernetesClientset.CoreV1().Secrets(mockRelease.Namespace).Get(context.TODO(), sapSecretName, metav1.GetOptions{})
	assert.Nil(t, err, "Secret should have been read without error")

	assert.Equal(t, secret.StringData[".dockerconfigjson"], expectedUpdatedHubDockerSecret)
}

func Test_DeleteSecret_Successfully(t *testing.T) {
	mockRelease := createMockRelease(true, false)

	secretManager := imageSecretManager{
		kubernetesClientset: fake.NewSimpleClientset(),
	}

	ctx := context.WithValue(context.TODO(), util.LoggerKey{}, zapr.NewLogger(zap.NewNop()))

	initSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sapSecretName,
			Namespace: mockRelease.Namespace,
		},
		StringData: map[string]string{
			".dockerconfigjson": expectedHubDockerSecret,
		},
		Type: corev1.SecretTypeDockerConfigJson,
	}

	_, err := secretManager.kubernetesClientset.CoreV1().Secrets(mockRelease.Namespace).Create(context.TODO(), &initSecret, metav1.CreateOptions{})
	assert.Nil(t, err, "Secret should have been created without error")

	err = secretManager.deleteImageSecret(ctx, mockRelease)
	assert.Nil(t, err, "Secret should have been deleted without error")

	_, err = secretManager.kubernetesClientset.CoreV1().Secrets(mockRelease.Namespace).Get(context.TODO(), sapSecretName, metav1.GetOptions{})
	assert.Error(t, err, "Expected Not Found err")
	assert.True(t, errors.IsNotFound(err), "Error should be an not found err")
}

func Test_IsHubSecretEnabled(t *testing.T) {
	tests := []struct {
		name                       string
		values                     map[string]interface{}
		config                     map[string]interface{}
		expectedIsHubSecretEnabled bool
	}{
		{
			name:                       "test with values and config not set",
			expectedIsHubSecretEnabled: false,
		},
		{
			name: "test with invalid value for property 'enabled' in values",
			values: map[string]interface{}{
				"hubsec": map[string]interface{}{
					"enabled": "som som som",
				},
			},
			expectedIsHubSecretEnabled: false,
		},
		{
			name: "test enabled via values",
			values: map[string]interface{}{
				"hubsec": map[string]interface{}{
					"enabled": true,
				},
			},
			expectedIsHubSecretEnabled: true,
		},
		{
			name: "test with invalid value for property 'enabled' in config",
			config: map[string]interface{}{
				"hubsec": map[string]interface{}{
					"enabled": "som som som",
				},
			},
			expectedIsHubSecretEnabled: false,
		},
		{
			name: "test enabled via config",
			config: map[string]interface{}{
				"hubsec": map[string]interface{}{
					"enabled": true,
				},
			},
			expectedIsHubSecretEnabled: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ch := &chart.Chart{
				Values: tt.values,
			}
			rel := &release.Release{
				Chart:  ch,
				Config: tt.config,
			}

			isHubsecretEnabled := isHubSecretEnabled(rel)

			assert.Equal(t, tt.expectedIsHubSecretEnabled, isHubsecretEnabled, "isHubsecretEnabled")
		})
	}
}
