package admission

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/go-logr/logr"
	"k8s.io/api/admission/v1beta1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	v12 "github.wdf.sap.corp/kubernetes/hub-controller/api/v1"
	"github.wdf.sap.corp/kubernetes/hub-controller/pkg/secrets"
	"github.wdf.sap.corp/kubernetes/hub-controller/pkg/synchronize"
	"github.wdf.sap.corp/kubernetes/hub-controller/pkg/util"
)

type secretReviewer struct {
	log                 logr.Logger
	requestReview       *v1beta1.AdmissionReview
	hubControllerClient synchronize.UncachedClient
}

func (reviewer *secretReviewer) review() *v1beta1.AdmissionReview {
	switch reviewer.requestReview.Request.Operation {
	case v1beta1.Update:
		return reviewer.reviewUpdate()
	case v1beta1.Delete:
		return reviewer.reviewDelete()
	default:
		reviewer.log.Error(nil, "Secret admission hook called for unexpected operation",
			"operation", reviewer.requestReview.Request.Operation)
		return reviewer.allow()
	}
}

func (reviewer *secretReviewer) reviewUpdate() *v1beta1.AdmissionReview {
	var oldSecret *v1.Secret
	err := json.Unmarshal(reviewer.requestReview.Request.OldObject.Raw, &oldSecret)
	if err != nil {
		reviewer.log.Error(err, "error when unmarshalling old secret")
		return reviewer.deny("error when unmarshalling old secret : " + err.Error())
	}

	var newSecret *v1.Secret
	err = json.Unmarshal(reviewer.requestReview.Request.Object.Raw, &newSecret)
	if err != nil {
		reviewer.log.Error(err, "error when unmarshalling new secret")
		return reviewer.deny("error when unmarshalling new secret : " + err.Error())
	}

	reviewer.log = reviewer.log.WithValues(util.LogKeySecretName, types.NamespacedName{
		Namespace: oldSecret.Namespace,
		Name:      oldSecret.Name,
	})

	reviewer.log.V(util.LogLevelDebug).Info("Reviewing Secret for update")

	ok := reviewer.checkResponsibility(oldSecret) || reviewer.checkResponsibility(newSecret)
	if !ok {
		reviewer.log.Error(nil, "Secret admission hook called for updating a secret without label")
		return reviewer.allow()
	}

	ok, err = reviewer.validateSecretDeletionToken(newSecret)
	if err != nil {
		reviewer.log.Error(err, "Error when validating update request")
		return reviewer.deny("Error when validating update request")
	} else if !ok {
		return reviewer.deny("You are not allowed to update a hub-managed secret")
	}

	return reviewer.allow()
}

func (reviewer *secretReviewer) reviewDelete() *v1beta1.AdmissionReview {
	var oldSecret *v1.Secret
	err := json.Unmarshal(reviewer.requestReview.Request.OldObject.Raw, &oldSecret)
	if err != nil {
		reviewer.log.Error(err, "error when unmarshalling old secret")
		return reviewer.deny("error when unmarshalling old secret : " + err.Error())
	}

	reviewer.log = reviewer.log.WithValues(util.LogKeySecretName, types.NamespacedName{
		Namespace: oldSecret.Namespace,
		Name:      oldSecret.Name,
	})

	reviewer.log.V(util.LogLevelDebug).Info("Reviewing Secret for delete")

	ok := reviewer.checkResponsibility(oldSecret)
	if !ok {
		reviewer.log.Error(nil, "Secret admission hook called for deleting secret without label")
		return reviewer.allow()
	}

	ok, err = reviewer.validateSecretDeletionToken(oldSecret)
	if err != nil {
		reviewer.log.Error(err, "Error when validating deletion request")
		return reviewer.deny("Error when validating deletion request")
	} else if !ok {
		return reviewer.deny("You are not allowed to delete a hub-managed secret")
	}

	return reviewer.allow()
}

func (reviewer *secretReviewer) checkResponsibility(secret *v1.Secret) bool {
	value, ok := secret.ObjectMeta.Labels[v12.LabelPurpose]
	return ok && (value == util.PurposeSecretValues)
}

func (reviewer *secretReviewer) validateSecretDeletionToken(secret *v1.Secret) (bool, error) {
	receivedToken, ok := secret.Data[util.KeyDeletionToken]
	if !ok {
		reviewer.log.V(util.LogLevelWarning).Info("Someone tried to delete a hub-managed secret without a token")
		return false, nil
	}

	ctx := context.Background()
	ctx = context.WithValue(ctx, util.LoggerKey{}, reviewer.log)

	secretDeletionKey, err := secrets.GetSecretDeletionKey(ctx, reviewer.hubControllerClient, true)
	if err != nil {
		reviewer.log.Error(err, "Fetching secret deletion key failed")
		return false, err
	}

	expectedToken, err := util.ComputeSecretDeletionToken(secretDeletionKey, secret.Name)
	if err != nil {
		reviewer.log.Error(err, "Computation of secret deletion token failed")
		return false, err
	}

	if !bytes.Equal(receivedToken, expectedToken) {
		// retry without cache
		secretDeletionKey, err := secrets.GetSecretDeletionKey(ctx, reviewer.hubControllerClient, false)
		if err != nil {
			reviewer.log.Error(err, "Fetching secret deletion key failed")
			return false, err
		}

		expectedToken, err = util.ComputeSecretDeletionToken(secretDeletionKey, secret.Name)
		if err != nil {
			reviewer.log.Error(err, "Computation of secret deletion token failed")
			return false, err
		}

		if !bytes.Equal(receivedToken, expectedToken) {
			reviewer.log.V(util.LogLevelWarning).Info("Someone tried to delete a hub-managed secret without valid token")
			return false, nil
		}
	}

	return true, nil
}

func (reviewer *secretReviewer) allow() *v1beta1.AdmissionReview {
	return &v1beta1.AdmissionReview{
		TypeMeta: reviewer.requestReview.TypeMeta,
		Response: &v1beta1.AdmissionResponse{
			UID:     reviewer.requestReview.Request.UID,
			Allowed: true,
		},
	}
}

func (reviewer *secretReviewer) deny(message string) *v1beta1.AdmissionReview {
	return &v1beta1.AdmissionReview{
		TypeMeta: reviewer.requestReview.TypeMeta,
		Response: &v1beta1.AdmissionResponse{
			UID:     reviewer.requestReview.Request.UID,
			Allowed: false,
			Result: &metav1.Status{
				Message: message,
			},
		},
	}
}
