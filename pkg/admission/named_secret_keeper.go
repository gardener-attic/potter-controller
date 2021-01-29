package admission

import (
	"context"
	"errors"
	"reflect"
	"strconv"

	hubv1 "github.com/gardener/potter-controller/api/v1"
	"github.com/gardener/potter-controller/pkg/synchronize"
	"github.com/gardener/potter-controller/pkg/util"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type NamedSecretKeeper struct {
	client       synchronize.UncachedClient
	dryRun       bool
	clusterBom   *hubv1.ClusterBom
	appIndex     int
	appConfig    *hubv1.ApplicationConfig
	oldAppConfig *hubv1.ApplicationConfig
}

func (s *NamedSecretKeeper) handleAppConfig(ctx context.Context, patches []patch) ([]patch, error) {
	log := util.GetLoggerFromContext(ctx)
	log.Info("Handling named secrets of app")

	if s.oldAppConfig == nil || s.oldAppConfig.NamedSecretValues == nil {
		if s.appConfig.NamedSecretValues == nil {
			return patches, nil
		}
	}

	oldNamedSecretValues := s.getNamedSecretValues(s.oldAppConfig)
	newNamedSecretValues := s.getNamedSecretValues(s.appConfig)

	logicalSecretNames := s.getKeys(oldNamedSecretValues, newNamedSecretValues)

	if s.appConfig.NamedSecretValues == nil {
		patches = s.appendPatchToAddBase(patches)
	}

	namedSecretSectionNeeded := false

	tmpPatches := []patch{}

	for _, logicalSecretName := range logicalSecretNames {
		oldNamedSecretsValue, oldOk := oldNamedSecretValues[logicalSecretName]
		newNamedSecretsValue, newOk := newNamedSecretValues[logicalSecretName]

		var err error
		var needed bool

		tmpPatches, needed, err = s.handleNamedSecret(ctx, logicalSecretName, oldNamedSecretsValue, oldOk, &newNamedSecretsValue, newOk, tmpPatches)
		if err != nil {
			return nil, err
		}

		namedSecretSectionNeeded = namedSecretSectionNeeded || needed
	}

	patches = append(patches, tmpPatches...)

	if !namedSecretSectionNeeded {
		patches = s.appendPatchToRemoveBase(patches)
	}

	return patches, nil
}

func (s *NamedSecretKeeper) getNamedSecretValues(appConfig *hubv1.ApplicationConfig) map[string]hubv1.NamedSecretValues {
	if appConfig == nil || appConfig.NamedSecretValues == nil {
		return make(map[string]hubv1.NamedSecretValues)
	}

	return appConfig.NamedSecretValues
}

func (s *NamedSecretKeeper) getKeys(oldMap, newMap map[string]hubv1.NamedSecretValues) (keys []string) {
	keys = []string{}

	for k := range oldMap {
		keys = append(keys, k)
	}

	for k := range newMap {
		if _, ok := oldMap[k]; !ok {
			keys = append(keys, k)
		}
	}

	return keys
}

func (s *NamedSecretKeeper) handleNamedSecret(ctx context.Context, logicalSecretName string, oldNamedSecretsValue hubv1.NamedSecretValues, oldOk bool,
	newNamedSecretsValue *hubv1.NamedSecretValues, newOk bool, patches []patch) ([]patch, bool, error) {
	log := util.GetLoggerFromContext(ctx)
	log.Info("Handling named secret", "logicalSecretName", logicalSecretName)

	if !oldOk && newOk {
		if newNamedSecretsValue.Operation == operationDelete {
			// allow operation delete even if the old app config has no secret values
			patches = s.appendPatchToRemoveSecretValues(patches, logicalSecretName)
			return patches, false, nil
		}

		secret, err := s.buildSecretObject(ctx, logicalSecretName, newNamedSecretsValue)
		if err != nil {
			return nil, false, err
		}

		err = s.createSecret(ctx, secret)
		if err != nil {
			return nil, false, err
		}

		patches = s.appendPatchToReplaceSecretValues(patches, logicalSecretName, secret.GetName())
		return patches, true, nil
	} else if oldOk && !newOk {
		patches = s.appendPatchToAddSecretValues(patches, logicalSecretName, oldNamedSecretsValue.InternalSecretName)
		return patches, true, nil
	} else {
		// old and new secret values
		switch newNamedSecretsValue.Operation {
		case operationDelete:
			patches = s.appendPatchToRemoveSecretValues(patches, logicalSecretName)
			return patches, false, nil

		case operationEmpty:
			if len(newNamedSecretsValue.StringData) == 0 {
				// keep
				patches = s.appendPatchToReplaceSecretValues(patches, logicalSecretName, oldNamedSecretsValue.InternalSecretName)
				return patches, true, nil
			}

			// replace
			tmpPatches, err := s.replaceSecretValues(ctx, logicalSecretName, newNamedSecretsValue, oldNamedSecretsValue.InternalSecretName, patches)
			return tmpPatches, true, err

		default:
			message := "rejected clusterbom, because spec.applicationConfigs[].namedSecretValues.Operation is not supported: " + newNamedSecretsValue.Operation
			log.Error(nil, message)
			return nil, false, errors.New(message)
		}
	}
}

func (s *NamedSecretKeeper) replaceSecretValues(ctx context.Context, logicalSecretName string,
	newSecretValues *hubv1.NamedSecretValues, oldInternalSecretName string, patches []patch) ([]patch, error) {
	oldSecretKey := types.NamespacedName{
		Namespace: s.clusterBom.Namespace,
		Name:      oldInternalSecretName,
	}
	oldSecret, err := s.getSecret(ctx, &oldSecretKey)
	if err != nil {
		return nil, err
	}

	newSecret, err := s.buildSecretObject(ctx, logicalSecretName, newSecretValues)
	if err != nil {
		return nil, err
	}

	equal := reflect.DeepEqual(oldSecret.Data, newSecret.Data)

	internalSecretName := oldInternalSecretName
	if !equal {
		err = s.createSecret(ctx, newSecret)
		if err != nil {
			return nil, err
		}

		internalSecretName = newSecret.GetName()
	}
	patches = s.appendPatchToReplaceSecretValues(patches, logicalSecretName, internalSecretName)
	return patches, nil
}

func (s *NamedSecretKeeper) buildSecretObject(ctx context.Context, logicalSecretName string, newNamedSecretsValue *hubv1.NamedSecretValues) (*v1.Secret, error) {
	log := ctx.Value(util.LoggerKey{}).(logr.Logger)

	// check that secret data are provided
	if newNamedSecretsValue.StringData == nil {
		message := "rejected clusterbom, because spec.applicationConfigs." + s.appConfig.ID + ".namedSecretValues." + logicalSecretName + ".data must be provided to replace secret values"
		log.Error(nil, message)
		return nil, errors.New(message)
	}

	internalSecretName := util.CreateSecretName(s.clusterBom.Name, s.appConfig.ID)
	secret := s.makeSecret(internalSecretName, logicalSecretName, newNamedSecretsValue.StringData)

	return secret, nil
}

func (s *NamedSecretKeeper) makeSecret(secretName, logicalSecretName string,
	secretValuesMap map[string]string) *v1.Secret {
	data := make(map[string][]byte)

	for key, value := range secretValuesMap {
		data[key] = []byte(value)
	}

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: s.clusterBom.Namespace,
			Labels: map[string]string{
				hubv1.LabelClusterBomName:      s.clusterBom.Name,
				hubv1.LabelApplicationConfigID: s.appConfig.ID,
				hubv1.LabelLogicalSecretName:   logicalSecretName,
				hubv1.LabelPurpose:             util.PurposeSecretValues,
			},
		},
		Data: data,
		Type: v1.SecretTypeOpaque,
	}

	return secret
}

func (s *NamedSecretKeeper) createSecret(ctx context.Context, secret *v1.Secret) error {
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

func (s *NamedSecretKeeper) getSecret(ctx context.Context, secretKey *types.NamespacedName) (*v1.Secret, error) {
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

func (s *NamedSecretKeeper) appendPatchToAddBase(patches []patch) []patch {
	return append(patches, patch{
		Op:    "add",
		Path:  s.getBasePath(),
		Value: map[string]hubv1.SecretValues{},
	})
}

func (s *NamedSecretKeeper) appendPatchToRemoveBase(patches []patch) []patch {
	return append(patches, patch{
		Op:   "remove",
		Path: s.getBasePath(),
	})
}

func (s *NamedSecretKeeper) appendPatchToAddSecretValues(patches []patch, logicalSecretName, internalSecretName string) []patch {
	return append(patches, patch{
		Op:   "add",
		Path: s.getLogicalPath(logicalSecretName),
		Value: &hubv1.SecretValues{
			InternalSecretName: internalSecretName,
		},
	})
}

func (s *NamedSecretKeeper) appendPatchToReplaceSecretValues(patches []patch, logicalSecretName, internalSecretName string) []patch {
	return append(patches, patch{
		Op:   "replace",
		Path: s.getLogicalPath(logicalSecretName),
		Value: &hubv1.SecretValues{
			InternalSecretName: internalSecretName,
		},
	})
}

func (s *NamedSecretKeeper) appendPatchToRemoveSecretValues(patches []patch, logicalSecretName string) []patch {
	return append(patches, patch{
		Op:   "remove",
		Path: s.getLogicalPath(logicalSecretName),
	})
}

func (s *NamedSecretKeeper) getBasePath() string {
	return "/spec/applicationConfigs/" + strconv.Itoa(s.appIndex) + "/namedSecretValues"
}

func (s *NamedSecretKeeper) getLogicalPath(logicalSecretName string) string {
	return s.getBasePath() + "/" + logicalSecretName
}
