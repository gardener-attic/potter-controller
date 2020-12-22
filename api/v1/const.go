package v1

const (
	AnnotationKeyLandscaperManaged   = "potter.gardener.cloud/landscaper-managed"
	AnnotationValueLandscaperManaged = "true"

	LabelClusterBomName         = "hub.kubernetes.sap.com/bom-name"
	LabelLandscaperManaged      = "potter.gardener.cloud/landscaper-managed"
	LabelApplicationConfigID    = "hub.kubernetes.sap.com/application-config-id"
	LabelConfigType             = "hub.kubernetes.sap.com/configType"
	LabelPurpose                = "hub.k8s.sap.com/purpose"
	LabelLogicalSecretName      = "hub.k8s.sap.com/logical-secret-name" // nolint
	LabelValueLandscaperManaged = "true"
)
