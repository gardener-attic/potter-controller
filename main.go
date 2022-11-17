/*

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"encoding/json"
	"flag"
	"net/http"
	_ "net/http/pprof"
	"os"
	"strings"
	"time"

	landscaper "github.com/gardener/landscaper/apis/core/v1alpha1"
	appRepov1 "github.com/gardener/potter-controller/api/external/apprepository/v1alpha1"
	"github.com/gardener/potter-controller/pkg/synchronize"

	"github.com/go-logr/logr"
	kappcrtl "github.com/vmware-tanzu/carvel-kapp-controller/pkg/apis/kappctrl/v1alpha1"
	uberzap "go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	apicorev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	// +kubebuilder:scaffold:imports

	hubv1 "github.com/gardener/potter-controller/api/v1"
	"github.com/gardener/potter-controller/pkg/admission"
	"github.com/gardener/potter-controller/pkg/avcheck"
	"github.com/gardener/potter-controller/pkg/controllersdi"
	"github.com/gardener/potter-controller/pkg/util"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

const (
	pprofListenAddr = "0.0.0.0:6060"
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = appRepov1.AddToScheme(scheme)
	_ = kappcrtl.AddToScheme(scheme)
	_ = hubv1.AddToScheme(scheme)
	_ = landscaper.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	setupLog.V(util.LogLevelWarning).Info("Setup step: start main")

	var metricsAddr string
	var enableLeaderElection bool
	var runsLocally bool
	var skipAdmissionHook bool
	var skipReconcile bool
	var appRepoKubeconfig string
	var hubControllerKubeconfig string
	var landscaperEnabled bool
	var extendedLogEnabled bool
	var tokenReviewEnabled bool
	var tokenIssuer string
	var reconcileIntervalMinutes int64
	var restartKappIntervalMinutes int64
	var auditLog bool
	var logLevel string
	var configTypesStringList string

	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&appRepoKubeconfig, "apprepo-kubeconfig", "", "Kubeconfig of the cluster with the appRepo resource")
	flag.StringVar(&hubControllerKubeconfig, "hubcontroller-kubeconfig", "", "Kubeconfig of the local hub controller cluster")
	flag.BoolVar(&extendedLogEnabled, "extended-log-enabled", false, "Flag to enable additional logs")
	flag.BoolVar(&landscaperEnabled, "landscaper-enabled", false, "Flag to enable clusterbom handling via landscaper")
	flag.BoolVar(&tokenReviewEnabled, "tokenreview-enabled", false, "Flag to enable token reviewing for the admission webhook")
	flag.StringVar(&tokenIssuer, "token-issuer", "", "Issuer for validation of webhook jwt tokens")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&runsLocally, "runs-locally", false, "Flag to distinguish between local and productive run. Default value is false (productive).")
	flag.BoolVar(&skipAdmissionHook, "skip-admission-hook", false, "Flag to run without the admission hook. Defaults to false.")
	flag.BoolVar(&skipReconcile, "skip-reconcile", false, "Flag to run without the reconcile loop")
	flag.Int64Var(&reconcileIntervalMinutes, "reconcile-interval-minutes", 60, "Reconcile interval in minutes")
	flag.Int64Var(&restartKappIntervalMinutes, "restart-kapp-interval-minutes", 0, "Restart kapp-controller interval in minutes")
	flag.StringVar(&logLevel, "loglevel", util.LogLevelStringInfo, "log level debug/info/warning/error")
	flag.StringVar(&configTypesStringList, "configtypes", util.ConfigTypeHelm, "supported config types")
	flag.BoolVar(&auditLog, "audit-log", false, "Flag to enable audit logging (requires additional container). Default false")
	flag.Parse()

	zapcoreLogLevel := zapcore.InfoLevel
	if logLevel == util.LogLevelStringDebug {
		zapcoreLogLevel = zapcore.DebugLevel
	} else if logLevel == util.LogLevelStringWarning {
		zapcoreLogLevel = zapcore.WarnLevel
	} else if logLevel == util.LogLevelStringError {
		zapcoreLogLevel = zapcore.ErrorLevel
	}

	ctrl.SetLogger(zap.New(func(o *zap.Options) {
		o.Development = false
		o.Encoder = createLogEncoder()
		o.Level = zapcoreLogLevel
	}))

	setupLog.V(util.LogLevelWarning).Info("Starting hub controller")

	config := ctrl.GetConfigOrDie()

	appRepoClient := getAppRepoClient(appRepoKubeconfig)

	uncachedClient := getUncachedClient(config)

	hubControllerClient := getHubControllerClient(hubControllerKubeconfig, runsLocally)

	mgr := createManager(config, metricsAddr, enableLeaderElection)

	eventBroadcaster, eventRecorder := setupEventRecording(config)
	defer eventBroadcaster.Shutdown()

	avCheckConfig := parseAVCheckConfig()

	blockObject := setupBlockObject(avCheckConfig, runsLocally)

	cbReconciler := setupClusterBomReconciler(mgr, uncachedClient, hubControllerClient, blockObject, auditLog, avCheckConfig)
	defer cbReconciler.Close()

	cbStateReconciler := setupClusterBomStateReconciler(mgr, uncachedClient, blockObject, avCheckConfig)

	deploymentReconciler := setupDeploymentReconciler(mgr, appRepoClient, uncachedClient, blockObject, eventRecorder,
		reconcileIntervalMinutes)

	configTypes := strings.Split(configTypesStringList, ",")
	admissionHookConfig := admission.AdmissionHookConfig{
		UncachedClient:      uncachedClient,
		HubControllerClient: hubControllerClient,
		ConfigTypes:         configTypes,
		ExtendedLogEnabled:  extendedLogEnabled,
		LandscaperEnabled:   landscaperEnabled,
		RunsLocally:         runsLocally,
		TokenIssuer:         tokenIssuer,
		TokenReviewEnabled:  tokenReviewEnabled,
	}
	startAdmissionHook(&admissionHookConfig, skipAdmissionHook)

	startReconciler(mgr, uncachedClient, hubControllerClient, reconcileIntervalMinutes, restartKappIntervalMinutes, skipReconcile, runsLocally)

	startAvailabilityCheck(avCheckConfig, mgr, []avcheck.Controller{
		cbReconciler,
		cbStateReconciler,
		deploymentReconciler,
	})

	go func() {
		err := http.ListenAndServe(pprofListenAddr, nil)
		if err != nil {
			setupLog.Error(err, "failed starting serving pprof")
		}
	}()

	// +kubebuilder:scaffold:builder
	setupLog.V(util.LogLevelWarning).Info("Starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "Problem running manager")
		shutDown(setupLog)
		os.Exit(1)
	}

	shutDown(setupLog)
}

func createManager(config *rest.Config, metricsAddr string, enableLeaderElection bool) manager.Manager {
	setupLog.V(util.LogLevelDebug).Info("Creating manager")

	mgr, err := ctrl.NewManager(config, ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		LeaderElection:     enableLeaderElection,
		Port:               9443,
	})
	if err != nil {
		setupLog.Error(err, "Unable to create manager")
		os.Exit(1)
	}
	return mgr
}

func setupEventRecording(config *rest.Config) (record.EventBroadcaster, record.EventRecorder) {
	setupLog.V(util.LogLevelDebug).Info("Setup event recording")

	var coreClient = typedcorev1.NewForConfigOrDie(config)
	var eventSink record.EventSink = &typedcorev1.EventSinkImpl{
		Interface: coreClient.Events(""),
	}
	var eventSource = apicorev1.EventSource{
		Component: "DeploymentController",
	}
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartRecordingToSink(eventSink)
	eventRecorder := eventBroadcaster.NewRecorder(scheme, eventSource)
	return eventBroadcaster, eventRecorder
}

func setupBlockObject(avCheckConfig *avcheck.Configuration, syncDisabled bool) *synchronize.BlockObject {
	setupLog.V(util.LogLevelDebug).Info("Setup block object")

	var excludedBoms []types.NamespacedName
	if avCheckConfig != nil {
		excludedBoms = append(excludedBoms, types.NamespacedName{
			Namespace: avCheckConfig.Namespace,
			Name:      avCheckConfig.BomName,
		})
	}

	return synchronize.NewBlockObject(excludedBoms, syncDisabled)
}

func setupClusterBomReconciler(mgr manager.Manager, uncachedClient, hubControllerClient synchronize.UncachedClient,
	blockObject *synchronize.BlockObject, auditLog bool, avCheckConfig *avcheck.Configuration) *controllersdi.ClusterBomReconciler {
	setupLog.V(util.LogLevelDebug).Info("Setup clusterbom reconciler")

	cbReconciler, err := controllersdi.NewClusterBomReconciler(
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName("ClusterBomReconciler"),
		mgr.GetScheme(),
		auditLog,
		blockObject,
		avcheck.NewAVCheck(),
		uncachedClient,
		hubControllerClient,
		avCheckConfig)

	if err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ClusterBomReconciler")
		os.Exit(1)
	}
	if err = cbReconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ClusterBomReconciler")
		os.Exit(1)
	}
	return cbReconciler
}

func setupClusterBomStateReconciler(mgr manager.Manager, uncachedClient synchronize.UncachedClient,
	blockObject *synchronize.BlockObject, avCheckConfig *avcheck.Configuration) avcheck.Controller {
	setupLog.V(util.LogLevelDebug).Info("Setup clusterbom state reconciler")

	cbStateReconciler := &controllersdi.ClusterBomStateReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("ClusterBomStateReconciler"),
		Scheme: mgr.GetScheme(),
		Cleaner: controllersdi.ClusterBomCleaner{
			LastCheck: 0,
			Succeeded: false,
		},
		BlockObject:    blockObject,
		AVCheck:        avcheck.NewAVCheck(),
		AvCheckConfig:  avCheckConfig,
		UncachedClient: uncachedClient,
	}

	if err := cbStateReconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ClusterBomStateReconciler")
		os.Exit(1)
	}

	return cbStateReconciler
}

func setupDeploymentReconciler(mgr manager.Manager, appRepoClient client.Client, uncachedClient synchronize.UncachedClient,
	blockObject *synchronize.BlockObject, eventRecorder record.EventRecorder, reconcileIntervalMinutes int64) avcheck.Controller {
	setupLog.V(util.LogLevelDebug).Info("Setup deployment controller")

	logger := ctrl.Log.WithName("controllers").WithName("DeploymentReconciler")

	crAndSecretClient := mgr.GetClient()

	deployerFactory := controllersdi.NewDeploymentFactory(crAndSecretClient, uncachedClient, appRepoClient, blockObject, reconcileIntervalMinutes)

	deploymentReconciler := controllersdi.NewDeploymentReconciler(deployerFactory, crAndSecretClient, logger, mgr.GetScheme(),
		util.NewThreadCounterMap(logger), blockObject, avcheck.NewAVCheck(), uncachedClient, eventRecorder)

	if err := deploymentReconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DeploymentReconciler")
		os.Exit(1)
	}
	return deploymentReconciler
}

func startAdmissionHook(admissionHookConfig *admission.AdmissionHookConfig, skipAdmissionHook bool) {
	if !skipAdmissionHook {
		setupLog.V(util.LogLevelWarning).Info("Starting admission hook")
		go admission.StartAdmissionServer(admissionHookConfig)
		setupLog.V(util.LogLevelWarning).Info("Admission hook started")
	}
}

func startReconciler(mgr manager.Manager, uncachedClient, hubControllerClient synchronize.UncachedClient,
	reconcileIntervalMinutes, restartKappIntervalMinutes int64, skipReconcile, runsLocally bool) {
	if !skipReconcile {
		setupLog.V(util.LogLevelDebug).Info("Starting reconciler")

		reconcileController := controllersdi.ReconcileController{
			Client:              mgr.GetClient(),
			UncachedClient:      uncachedClient,
			HubControllerClient: hubControllerClient,
			Log:                 ctrl.Log.WithName("controllers").WithName("ReconcileController"),
			Scheme:              mgr.GetScheme(),
			Clock:               &controllersdi.RealReconcileClock{},
			UniqueID:            string(uuid.NewUUID()),
			ConfigMapKey:        types.NamespacedName{Name: "reconcilemap", Namespace: util.GetPodNamespace()},
			SyncDisabled:        runsLocally,
		}

		reconcileInterval := time.Duration(reconcileIntervalMinutes) * time.Minute
		restartKappInterval := time.Duration(restartKappIntervalMinutes) * time.Minute
		go reconcileController.Reconcile(reconcileInterval, restartKappInterval)
	}
}

func startAvailabilityCheck(avCheckConfig *avcheck.Configuration, mgr manager.Manager, monitoredControllers []avcheck.Controller) {
	if avCheckConfig != nil {
		setupLog.V(util.LogLevelWarning).Info("Starting availability check")

		actor := avcheck.Actor{
			K8sClient:      mgr.GetClient(),
			Log:            ctrl.Log.WithName("Availability Check Actor"),
			InitialBom:     buildAVCheckBom(avCheckConfig),
			ChangeInterval: avCheckConfig.ChangeInterval,
		}

		go actor.Start()
		go avcheck.StartServer(avCheckConfig, monitoredControllers)
	} else {
		go avcheck.StartServerForBasicCheck()
	}
}

// shutDown waits a moment to allow the controllers and the admission hook to finish their current work.
func shutDown(log logr.Logger) {
	log.V(util.LogLevelWarning).Info("Start of shutdown interval")
	time.Sleep(25 * time.Second)
	log.V(util.LogLevelWarning).Info("End of shutdown interval")
}

func getAppRepoClient(appRepoKubeconfig string) client.Client {
	setupLog.V(util.LogLevelDebug).Info("Creating AppRepository client")

	apprepoConfig, err := clientcmd.BuildConfigFromFlags("", appRepoKubeconfig)
	if err != nil {
		setupLog.Error(err, "Unable to read AppRepository kubeconfig. Is the CLI flag set?")
		os.Exit(1)
	}

	appRepoClient, err := client.New(apprepoConfig, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		setupLog.Error(err, "Unable to create AppRepository client. Is the CLI flag set?")
		os.Exit(1)
	}

	return appRepoClient
}

func getUncachedClient(config *rest.Config) synchronize.UncachedClient {
	setupLog.V(util.LogLevelDebug).Info("Creating uncached client")

	uncachedClient, err := synchronize.NewUncachedClient(config, client.Options{Scheme: scheme})
	if err != nil {
		setupLog.Error(err, "Unable to create uncached client")
		os.Exit(1)
	}

	return uncachedClient
}

func getHubControllerClient(hubControllerKubeconfig string, runsLocally bool) synchronize.UncachedClient {
	var hubControllerConfig *rest.Config
	var err error

	setupLog.V(util.LogLevelDebug).Info("Creating hub controller client")

	if runsLocally {
		hubControllerConfig, err = clientcmd.BuildConfigFromFlags("", hubControllerKubeconfig)
	} else {
		hubControllerConfig, err = rest.InClusterConfig()
	}

	if err != nil {
		setupLog.Error(err, "Unable to get config for hub controller client")
		os.Exit(1)
	}

	hubControllerClient, err := synchronize.NewUncachedClient(hubControllerConfig, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		setupLog.Error(err, "Unable to create hub controller client")
		os.Exit(1)
	}

	return hubControllerClient
}

func createLogEncoder() zapcore.Encoder {
	encodeTime := func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(t.Format(time.RFC3339))
	}

	encoderCfg := uberzap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = "time"
	encoderCfg.EncodeTime = encodeTime
	return zapcore.NewJSONEncoder(encoderCfg)
}

func parseAVCheckConfig() *avcheck.Configuration {
	setupLog.V(util.LogLevelDebug).Info("Reading config for availability check")

	avCheckConfigJSON := os.Getenv("AVAILABILITY_CHECK")
	if avCheckConfigJSON == "" {
		return nil
	}

	var config avcheck.Configuration
	err := json.Unmarshal([]byte(avCheckConfigJSON), &config)
	if err != nil {
		setupLog.Error(err, "cannot unmarshal availability check configJSON")
		os.Exit(1)
	}

	err = config.Validate()
	if err != nil {
		setupLog.Error(err, "invalid availability check configJSON")
		os.Exit(1)
	}
	return &config
}

func buildAVCheckBom(config *avcheck.Configuration) *hubv1.ClusterBom {
	bom, err := avcheck.BuildBom(config.Namespace, config.BomName, config.SecretRef, config.InstallNamespace,
		config.TarballURL, config.CatalogDefinition)
	if err != nil {
		setupLog.Error(err, "cannot build availability check bom")
		os.Exit(1)
	}
	return bom
}

func getAllResourceNames(bom *hubv1.ClusterBom) []*types.NamespacedName {
	resourceNames := []*types.NamespacedName{
		{
			Namespace: bom.GetNamespace(),
			Name:      bom.GetName(),
		},
	}
	for _, applicationConfig := range bom.Spec.ApplicationConfigs {
		resourceName := types.NamespacedName{
			Namespace: bom.GetNamespace(),
			Name:      bom.GetName() + "-" + applicationConfig.ID,
		}
		resourceNames = append(resourceNames, &resourceName)
	}

	return resourceNames
}
