package util

import "k8s.io/apimachinery/pkg/types"

type IntegrationTestConfig struct {
	GardenKubeconfigPath    string
	GardenNamespace         string
	TargetClusterName       string
	TargetClusterNamespace1 string
	TargetClusterNamespace2 string
	TestPrefix              string
	TestLandscaper          bool
}

func (c *IntegrationTestConfig) GetTestClusterBomKey(suffix string) types.NamespacedName {
	if c.TestLandscaper {
		return types.NamespacedName{
			Namespace: c.GardenNamespace,
			Name:      c.TestPrefix + "test.ls." + suffix,
		}
	}

	return types.NamespacedName{
		Namespace: c.GardenNamespace,
		Name:      c.TestPrefix + "test." + suffix,
	}
}
