package deployutil

import (
	"context"
	"errors"

	v12 "github.com/gardener/potter-controller/api/v1"
	"github.com/gardener/potter-controller/pkg/util"

	"github.com/gardener/landscaper/apis/core/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type SecretHandler struct {
	secretClient client.Client
}

func NewSecretHandler(secretClient client.Client) *SecretHandler {
	return &SecretHandler{
		secretClient: secretClient,
	}
}

func (r *SecretHandler) CreateExportSecretForDi(ctx context.Context, deployData *DeployData,
	exportData []byte) (secretName string, err error) {
	log := util.GetLoggerFromContext(ctx)

	newData := map[string][]byte{
		v1alpha1.DataObjectSecretDataKey: exportData,
	}

	deployItemKey := deployData.GetDeployItemKey()

	clusterBomKey := util.GetClusterBomKeyFromDeployItemKey(deployItemKey)

	configID := deployData.GetConfigID()

	secretName = util.CreateSecretName(clusterBomKey.Name, configID)

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: deployData.GetNamespace(),
			Labels: map[string]string{
				v12.LabelClusterBomName:      clusterBomKey.Name,
				v12.LabelApplicationConfigID: configID,
				v12.LabelPurpose:             util.PurposeDiExportData,
			},
		},
		Data: newData,
		Type: v1.SecretTypeOpaque,
	}

	err = r.secretClient.Create(ctx, secret)
	if err != nil {
		message := "Error creating secret for deploy item"
		log.Error(err, message)
		return "", errors.New(message + " - " + err.Error())
	}

	return secretName, nil
}

func (r *SecretHandler) RemoveUnreferencedExportSecrets(ctx context.Context, deployItem *v1alpha1.DeployItem,
	referencedSecret *v1alpha1.ObjectReference) error {
	log := util.GetLoggerFromContext(ctx)

	clusterBomKey := util.GetClusterBomKeyFromDeployItem(deployItem)
	appConfigID := util.GetAppConfigIDFromDeployItem(deployItem)

	secretList := v1.SecretList{}
	err := r.secretClient.List(ctx, &secretList, client.InNamespace(deployItem.Namespace),
		client.MatchingLabels{v12.LabelClusterBomName: clusterBomKey.Name, v12.LabelApplicationConfigID: appConfigID, v12.LabelPurpose: util.PurposeDiExportData})
	if err != nil {
		log.Error(err, "Error fetching secret list")
		return err
	}

	for i := range secretList.Items {
		secret := &secretList.Items[i]

		if referencedSecret == nil || referencedSecret.Name != secret.Name {
			err = r.secretClient.Delete(ctx, secret)

			if err != nil {
				log.Error(err, "Error deleting export secret")
				return err
			}
		}
	}

	return nil
}
