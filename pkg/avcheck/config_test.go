package avcheck

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/arschles/assert"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name        string
		config      Configuration
		producesErr bool
		errMsg      string
	}{
		{
			name: "valid",
			config: Configuration{
				Namespace:        "test",
				BomName:          "test",
				SecretRef:        "test",
				InstallNamespace: "test",
				ChangeInterval:   minChangeInterval,
				FailureThreshold: minFailureThreshold,
			},
			producesErr: false,
			errMsg:      "",
		},
		{
			name: "no Namespace",
			config: Configuration{
				Namespace:        "",
				BomName:          "test",
				SecretRef:        "test",
				InstallNamespace: "test",
				ChangeInterval:   minChangeInterval,
				FailureThreshold: minFailureThreshold,
			},
			producesErr: true,
			errMsg:      "namespace must not be empty",
		},
		{
			name: "no BomName",
			config: Configuration{
				Namespace:        "test",
				BomName:          "",
				SecretRef:        "test",
				InstallNamespace: "test",
				ChangeInterval:   minChangeInterval,
				FailureThreshold: minFailureThreshold,
			},
			producesErr: true,
			errMsg:      "bomName must not be empty",
		},
		{
			name: "no SecretRef",
			config: Configuration{
				Namespace:        "test",
				BomName:          "test",
				SecretRef:        "",
				InstallNamespace: "test",
				ChangeInterval:   minChangeInterval,
				FailureThreshold: minFailureThreshold,
			},
			producesErr: true,
			errMsg:      "secretRef must not be empty",
		},
		{
			name: "no InstallNamespace",
			config: Configuration{
				Namespace:        "test",
				BomName:          "test",
				SecretRef:        "test",
				InstallNamespace: "",
				ChangeInterval:   minChangeInterval,
				FailureThreshold: minFailureThreshold,
			},
			producesErr: true,
			errMsg:      "installNamespace must not be empty",
		},
		{
			name: "ChangeInterval too small",
			config: Configuration{
				Namespace:        "test",
				BomName:          "test",
				SecretRef:        "test",
				InstallNamespace: "test",
				ChangeInterval:   5 * time.Millisecond,
				FailureThreshold: minFailureThreshold,
			},
			producesErr: true,
			errMsg:      fmt.Sprintf("changeInterval must be greater than %s", minChangeInterval),
		},
		{
			name: "FailureThreshold too small",
			config: Configuration{
				Namespace:        "test",
				BomName:          "test",
				SecretRef:        "test",
				InstallNamespace: "test",
				ChangeInterval:   minChangeInterval,
				FailureThreshold: 5 * time.Millisecond,
			},
			producesErr: true,
			errMsg:      fmt.Sprintf("failureThreshold must be greater than %s", minFailureThreshold),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.producesErr {
				assert.NotNil(t, err, "validation error")
				assert.Equal(t, err.Error(), tt.errMsg, "error message")
			} else {
				assert.Nil(t, err, "validation error")
			}
		})
	}
}

func TestUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name           string
		configJSON     string
		producesErr    bool
		expectedConfig Configuration
	}{
		{
			name: "valid",
			configJSON: `{
				"namespace":        "test",
				"bomName":          "test",
				"secretRef":        "test",
				"installNamespace": "test",
				"changeInterval":   "15m",
				"failureThreshold": "15m"
			}`,
			producesErr: false,
			expectedConfig: Configuration{
				Namespace:        "test",
				BomName:          "test",
				SecretRef:        "test",
				InstallNamespace: "test",
				ChangeInterval:   15 * time.Minute,
				FailureThreshold: 15 * time.Minute,
			},
		},
		{
			name: "invalid changeInterval",
			configJSON: `{
				"namespace":        "test",
				"bomName":          "test",
				"secretRef":        "test",
				"installNamespace": "test",
				"changeInterval":   "15notparseable"
			}`,
			producesErr: true,
		},
		{
			name: "invalid failureThreshold",
			configJSON: `{
				"namespace":        "test",
				"bomName":          "test",
				"secretRef":        "test",
				"installNamespace": "test",
				"changeInterval":   "15m",
				"failureThreshold": "15notparseable"
			}`,
			producesErr: true,
		},
		{
			name: "invalid datatype",
			configJSON: `{
				"namespace":        23,
				"bomName":          "test",
				"secretRef":        "test",
				"installNamespace": "test",
				"changeInterval":   "15m"
				"failureThreshold": "15m"
			}`,
			producesErr: true,
		},
		{
			name: "invalid JSON structure",
			configJSON: `{
				"namespace":        "test",
				"bomName":          "test",
				"secretRef":        "test",{{}
				"installNamespace": "test",
				"changeInterval":   "15m",
				"failureThreshold": "15m"
			}`,
			producesErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			var config Configuration
			err := json.Unmarshal([]byte(tt.configJSON), &config)
			if tt.producesErr {
				assert.NotNil(t, err, "unmarshaling error")
			} else {
				assert.Nil(t, err, "unmarshaling error")
				assert.Equal(t, config, tt.expectedConfig, "config")
			}
		})
	}
}
