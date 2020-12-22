package util

import (
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func HasLabel(obj v1.Object, key, value string) bool {
	labels := obj.GetLabels()
	v, ok := labels[key]
	return ok && v == value
}

func GetLabel(obj v1.Object, key string) (string, bool) {
	labels := obj.GetLabels()
	v, ok := labels[key]
	return v, ok
}

func AddLabels(obj v1.Object, key, value string) {
	labels := obj.GetLabels()
	if len(labels) == 0 {
		labels = make(map[string]string)
	}
	labels[key] = value
	obj.SetLabels(labels)
}

func RemoveLabel(obj v1.Object, key string) {
	labels := obj.GetLabels()
	delete(labels, key)
}
