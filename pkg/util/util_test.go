package util

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func Test_DecodeBasicAuthCredentials_Successfully(t *testing.T) {
	credentials := "aHVtYW46c2VjcmV0"
	expectedUsername := "human"
	expectedPassword := "secret"

	actualUsername, actualPassword, err := DecodeBasicAuthCredentials(credentials)

	assert.Nil(t, err, "Unexpected error when Decoding basic authentication")
	assert.Equal(t, expectedUsername, actualUsername, "username")
	assert.Equal(t, expectedPassword, actualPassword, "password")
}

func Test_DecodeBasicAuthCredentials_WithError(t *testing.T) {
	credentials := "a"

	_, _, err := DecodeBasicAuthCredentials(credentials)

	assert.NotNil(t, err, "Expected decoding error but no error was thrown")
}

func Test_DecodeBasicAuthCredentials_WithSplitError(t *testing.T) {
	credentials := "aHVtYW4="

	_, _, err := DecodeBasicAuthCredentials(credentials)

	assert.NotNil(t, err, "Expected an error when input credentials do not contain a colon")
}

func Test_CalculateRequeueTimeout(t *testing.T) {
	tests := []struct {
		name                   string
		numberOfTries          int32
		expectedRequeueTimeout time.Duration
	}{
		{
			name:                   "negative number of tries",
			numberOfTries:          -3,
			expectedRequeueTimeout: requeueTimeoutBase,
		},
		{
			name:                   "number of tries is zero",
			numberOfTries:          0,
			expectedRequeueTimeout: 1 * requeueTimeoutBase,
		},
		{
			name:                   "positive number of tries",
			numberOfTries:          2,
			expectedRequeueTimeout: 4 * requeueTimeoutBase,
		},
		{
			name:                   "number of tries equal to max",
			numberOfTries:          8,
			expectedRequeueTimeout: 256 * requeueTimeoutBase,
		},
		{
			name:                   "number of tries greater than max",
			numberOfTries:          10000,
			expectedRequeueTimeout: requeueTimeoutMax,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			requeueTimeout := CalculateRequeueTimeout(tt.numberOfTries)
			assert.Equal(t, tt.expectedRequeueTimeout, requeueTimeout, "requeueTimeout")
		})
	}
}

func TestWithBasicAuth(t *testing.T) {
	tests := []struct {
		name             string
		expectedUser     string
		expectedPassword string
		user             string
		password         string
		expectedStatus   int
	}{
		{
			name:             "valid auth",
			expectedUser:     "user",
			expectedPassword: "pw",
			user:             "user",
			password:         "pw",
			expectedStatus:   http.StatusOK,
		},
		{
			name:             "no auth configured",
			expectedUser:     "",
			expectedPassword: "",
			user:             "",
			password:         "",
			expectedStatus:   http.StatusOK,
		},
		{
			name:             "empty credentials",
			expectedUser:     "user",
			expectedPassword: "pw",
			user:             "",
			password:         "",
			expectedStatus:   http.StatusUnauthorized,
		},
		{
			name:             "user correct, password not",
			expectedUser:     "user",
			expectedPassword: "pw",
			user:             "user",
			password:         "pw2",
			expectedStatus:   http.StatusUnauthorized,
		},
		{
			name:             "password correct, user not",
			expectedUser:     "user",
			expectedPassword: "pw",
			user:             "user2",
			password:         "pw",
			expectedStatus:   http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", "/test", nil)
			req.SetBasicAuth(tt.user, tt.password)
			if err != nil {
				t.Fatal(err)
			}

			called := false
			innerHandler := func(w http.ResponseWriter, r *http.Request) { called = true }
			handler := WithBasicAuth(innerHandler, tt.expectedUser, tt.expectedPassword)

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)
			if tt.expectedStatus == http.StatusOK {
				assert.True(t, called, "inner handler must be called")
			} else {
				assert.False(t, called, "inner handler must not be called")
			}
		})
	}
}

func TestIsStatusErrorConflicts(t *testing.T) {
	newStatusError := func(code int32) *apierrors.StatusError {
		return &apierrors.StatusError{
			ErrStatus: v1.Status{
				Code: code,
			},
		}
	}

	assert.True(t, IsStatusErrorConflict(newStatusError(http.StatusConflict)), "conflict error not recognized")
	assert.False(t, IsStatusErrorConflict(newStatusError(http.StatusBadRequest)), "bad request error was mistaken for a conflict error")
	assert.False(t, IsStatusErrorConflict(nil), "nil was mistaken for a conflict error")
	assert.False(t, IsStatusErrorConflict(errors.New("test")), "normal error was mistaken for a conflict error")
}

func TestGetFieldByJSONPath(t *testing.T) {
	tests := []struct {
		name           string
		inputData      map[string]interface{}
		fieldPath      string
		expectedValues [][]interface{}
		errorMsg       string
	}{
		{
			name: "everything valid",
			inputData: map[string]interface{}{
				"apiVersion": "v1",
				"spec": map[string]interface{}{
					"replicas": 5,
				},
			},
			fieldPath: "{ .spec }{ .apiVersion }",
			expectedValues: [][]interface{}{
				{
					map[string]interface{}{
						"replicas": 5,
					},
				},
				{
					"v1",
				},
			},
		},
		{
			name: "no results for fieldPath",
			inputData: map[string]interface{}{
				"apiVersion": "v1",
				"spec": map[string]interface{}{
					"replicas": 5,
				},
			},
			fieldPath: "{ .propWhichDoesntExist }",
			errorMsg:  "cannot find results:",
		},
		{
			name:      "inputData is nil",
			fieldPath: "{ .apiVersion }",
			errorMsg:  "cannot find results:",
		},
		{
			name: "invalid fieldPath",
			inputData: map[string]interface{}{
				"apiVersion": "v1",
				"spec": map[string]interface{}{
					"replicas": 5,
				},
			},
			fieldPath: `{ \123 }`,
			errorMsg:  "cannot parse fieldPath:",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			results, err := GetFieldsByJSONPath(tt.inputData, tt.fieldPath)

			if tt.errorMsg != "" {
				assert.Error(t, err, "err")
				assert.Contains(t, err.Error(), tt.errorMsg, "errorMsg")
				return
			}

			assert.NoError(t, err, "err")
			assert.Len(t, results, len(tt.expectedValues), "number of results")

			for i, result := range results {
				assert.Len(t, result, len(tt.expectedValues[i]), "number of values for a single result")
				for j, value := range result {
					assert.Equal(t, tt.expectedValues[i][j], value.Interface(), "value")
				}
			}
		})
	}
}

func TestParseSuccessValue(t *testing.T) {
	tests := []struct {
		name         string
		inputValues  []runtime.RawExtension
		outputValues []interface{}
		errorMsg     string
	}{
		{
			name: "everything valid",
			inputValues: []runtime.RawExtension{
				*CreateRawExtensionOrPanic(map[string]interface{}{
					"value": 42,
				}),
				*CreateRawExtensionOrPanic(map[string]interface{}{
					"value": "True",
				}),
				*CreateRawExtensionOrPanic(map[string]interface{}{
					"value": true,
				}),
				*CreateRawExtensionOrPanic(map[string]interface{}{
					"value": []interface{}{
						"test1",
						42,
					},
				}),
				*CreateRawExtensionOrPanic(map[string]interface{}{
					"value": map[string]interface{}{
						"key1": "val1",
						"key2": 42,
					},
				}),
			},
			outputValues: []interface{}{
				// when unmarshalling the RawExtension, json.Unmarshal uses float64 as the type
				// for all numbers --> we also have to use float64 for the expected results
				float64(42),
				"True",
				true,
				[]interface{}{
					"test1",
					float64(42),
				},
				map[string]interface{}{
					"key1": "val1",
					"key2": float64(42),
				},
			},
		},
		{
			name: "object does not contain the value key",
			inputValues: []runtime.RawExtension{
				*CreateRawExtensionOrPanic(map[string]interface{}{
					"invalid-key": 42,
				}),
			},
			errorMsg: "object at index 0 does not contain the value key",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			parsedValues, err := ParseSuccessValues(tt.inputValues)

			if tt.errorMsg != "" {
				assert.Error(t, err, "err")
				assert.Contains(t, err.Error(), tt.errorMsg, "errorMsg")
				return
			}

			assert.NoError(t, err, "err")
			assert.Len(t, parsedValues, len(tt.outputValues), "number of values")

			for i, parsedValue := range parsedValues {
				assert.Equal(t, tt.outputValues[i], parsedValue, "value")
			}
		})
	}
}
