package kapp

import (
	"reflect"
	"testing"

	"github.com/arschles/assert"
)

func TestReplace(t *testing.T) {
	obj := map[string]interface{}{
		secretRef: map[string]interface{}{
			"name": "log1",
		},
		"k0": map[string]interface{}{
			"name": "log1",
		},
		"k1": "log1",
		"k2": map[string]interface{}{
			secretRef: map[string]interface{}{
				"name": "log2",
			},
			"k3": []interface{}{
				123,
				false,
				secretRef,
				map[string]interface{}{
					secretRef: map[string]interface{}{
						"name": "log3",
					},
					"k4": "log3",
				},
				map[string]interface{}{
					secretRef: map[string]interface{}{
						"name": "without-mapping",
					},
				},
				"log4",
			},
		},
	}

	expectedResult := map[string]interface{}{
		secretRef: map[string]interface{}{
			"name": "int1",
		},
		"k0": map[string]interface{}{
			"name": "log1",
		},
		"k1": "log1",
		"k2": map[string]interface{}{
			secretRef: map[string]interface{}{
				"name": "int2",
			},
			"k3": []interface{}{
				123,
				false,
				secretRef,
				map[string]interface{}{
					secretRef: map[string]interface{}{
						"name": "int3",
					},
					"k4": "log3",
				},
				map[string]interface{}{
					secretRef: map[string]interface{}{
						"name": "without-mapping",
					},
				},
				"log4",
			},
		},
	}

	mapping := map[string]string{
		"log1": "int1",
		"log2": "int2",
		"log3": "int3",
		"log4": "int4",
	}

	kappDeployer := kappDeployerDI{}
	kappDeployer.replace(obj, mapping, "")

	assert.True(t, reflect.DeepEqual(obj, expectedResult), "wrong result of secret name replacement")
}
