package util

import (
	"testing"

	"github.com/gardener/landscaper/pkg/apis/core/v1alpha1"

	"github.com/arschles/assert"
)

func TestHasFinalizer(t *testing.T) {
	tests := []struct {
		name                 string
		finalizers           []string
		expectedHasFinalizer bool
	}{
		{
			name:                 "noFinalizer",
			finalizers:           []string{},
			expectedHasFinalizer: false,
		},
		{
			name:                 "oneFinalizerPositive",
			finalizers:           []string{HubControllerFinalizer},
			expectedHasFinalizer: true,
		},
		{
			name:                 "oneFinalizerNegative",
			finalizers:           []string{"other"},
			expectedHasFinalizer: false,
		},
		{
			name:                 "manyFinalizerPositive",
			finalizers:           []string{"other", HubControllerFinalizer, "yet-another"},
			expectedHasFinalizer: true,
		},
		{
			name:                 "manyFinalizerNegative",
			finalizers:           []string{"other", "yet-another"},
			expectedHasFinalizer: false,
		},
	}

	for i := range tests {
		test := &tests[i]

		t.Run(test.name, func(t *testing.T) {
			deployItem := v1alpha1.DeployItem{}
			deployItem.SetFinalizers(test.finalizers)
			actualHasFinalizer := HasFinalizer(&deployItem, HubControllerFinalizer)
			assert.Equal(t, actualHasFinalizer, test.expectedHasFinalizer, "has finalizer")
		})
	}
}

func TestAddFinalizer(t *testing.T) {
	hdc := v1alpha1.DeployItem{}
	AddFinalizer(&hdc, "a")
	AddFinalizer(&hdc, "b")
	assert.Equal(t, len(hdc.GetFinalizers()), 2, "number of finalizers")
}

func TestRemoveFinalizer(t *testing.T) {
	tests := []struct {
		name                       string
		finalizers                 []string
		expectedNumberOfFinalizers int
	}{
		{
			name:                       "removeNone",
			finalizers:                 []string{},
			expectedNumberOfFinalizers: 0,
		},
		{
			name:                       "removeNoneFromMany",
			finalizers:                 []string{"a", "b"},
			expectedNumberOfFinalizers: 2,
		},
		{
			name:                       "removeOne",
			finalizers:                 []string{HubControllerFinalizer},
			expectedNumberOfFinalizers: 0,
		},
		{
			name:                       "removeOneFromMany",
			finalizers:                 []string{"a", HubControllerFinalizer, "b"},
			expectedNumberOfFinalizers: 2,
		},
		{
			name:                       "removeMany",
			finalizers:                 []string{HubControllerFinalizer, "a", HubControllerFinalizer},
			expectedNumberOfFinalizers: 1,
		},
		{
			name:                       "removeAll",
			finalizers:                 []string{HubControllerFinalizer, HubControllerFinalizer},
			expectedNumberOfFinalizers: 0,
		},
	}

	for i := range tests {
		test := &tests[i]

		t.Run(test.name, func(t *testing.T) {
			hdc := v1alpha1.DeployItem{}
			hdc.SetFinalizers(test.finalizers)
			RemoveFinalizer(&hdc, HubControllerFinalizer)
			assert.Equal(t, len(hdc.GetFinalizers()), test.expectedNumberOfFinalizers, "number of finalizers")
		})
	}
}
