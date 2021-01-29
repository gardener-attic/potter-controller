package admission

import (
	"encoding/json"
	"regexp"

	"github.com/gardener/potter-controller/api/apitypes"
	helmref "github.com/gardener/potter-controller/pkg/helm"

	"k8s.io/apimachinery/pkg/runtime"
)

type helmReviewer struct{}

func newHelmReviewer() *helmReviewer {
	return &helmReviewer{}
}

func (r *helmReviewer) reviewTypeSpecificData(report *report, typeSpecificData, oldTypeSpecificData *runtime.RawExtension) {
	ok, message := r.checkHelmSpecificData(typeSpecificData, oldTypeSpecificData)
	if !ok {
		report.deny(message)
		return
	}
}

func (r *helmReviewer) checkHelmSpecificData(typeSpecificData, oldTypeSpecificData *runtime.RawExtension) (bool, string) {
	var helmData apitypes.HelmSpecificData
	var oldHelmData apitypes.HelmSpecificData

	err := json.Unmarshal(typeSpecificData.Raw, &helmData)
	if err != nil {
		return false, "error typeSpecificData - " + err.Error()
	}

	if oldTypeSpecificData != nil {
		err := json.Unmarshal(oldTypeSpecificData.Raw, &oldHelmData)
		if err != nil {
			return false, "error when unmarshalling old typeSpecificData - " + err.Error()
		}
	}

	if ok, message := r.checkInstallationNameAndNamespace(&helmData); !ok {
		return false, message
	}

	if ok, message := r.checkTimeouts(&helmData); !ok {
		return false, message
	}

	if helmData.TarballAccess == nil && helmData.CatalogAccess == nil {
		return false, "either helm.tarballAccess or helm.catalogAccess must be set"
	} else if helmData.TarballAccess != nil && helmData.CatalogAccess != nil {
		return false, "not both helm.catalogAccess and helm.tarballAccess are allowed to have entries"
	}

	if ok, message := r.checkTarballAccess(&helmData); !ok {
		return false, message
	}

	if ok, message := r.checkCatalogAccess(&helmData); !ok {
		return false, message
	}

	// during an update, only the version is allowed to be changed
	if oldTypeSpecificData != nil {
		if helmData.InstallName != oldHelmData.InstallName {
			return false, "helm.installName must not be updated"
		}

		if helmData.Namespace != oldHelmData.Namespace {
			return false, "helm.namespace must not be updated"
		}

		if helmData.CatalogAccess != nil && oldHelmData.CatalogAccess == nil {
			return false, "switch to catalogAccess not allowed"
		} else if helmData.CatalogAccess == nil && oldHelmData.CatalogAccess != nil {
			return false, "switch to tarballAccess not allowed"
		} else if helmData.CatalogAccess != nil {
			if helmData.CatalogAccess.ChartName != oldHelmData.CatalogAccess.ChartName {
				return false, "helm.catalogAccess.chartName must not be changed"
			}

			if helmData.CatalogAccess.Repo != oldHelmData.CatalogAccess.Repo {
				return false, "helm.catalogAccess.repo must not be changed"
			}
		}
	}

	if ok, message := r.checkArguments(&helmData); !ok {
		return false, message
	}

	return true, ""
}

func (r *helmReviewer) checkInstallationNameAndNamespace(helmData *apitypes.HelmSpecificData) (bool, string) {
	if helmData.InstallName == "" {
		return false, "helm.installName is empty"
	}

	if containsSpiffTemplate(helmData.InstallName) {
		return false, "helm.installName cannot be templated"
	}

	if helmData.Namespace == "" {
		return false, "helm.namespace is empty"
	}

	if containsSpiffTemplate(helmData.Namespace) {
		return false, "helm.namespace cannot be templated"
	}

	return true, ""
}

func (r *helmReviewer) checkTimeouts(helmData *apitypes.HelmSpecificData) (bool, string) {
	if helmData.InstallTimeout != nil && *helmData.InstallTimeout <= 0 {
		return false, "helm.installTimeout must be larger than 0"
	}

	if helmData.UpgradeTimeout != nil && *helmData.UpgradeTimeout <= 0 {
		return false, "helm.upgradeTimeout must be larger than 0"
	}

	if helmData.RollbackTimeout != nil && *helmData.RollbackTimeout <= 0 {
		return false, "helm.rollbackTimeout must be larger than 0"
	}

	if helmData.UninstallTimeout != nil && *helmData.UninstallTimeout <= 0 {
		return false, "helm.uninstallTimeout must be larger than 0"
	}

	return true, ""
}

func (r *helmReviewer) checkCatalogAccess(helmData *apitypes.HelmSpecificData) (bool, string) {
	if helmData.CatalogAccess == nil {
		return true, ""
	}

	if helmData.CatalogAccess.Repo == "" {
		return false, "helm.catalogAccess.repo missing"
	}

	if containsSpiffTemplate(helmData.CatalogAccess.Repo) {
		return false, "helm.catalogAccess.repo cannot be templated"
	}

	if helmData.CatalogAccess.ChartName == "" {
		return false, "helm.catalogAccess.chartName missing"
	}

	if containsSpiffTemplate(helmData.CatalogAccess.ChartName) {
		return false, "helm.catalogAccess.chartName cannot be templated"
	}

	if helmData.CatalogAccess.ChartVersion == "" {
		return false, "helm.catalogAccess.chartVersion missing"
	}

	pattern := `^(?P<major>0|[1-9]\d*)\.(?P<minor>0|[1-9]\d*)\.(?P<patch>0|[1-9]\d*)(?:-(?P<prerelease>(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+(?P<buildmetadata>[0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`
	matched, err := regexp.MatchString(pattern, helmData.CatalogAccess.ChartVersion)
	if err != nil {
		return false, "error when matching helm.catalogAccess.chartVersion against pattern " + pattern + " - " + err.Error()
	}

	if !matched {
		return false, "helm.catalogAccess.chartVersion does not fulfill the pattern " + pattern
	}

	return true, ""
}

func (r *helmReviewer) checkTarballAccess(helmData *apitypes.HelmSpecificData) (bool, string) {
	if helmData.TarballAccess == nil {
		return true, ""
	}

	if helmData.TarballAccess.URL == "" {
		return false, "helm.tarballAccess.url missing"
	}

	if containsSpiffTemplate(helmData.TarballAccess.URL) {
		return false, "helm.tarballAccess.url cannot be templated"
	}

	if helmData.TarballAccess.AuthHeader != "" && helmData.TarballAccess.SecretRef.Name != "" {
		return false, "helm.tarballAccess.authHeader and helm.tarballAccess.secretRef.name both set"
	}

	return true, ""
}

func (r *helmReviewer) checkArguments(helmData *apitypes.HelmSpecificData) (bool, string) {
	for _, argument := range helmData.InstallArguments {
		if argument != helmref.InstallArgAtomic {
			return false, "install argument not supported: " + argument
		}
	}

	for _, argument := range helmData.UpdateArguments {
		if argument != helmref.UpdateArgAtomic {
			return false, "update argument not supported: " + argument
		}
	}

	for _, argument := range helmData.RemoveArguments {
		return false, "remove argument not supported: " + argument
	}

	return true, ""
}
