package apitypes

import (
	"context"
	"encoding/json"
	"time"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
)

const defaultTimeout = 5 * time.Minute

type HelmSpecificData struct { // nolint
	InstallName   string         `json:"installName,omitempty"`
	Namespace     string         `json:"namespace,omitempty"`
	CatalogAccess *CatalogAccess `json:"catalogAccess,omitempty"`
	TarballAccess *TarballAccess `json:"tarballAccess,omitempty"`

	InstallTimeout   *int64 `json:"installTimeout,omitempty"`
	UpgradeTimeout   *int64 `json:"upgradeTimeout,omitempty"`
	RollbackTimeout  *int64 `json:"rollbackTimeout,omitempty"`
	UninstallTimeout *int64 `json:"uninstallTimeout,omitempty"`

	InstallArguments []string `json:"installArguments,omitempty"`
	UpdateArguments  []string `json:"updateArguments,omitempty"`
	RemoveArguments  []string `json:"removeArguments,omitempty"`

	InternalExport map[string]InternalExportEntry `json:"internalExport,omitempty"`
}

type CatalogAccess struct {
	Repo         string `json:"repo,omitempty"`
	ChartName    string `json:"chartName,omitempty"`
	ChartVersion string `json:"chartVersion,omitempty"`
}

type TarballAccess struct {
	URL          string    `json:"url,omitempty"`
	CustomCAData string    `json:"customCAData,omitempty"`
	AuthHeader   string    `json:"authHeader,omitempty"`
	SecretRef    SecretRef `json:"secretRef,omitempty"`
}

type SecretRef struct {
	Name string `json:"name,omitempty"`
}

type InternalExportEntry struct {
	Name       string `json:"name,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	APIVersion string `json:"apiVersion,omitempty"`
	Resource   string `json:"resource,omitempty"`
	FieldPath  string `json:"fieldPath,omitempty"`
}

func (e *InternalExportEntry) String() string {
	return "InternalExportEntry - APIVersion: " + e.APIVersion + " - Resource: " + e.Resource +
		" - Namespace: " + e.Namespace + " - Name: " + e.Name + " - FieldPath: " + e.FieldPath
}

func NewHelmSpecificData(typeSpecificData *runtime.RawExtension) (*HelmSpecificData, error) {
	var helmSpecificData HelmSpecificData
	if err := json.Unmarshal(typeSpecificData.Raw, &helmSpecificData); err != nil {
		return nil, err
	}

	if err := helmSpecificData.validate(); err != nil {
		return nil, err
	}

	return &helmSpecificData, nil
}

// validate to please the unit tests, but already checked by the hook
func (h *HelmSpecificData) validate() error {
	if h.InstallName == "" {
		return errors.New("property \"installName\" not found")
	}
	if h.Namespace == "" {
		return errors.New("property \"namespace\" not found")
	}
	if h.CatalogAccess != nil {
		if h.CatalogAccess.Repo == "" {
			return errors.New("property \"repo\" not found")
		}
		if h.CatalogAccess.ChartName == "" {
			return errors.New("property \"chartName\" not found")
		}
		if h.CatalogAccess.ChartVersion == "" {
			return errors.New("property \"chartVersion\" not found")
		}
	}
	if h.TarballAccess != nil {
		if h.TarballAccess.URL == "" {
			return errors.New("property \"url\" not found")
		}
	}

	return nil
}

func (h *HelmSpecificData) GetInstallTimeout() time.Duration {
	return convertMinutesToDuration(h.InstallTimeout, defaultTimeout)
}

func (h *HelmSpecificData) GetUpgradeTimeout() time.Duration {
	return convertMinutesToDuration(h.UpgradeTimeout, defaultTimeout)
}

func (h *HelmSpecificData) GetRollbackTimeout() time.Duration {
	return convertMinutesToDuration(h.RollbackTimeout, defaultTimeout)
}

func (h *HelmSpecificData) GetUninstallTimeout() time.Duration {
	return convertMinutesToDuration(h.UninstallTimeout, defaultTimeout)
}

func convertMinutesToDuration(minutes *int64, defaultValue time.Duration) time.Duration { // nolint
	if minutes == nil || *minutes == 0 {
		return defaultValue
	}

	return time.Duration(*minutes) * time.Minute
}

func (h *HelmSpecificData) GetAuthHeader(ctx context.Context, namedSecretResolver *NamedSecretResolver) (string, error) {
	if h.TarballAccess == nil {
		return "", nil
	}

	if h.TarballAccess.SecretRef.Name != "" && namedSecretResolver != nil {
		value, ok, err := namedSecretResolver.ResolveSecretValue(ctx, h.TarballAccess.SecretRef.Name, "authHeader")
		if err != nil {
			return "", err
		}

		if ok {
			return value, nil
		}
	}

	return h.TarballAccess.AuthHeader, nil
}
