package apitypes

import (
	"encoding/json"

	"github.com/vmware-tanzu/carvel-kapp-controller/pkg/apis/kappctrl/v1alpha1"
)

type KappSpecificData struct {
	*v1alpha1.AppSpec `json:",inline"`
	InternalExport    map[string]InternalExportEntry `json:"internalExport,omitempty"`
}

func NewKappSpecificData(typeSpecificData []byte) (*KappSpecificData, error) {
	var kappSpecificData KappSpecificData
	if err := json.Unmarshal(typeSpecificData, &kappSpecificData); err != nil {
		return nil, err
	}

	return &kappSpecificData, nil
}
