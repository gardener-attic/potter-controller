package admission

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/gardener/landscaper/pkg/apis/core/v1alpha1"
	"github.com/go-logr/logr"
	"k8s.io/api/admission/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/jsonpath"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hubv1 "github.wdf.sap.corp/kubernetes/hub-controller/api/v1"
	"github.wdf.sap.corp/kubernetes/hub-controller/pkg/synchronize"
	"github.wdf.sap.corp/kubernetes/hub-controller/pkg/util"
)

type clusterBomReviewer struct {
	log               logr.Logger
	requestReview     *v1beta1.AdmissionReview
	reader            synchronize.UncachedClient
	configTypes       []string
	landscaperEnabled bool
}

func (r *clusterBomReviewer) review() *v1beta1.AdmissionReview {
	report := newReport(r.requestReview)

	clusterBom, oldClusterBom, oldApplConfigs := r.getObjectsToBeChecked(report)
	if report.denied() {
		return report.getResponseReview()
	}

	r.log = r.log.WithValues(util.LogKeyClusterBomName, util.GetKey(clusterBom))
	r.log.Info("Reviewing ClusterBom")

	r.checkName(report, clusterBom)
	if report.denied() {
		return report.getResponseReview()
	}

	r.checkSecretRef(report, clusterBom, oldClusterBom)
	if report.denied() {
		return report.getResponseReview()
	}

	r.checkLandscaperManaged(report, clusterBom, oldClusterBom)
	if report.denied() {
		return report.getResponseReview()
	}

	r.checkApplicationConfigs(report, clusterBom, oldApplConfigs)
	if report.denied() {
		return report.getResponseReview()
	}

	r.mutateClusterBom(report, clusterBom, oldApplConfigs)

	return report.getResponseReview()
}

func (r *clusterBomReviewer) getObjectsToBeChecked(report *report) (*hubv1.ClusterBom, *hubv1.ClusterBom, map[string]*hubv1.ApplicationConfig) {
	var clusterBom *hubv1.ClusterBom
	var oldClusterBom *hubv1.ClusterBom
	var oldApplConfigs = make(map[string]*hubv1.ApplicationConfig)

	err := json.Unmarshal(r.requestReview.Request.Object.Raw, &clusterBom)
	if err != nil {
		r.log.V(util.LogLevelWarning).Info("error when unmarshalling clusterbom", "error", err)
		report.deny("error when unmarshalling clusterbom: " + err.Error())
		return nil, nil, nil
	}

	if r.requestReview.Request.Operation == v1beta1.Update {
		err = json.Unmarshal(r.requestReview.Request.OldObject.Raw, &oldClusterBom)
		if err != nil {
			r.log.V(util.LogLevelWarning).Info("error when unmarshalling old clusterbom", "error", err)
			report.deny("error when unmarshalling old clusterbom : " + err.Error())
			return nil, nil, nil
		}

		for i := range oldClusterBom.Spec.ApplicationConfigs {
			oldAppConfig := &oldClusterBom.Spec.ApplicationConfigs[i]
			oldApplConfigs[oldAppConfig.ID] = oldAppConfig
		}
	}

	return clusterBom, oldClusterBom, oldApplConfigs
}

func (r *clusterBomReviewer) checkLandscaperManaged(report *report, clusterBom, oldClusterBom *hubv1.ClusterBom) {
	isLandscaperManaged := util.HasAnnotation(clusterBom, hubv1.AnnotationKeyLandscaperManaged, hubv1.AnnotationValueLandscaperManaged)

	if isLandscaperManaged && !r.landscaperEnabled {
		r.log.V(util.LogLevelWarning).Info("rejected clusterbom, because landscaper managed clusterboms are not supported")
		report.deny("landscaper managed clusterboms are not supported")
		return
	}

	if r.requestReview.Request.Operation == v1beta1.Update {
		wasLandscaperManaged := util.HasAnnotation(oldClusterBom, hubv1.AnnotationKeyLandscaperManaged, hubv1.AnnotationValueLandscaperManaged)
		if (isLandscaperManaged && !wasLandscaperManaged) ||
			(!isLandscaperManaged && wasLandscaperManaged && r.landscaperEnabled) {
			r.log.V(util.LogLevelWarning).Info("rejected clusterbom, because a switch between landscaper managed and not landscaper managed is forbidden")
			report.deny("a switch between landscaper managed and not landscaper managed is forbidden")
			return
		}
	}

	if !isLandscaperManaged && r.hasExportOrImportParameters(clusterBom) {
		r.log.V(util.LogLevelWarning).Info("rejected clusterbom, because export/import is only supported for landscaper managed clusterboms")
		report.deny("export/import is only supported for landscaper managed clusterboms, i.e. clusterboms with annotation potter.gardener.cloud/landscaper-managed: true")
		return
	}
}

func (r *clusterBomReviewer) hasExportOrImportParameters(clusterBom *hubv1.ClusterBom) bool {
	for i := range clusterBom.Spec.ApplicationConfigs {
		appConfig := &clusterBom.Spec.ApplicationConfigs[i]

		if len(appConfig.ExportParameters.Parameters) > 0 ||
			len(appConfig.ImportParameters) > 0 ||
			len(appConfig.InternalImportParameters.Parameters) > 0 {
			return true
		}
	}

	return false
}

func (r *clusterBomReviewer) checkName(report *report, clusterBom *hubv1.ClusterBom) {
	message := "The name of a clusterbom must consist of lower case alphanumeric characters or '-' or '.', " +
		"must start and end with an alphanumeric character, " +
		"and must not be longer than 63 characters (e.g. 'testclusterbom.01')."

	if len(clusterBom.ObjectMeta.Name) > 63 || len(clusterBom.ObjectMeta.Name) < 1 {
		r.log.V(util.LogLevelWarning).Info("rejected clusterbom, because its name is longer than 63 or shorter than one character")
		report.deny(message)
		return
	}

	pattern := `^[0-9a-z\.\-]+$`
	matched, err := regexp.MatchString(pattern, clusterBom.ObjectMeta.Name)
	if err != nil {
		r.log.Error(err, "error when matching name against pattern", "pattern", pattern)
		report.deny(message + " - " + err.Error())
		return
	}

	if !matched {
		r.log.V(util.LogLevelWarning).Info("rejected clusterbom, because its name does not fulfill the pattern", "pattern", pattern)
		report.deny(message)
		return
	}

	if strings.Contains(clusterBom.ObjectMeta.Name, util.DoubleSeparator) {
		text := "rejected clusterbom, because its name contains more than one consecutive minus sign"
		r.log.V(util.LogLevelWarning).Info(text)
		report.deny(text)
		return
	}
}

func (r *clusterBomReviewer) checkSecretRef(report *report, clusterBom, oldClusterBom *hubv1.ClusterBom) {
	r.log.Info("Reviewing SecretRef")

	if clusterBom.Spec.SecretRef == "" {
		r.log.V(util.LogLevelWarning).Info("rejected clusterbom, because spec.secretRef is empty")
		report.deny("spec.secretRef is empty")
		return
	}

	if r.requestReview.Request.Operation == v1beta1.Update && clusterBom.Spec.SecretRef != oldClusterBom.Spec.SecretRef {
		r.log.V(util.LogLevelWarning).Info("rejected clusterbom, because spec.secretRef must not be changed")
		report.deny("spec.secretRef must not be changed")
		return
	}

	if containsSpiffTemplate(clusterBom.Spec.SecretRef) {
		r.log.V(util.LogLevelWarning).Info("rejected clusterbom, because spec.secretRef cannot be templated")
		report.deny("spec.secretRef cannot be templated")
		return
	}
}

func (r *clusterBomReviewer) checkApplicationConfigs(report *report, clusterBom *hubv1.ClusterBom, oldApplConfigs map[string]*hubv1.ApplicationConfig) {
	r.log.Info("Reviewing Application Configs")

	r.checkApplicationConfigIDs(report, clusterBom)
	if report.denied() {
		return
	}

	for i := range clusterBom.Spec.ApplicationConfigs {
		applConfig := &clusterBom.Spec.ApplicationConfigs[i]
		oldApplConfig, oldApplConfigExists := oldApplConfigs[applConfig.ID]

		r.checkConfigType(report, applConfig, oldApplConfig, oldApplConfigExists)
		if report.denied() {
			return
		}

		r.checkTypeSpecificData(report, clusterBom, applConfig, oldApplConfig, oldApplConfigExists)
		if report.denied() {
			return
		}

		r.checkConflictWithExistingDeployment(report, clusterBom, applConfig, oldApplConfigExists)
		if report.denied() {
			return
		}

		r.checkResourceReadyRequirements(report, applConfig)
		if report.denied() {
			return
		}
	}
}

func (r *clusterBomReviewer) checkApplicationConfigIDs(report *report, clusterBom *hubv1.ClusterBom) {
	containedIdsMap := make(map[string]bool)

	for i := range clusterBom.Spec.ApplicationConfigs {
		applConfig := clusterBom.Spec.ApplicationConfigs[i]
		if applConfig.ID == "" {
			r.log.V(util.LogLevelWarning).Info("rejected clusterbom, because spec.applicationConfigs.id is empty")
			report.deny("spec.applicationConfigs.id is empty")
			return
		}

		if containsSpiffTemplate(applConfig.ID) {
			r.log.V(util.LogLevelWarning).Info("rejected clusterbom, because spec.applicationConfigs.id cannot be templated")
			report.deny("spec.applicationConfigs.id")
			return
		}

		if _, ok := containedIdsMap[applConfig.ID]; ok {
			r.log.V(util.LogLevelWarning).Info("rejected clusterbom, because applicationConfigs.id is a duplicate", "applConfig.ID", applConfig.ID)
			report.deny("applConfig.ID is a duplicate " + applConfig.ID)
			return
		}

		containedIdsMap[applConfig.ID] = true
	}
}

func (r *clusterBomReviewer) checkConfigType(report *report, applConfig, oldApplConfig *hubv1.ApplicationConfig, oldApplConfigExists bool) {
	if applConfig.ConfigType == "" {
		r.log.V(util.LogLevelWarning).Info("rejected clusterbom, because spec.applicationConfigs.configType is empty", "applConfig.ID", applConfig.ID)
		report.deny("spec.applicationConfigs.configType is empty")
		return
	}

	if !util.ContainsString(applConfig.ConfigType, r.configTypes) {
		r.log.V(util.LogLevelWarning).Info("rejected clusterbom, because spec.applicationConfigs.configType is not supported", "applConfig.ID", applConfig.ID)
		report.deny("spec.applicationConfigs.configType " + applConfig.ConfigType + " is not supported")
		return
	}

	// check that the configType was not updated
	if r.isUpdate() && oldApplConfigExists && applConfig.ConfigType != oldApplConfig.ConfigType {
		r.log.V(util.LogLevelWarning).Info("rejected clusterbom, because spec.applicationConfigs.configType must not be updated", "applConfig.ID", applConfig.ID)
		report.deny("spec.applicationConfigs.configType must not be updated")
		return
	}
}

func (r *clusterBomReviewer) checkTypeSpecificData(report *report, clusterBom *hubv1.ClusterBom, applConfig, oldApplConfig *hubv1.ApplicationConfig, oldApplConfigExists bool) {
	if applConfig.TypeSpecificData.Raw == nil {
		r.log.V(util.LogLevelWarning).Info("rejected clusterbom, because spec.applicationConfigs.typeSpecificData is empty", "applConfig.ID", applConfig.ID)
		report.deny("spec.applicationConfigs.typeSpecificData is empty")
		return
	}

	var oldTypeSpecificData *runtime.RawExtension
	if oldApplConfigExists {
		oldTypeSpecificData = &(oldApplConfig.TypeSpecificData)
	}

	switch applConfig.ConfigType {
	case util.ConfigTypeHelm:
		newHelmReviewer().reviewTypeSpecificData(report, &applConfig.TypeSpecificData, oldTypeSpecificData)
		if report.denied() {
			return
		}

	case util.ConfigTypeKapp:
		newKappReviewer().reviewTypeSpecificData(r.log, report, &applConfig.TypeSpecificData, clusterBom.Spec.SecretRef)
		if report.denied() {
			return
		}
	}
}

func (r *clusterBomReviewer) checkConflictWithExistingDeployment(report *report, clusterBom *hubv1.ClusterBom, applConfig *hubv1.ApplicationConfig, oldApplConfigExists bool) {
	r.checkConflictWithExistingDeployItem(report, clusterBom, applConfig, oldApplConfigExists)
}

func (r *clusterBomReviewer) checkConflictWithExistingDeployItem(report *report, clusterBom *hubv1.ClusterBom, applConfig *hubv1.ApplicationConfig, oldApplConfigExists bool) {
	if oldApplConfigExists {
		return
	}

	exists, err := r.existsDeployItem(clusterBom, applConfig.ID)
	if err != nil {
		r.log.Error(err, "cannot find out whether there still exists a deployitem for a new applicationConfig", "applConfig.ID", applConfig.ID)
		report.deny("cannot find out whether there still exists a deployitem for a new applicationConfig: " + err.Error())
		return
	} else if exists {
		report.deny("there still exists a deployitem for a new applicationConfig")
		return
	}
}

func (r *clusterBomReviewer) isCreate() bool {
	return r.requestReview.Request.Operation == v1beta1.Create
}

func (r *clusterBomReviewer) isUpdate() bool {
	return r.requestReview.Request.Operation == v1beta1.Update
}

func (r *clusterBomReviewer) mutateClusterBom(report *report, clusterBom *hubv1.ClusterBom, oldApplConfigs map[string]*hubv1.ApplicationConfig) {
	r.log.Info("Mutate ClusterBom")

	r.patchClusterBomLabels(report, clusterBom)
	r.patchFinalizer(report)
	r.patchSecretValues(report, clusterBom, oldApplConfigs)
	r.patchNamedSecretValues(report, clusterBom, oldApplConfigs)
}

// Adds secretRef to labels
func (r *clusterBomReviewer) patchClusterBomLabels(report *report, clusterBom *hubv1.ClusterBom) {
	r.log.Info("Patching ClusterBom Labels")

	labels := clusterBom.ObjectMeta.Labels
	if labels == nil {
		labels = make(map[string]string)
	}
	labels["hub.k8s.sap.com/cluster-name"] = clusterBom.Spec.SecretRef
	report.appendPatch("add", "/metadata/labels", labels)
}

// Adds a finalizer (only at creation). Its purpose is to postpone the deletion of the clusterbom until cleanup is done.
func (r *clusterBomReviewer) patchFinalizer(report *report) {
	r.log.Info("Patching Finalizer")

	if r.isCreate() {
		report.appendPatch("add", "/metadata/finalizers", []string{"hub-controller"})
	}
}

func (r *clusterBomReviewer) patchNamedSecretValues(report *report, clusterBom *hubv1.ClusterBom, oldApplConfigs map[string]*hubv1.ApplicationConfig) {
	r.log.Info("Patching Named Secret Values")

	patches := []patch{}

	for i := range clusterBom.Spec.ApplicationConfigs {
		appConfig := &clusterBom.Spec.ApplicationConfigs[i]
		oldAppConfig := oldApplConfigs[appConfig.ID]

		log := r.log.WithValues("app-id", appConfig.ID, "app-index", i)
		ctx := context.Background()
		ctx = context.WithValue(ctx, util.LoggerKey{}, log)

		var err error

		secretKeeper := &NamedSecretKeeper{
			client:       r.reader,
			dryRun:       (r.requestReview.Request.DryRun != nil) && *r.requestReview.Request.DryRun,
			clusterBom:   clusterBom,
			appIndex:     i,
			oldAppConfig: oldAppConfig,
			appConfig:    appConfig,
		}

		patches, err = secretKeeper.handleAppConfig(ctx, patches)
		if err != nil {
			report.deny("error when handling named secret values: " + err.Error())
			return
		}
	}

	report.appendPatches(patches...)
}

func (r *clusterBomReviewer) patchSecretValues(report *report, clusterBom *hubv1.ClusterBom, oldApplConfigs map[string]*hubv1.ApplicationConfig) {
	r.log.Info("Patching Secret Values")

	secretKeeper := &SecretKeeper{
		client: r.reader,
		dryRun: (r.requestReview.Request.DryRun != nil) && *r.requestReview.Request.DryRun,
	}

	patches := []patch{}

	for i := range clusterBom.Spec.ApplicationConfigs {
		appConfig := &clusterBom.Spec.ApplicationConfigs[i]
		oldAppConfig := oldApplConfigs[appConfig.ID]

		log := r.log.WithValues("app-id", appConfig.ID, "app-index", i)
		ctx := context.Background()
		ctx = context.WithValue(ctx, util.LoggerKey{}, log)

		var err error
		patches, err = secretKeeper.handleAppConfig(ctx, clusterBom, i, appConfig, oldAppConfig, patches)
		if err != nil {
			report.deny("error when handling secret values: " + err.Error())
			return
		}
	}

	report.appendPatches(patches...)
}

func (r *clusterBomReviewer) existsDeployItem(clusterBom *hubv1.ClusterBom, appConfigID string) (exists bool, err error) {
	ctx := context.Background()

	var deployItemList = v1alpha1.DeployItemList{}
	err = r.reader.ListUncached(ctx, &deployItemList,
		client.InNamespace(clusterBom.Namespace),
		client.MatchingLabels{
			hubv1.LabelClusterBomName:      clusterBom.Name,
			hubv1.LabelApplicationConfigID: appConfigID,
		})
	if err != nil {
		r.log.Error(err, "error listing deploy items")
		return false, err
	}

	exists = len(deployItemList.Items) > 0
	return exists, nil
}

func (r *clusterBomReviewer) checkResourceReadyRequirements(report *report, applConfig *hubv1.ApplicationConfig) {
	for i, readyRequirement := range applConfig.ReadyRequirements.Resources {
		if readyRequirement.Name == "" {
			msg := fmt.Sprintf("%s.readyRequirements.resources[%d].name is empty", applConfig.ID, i)
			r.log.V(util.LogLevelWarning).Info("rejected clusterbom, because " + msg)
			report.deny(msg)
			return
		}

		if readyRequirement.Namespace == "" {
			msg := fmt.Sprintf("%s.readyRequirements.resources[%d].namespace is empty", applConfig.ID, i)
			r.log.V(util.LogLevelWarning).Info("rejected clusterbom, because " + msg)
			report.deny(msg)
			return
		}

		if readyRequirement.APIVersion == "" {
			msg := fmt.Sprintf("%s.readyRequirements.resources[%d].apiVersion is empty", applConfig.ID, i)
			r.log.V(util.LogLevelWarning).Info("rejected clusterbom, because " + msg)
			report.deny(msg)
			return
		}

		if readyRequirement.Resource == "" {
			msg := fmt.Sprintf("%s.readyRequirements.resources[%d].resource is empty", applConfig.ID, i)
			r.log.V(util.LogLevelWarning).Info("rejected clusterbom, because " + msg)
			report.deny(msg)
			return
		}

		if readyRequirement.FieldPath == "" {
			msg := fmt.Sprintf("%s.readyRequirements.resources[%d].fieldPath is empty", applConfig.ID, i)
			r.log.V(util.LogLevelWarning).Info("rejected clusterbom, because " + msg)
			report.deny(msg)
			return
		}

		jsonPath := jsonpath.New("fieldPath")
		err := jsonPath.Parse(readyRequirement.FieldPath)
		if err != nil {
			msg := fmt.Sprintf("%s.readyRequirements.resources[%d].fieldPath cannot be parsed: %s", applConfig.ID, i, err.Error())
			r.log.V(util.LogLevelWarning).Info("rejected clusterbom, because " + msg)
			report.deny(msg)
			return
		}

		successValues, err := util.ParseSuccessValues(readyRequirement.SuccessValues)
		if err != nil {
			msg := fmt.Sprintf("%s.readyRequirements.resources[%d].successValues cannot be parsed: %s", applConfig.ID, i, err.Error())
			r.log.V(util.LogLevelWarning).Info("rejected clusterbom, because " + msg)
			report.deny(msg)
			return
		}

		if len(successValues) == 0 {
			msg := fmt.Sprintf("%s.readyRequirements.resources[%d].successValues is empty", applConfig.ID, i)
			r.log.V(util.LogLevelWarning).Info("rejected clusterbom, because " + msg)
			report.deny(msg)
			return
		}
	}
}
