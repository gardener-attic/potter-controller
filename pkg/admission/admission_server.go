package admission

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/gardener/potter-controller/pkg/synchronize"
	"github.com/gardener/potter-controller/pkg/util"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	"k8s.io/api/admission/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
)

type AdmissionHookConfig struct { // nolint
	UncachedClient      synchronize.UncachedClient
	HubControllerClient synchronize.UncachedClient
	ConfigTypes         []string
	ExtendedLogEnabled  bool
	LandscaperEnabled   bool
	RunsLocally         bool
	TokenIssuer         string
	TokenReviewEnabled  bool
}

func StartAdmissionServer(config *AdmissionHookConfig) {
	var log = ctrl.Log.WithName("ClusterBom Admission Hook")

	router := mux.NewRouter()

	clusterBomHandler := newClusterBomHandler(config, log)
	clusterBomHandler = buildHandlerChain(clusterBomHandler, config, log)
	router.Handle("/checkClusterBom", clusterBomHandler).Methods("POST")

	secretHandler := newSecretHandler(config, log)
	secretHandler = buildHandlerChain(secretHandler, config, log)
	router.Handle("/checkSecret", secretHandler).Methods("POST")

	if config.RunsLocally {
		// execution in local mode
		server := &http.Server{
			Handler:      router,
			Addr:         "0.0.0.0:8000",
			WriteTimeout: 15 * time.Second,
			ReadTimeout:  15 * time.Second,
		}

		err := server.ListenAndServeTLS("/home/vagrant/tmp/certs/cfssl/server.pem", "/home/vagrant/tmp/certs/cfssl/server-key.pem")
		if err != nil {
			log.Error(err, "http server of admission webhook failed")
		}

		return
	}

	// execution in productive mode
	server := &http.Server{
		Handler:      router,
		Addr:         ":8085",
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	err := server.ListenAndServe()
	if err != nil {
		log.Error(err, "http server of clusterbom admission webhook failed")
	}
}

func buildHandlerChain(handler http.Handler, config *AdmissionHookConfig, log logr.Logger) http.Handler {
	if !config.TokenReviewEnabled {
		log.V(util.LogLevelWarning).Info("webhook token review is not enabled")
		return handler
	}

	log.V(util.LogLevelWarning).Info("webhook token review is enabled")
	return newTokenReviewer(handler, config.TokenIssuer, log)
}

type secretHandler struct {
	hubControllerClient synchronize.UncachedClient
	log                 logr.Logger
}

func newSecretHandler(config *AdmissionHookConfig, log logr.Logger) http.Handler {
	return &secretHandler{
		hubControllerClient: config.HubControllerClient,
		log:                 log,
	}
}

func (h *secretHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	defer req.Body.Close()

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		message := "reading request body failed for secret"
		h.log.Error(err, message)
		http.Error(w, message+": "+err.Error(), http.StatusInternalServerError)
		return
	}

	requestReview := v1beta1.AdmissionReview{}
	err = json.Unmarshal(body, &requestReview)
	if err != nil {
		message := "unmarshaling request admission review failed for secret"
		h.log.Error(err, message)
		http.Error(w, message+": "+err.Error(), http.StatusBadRequest)
		return
	}

	reviewer := secretReviewer{
		log:                 h.log,
		requestReview:       &requestReview,
		hubControllerClient: h.hubControllerClient,
	}
	responseReview := reviewer.review()

	responseBody, err := json.Marshal(responseReview)
	if err != nil {
		message := "marshaling response admission review failed for secret"
		h.log.Error(err, message)
		http.Error(w, message+": "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, err = w.Write(responseBody)
	if err != nil {
		h.log.Error(err, "writing http response failed for secret")
	}
}

type clusterBomHandler struct {
	cl                 synchronize.UncachedClient
	log                logr.Logger
	configTypes        []string
	extendedLogEnabled bool
	landscaperEnabled  bool
}

func newClusterBomHandler(config *AdmissionHookConfig, log logr.Logger) http.Handler {
	return &clusterBomHandler{
		cl:                 config.UncachedClient,
		log:                log,
		configTypes:        config.ConfigTypes,
		extendedLogEnabled: config.ExtendedLogEnabled,
		landscaperEnabled:  config.LandscaperEnabled,
	}
}

func (h *clusterBomHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	defer req.Body.Close()

	if h.extendedLogEnabled {
		headerMap := req.Header
		authHeader, ok := headerMap["Authorization"]
		if ok {
			h.log.Error(nil, "webhook auth", "headers", authHeader)
		} else {
			h.log.Error(nil, "webhook auth: no auth header")
		}
	}

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		message := "reading request body failed for cluster bom"
		h.log.Error(err, message)
		http.Error(w, message+": "+err.Error(), http.StatusInternalServerError)
		return
	}

	requestReview := v1beta1.AdmissionReview{}
	err = json.Unmarshal(body, &requestReview)
	if err != nil {
		message := "unmarshaling request admission review failed for cluster bom"
		h.log.Error(err, message)
		http.Error(w, message+": "+err.Error(), http.StatusBadRequest)
		return
	}

	reviewer := clusterBomReviewer{
		log:               h.log,
		requestReview:     &requestReview,
		reader:            h.cl,
		configTypes:       h.configTypes,
		landscaperEnabled: h.landscaperEnabled,
	}
	responseReview := reviewer.review()

	responseBody, err := json.Marshal(responseReview)
	if err != nil {
		message := "marshaling response admission review failed for cluster bom"
		h.log.Error(err, message)
		http.Error(w, message+": "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, err = w.Write(responseBody)
	if err != nil {
		h.log.Error(err, "writing http response failed for cluster bom")
	}
}
