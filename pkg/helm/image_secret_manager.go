package helm

import (
	"context"
	"encoding/base64"
	"io/ioutil"

	"k8s.io/client-go/tools/clientcmd"

	"github.com/gardener/potter-controller/pkg/util"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/release"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const sapSecretName = "hubsec"

// imageSecretManager manages the creation and deletion of image pull secrets for private repositories
type imageSecretManager struct {
	kubernetesClientset kubernetes.Interface
}

func readDockerconfigSecret() (*string, error) {
	dataBytes, err := ioutil.ReadFile("/usr/image-pull-secret/hub")
	if err != nil {
		return nil, err
	}
	data := string(dataBytes)
	return &data, nil
}

// NewImageSecrets creates a new instance of an image secret.
// Please pass an instance of *ValidationObject* for kubeconfig creation and
// a string *namespace* in which the secret shall be created/updated.
func newImageSecretManager(ctx context.Context, kubeconfig string) *imageSecretManager {
	log := ctx.Value(util.LoggerKey{}).(logr.Logger)

	config, err := clientcmd.RESTConfigFromKubeConfig([]byte(kubeconfig))
	if err != nil {
		log.Error(err, "Error creating kubernetes client")
		return nil
	}
	clientSet := kubernetes.NewForConfigOrDie(config)

	manager := imageSecretManager{
		kubernetesClientset: clientSet,
	}

	return &manager
}

// createOrUpdateImageSecret Ensures (creates/updates) the image pull secret.
func (i *imageSecretManager) createOrUpdateImageSecret(ctx context.Context, rel *release.Release, dockerSecret *string) error {
	log := ctx.Value(util.LoggerKey{}).(logr.Logger)

	decodedDockerSecret, err := base64.StdEncoding.DecodeString(*dockerSecret)
	if err != nil {
		return errors.Wrap(err, "Error decoding docker config image pull secret")
	}

	sapSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sapSecretName,
			Namespace: rel.Namespace,
		},
		StringData: map[string]string{
			".dockerconfigjson": string(decodedDockerSecret),
		},
		Type: corev1.SecretTypeDockerConfigJson,
	}

	_, err = i.kubernetesClientset.CoreV1().Secrets(rel.Namespace).Create(context.TODO(), sapSecret, metav1.CreateOptions{})
	if k8sErrors.IsAlreadyExists(err) {
		_, err = i.kubernetesClientset.CoreV1().Secrets(rel.Namespace).Update(context.TODO(), sapSecret, metav1.UpdateOptions{})
		if err != nil {
			return errors.Wrapf(err, "Unable to update sap image secret %s in namespace %s", sapSecretName, rel.Namespace)
		}
	} else if err != nil {
		return errors.Wrapf(err, "Unable to create/update sap image secret %s in namespace %s", sapSecretName, rel.Namespace)
	}

	log.V(util.LogLevelDebug).Info("sap image secret %s successfully installed in %s", sapSecretName, rel.Namespace)

	return nil
}

// deleteImageSecret Deletes the image pull secret.
func (i *imageSecretManager) deleteImageSecret(ctx context.Context, rel *release.Release) error {
	log := ctx.Value(util.LoggerKey{}).(logr.Logger)

	log.V(util.LogLevelDebug).Info("Deleting sap image secret %s from namespace %s", sapSecretName, rel.Namespace)

	err := i.kubernetesClientset.CoreV1().Secrets(rel.Namespace).Delete(context.TODO(), sapSecretName, metav1.DeleteOptions{})
	if err != nil && !k8sErrors.IsNotFound(err) {
		return errors.Wrapf(err, "Unable to delete sap image secret %s in namespace %s",
			sapSecretName, rel.Namespace)
	}
	return nil
}

func isHubSecretEnabled(rel *release.Release) bool {
	// We have to check the secret configuration in both, the chart Values and the override Values
	if sapSecret, ok := rel.Chart.Values["hubsec"]; ok {
		sapSecretConfiguration := sapSecret.(map[string]interface{})
		// Comparison to with boolean needed to parse to bool type
		return true == sapSecretConfiguration["enabled"]
	}

	if sapSecret, ok := rel.Config["hubsec"]; ok {
		sapSecretConfiguration := sapSecret.(map[string]interface{})
		// Comparison to with boolean needed to parse to bool type
		return true == sapSecretConfiguration["enabled"]
	}
	return false
}
