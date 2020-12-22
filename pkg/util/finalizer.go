package util

import v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

func HasFinalizer(obj v1.Object, finalizer string) bool {
	finalizers := obj.GetFinalizers()
	for _, fin := range finalizers {
		if fin == finalizer {
			return true
		}
	}

	return false
}

func AddFinalizer(obj v1.Object, finalizer string) {
	finalizers := obj.GetFinalizers()
	finalizers = append(finalizers, finalizer)
	obj.SetFinalizers(finalizers)
}

func RemoveFinalizer(obj v1.Object, finalizer string) {
	oldFinalizers := obj.GetFinalizers()
	newFinalizers := []string{}

	for _, fin := range oldFinalizers {
		if fin != finalizer {
			newFinalizers = append(newFinalizers, fin)
		}
	}

	if len(newFinalizers) == 0 {
		newFinalizers = nil
	}

	obj.SetFinalizers(newFinalizers)
}
