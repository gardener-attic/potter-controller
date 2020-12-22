package util

import "time"

const (
	StateOk            = "ok"
	StatePending       = "pending"
	StateFailed        = "failed"
	StateUnknown       = "unknown"
	StateNotRelevant   = "notRelevant"
	StateFinallyFailed = "finallyFailed"

	// Log level constants. Larger numbers represent less important logs.
	LogLevelDebug   = 1
	LogLevelWarning = -1

	// Strings for readable log levels
	LogLevelStringError   = "error"
	LogLevelStringWarning = "warning"
	LogLevelStringInfo    = "info"
	LogLevelStringDebug   = "debug"

	// Keys for logging common fields
	LogKeyClusterBomName        = "clusterbom-name"
	LogKeyDeployItemName        = "deployitem-name"
	LogKeyInstallationName      = "installation-name"
	LogKeyCorrelationID         = "correlation-id"
	LogKeyInterval              = "interval"
	LogKeyConfigmap             = "configmap"
	LogKeyResponseBody          = "response-body"
	LogKeySecretName            = "secret-name"
	LogKeyKappAppNamespacedName = "kappapp-name"

	defaultNamespace = "hub"

	requeueTimeoutBase = 10 * time.Second
	requeueTimeoutMax  = 1 * time.Hour

	TextShootNotExisting = "shoot cluster does not exist"

	APIVersionExtensionsV1beta1 = "extensions/v1beta1"
	APIVersionAppsV1beta1       = "apps/v1beta1"
	APIVersionAppsV1beta2       = "apps/v1beta2"
	APIVersionAppsV1            = "apps/v1"
	APIVersionBatchV1           = "batch/v1"

	KindDaemonSet   = "DaemonSet"
	KindDeployment  = "Deployment"
	KindStatefulSet = "StatefulSet"
	KindJob         = "Job"

	PurposeSecretValues = "secret-values"
	PurposeDiExportData = "di-export-data"

	// annotations
	AnnotationKeyReconcile   = "hub.k8s.sap.com/reconcile"
	AnnotationValueReconcile = "reconcile"

	AnnotationKeyInstallationHash = "potter.gardener.cloud/installation-hash"

	// keys
	SecretValuesKey  = "secretValues"
	KeyDeletionToken = "deletionToken"

	OperationInstall = "install"
	OperationRemove  = "remove"

	ConfigTypeHelm = "helm"
	ConfigTypeKapp = "kapp"

	HubControllerFinalizer = "hub-controller"

	DeployItemConfigVersion = "potter.gardener.cloud/v1"
	DeployItemStatusVersion = "potter.gardener.cloud/v1"
)
