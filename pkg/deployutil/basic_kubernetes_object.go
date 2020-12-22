package deployutil

import (
	"github.wdf.sap.corp/kubernetes/hub-controller/pkg/util"

	"k8s.io/apimachinery/pkg/types"
)

type BasicKubernetesObject struct {
	APIVersion string               `yaml:"apiVersion"`
	Kind       string               `yaml:"kind"`
	ObjectMeta types.NamespacedName `yaml:"metadata"`
}

func AcceptAllFilter(obj *BasicKubernetesObject) bool {
	return true
}

func ReadinessFilter(obj *BasicKubernetesObject) bool {
	return obj.Kind == util.KindDeployment || obj.Kind == util.KindStatefulSet || obj.Kind == util.KindDaemonSet
}
