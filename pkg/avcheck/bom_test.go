package avcheck

import (
	"encoding/json"
	"testing"

	"github.com/arschles/assert"

	hubv1 "github.wdf.sap.corp/kubernetes/hub-controller/api/v1"
)

const (
	namespace            = "test-namespace"
	name                 = "test-bom"
	secretRef            = "mycluster.kubeconfig"
	installNamespace     = "install-here"
	testURL              = "http://acme.com/mycoolchart.tgz"
	customCatalogChart   = "CoolChart"
	customCatalogRepo    = "sap-greatest"
	customCatalogVersion = "1.2.3"
	catalogTriple        = customCatalogChart + ":" + customCatalogRepo + ":" + customCatalogVersion
)

func TestBuildBom(t *testing.T) {
	bom, err := BuildBom(namespace, name, secretRef, installNamespace, "", "")
	assert.NoErr(t, err)
	testBuildBom(t, bom)
}

func TestBuildBomTarball(t *testing.T) {
	var tarballURL = testURL
	bom, err := BuildBom(namespace, name, secretRef, installNamespace, tarballURL, "")

	assert.NoErr(t, err)
	assert.NoErr(t, err)
	testBuildBom(t, bom)

	var tba map[string]interface{}
	var values map[string]interface{}
	err = json.Unmarshal(bom.Spec.ApplicationConfigs[0].TypeSpecificData.Raw, &values)
	assert.NoErr(t, err)
	tba = values["tarballAccess"].(map[string]interface{})
	assert.Equal(t, tba["url"], testURL, "tarball URL")
}

func TestBuildBomCustomCatalog(t *testing.T) {
	var catalogDef = catalogTriple
	bom, err := BuildBom(namespace, name, secretRef, installNamespace, "", catalogDef)

	assert.NoErr(t, err)
	assert.NoErr(t, err)
	testBuildBom(t, bom)

	var cat map[string]interface{}
	var values map[string]interface{}
	err = json.Unmarshal(bom.Spec.ApplicationConfigs[0].TypeSpecificData.Raw, &values)
	assert.NoErr(t, err)
	cat = values["catalogAccess"].(map[string]interface{})

	assert.Equal(t, cat["chartName"], customCatalogChart, "Catalog Chart Name")
	assert.Equal(t, cat["repo"], customCatalogRepo, "Catalog repo")
	assert.Equal(t, cat["chartVersion"], customCatalogVersion, "Catalog Chart Version")
}

func testBuildBom(t *testing.T, bom *hubv1.ClusterBom) {
	assert.Equal(t, bom.GetNamespace(), namespace, "namespace")
	assert.Equal(t, bom.GetName(), name, "name")
	assert.True(t, len(bom.Spec.ApplicationConfigs) > 0, "bom is empty")

	for i := range bom.Spec.ApplicationConfigs {
		applicationConfig := bom.Spec.ApplicationConfigs[i]
		var values map[string]interface{}

		err := json.Unmarshal(applicationConfig.Values.Raw, &values)
		assert.NoErr(t, err)

		flipStr, ok := values[switchKey]
		assert.True(t, ok, "the values of each applicationConfig must contain the key %s. applicationConfig = %+v", switchKey, applicationConfig)

		_, ok = flipStr.(bool)
		assert.True(t, ok, "the value for key %s must be of type bool. applicationConfig = %+v", switchKey, applicationConfig)
	}
}
