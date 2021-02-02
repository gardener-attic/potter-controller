package util

import (
	"encoding/json"

	"github.com/gardener/potter-controller/api/apitypes"
	hubv1 "github.com/gardener/potter-controller/api/v1"

	log "github.com/sirupsen/logrus"
)

func WriteSectionHeader(title string, landscaperManaged bool) {
	linebreak := "\n"
	log.Println(linebreak)
	if landscaperManaged {
		log.Println("=== " + title + " (Landscaper Managed)")
	} else {
		log.Println("=== " + title)
	}
}
func WriteSubSectionHeader(title string) {
	log.Println("--- " + title)
}

func Write(args ...interface{}) {
	log.Println(args...)
}

func WriteClusterBom(clusterbom *hubv1.ClusterBom) {
	data, err := json.Marshal(clusterbom)
	if err != nil {
		Write("Cannot marshal ClusterBom")
	}
	Write("ClusterBom: ", string(data))
}

func WriteAddAppConfig(appConfigID string, helmData *apitypes.HelmSpecificData) {
	Write("Add appconfig " + appConfigID + " (" + displayHelmData(helmData) + ")")
}

func WriteUpgradeAppConfig(appConfigID string, helmData *apitypes.HelmSpecificData) {
	Write("Upgrade appconfig " + appConfigID + " (" + displayHelmData(helmData) + ")")
}

func displayHelmData(helmData *apitypes.HelmSpecificData) string {
	if helmData.CatalogAccess != nil {
		return displayCatalogAccess(helmData.CatalogAccess)
	} else if helmData.TarballAccess != nil {
		return displayTarballAccess(helmData.TarballAccess)
	}
	return ""
}

func displayCatalogAccess(catalogAccess *apitypes.CatalogAccess) string {
	return catalogAccess.Repo + "/" + catalogAccess.ChartName + " " + catalogAccess.ChartVersion
}

func displayTarballAccess(tarballAccess *apitypes.TarballAccess) string {
	return tarballAccess.URL
}
