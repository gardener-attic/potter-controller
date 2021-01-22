package avcheck

import (
	"strings"

	"github.com/pkg/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hubv1 "github.com/gardener/potter-controller/api/v1"
	"github.com/gardener/potter-controller/pkg/util"
)

func BuildBom(namespace, bomName, secretRef, installNamespace, tarballURL, catalogTriple string) (*hubv1.ClusterBom, error) {
	// contains all application configs of the availability check bom
	type applicationConfigType = struct {
		ID               string
		ConfigType       string
		TypeSpecificData map[string]interface{}
		Values           map[string]interface{}
	}

	var applicationConfigs = []applicationConfigType{
		{
			ID:         "helm",
			ConfigType: util.ConfigTypeHelm,
			TypeSpecificData: map[string]interface{}{
				"installName": "app-0",
				"namespace":   installNamespace,
			},
			Values: map[string]interface{}{
				switchKey: true,
			},
		},
	}
	if tarballURL == "" && catalogTriple == "" {
		applicationConfigs[0].TypeSpecificData["catalogAccess"] = map[string]interface{}{
			"chartName":    "echo-server",
			"repo":         "sap-incubator",
			"chartVersion": "1.0.1",
		}
	} else if catalogTriple != "" {
		parts := strings.Split(catalogTriple, ":")
		if len(parts) == 3 {
			applicationConfigs[0].TypeSpecificData["catalogAccess"] = map[string]interface{}{
				"chartName":    parts[0],
				"repo":         parts[1],
				"chartVersion": parts[2],
			}
		} else {
			return nil, errors.Errorf("invalid config string for catalogAccess %s must be chartName:repo:version", catalogTriple)
		}
	} else {
		applicationConfigs[0].TypeSpecificData["tarballAccess"] = map[string]interface{}{
			"url": tarballURL,
			// "customCAData": "...",
			// "authHeader": "Basic blablubb",
		}
	}

	var bom = hubv1.ClusterBom{
		ObjectMeta: v1.ObjectMeta{
			Namespace: namespace,
			Name:      bomName,
		},
		Spec: hubv1.ClusterBomSpec{
			SecretRef:          secretRef,
			ApplicationConfigs: []hubv1.ApplicationConfig{},
		},
	}

	for _, applicationConfig := range applicationConfigs {
		rawValues, err := util.CreateRawExtension(applicationConfig.Values)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot create raw extension from values. applicationConfig = %+v", applicationConfig)
		}

		rawTypeSpecificData, err := util.CreateRawExtension(applicationConfig.TypeSpecificData)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot create raw extension from typeSpecificData. applicationConfig = %+v", applicationConfig)
		}

		applConfig := hubv1.ApplicationConfig{
			ID:               applicationConfig.ID,
			ConfigType:       applicationConfig.ConfigType,
			TypeSpecificData: *rawTypeSpecificData,
			Values:           rawValues,
		}

		bom.Spec.ApplicationConfigs = append(bom.Spec.ApplicationConfigs, applConfig)
	}

	return &bom, nil
}
