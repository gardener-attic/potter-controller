package util

import (
	"testing"

	"github.com/arschles/assert"
	v1 "k8s.io/api/core/v1"
)

func TestAnnotations(t *testing.T) {
	const (
		testKey   = "testKey"
		testValue = "testValue"
	)

	obj := &v1.ConfigMap{}

	ok := HasAnnotation(obj, testKey, testValue)
	assert.False(t, ok, "Failed to check missing annotation")

	RemoveAnnotation(obj, testKey)

	AddAnnotation(obj, testKey, testValue)
	ok = HasAnnotation(obj, testKey, testValue)
	assert.True(t, ok, "Failed to add and check annotation")

	RemoveAnnotation(obj, testKey)
	ok = HasAnnotation(obj, testKey, testValue)
	assert.False(t, ok, "Failed to remove and check annotation")
}

