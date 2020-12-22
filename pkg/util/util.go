package util

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/jsonpath"

	hubv1 "github.wdf.sap.corp/kubernetes/hub-controller/api/v1"
)

type LoggerKey struct{}
type CRAndSecretClientKey struct{}
type AuditLogKey struct{}

// DecodeBasicAuthCredentials Decodes basic auth credential string and returns username, password or an error
func DecodeBasicAuthCredentials(base64EncodedBasicAuthCredentials string) (string, string, error) {
	decodedCredentials, err := base64.StdEncoding.DecodeString(base64EncodedBasicAuthCredentials)
	if err != nil {
		return "", "", errors.Wrap(err, "Couldn't decode basic auth credentials")
	}
	splittedCredentials := strings.SplitN(string(decodedCredentials), ":", 2)
	if len(splittedCredentials) < 2 {
		return "", "", errors.New("Password missing in credential string. Could not split by colon ':'")
	}

	username := splittedCredentials[0]
	password := splittedCredentials[1]
	return username, password, nil
}

func GetPodNamespace() string {
	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		namespace = defaultNamespace
	}

	return namespace
}

func GetApprepoNamespace() string {
	namespace := os.Getenv("APPREPO_NAMESPACE")
	if namespace == "" {
		namespace = defaultNamespace
	}

	return namespace
}

func CalculateRequeueTimeout(numberOfTries int32) time.Duration {
	if numberOfTries < 0 {
		return requeueTimeoutBase
	}
	if numberOfTries > 8 {
		return requeueTimeoutMax
	}
	multiplier := math.Pow(2, float64(numberOfTries))
	requeueTimeout := requeueTimeoutBase * time.Duration(multiplier)
	return requeueTimeout
}

func CalculateRequeueDurationForPrematureRetry(lastOp *hubv1.LastOperation) (bool, *time.Duration) {
	lastTime := lastOp.Time.Time
	currentTime := time.Now()
	nextScheduledRun := lastTime.Add(CalculateRequeueTimeout(lastOp.NumberOfTries))

	if currentTime.Before(nextScheduledRun) {
		duration := nextScheduledRun.Sub(currentTime)
		return true, &duration
	}

	return false, nil
}

func CreateInitialLastOperation() *hubv1.LastOperation {
	return &hubv1.LastOperation{
		Operation:         OperationInstall,
		SuccessGeneration: 0,
		State:             StateOk,
		NumberOfTries:     0,
		Time:              metav1.Now(),
		Description:       "no last operation",
	}
}

func CreateRawExtension(data map[string]interface{}) (*runtime.RawExtension, error) {
	rawData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	object := runtime.RawExtension{Raw: rawData}
	return &object, nil
}

func CreateRawExtensionOrPanic(data map[string]interface{}) *runtime.RawExtension {
	rawData, err := json.Marshal(data)
	if err != nil {
		panic(err)
	}
	object := runtime.RawExtension{Raw: rawData}
	return &object
}

func GetEnvInteger(name string, defaultValue int, logger logr.Logger) int {
	envValue := defaultValue

	envString, ok := os.LookupEnv(name)
	if ok {
		tmpValue, err := strconv.Atoi(envString)

		if err != nil {
			logger.Error(err, "environment value wrong integer value", "name", name, "value", envString)
		} else {
			envValue = tmpValue
		}
	} else {
		logger.V(LogLevelWarning).Info("environment value not configured", "name", name)
	}

	logger.V(LogLevelWarning).Info("environment value used", "name", name, "value", envString)

	return envValue
}

func WithBasicAuth(handler http.HandlerFunc, username, password string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, pass, _ := r.BasicAuth()
		if !(user == username && pass == password) {
			http.Error(w, "Unauthorized.", http.StatusUnauthorized)
			return
		}
		handler(w, r)
	}
}

func Repeat(f func() bool, repetitions int, pause time.Duration) bool {
	for i := 0; i < repetitions; i++ {
		if i > 0 {
			time.Sleep(pause)
		}

		done := f()
		if done {
			return true
		}
	}
	return false
}

func IsStatusErrorConflict(err error) bool {
	return IsStatusErrorWithCode(err, http.StatusConflict)
}

func IsStatusErrorWithCode(err error, code int32) bool {
	if err == nil {
		return false
	}

	statusErr, ok := err.(*apierrors.StatusError)
	return ok && statusErr.ErrStatus.Code == code
}

func IsConcurrentModificationErr(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "the object has been modified")
}

func ContainsString(s string, stringSlice []string) bool {
	for _, next := range stringSlice {
		if next == s {
			return true
		}
	}

	return false
}

func ContainsValue(value interface{}, values []interface{}) bool {
	for _, v := range values {
		if reflect.DeepEqual(value, v) {
			return true
		}
	}
	return false
}

func GetFieldsByJSONPath(obj map[string]interface{}, fieldPath string) ([][]reflect.Value, error) {
	p := jsonpath.New("fieldPath")
	err := p.Parse(fieldPath)
	if err != nil {
		return nil, errors.Wrap(err, "cannot parse fieldPath")
	}

	results, err := p.FindResults(obj)
	if err != nil {
		return nil, errors.Wrap(err, "cannot find results")
	}

	return results, nil
}

func ParseSuccessValues(successValues []runtime.RawExtension) ([]interface{}, error) {
	parsedValues := []interface{}{}
	for i, successValue := range successValues {
		var tmp map[string]interface{}
		err := json.Unmarshal(successValue.Raw, &tmp)
		if err != nil {
			return nil, errors.Wrap(err, fmt.Sprintf("cannot unmarshal object at index %d", i))
		}

		if val, ok := tmp["value"]; ok {
			parsedValues = append(parsedValues, val)
		} else {
			return nil, errors.New(fmt.Sprintf("object at index %d does not contain the value key", i))
		}
	}
	return parsedValues, nil
}
