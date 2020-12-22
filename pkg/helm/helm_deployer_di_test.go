package helm

import (
	"github.com/arschles/assert"
	"sigs.k8s.io/yaml"

	"testing"
)

func TestMergeValues(t *testing.T) {
	s := "yellow"
	rawValue := []byte(s)
	var value interface{}
	err := yaml.Unmarshal(rawValue, &value)
	assert.Nil(t, err, "error")
	assert.Equal(t, value, "yellow", "value")
}
