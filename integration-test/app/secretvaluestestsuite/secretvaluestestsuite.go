package secretvaluestestsuite

import (
	"context"
	"encoding/json"
	"os"

	"github.com/gardener/potter-controller/api/apitypes"
	hubv1 "github.com/gardener/potter-controller/api/v1"
	"github.com/gardener/potter-controller/integration-test/app/builder"
	"github.com/gardener/potter-controller/integration-test/app/catalog"
	"github.com/gardener/potter-controller/integration-test/app/helm"
	"github.com/gardener/potter-controller/integration-test/app/util"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Run(ctx context.Context, config *util.IntegrationTestConfig, gardenClient client.Client) {
	util.WriteSectionHeader("Secret Values Test", config.TestLandscaper)

	s := newSecretValuesTestSuite(ctx, config, gardenClient)

	s.cleanup(ctx)

	s.runCreateSecretValues(ctx)
	s.runUpdateIdempotency(ctx)
	s.runKeepSecretValuesByEmptySection(ctx)
	s.runKeepSecretValuesByKeepOperation(ctx)
	s.runKeepSecretValuesByReplaceOperation(ctx)
	s.runKeepSecretValuesByNoOperation(ctx)
	s.runReplaceSecretValuesByOperation(ctx)
	s.runReplaceSecretValuesNoOperation(ctx)
	s.runDeleteSecretValues(ctx)
	// test if operation is idempotent
	s.runDeleteSecretValues(ctx)

	s.cleanup(ctx)

	s.runDeleteClusterBomWithSecretValues(ctx)
}

type secretValuesTestSuite struct {
	config           *util.IntegrationTestConfig
	gardenClient     client.Client
	helmClient       *helm.HelmClient
	clusterBomClient *util.ClusterBomClient
	targetKubeConfig string
	clusterBomKey    types.NamespacedName
	appConfigID      string
	helmdata         *apitypes.HelmSpecificData
}

func newSecretValuesTestSuite(ctx context.Context, config *util.IntegrationTestConfig, gardenClient client.Client) *secretValuesTestSuite {
	clusterBomKey := config.GetTestClusterBomKey("secretvalues")
	helmdata := builder.NewHelmDataWithCatalogAccess(util.CreateEchoServerInstallationName(config), config.TargetClusterNamespace1, &catalog.ChartEcho2)

	targetKubeConfig := util.GetTargetKubeConfig(ctx, gardenClient, config)

	return &secretValuesTestSuite{
		config:           config,
		gardenClient:     gardenClient,
		helmClient:       helm.NewHelmClient(ctx, config, gardenClient),
		clusterBomClient: util.NewClusterBomClient(gardenClient),
		targetKubeConfig: targetKubeConfig,
		clusterBomKey:    clusterBomKey,
		appConfigID:      "echo",
		helmdata:         helmdata,
	}
}

func (s *secretValuesTestSuite) cleanup(ctx context.Context) {
	util.WriteSubSectionHeader("Cleanup")
	s.clusterBomClient.DeleteClusterBom(ctx, s.clusterBomKey)
}

func (s *secretValuesTestSuite) runCreateSecretValues(ctx context.Context) {
	util.WriteSubSectionHeader("Create ClusterBom with secret values")
	util.Write("Build ClusterBom " + s.clusterBomKey.String())

	clusterBom := builder.NewClusterBom(s.clusterBomKey, s.config.TargetClusterName, s.config.TestLandscaper)
	appConfig := builder.NewAppConfigForHelm(s.appConfigID, s.helmdata)
	builder.SetValues(appConfig, map[string]interface{}{
		"test": map[string]interface{}{
			"key1": "val11",
			"key2": []string{"val21", "val22", "val23"},
			"key4": "val41",
		},
	})
	builder.SetSecretValues(appConfig, "", map[string]interface{}{
		"test": map[string]interface{}{
			"key1": "val12",
			"key2": []string{"val24", "val25"},
			"key3": "val31",
		},
	})
	util.WriteAddAppConfig(appConfig.ID, s.helmdata)
	builder.AddAppConfig(clusterBom, appConfig)

	s.clusterBomClient.CreateClusterBom(ctx, clusterBom)
	util.CheckClusterBomCondition(ctx, clusterBom, s.clusterBomClient)

	s.assertDeployedValues(ctx, map[string]interface{}{
		"test": map[string]interface{}{
			"key1": "val12",
			"key2": []string{"val24", "val25"},
			"key3": "val31",
			"key4": "val41",
		},
	})
}

func (s *secretValuesTestSuite) runUpdateIdempotency(ctx context.Context) {
	util.WriteSubSectionHeader("Update ClusterBom with identical secret values")
	util.Write("Build ClusterBom " + s.clusterBomKey.String())
	util.WriteAddAppConfig(s.appConfigID, s.helmdata)

	clusterBom := s.getClusterBom(ctx)

	builder.SetSecretValues(&clusterBom.Spec.ApplicationConfigs[0], "", map[string]interface{}{
		"test": map[string]interface{}{
			"key1": "val12",
			"key2": []string{"val24", "val25"},
			"key3": "val31",
		},
	})

	s.updateClusterBom(ctx, clusterBom)

	util.CheckClusterBomCondition(ctx, clusterBom, s.clusterBomClient)

	s.assertDeployedValues(ctx, map[string]interface{}{
		"test": map[string]interface{}{
			"key1": "val12",
			"key2": []string{"val24", "val25"},
			"key3": "val31",
			"key4": "val41",
		},
	})
}

func (s *secretValuesTestSuite) runKeepSecretValuesByEmptySection(ctx context.Context) {
	util.WriteSubSectionHeader("Keep secret values by empty section")
	oldClusterBom := s.getClusterBom(ctx)

	util.Write("Set secret values section = nil")
	oldClusterBom.Spec.ApplicationConfigs[0].SecretValues = nil

	s.updateClusterBom(ctx, oldClusterBom)
	newClusterBom := s.getClusterBom(ctx)
	s.assertSameGeneration(oldClusterBom, newClusterBom)
}

func (s *secretValuesTestSuite) runKeepSecretValuesByKeepOperation(ctx context.Context) {
	util.WriteSubSectionHeader("Keep secret values by keep operation")
	oldClusterBom := s.getClusterBom(ctx)

	util.Write("Set secret values operation = keep")
	builder.SetSecretValues(&oldClusterBom.Spec.ApplicationConfigs[0], "keep", nil)

	s.updateClusterBom(ctx, oldClusterBom)
	newClusterBom := s.getClusterBom(ctx)
	s.assertSameGeneration(oldClusterBom, newClusterBom)
}

func (s *secretValuesTestSuite) runKeepSecretValuesByReplaceOperation(ctx context.Context) {
	util.WriteSubSectionHeader("Keep secret values by replace operation")
	oldClusterBom := s.getClusterBom(ctx)

	util.Write("Set secret values operation = replace and set same data")
	secretData := map[string]interface{}{
		"test": map[string]interface{}{
			"key1": "val12",
			"key2": []string{"val24", "val25"},
			"key3": "val31",
		},
	}
	builder.SetSecretValues(&oldClusterBom.Spec.ApplicationConfigs[0], "replace", secretData)

	s.updateClusterBom(ctx, oldClusterBom)
	newClusterBom := s.getClusterBom(ctx)
	s.assertSameGeneration(oldClusterBom, newClusterBom)
}

func (s *secretValuesTestSuite) runKeepSecretValuesByNoOperation(ctx context.Context) {
	util.WriteSubSectionHeader("Keep secret values by replace operation")
	oldClusterBom := s.getClusterBom(ctx)

	util.Write("Set secret values operation = replace and set same data")
	secretData := map[string]interface{}{
		"test": map[string]interface{}{
			"key1": "val12",
			"key2": []string{"val24", "val25"},
			"key3": "val31",
		},
	}
	builder.SetSecretValues(&oldClusterBom.Spec.ApplicationConfigs[0], "", secretData)

	s.updateClusterBom(ctx, oldClusterBom)
	newClusterBom := s.getClusterBom(ctx)
	s.assertSameGeneration(oldClusterBom, newClusterBom)
}

func (s *secretValuesTestSuite) runReplaceSecretValuesByOperation(ctx context.Context) {
	util.WriteSubSectionHeader("Replace secret values with operation")
	oldClusterBom := s.getClusterBom(ctx)
	oldSecretName := oldClusterBom.Spec.ApplicationConfigs[0].SecretValues.InternalSecretName

	util.Write("Set secret values operation = replace and set different data")
	secretData := map[string]interface{}{
		"test": map[string]interface{}{
			"key1": "val12",
			"key2": []string{"val24", "val25"},
			"key3": "val32",
		},
	}
	builder.SetSecretValues(&oldClusterBom.Spec.ApplicationConfigs[0], "replace", secretData)

	s.updateClusterBom(ctx, oldClusterBom)
	util.CheckClusterBomCondition(ctx, oldClusterBom, s.clusterBomClient)
	newClusterBom := s.getClusterBom(ctx)

	if oldSecretName == newClusterBom.Spec.ApplicationConfigs[0].SecretValues.InternalSecretName {
		util.Write("Secret name has not changed " + s.clusterBomKey.String())
		os.Exit(1)
	}

	s.assertDeployedValues(ctx, map[string]interface{}{
		"test": map[string]interface{}{
			"key1": "val12",
			"key2": []string{"val24", "val25"},
			"key3": "val32",
			"key4": "val41",
		},
	})
}

func (s *secretValuesTestSuite) runReplaceSecretValuesNoOperation(ctx context.Context) {
	util.WriteSubSectionHeader("Replace secret values no operation")
	oldClusterBom := s.getClusterBom(ctx)
	oldSecretName := oldClusterBom.Spec.ApplicationConfigs[0].SecretValues.InternalSecretName

	util.Write("Set secret values without operation and set different data")
	secretData := map[string]interface{}{
		"test": map[string]interface{}{
			"key1": "val12",
			"key2": []string{"val24", "val25"},
			"key3": "val33",
		},
	}
	builder.SetSecretValues(&oldClusterBom.Spec.ApplicationConfigs[0], "", secretData)

	s.updateClusterBom(ctx, oldClusterBom)
	util.CheckClusterBomCondition(ctx, oldClusterBom, s.clusterBomClient)
	newClusterBom := s.getClusterBom(ctx)

	if oldSecretName == newClusterBom.Spec.ApplicationConfigs[0].SecretValues.InternalSecretName {
		util.Write("Secret name has not changed " + s.clusterBomKey.String())
		os.Exit(1)
	}

	s.assertDeployedValues(ctx, map[string]interface{}{
		"test": map[string]interface{}{
			"key1": "val12",
			"key2": []string{"val24", "val25"},
			"key3": "val33",
			"key4": "val41",
		},
	})
}

func (s *secretValuesTestSuite) runDeleteSecretValues(ctx context.Context) {
	util.WriteSubSectionHeader("Delete secret values")
	oldClusterBom := s.getClusterBom(ctx)

	builder.SetSecretValues(&oldClusterBom.Spec.ApplicationConfigs[0], "delete", nil)

	s.updateClusterBom(ctx, oldClusterBom)
	util.CheckClusterBomCondition(ctx, oldClusterBom, s.clusterBomClient)
	newClusterBom := s.getClusterBom(ctx)

	if newClusterBom.Spec.ApplicationConfigs[0].SecretValues != nil {
		util.Write("Secret values not empty " + s.clusterBomKey.String())
		os.Exit(1)
	}

	s.assertDeployedValues(ctx, map[string]interface{}{
		"test": map[string]interface{}{
			"key1": "val11",
			"key2": []string{"val21", "val22", "val23"},
			"key4": "val41",
		},
	})
}

func (s *secretValuesTestSuite) runDeleteClusterBomWithSecretValues(ctx context.Context) {
	util.WriteSubSectionHeader("Delete clusterbom with secret values")
	util.Write("Build ClusterBom " + s.clusterBomKey.String())

	clusterBom := builder.NewClusterBom(s.clusterBomKey, s.config.TargetClusterName, s.config.TestLandscaper)

	appConfig := builder.NewAppConfigForHelm(s.appConfigID, s.helmdata)
	builder.SetValues(appConfig, map[string]interface{}{
		"test": map[string]interface{}{
			"key1": "val11",
			"key2": []string{"val21", "val22", "val23"},
			"key4": "val41",
		},
	})
	builder.SetSecretValues(appConfig, "", map[string]interface{}{
		"test": map[string]interface{}{
			"key1": "val12",
			"key2": []string{"val24", "val25"},
			"key3": "val31",
		},
	})
	util.WriteAddAppConfig(appConfig.ID, s.helmdata)
	builder.AddAppConfig(clusterBom, appConfig)

	s.clusterBomClient.CreateClusterBom(ctx, clusterBom)

	clusterBom = s.getClusterBom(ctx)
	secretKey := types.NamespacedName{
		Namespace: clusterBom.Namespace,
		Name:      clusterBom.Spec.ApplicationConfigs[0].SecretValues.InternalSecretName,
	}

	s.clusterBomClient.DeleteClusterBom(ctx, s.clusterBomKey)

	secret := s.getSecret(ctx, &secretKey)
	if secret != nil {
		util.Write("Secret was not deleted after clusterbom deletion")
		os.Exit(1)
	}

	util.Write("Success: Secret was deleted after clusterbom deletion")
}

func (s *secretValuesTestSuite) getClusterBom(ctx context.Context) *hubv1.ClusterBom {
	return s.clusterBomClient.GetClusterBom(ctx, s.clusterBomKey)
}

func (s *secretValuesTestSuite) getSecret(ctx context.Context, secretKey *types.NamespacedName) *v1.Secret {
	secret := v1.Secret{}
	err := s.gardenClient.Get(ctx, *secretKey, &secret)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}

		util.Write(err, "Unable to fetch secret "+secretKey.String())
		os.Exit(1)
	}
	return &secret
}

func (s *secretValuesTestSuite) updateClusterBom(ctx context.Context, clusterBom *hubv1.ClusterBom) {
	util.Write("Updating clusterbom")
	err := s.gardenClient.Update(ctx, clusterBom)
	if err != nil {
		util.Write(err, "Unable to update clusterbom "+s.clusterBomKey.String())
		os.Exit(1)
	}
}

func (s *secretValuesTestSuite) assertSameGeneration(oldClusterBom, newClusterBom *hubv1.ClusterBom) {
	if oldClusterBom.ObjectMeta.Generation == newClusterBom.ObjectMeta.Generation {
		util.Write("Success: clusterbom generation unchanged, as expected")
	} else {
		util.Write("Generation differs " + s.clusterBomKey.String())
		os.Exit(1)
	}
}

func (s *secretValuesTestSuite) assertDeployedValues(ctx context.Context, expectedDeployedValues map[string]interface{}) {
	deployedValues := s.helmClient.GetDeployedValues(ctx, s.helmdata.InstallName, s.helmdata.Namespace)
	deployedValuesJSON := s.marshal(deployedValues)
	expectedValuesJSON := s.marshal(expectedDeployedValues)

	if deployedValuesJSON == expectedValuesJSON {
		util.Write("Success: Merged secret values are as expected")
	} else {
		util.Write("Merged secret values differ")
		util.Write("Expected values: " + expectedValuesJSON)
		util.Write("Deployed values: " + deployedValuesJSON)
		os.Exit(1)
	}
}

func (s *secretValuesTestSuite) marshal(values map[string]interface{}) string {
	marshaledValues, err := json.Marshal(values)
	if err != nil {
		util.Write(err, "Unable to marshal values")
		os.Exit(1)
	}
	return string(marshaledValues)
}
