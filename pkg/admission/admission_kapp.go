package admission

import (
	"encoding/json"

	"github.wdf.sap.corp/kubernetes/hub-controller/pkg/util"

	"github.com/go-logr/logr"
	"github.com/vmware-tanzu/carvel-kapp-controller/pkg/apis/kappctrl/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
)

type kappReviewer struct{}

func newKappReviewer() *kappReviewer {
	return &kappReviewer{}
}

func (r *kappReviewer) reviewTypeSpecificData(log logr.Logger, report *report, typeSpecificData *runtime.RawExtension, secretRef string) {
	var appSpec *v1alpha1.AppSpec
	err := json.Unmarshal(typeSpecificData.Raw, &appSpec)
	if err != nil {
		log.V(util.LogLevelWarning).Info("error when unmarshalling kapp specific data", "error", err)
		report.deny("error when unmarshalling kapp specific data: " + err.Error())
		return
	}

	if appSpec.Cluster != nil && appSpec.Cluster.KubeconfigSecretRef != nil {
		if appSpec.Cluster.KubeconfigSecretRef.Name != "" && appSpec.Cluster.KubeconfigSecretRef.Name != secretRef {
			message := "target cluster of kapp app differs from target cluster in clusterbom"
			log.V(util.LogLevelWarning).Info("rejected clusterbom, because " + message)
			report.deny(message)
			return
		}
	}
}
