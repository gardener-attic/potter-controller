package admission

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"

	hubv1 "github.wdf.sap.corp/kubernetes/hub-controller/api/v1"
	"github.wdf.sap.corp/kubernetes/hub-controller/pkg/synchronize"
	"github.wdf.sap.corp/kubernetes/hub-controller/pkg/util"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	operationReplace = "replace"
	operationKeep    = "keep"
	operationDelete  = "delete"
	operationEmpty   = ""
)

type SecretKeeper struct {
	client synchronize.UncachedClient
	dryRun bool
}

// Suppose the old app config has no secret values.
// - If the new app config has no secret values, do nothing.
// - If operation == delete, do nothing.
// - Otherwise create secret (reject clusterbom if data missing).
// Suppose the old app config has secret values.
// - If the new app config has no secret values, keep the existing secret values.
// - If operation == keep, keep the existing secret values.
// - If operation == delete, delete the existing secret values.
// - If operation == replace, replace the existing secret values (check for changes; reject clusterbom if data  missing)
// - If operation == "" and no data provided => keep.
// - If operation == "" and data provided => replace.
// It has no impact whether and which internalSecretName is provided during an update of a clusterbom.
func (s *SecretKeeper) handleAppConfig(ctx context.Context, clusterBom *hubv1.ClusterBom,
	appIndex int, appConfig, oldAppConfig *hubv1.ApplicationConfig, patches []patch) ([]patch, error) {
	log := ctx.Value(util.LoggerKey{}).(logr.Logger)

	if oldAppConfig == nil || oldAppConfig.SecretValues == nil {
		// no old secret values

		if appConfig.SecretValues == nil {
			// no secret values, neither in the old nor new app config
			return patches, nil
		}

		if appConfig.SecretValues.Operation == operationDelete {
			// allow operation delete even if the old app config has no secret values
			patches = s.appendPatchesForDeleteSecretValues(patches, appIndex)
			return patches, nil
		}

		// no old, but new secret values => create
		secretName, err := s.moveSecretValuesToSecret(ctx, clusterBom, appConfig)
		if err != nil {
			return nil, err
		}
		patches = s.appendPatchesForSecretValues(patches, appConfig, appIndex, secretName)
		return patches, nil
	}

	oldInternalSecretName := oldAppConfig.SecretValues.InternalSecretName

	if appConfig.SecretValues == nil {
		// old, but no new secret values => keep
		patches = s.appendPatchesForSecretValues(patches, appConfig, appIndex, oldInternalSecretName)
		return patches, nil
	}

	if appConfig.SecretValues.Operation == operationKeep {
		patches = s.appendPatchesForSecretValues(patches, appConfig, appIndex, oldInternalSecretName)
		return patches, nil
	}

	if appConfig.SecretValues.Operation == operationDelete {
		patches = s.appendPatchesForDeleteSecretValues(patches, appIndex)
		return patches, nil
	}

	if appConfig.SecretValues.Operation == operationReplace {
		return s.replaceSecretValues(ctx, clusterBom, appIndex, appConfig, oldInternalSecretName, patches)
	}

	if appConfig.SecretValues.Operation == operationEmpty {
		if appConfig.SecretValues.Data == nil {
			return s.keepSecretValues(appIndex, appConfig, oldInternalSecretName, patches)
		}

		return s.replaceSecretValues(ctx, clusterBom, appIndex, appConfig, oldInternalSecretName, patches)
	}

	// unsupported operation
	// unsupported operation
	message := "rejected clusterbom, because spec.applicationConfigs[].secretValues.Operation is not supported: " + appConfig.SecretValues.Operation
	log.Error(nil, message)
	return nil, errors.New(message)
}

func (s *SecretKeeper) keepSecretValues(appIndex int,
	appConfig *hubv1.ApplicationConfig, oldInternalSecretName string, patches []patch) ([]patch, error) {
	if appConfig.SecretValues != nil && appConfig.SecretValues.InternalSecretName == oldInternalSecretName {
		return patches, nil
	}

	patches = s.appendPatchesForSecretValues(patches, appConfig, appIndex, oldInternalSecretName)
	return patches, nil
}

func (s *SecretKeeper) replaceSecretValues(ctx context.Context, clusterBom *hubv1.ClusterBom, appIndex int,
	appConfig *hubv1.ApplicationConfig, oldInternalSecretName string, patches []patch) ([]patch, error) {
	log := ctx.Value(util.LoggerKey{}).(logr.Logger)

	oldSecretKey := types.NamespacedName{
		Namespace: clusterBom.Namespace,
		Name:      oldInternalSecretName,
	}
	oldSecret, err := s.getSecret(ctx, &oldSecretKey)
	if err != nil {
		return nil, err
	}

	if appConfig.SecretValues.Data == nil {
		message := "rejected clusterbom, because spec.applicationConfigs[].secretValues.data must be provided to replace secret values"
		log.V(util.LogLevelWarning).Info(message)
		return nil, errors.New(message)
	}

	equal, err := s.isEqualSecretData(oldSecret.Data[util.SecretValuesKey], appConfig.SecretValues.Data.Raw)
	if err != nil {
		message := "rejected clusterbom, old secret could not be compared with new secret"
		log.Error(err, message)
		return nil, errors.New(message + " - " + err.Error())
	}
	secretName := oldInternalSecretName
	if !equal {
		secretName, err = s.moveSecretValuesToSecret(ctx, clusterBom, appConfig)
		if err != nil {
			return nil, err
		}
	}
	patches = s.appendPatchesForSecretValues(patches, appConfig, appIndex, secretName)
	return patches, nil
}

func (s *SecretKeeper) unmarshalSecretValuesFromAppConfig(ctx context.Context, appConfig *hubv1.ApplicationConfig) (map[string]interface{}, error) {
	log := ctx.Value(util.LoggerKey{}).(logr.Logger)

	var secretValues map[string]interface{}
	err := json.Unmarshal(appConfig.SecretValues.Data.Raw, &secretValues)
	if err != nil {
		message := "Rejected clusterbom, because secret data could not be unmarshalled"
		log.Error(err, message)
		return nil, errors.New(message + " - " + err.Error())
	}

	return secretValues, nil
}

func (s *SecretKeeper) isEqualSecretData(value1, value2 []byte) (bool, error) {
	var f1 interface{}
	var f2 interface{}

	if value1 == nil && value2 == nil {
		return true, nil
	}
	if value1 == nil || value2 == nil {
		return false, nil
	}
	err := json.Unmarshal(value1, &f1)
	if err != nil {
		return false, err
	}

	err = json.Unmarshal(value2, &f2)
	if err != nil {
		return false, err
	}

	marshaled1, err := json.Marshal(f1)
	if err != nil {
		return false, err
	}

	marshaled2, err := json.Marshal(f2)
	if err != nil {
		return false, err
	}

	return reflect.DeepEqual(marshaled1, marshaled2), nil
}

func (s *SecretKeeper) moveSecretValuesToSecret(ctx context.Context, clusterBom *hubv1.ClusterBom,
	appConfig *hubv1.ApplicationConfig) (string, error) {
	log := ctx.Value(util.LoggerKey{}).(logr.Logger)

	// check that secret data are provided
	if appConfig.SecretValues.Data == nil {
		message := "rejected clusterbom, because spec.applicationConfigs[].secretValues.data is empty"
		log.Error(nil, message)
		return "", errors.New(message)
	}

	// check that the secret data are valid json
	_, err := s.unmarshalSecretValuesFromAppConfig(ctx, appConfig)
	if err != nil {
		return "", err
	}

	secretName := util.CreateSecretName(clusterBom.Name, appConfig.ID)
	secret := s.makeSecret(clusterBom, appConfig, secretName)
	err = s.createSecret(ctx, secret)
	if err != nil {
		return "", err
	}

	return secretName, nil
}

func (s *SecretKeeper) makeSecret(clusterBom *hubv1.ClusterBom, appConfig *hubv1.ApplicationConfig, secretName string) *v1.Secret {
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: clusterBom.Namespace,
			Labels: map[string]string{
				hubv1.LabelClusterBomName:      clusterBom.Name,
				hubv1.LabelApplicationConfigID: appConfig.ID,
				hubv1.LabelPurpose:             util.PurposeSecretValues,
			},
		},
		Data: map[string][]byte{
			util.SecretValuesKey: appConfig.SecretValues.Data.Raw,
		},
		Type: v1.SecretTypeOpaque,
	}
	return secret
}

func (s *SecretKeeper) createSecret(ctx context.Context, secret *v1.Secret) error {
	log := ctx.Value(util.LoggerKey{}).(logr.Logger)

	if s.dryRun {
		log.V(util.LogLevelWarning).Info("Dry run: skip secret creation")
		return nil
	}

	err := s.client.Create(ctx, secret)
	if err != nil {
		message := "Error creating secret"
		log.Error(err, message)
		return errors.New(message + " - " + err.Error())
	}
	return nil
}

func (s *SecretKeeper) getSecret(ctx context.Context, secretKey *types.NamespacedName) (*v1.Secret, error) {
	log := ctx.Value(util.LoggerKey{}).(logr.Logger)

	secret := v1.Secret{}
	err := s.client.GetUncached(ctx, *secretKey, &secret)
	if err != nil {
		message := "Error fetching secret"
		log.Error(err, message)
		return nil, errors.New(message + " - " + err.Error())
	}

	return &secret, nil
}

// Appends the necessary patches so that
// - secretValues exists,
// - secretValues.internalSecretName is set to the specified secret name,
// - secretValues.operation is not set,
// - secretValues.data is not set.
func (s *SecretKeeper) appendPatchesForSecretValues(patches []patch, appConfig *hubv1.ApplicationConfig, appIndex int,
	secretName string) []patch {
	if appConfig.SecretValues == nil {
		patches = append(patches, patch{
			Op:   "add",
			Path: "/spec/applicationConfigs/" + strconv.Itoa(appIndex) + "/secretValues",
			Value: &hubv1.SecretValues{
				InternalSecretName: secretName,
			},
		})
	} else if appConfig.SecretValues.InternalSecretName == "" {
		patches = append(patches, patch{
			Op:    "add",
			Path:  "/spec/applicationConfigs/" + strconv.Itoa(appIndex) + "/secretValues/internalSecretName",
			Value: secretName,
		})
	} else {
		patches = append(patches, patch{
			Op:    "replace",
			Path:  "/spec/applicationConfigs/" + strconv.Itoa(appIndex) + "/secretValues/internalSecretName",
			Value: secretName,
		})
	}

	if appConfig.SecretValues != nil && appConfig.SecretValues.Operation != "" {
		patches = append(patches, patch{
			Op:   "remove",
			Path: "/spec/applicationConfigs/" + strconv.Itoa(appIndex) + "/secretValues/operation",
		})
	}

	if appConfig.SecretValues != nil && appConfig.SecretValues.Data != nil {
		patches = append(patches, patch{
			Op:   "remove",
			Path: "/spec/applicationConfigs/" + strconv.Itoa(appIndex) + "/secretValues/data",
		})
	}

	return patches
}

func (s *SecretKeeper) appendPatchesForDeleteSecretValues(patches []patch, appIndex int) []patch {
	patches = append(patches, patch{
		Op:   "remove",
		Path: "/spec/applicationConfigs/" + strconv.Itoa(appIndex) + "/secretValues",
	})
	return patches
}
