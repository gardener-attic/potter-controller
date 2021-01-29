package helm

import (
	"bytes"
	"io"

	"github.wdf.sap.corp/kubernetes/hub-controller/pkg/deployutil"

	"gopkg.in/yaml.v3"
)

func unmarshalManifest(manifest *string, filter func(object *deployutil.BasicKubernetesObject) bool) ([]deployutil.BasicKubernetesObject, error) {
	var basicKubernetesObjects []deployutil.BasicKubernetesObject

	decoder := yaml.NewDecoder(bytes.NewReader([]byte(*manifest)))

	for {
		obj := deployutil.BasicKubernetesObject{}

		err := decoder.Decode(&obj)

		if err != nil {
			switch err {
			case io.EOF:
				return basicKubernetesObjects, nil
			default:
				return nil, err
			}
		}

		if filter(&obj) {
			basicKubernetesObjects = append(basicKubernetesObjects, obj)
		}
	}
}
