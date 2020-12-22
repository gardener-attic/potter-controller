package avcheck

import (
	"encoding/json"
	"time"

	"github.com/pkg/errors"
)

const minChangeInterval = time.Second * 10
const minFailureThreshold = time.Second * 15

type Configuration struct {
	Namespace         string
	BomName           string
	SecretRef         string
	InstallNamespace  string
	TarballURL        string
	CatalogDefinition string
	ChangeInterval    time.Duration
	FailureThreshold  time.Duration
}

func (c *Configuration) Validate() error {
	if c.Namespace == "" {
		return errors.New("namespace must not be empty")
	}

	if c.BomName == "" {
		return errors.New("bomName must not be empty")
	}

	if c.SecretRef == "" {
		return errors.New("secretRef must not be empty")
	}

	if c.InstallNamespace == "" {
		return errors.New("installNamespace must not be empty")
	}

	if c.ChangeInterval < minChangeInterval {
		return errors.Errorf("changeInterval must be greater than %s", minChangeInterval)
	}

	if c.FailureThreshold < minFailureThreshold {
		return errors.Errorf("failureThreshold must be greater than %s", minFailureThreshold)
	}

	return nil
}

func (c *Configuration) UnmarshalJSON(data []byte) error {
	var tmp struct {
		Namespace         string `json:"namespace"`
		BomName           string `json:"bomName"`
		SecretRef         string `json:"secretRef"`
		InstallNamespace  string `json:"installNamespace"`
		TarballURL        string `json:"tarballUrl"`
		CatalogDefinition string `json:"catalogDefinition"`
		ChangeInterval    string `json:"changeInterval"`
		FailureThreshold  string `json:"failureThreshold"`
	}

	err := json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}

	c.Namespace = tmp.Namespace
	c.BomName = tmp.BomName
	c.SecretRef = tmp.SecretRef
	c.InstallNamespace = tmp.InstallNamespace
	c.TarballURL = tmp.TarballURL
	c.CatalogDefinition = tmp.CatalogDefinition

	c.ChangeInterval, err = time.ParseDuration(tmp.ChangeInterval)
	if err != nil {
		return err
	}

	c.FailureThreshold, err = time.ParseDuration(tmp.FailureThreshold)
	if err != nil {
		return err
	}

	return nil
}
