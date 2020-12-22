package util

import (
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func HasAnnotation(obj v1.Object, key, value string) bool {
	annotations := obj.GetAnnotations()
	v, ok := annotations[key]
	return ok && v == value
}

func GetAnnotation(obj v1.Object, key string) (string, bool) {
	annotations := obj.GetAnnotations()
	v, ok := annotations[key]
	return v, ok
}

func AddAnnotation(obj v1.Object, key, value string) {
	annotations := obj.GetAnnotations()
	if len(annotations) == 0 {
		annotations = make(map[string]string)
	}
	annotations[key] = value
	obj.SetAnnotations(annotations)
}

func RemoveAnnotation(obj v1.Object, key string) {
	annotations := obj.GetAnnotations()
	delete(annotations, key)
}
