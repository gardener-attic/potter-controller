package avcheck

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/gardener/potter-controller/pkg/util"
)

const (
	serverAddress = ":8081"
)

type Controller interface {
	GetName() string
	GetLastAVCheckReconcileTime() time.Time
}

func StartServer(config *Configuration, monitoredControllers []Controller) {
	var log = ctrl.Log.WithName("Availability Check Server")

	avCheckHandler := createAVCheckHandler(monitoredControllers, config.FailureThreshold, log)

	router := mux.NewRouter()
	router.HandleFunc("/availability", avCheckHandler).Methods("GET")

	server := &http.Server{
		Handler: router,
		Addr:    serverAddress,
	}

	err := server.ListenAndServe()
	if err != nil {
		log.Error(err, "availability check http server failed")
	}
}

func StartServerForBasicCheck() {
	var log = ctrl.Log.WithName("Basic Availability Check Server")

	avCheckHandler := createAVBasicCheckHandler(log)

	router := mux.NewRouter()
	router.HandleFunc("/availability", avCheckHandler).Methods("GET")

	server := &http.Server{
		Handler: router,
		Addr:    serverAddress,
	}

	err := server.ListenAndServe()
	if err != nil {
		log.Error(err, "availability check http server failed")
	}
}

type response struct {
	Controllers []controllerInfo `json:"controllers"`
}

type controllerInfo struct {
	Name                     string    `json:"name"`
	LastAVCheckReconcileTime time.Time `json:"lastAVCheckReconcileTime"`
	IsAvailable              bool      `json:"isAvailable"`
}

func createAVCheckHandler(monitoredControllers []Controller, failureThreshold time.Duration, log logr.Logger) func(w http.ResponseWriter, req *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		handleAVCheck(w, req, monitoredControllers, failureThreshold, log)
	}
}

func createAVBasicCheckHandler(log logr.Logger) func(w http.ResponseWriter, req *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		handleAVBasicCheck(w)
	}
}

// nolint
func handleAVCheck(w http.ResponseWriter, req *http.Request, monitoredControllers []Controller, failureThreshold time.Duration, log logr.Logger) {
	isAvailable := true
	now := time.Now()
	controllerCheckDetails := []controllerInfo{}

	for _, controller := range monitoredControllers {
		isControllerAvailable := true

		diff := now.Sub(controller.GetLastAVCheckReconcileTime())
		if diff > failureThreshold {
			isControllerAvailable = false
			isAvailable = false
		}

		c := controllerInfo{
			Name:                     controller.GetName(),
			LastAVCheckReconcileTime: controller.GetLastAVCheckReconcileTime(),
			IsAvailable:              isControllerAvailable,
		}

		controllerCheckDetails = append(controllerCheckDetails, c)
	}

	body := response{
		Controllers: controllerCheckDetails,
	}

	marshaledBody, err := json.Marshal(body)
	if err != nil {
		message := "marshaling response body failed"
		log.Error(err, message)
		http.Error(w, message+": "+err.Error(), http.StatusInternalServerError)
		return
	}

	var responseCode int
	if isAvailable {
		responseCode = http.StatusOK
	} else {
		responseCode = http.StatusInternalServerError
		log.WithValues(util.LogKeyResponseBody, body).Info("avcheck failed")
	}

	w.WriteHeader(responseCode)
	_, err = w.Write(marshaledBody)
	if err != nil {
		log.Error(err, "writing http response failed")
	}
}

func handleAVBasicCheck(w http.ResponseWriter) {
	w.WriteHeader(http.StatusOK)
}
