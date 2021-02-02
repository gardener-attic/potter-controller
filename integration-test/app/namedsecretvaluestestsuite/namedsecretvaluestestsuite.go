package namedsecretvaluestestsuite

import (
	"context"
	"encoding/json"
	"os"
	"strconv"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/potter-controller/api/apitypes"
	hubv1 "github.com/gardener/potter-controller/api/v1"
	"github.com/gardener/potter-controller/integration-test/app/builder"
	"github.com/gardener/potter-controller/integration-test/app/catalog"
	"github.com/gardener/potter-controller/integration-test/app/helm"
	"github.com/gardener/potter-controller/integration-test/app/util"
)

func Run(ctx context.Context, config *util.IntegrationTestConfig, gardenClient client.Client) {
	util.WriteSectionHeader("Named Secret Values Test", config.TestLandscaper)

	s := newNamedSecretValuesTestSuite(ctx, config, gardenClient)

	s.cleanup(ctx)

	s.runCreateSecretValues(ctx)
	s.runUpdateIdempotency(ctx)
	s.runKeepSecretValuesByEmptySection(ctx)
	s.runKeepSecretValuesByNoOperation(ctx)
	s.runReplaceSecretValuesOne(ctx)
	s.runReplaceSecretValuesTwo(ctx)
	s.runDeleteFirstSecretValues(ctx)
	// test if operation is idempotent
	s.runDeleteFirstSecretValues(ctx)

	s.runDeleteClusterBom(ctx)
}

func (s *namedSecretValuesTestSuite) runDeleteClusterBom(ctx context.Context) {
	util.WriteSubSectionHeader("Delete clusterbom ")

	clusterBom := s.getClusterBom(ctx)

	secretKey := types.NamespacedName{
		Namespace: clusterBom.Namespace,
		Name:      clusterBom.Spec.ApplicationConfigs[0].NamedSecretValues["test2"].InternalSecretName,
	}

	util.Write("Delete clusterbom")

	s.clusterBomClient.DeleteClusterBom(ctx, s.clusterBomKey)

	secret := s.getSecret(ctx, &secretKey)
	if secret != nil {
		util.Write("Secret was not deleted after clusterbom deletion")
		os.Exit(1)
	}

	util.Write("Success: Secret was deleted after clusterbom deletion")
}

func (s *namedSecretValuesTestSuite) getSecret(ctx context.Context, secretKey *types.NamespacedName) *v1.Secret {
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

func (s *namedSecretValuesTestSuite) runDeleteFirstSecretValues(ctx context.Context) {
	util.WriteSubSectionHeader("Delete first secret value")
	oldClusterBom := s.getClusterBom(ctx)

	util.Write("Delete named secret values")

	oldClusterBom.Spec.ApplicationConfigs[0].NamedSecretValues = map[string]hubv1.NamedSecretValues{
		"test1": hubv1.NamedSecretValues{
			Operation: "delete",
			StringData: map[string]string{
				"key1": "val14",
				"key2": "[val24, val25]",
				"key3": "val31",
			},
		},
	}

	s.updateClusterBom(ctx, oldClusterBom)
	util.CheckClusterBomCondition(ctx, oldClusterBom, s.clusterBomClient)

	s.assertDeployedValues(ctx, map[string]interface{}{
		"key1": "val11",
		"key2": []string{"val21", "val22", "val23"},
		"key4": "val41",
		"key5": "val54",
	})

	util.Write("Generation: " + strconv.FormatInt(s.getClusterBom(ctx).Generation, 10))
}

func (s *namedSecretValuesTestSuite) runReplaceSecretValuesTwo(ctx context.Context) {
	util.WriteSubSectionHeader("Replace named secret values no operation")
	oldClusterBom := s.getClusterBom(ctx)

	util.Write("Set named secret values no operation")

	oldClusterBom.Spec.ApplicationConfigs[0].NamedSecretValues = map[string]hubv1.NamedSecretValues{
		"test1": hubv1.NamedSecretValues{
			StringData: map[string]string{
				"key1": "val14",
				"key2": "[val24, val25]",
				"key3": "val36",
			},
		},
		"test2": hubv1.NamedSecretValues{
			StringData: map[string]string{
				"key5": "val54",
			},
		},
	}

	s.updateClusterBom(ctx, oldClusterBom)
	util.CheckClusterBomCondition(ctx, oldClusterBom, s.clusterBomClient)

	s.assertDeployedValues(ctx, map[string]interface{}{
		"key1": "val14",
		"key2": []string{"val24", "val25"},
		"key3": "val36",
		"key4": "val41",
		"key5": "val54",
	})

	util.Write("Generation: " + strconv.FormatInt(s.getClusterBom(ctx).Generation, 10))
}

func (s *namedSecretValuesTestSuite) runReplaceSecretValuesOne(ctx context.Context) {
	util.WriteSubSectionHeader("Replace named secret values by operation")
	oldClusterBom := s.getClusterBom(ctx)

	util.Write("Set named secret values with replace operation")

	oldClusterBom.Spec.ApplicationConfigs[0].NamedSecretValues = map[string]hubv1.NamedSecretValues{
		"test1": hubv1.NamedSecretValues{
			StringData: map[string]string{
				"key1": "val13",
				"key2": "[val24, val25]",
				"key3": "val33",
			},
		},
		"test2": hubv1.NamedSecretValues{
			StringData: map[string]string{
				"key5": "val53",
			},
		},
	}

	s.updateClusterBom(ctx, oldClusterBom)
	util.CheckClusterBomCondition(ctx, oldClusterBom, s.clusterBomClient)

	s.assertDeployedValues(ctx, map[string]interface{}{
		"key1": "val13",
		"key2": []string{"val24", "val25"},
		"key3": "val33",
		"key4": "val41",
		"key5": "val53",
	})

	util.Write("Generation: " + strconv.FormatInt(s.getClusterBom(ctx).Generation, 10))
}

func (s *namedSecretValuesTestSuite) runKeepSecretValuesByNoOperation(ctx context.Context) {
	util.WriteSubSectionHeader("Keep secret values by no operation")
	oldClusterBom := s.getClusterBom(ctx)

	util.Write("Set secret values no operation")

	oldClusterBom.Spec.ApplicationConfigs[0].NamedSecretValues = map[string]hubv1.NamedSecretValues{
		"test1": hubv1.NamedSecretValues{
			StringData: map[string]string{
				"key1": "val12",
				"key2": "[val24, val25]",
				"key3": "val31",
			},
		},
		"test2": hubv1.NamedSecretValues{
			StringData: map[string]string{
				"key5": "val52",
			},
		},
	}

	s.updateClusterBom(ctx, oldClusterBom)
	newClusterBom := s.getClusterBom(ctx)
	s.assertSameGeneration(oldClusterBom, newClusterBom)

	util.Write("Generation: " + strconv.FormatInt(s.getClusterBom(ctx).Generation, 10))
}

func (s *namedSecretValuesTestSuite) runKeepSecretValuesByEmptySection(ctx context.Context) {
	util.WriteSubSectionHeader("Keep named secret values by empty section")
	oldClusterBom := s.getClusterBom(ctx)

	util.Write("Set secret values section = nil")
	oldClusterBom.Spec.ApplicationConfigs[0].NamedSecretValues = nil

	s.updateClusterBom(ctx, oldClusterBom)
	newClusterBom := s.getClusterBom(ctx)
	s.assertSameGeneration(oldClusterBom, newClusterBom)

	util.Write("Generation: " + strconv.FormatInt(s.getClusterBom(ctx).Generation, 10))
}

func (s *namedSecretValuesTestSuite) assertSameGeneration(oldClusterBom, newClusterBom *hubv1.ClusterBom) {
	if oldClusterBom.ObjectMeta.Generation == newClusterBom.ObjectMeta.Generation {
		util.Write("Success: clusterbom generation unchanged, as expected")
	} else {
		util.Write("Generation differs " + s.clusterBomKey.String())
		os.Exit(1)
	}
}

type namedSecretValuesTestSuite struct {
	config           *util.IntegrationTestConfig
	gardenClient     client.Client
	helmClient       *helm.HelmClient
	clusterBomClient *util.ClusterBomClient
	clusterBomKey    types.NamespacedName
	appConfigID      string
	helmdata         *apitypes.HelmSpecificData
}

func newNamedSecretValuesTestSuite(ctx context.Context, config *util.IntegrationTestConfig, gardenClient client.Client) *namedSecretValuesTestSuite {
	clusterBomKey := config.GetTestClusterBomKey("namedvalues")
	helmdata := builder.NewHelmDataWithCatalogAccess(util.CreateEchoServerInstallationName(config), config.TargetClusterNamespace1, &catalog.ChartEcho2)

	return &namedSecretValuesTestSuite{
		config:           config,
		gardenClient:     gardenClient,
		helmClient:       helm.NewHelmClient(ctx, config, gardenClient),
		clusterBomClient: util.NewClusterBomClient(gardenClient),
		clusterBomKey:    clusterBomKey,
		appConfigID:      "echo",
		helmdata:         helmdata,
	}
}

func (s *namedSecretValuesTestSuite) cleanup(ctx context.Context) {
	util.WriteSubSectionHeader("Cleanup")
	s.clusterBomClient.DeleteClusterBom(ctx, s.clusterBomKey)
}

func (s *namedSecretValuesTestSuite) runCreateSecretValues(ctx context.Context) {
	util.WriteSubSectionHeader("Create ClusterBom with named secret values")
	util.Write("Build ClusterBom " + s.clusterBomKey.String())

	clusterBom := builder.NewClusterBom(s.clusterBomKey, s.config.TargetClusterName, s.config.TestLandscaper)
	appConfig := builder.NewAppConfigForHelm(s.appConfigID, s.helmdata)
	builder.SetValues(appConfig, map[string]interface{}{
		"key1": "val11",
		"key2": []string{"val21", "val22", "val23"},
		"key4": "val41",
	})
	appConfig.NamedSecretValues = map[string]hubv1.NamedSecretValues{
		"test1": hubv1.NamedSecretValues{
			StringData: map[string]string{
				"key1": "val12",
				"key2": "[val24, val25]",
				"key3": "val31",
			},
		},
		"test2": hubv1.NamedSecretValues{
			StringData: map[string]string{
				"key5": "val52",
			},
		},
	}

	util.WriteAddAppConfig(appConfig.ID, s.helmdata)
	builder.AddAppConfig(clusterBom, appConfig)

	s.clusterBomClient.CreateClusterBom(ctx, clusterBom)
	util.CheckClusterBomCondition(ctx, clusterBom, s.clusterBomClient)

	s.assertDeployedValues(ctx, map[string]interface{}{
		"key1": "val12",
		"key2": []string{"val24", "val25"},
		"key3": "val31",
		"key4": "val41",
		"key5": "val52",
	})

	util.Write("Generation: " + strconv.FormatInt(s.getClusterBom(ctx).Generation, 10))
}

func (s *namedSecretValuesTestSuite) runUpdateIdempotency(ctx context.Context) {
	util.WriteSubSectionHeader("Update ClusterBom with identical named secret values")
	util.Write("Build ClusterBom " + s.clusterBomKey.String())
	util.WriteAddAppConfig(s.appConfigID, s.helmdata)

	clusterBom := s.getClusterBom(ctx)
	clusterBom.Spec.ApplicationConfigs[0].NamedSecretValues = map[string]hubv1.NamedSecretValues{
		"test1": {
			StringData: map[string]string{
				"key1": "val12",
				"key2": "[val24, val25]",
				"key3": "val31",
			},
		},
		"test2": {
			StringData: map[string]string{
				"key5": "val52",
			},
		},
	}

	s.updateClusterBom(ctx, clusterBom)
	util.CheckClusterBomCondition(ctx, clusterBom, s.clusterBomClient)

	s.assertDeployedValues(ctx, map[string]interface{}{
		"key1": "val12",
		"key2": []string{"val24", "val25"},
		"key3": "val31",
		"key4": "val41",
		"key5": "val52",
	})

	util.Write("Generation: " + strconv.FormatInt(s.getClusterBom(ctx).Generation, 10))
}

func (s *namedSecretValuesTestSuite) updateClusterBom(ctx context.Context, clusterBom *hubv1.ClusterBom) {
	util.Write("Updating clusterbom")

	err := s.gardenClient.Update(ctx, clusterBom)
	if err != nil {
		util.WriteClusterBom(clusterBom)
		util.Write(err, "")
		tmp := s.getClusterBom(ctx)
		util.WriteClusterBom(tmp)

		util.Write(err, "Unable to update clusterbom "+s.clusterBomKey.String())
		os.Exit(1)
	}
}

func (s *namedSecretValuesTestSuite) assertDeployedValues(ctx context.Context, expectedDeployedValues map[string]interface{}) {
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

func (s *namedSecretValuesTestSuite) marshal(values map[string]interface{}) string {
	marshaledValues, err := json.Marshal(values)
	if err != nil {
		util.Write(err, "Unable to marshal values")
		os.Exit(1)
	}
	return string(marshaledValues)
}

func (s *namedSecretValuesTestSuite) getClusterBom(ctx context.Context) *hubv1.ClusterBom {
	return s.clusterBomClient.GetClusterBom(ctx, s.clusterBomKey)
}
