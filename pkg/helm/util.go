package helm

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.wdf.sap.corp/kubernetes/hub-controller/api/apitypes"
	appRepov1 "github.wdf.sap.corp/kubernetes/hub-controller/api/external/apprepository/v1alpha1"
	hubv1 "github.wdf.sap.corp/kubernetes/hub-controller/api/v1"
	"github.wdf.sap.corp/kubernetes/hub-controller/pkg/util"

	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ReleaseMetadata struct {
	BomName string `json:"bomName"`
}

func ParseTypeSpecificData(ctx context.Context, namedSecretResolver *apitypes.NamedSecretResolver, hdc *hubv1.DeploymentConfig,
	helmSpecificData *apitypes.HelmSpecificData, loadRepoInfo bool, appRepoClient client.Client) (*ChartData, string, error) {
	var values map[string]interface{}
	if hdc.Values != nil {
		err := json.Unmarshal(hdc.Values.Raw, &values)
		if err != nil {
			return nil, "", err
		}
	}

	var chartData *ChartData
	var load ChartLoaderFunc

	if loadRepoInfo {
		if helmSpecificData.TarballAccess != nil {
			tarballAccess := helmSpecificData.TarballAccess

			var customCaData string
			if tarballAccess.CustomCAData != "" {
				encodedCaData := tarballAccess.CustomCAData

				var customCaDataBytes []byte
				customCaDataBytes, err := base64.StdEncoding.DecodeString(encodedCaData)
				if err != nil {
					return nil, "", errors.Wrap(err, "could not decode customCAData")
				}
				customCaData = string(customCaDataBytes)
			}

			authHeader, err := helmSpecificData.GetAuthHeader(ctx, namedSecretResolver)
			if err != nil {
				return nil, "", err
			}

			chartURL := tarballAccess.URL

			load = LoadRawURL(ctx, customCaData, authHeader, chartURL, loader.LoadArchive)
		} else if helmSpecificData.CatalogAccess != nil {
			chartName := helmSpecificData.CatalogAccess.ChartName
			chartVersion := helmSpecificData.CatalogAccess.ChartVersion
			repo := helmSpecificData.CatalogAccess.Repo

			apprepo, err := getApprepository(ctx, repo, appRepoClient)
			if err != nil {
				return nil, "", err
			}

			load = LoadCatalogChart(ctx, apprepo, chartName, chartVersion, loader.LoadArchive, appRepoClient)
		} else {
			return nil, "", errors.New("could not find property catalogAccess or tarballAccess")
		}
	}

	chartData = &ChartData{
		InstallName:      helmSpecificData.InstallName,
		Values:           values,
		Load:             load,
		InstallTimeout:   helmSpecificData.GetInstallTimeout(),
		UpgradeTimeout:   helmSpecificData.GetUpgradeTimeout(),
		RollbackTimeout:  helmSpecificData.GetRollbackTimeout(),
		UninstallTimeout: helmSpecificData.GetUninstallTimeout(),
		InstallArguments: helmSpecificData.InstallArguments,
		UpdateArguments:  helmSpecificData.UpdateArguments,
		RemoveArguments:  helmSpecificData.RemoveArguments,
	}

	return chartData, helmSpecificData.Namespace, nil
}

func getOptionalStringSlicePropSafe(propName string, propMap map[string]interface{}) ([]string, error) {
	value, ok := propMap[propName] // for example propName="installArguments" and value=["atomic"]
	if !ok {
		return nil, nil
	}

	valueAsSlice, ok := value.([]interface{})
	if !ok {
		msg := fmt.Sprintf("property %s is of type %T, not []interface{}", propName, value)
		return nil, errors.New(msg)
	}

	valueAsSliceOfStrings := make([]string, len(valueAsSlice))

	for i, item := range valueAsSlice {
		itemAsString, ok := item.(string)
		if !ok {
			msg := fmt.Sprintf("item %v of property %s is of type %T, not string", i, propName, item)
			return nil, errors.New(msg)
		}

		valueAsSliceOfStrings[i] = itemAsString
	}

	return valueAsSliceOfStrings, nil
}

type ChartData struct {
	InstallName      string
	Values           map[string]interface{}
	Load             ChartLoaderFunc
	InstallTimeout   time.Duration
	UpgradeTimeout   time.Duration
	RollbackTimeout  time.Duration
	UninstallTimeout time.Duration
	InstallArguments []string
	UpdateArguments  []string
	RemoveArguments  []string
}

type ChartLoaderFunc func() (*chart.Chart, error)

func IsReleaseNotFoundErr(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "release: not found")
}

func IsClusterUnreachableErr(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "kubernetes cluster unreachable")
}

func getApprepository(ctx context.Context, appRepoName string, appRepoClient client.Client) (*appRepov1.AppRepository, error) {
	namespace := util.GetApprepoNamespace()

	// We grab the specified app repository (for later access to the repo URL, as well as any specified auth).
	var appRepo appRepov1.AppRepository

	appRepoKey := types.NamespacedName{
		Namespace: namespace,
		Name:      appRepoName,
	}

	err := appRepoClient.Get(ctx, appRepoKey, &appRepo)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to get app repository %s", appRepoName)
	}

	return &appRepo, nil
}
