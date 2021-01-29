package deployutil

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/gardener/landscaper/pkg/apis/core/v1alpha1"
	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hubv1 "github.wdf.sap.corp/kubernetes/hub-controller/api/v1"
	"github.wdf.sap.corp/kubernetes/hub-controller/pkg/util"
)

type DeployData struct {
	deployItem     *v1alpha1.DeployItem
	Configuration  *hubv1.HubDeployItemConfiguration
	ProviderStatus *hubv1.HubDeployItemProviderStatus
	ExportValues   map[string]interface{}
}

func NewDeployData(deployItem *v1alpha1.DeployItem) (*DeployData, error) {
	configuration := &hubv1.HubDeployItemConfiguration{}
	if err := json.Unmarshal(deployItem.Spec.Configuration.Raw, configuration); err != nil {
		return nil, err
	}

	providerStatus := &hubv1.HubDeployItemProviderStatus{}
	if deployItem.Status.ProviderStatus != nil && len(deployItem.Status.ProviderStatus.Raw) > 0 {
		if err := json.Unmarshal(deployItem.Status.ProviderStatus.Raw, providerStatus); err != nil {
			return nil, err
		}
	}

	return &DeployData{
		deployItem:     deployItem,
		Configuration:  configuration,
		ProviderStatus: providerStatus,
	}, nil
}

func (d *DeployData) GetExportReference() *v1alpha1.ObjectReference {
	return d.deployItem.Status.ExportReference
}

func (d *DeployData) MarshalProviderStatus() error {
	encodedProviderStatus, err := json.Marshal(d.ProviderStatus)
	if err != nil {
		return err
	}

	d.deployItem.Status.ProviderStatus = &runtime.RawExtension{Raw: encodedProviderStatus}
	return nil
}

func (d *DeployData) GetStatus() (*v1alpha1.DeployItemStatus, error) {
	if err := d.MarshalProviderStatus(); err != nil {
		return nil, err
	}

	return &d.deployItem.Status, nil
}

func (d *DeployData) IsDeleteOperation() bool {
	return !d.deployItem.GetDeletionTimestamp().IsZero()
}

func (d *DeployData) IsInstallOperation() bool {
	return d.deployItem.GetDeletionTimestamp().IsZero()
}

func (d *DeployData) GetGeneration() int64 {
	return d.deployItem.GetGeneration()
}

func (d *DeployData) GetObservedGeneration() int64 {
	return d.deployItem.Status.ObservedGeneration
}

func (d *DeployData) GetNamespace() string {
	return d.deployItem.GetNamespace()
}

func (d *DeployData) GetDeployItemKey() *types.NamespacedName {
	return &types.NamespacedName{
		Name:      d.deployItem.Name,
		Namespace: d.deployItem.Namespace,
	}
}

func (d *DeployData) GetSecretKey() *types.NamespacedName {
	return &types.NamespacedName{
		Name:      d.Configuration.LocalSecretRef,
		Namespace: d.deployItem.Namespace,
	}
}

func (d *DeployData) GetConfigID() string {
	return d.Configuration.DeploymentConfig.ID
}

func (d *DeployData) IsNewOperation() bool {
	if !d.deployItem.GetDeletionTimestamp().IsZero() {
		return d.ProviderStatus.LastOperation.Operation != util.OperationRemove
	}

	return d.deployItem.Status.ObservedGeneration != d.deployItem.GetGeneration()
}

func (d *DeployData) IsFinallyFailed() bool {
	return d.ProviderStatus.Readiness != nil && d.ProviderStatus.Readiness.State == util.StateFinallyFailed
}

func (d *DeployData) IsLastDeployFailed() bool {
	return d.ProviderStatus.LastOperation.State == util.StateFailed
}

func (d *DeployData) IsInstallButNotReady() bool {
	isInstall := d.ProviderStatus.LastOperation.Operation == util.OperationInstall

	// readinessNotOk not really needed but we did not trust ourselves
	readinessNotOk := d.ProviderStatus.Readiness == nil || d.ProviderStatus.Readiness.State != util.StateOk

	readyCondition := util.GetDeployItemCondition(d.deployItem, hubv1.HubDeploymentReady)
	condition := util.GetDeployItemConditionStatus(readyCondition)
	isNotReadyStatus := condition != v1.ConditionTrue

	return isInstall && (readinessNotOk || isNotReadyStatus)
}

func (d *DeployData) IsReconcile() bool {
	// todo: the first and second checks can be removed
	return !d.Configuration.DeploymentConfig.NoReconcile &&
		d.deployItem.GetDeletionTimestamp().IsZero() &&
		util.HasAnnotation(d.deployItem, util.AnnotationKeyReconcile, util.AnnotationValueReconcile)
}

func (d *DeployData) SetPhase(phase v1alpha1.ExecutionPhase) {
	d.deployItem.Status.Phase = phase
}

func (d *DeployData) SetExportSecretName(secretName string) {
	d.deployItem.Status.ExportReference = &v1alpha1.ObjectReference{
		Name:      secretName,
		Namespace: d.GetNamespace(),
	}
}

func (d *DeployData) SetStatusForUnreachableCluster() {
	d.initLastOperation()

	d.ProviderStatus.Reachability = &hubv1.Reachability{
		Reachable: false,
		Time:      metav1.Now(),
	}
}

func (d *DeployData) SetStatusForReachableCluster() {
	d.initLastOperation()

	d.ProviderStatus.Reachability = &hubv1.Reachability{
		Reachable: true,
		Time:      metav1.Now(),
	}
}

// If a DeployItem was never processed, we must initialize the last operation
func (d *DeployData) initLastOperation() {
	if d.ProviderStatus.LastOperation.State != "" {
		// Already initialized
		return
	}

	d.ProviderStatus.LastOperation = hubv1.LastOperation{
		Operation:         util.OperationInstall,
		SuccessGeneration: 0,
		State:             util.StateOk,
		NumberOfTries:     0,
		Time:              metav1.Now(),
		Description:       "no last operation",
	}
}

func (d *DeployData) SetStatus(lastState, description string, numberOfTries int32, currentTime metav1.Time) {
	newSuccessGeneration := d.ProviderStatus.LastOperation.SuccessGeneration
	if lastState == util.StateOk {
		newSuccessGeneration = d.deployItem.GetGeneration()
	}

	newOperation := util.OperationRemove
	if d.deployItem.GetDeletionTimestamp().IsZero() {
		newOperation = util.OperationInstall
	}

	d.deployItem.Status.ObservedGeneration = d.deployItem.GetGeneration()

	d.ProviderStatus = &hubv1.HubDeployItemProviderStatus{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HubDeployItemProviderStatus",
			APIVersion: util.DeployItemStatusVersion,
		},
		LastOperation: hubv1.LastOperation{
			Operation:         newOperation,
			SuccessGeneration: newSuccessGeneration,
			NumberOfTries:     numberOfTries,
			State:             lastState,
			Time:              currentTime,
			Description:       description,
			ErrorHistory:      d.computeErrorHistory(lastState, description, numberOfTries, currentTime),
		},
		Reachability: &hubv1.Reachability{
			Reachable: true,
			Time:      currentTime,
		},
	}
}

func (d *DeployData) computeErrorHistory(lastState, description string, numberOfTries int32, currentTime metav1.Time) *hubv1.ErrorHistory {
	var errorHistory *hubv1.ErrorHistory

	if lastState == util.StateFailed {
		if numberOfTries < 2 || d.ProviderStatus.LastOperation.ErrorHistory == nil {
			errorEntry := hubv1.ErrorEntry{
				Description: description,
				Time:        currentTime,
			}

			errorHistory = &hubv1.ErrorHistory{
				ErrorEntries: []hubv1.ErrorEntry{errorEntry},
			}
		} else {
			errorEntry := hubv1.ErrorEntry{
				Description: description,
				Time:        currentTime,
			}

			errorEntries := d.ProviderStatus.LastOperation.ErrorHistory.ErrorEntries
			if len(errorEntries) < 5 {
				errorEntries = append(errorEntries, errorEntry)
			} else {
				d.sortErrorEntries(errorEntries)
				replaced := false

				for i := range errorEntries {
					nextErrorEntry := &errorEntries[i]
					if i > 0 && nextErrorEntry.Description == description {
						nextErrorEntry.Time = currentTime
						replaced = true
						break
					}
				}

				if !replaced {
					errorEntries[1].Time = currentTime
					errorEntries[1].Description = description
				}
			}

			d.sortErrorEntries(errorEntries)
			errorHistory = &hubv1.ErrorHistory{ErrorEntries: errorEntries}
		}
	}

	return errorHistory
}

func (d *DeployData) sortErrorEntries(errorEntries []hubv1.ErrorEntry) {
	sort.Slice(errorEntries, func(i, j int) bool {
		return errorEntries[i].Time.Time.Before(errorEntries[j].Time.Time)
	})
}

func (d *DeployData) WorsifyDeployItemCondition(conditionType hubv1.HubDeploymentConditionType,
	status v1.ConditionStatus, now metav1.Time, reason hubv1.HubDeploymentConditionReason, message string) {
	condition := d.GetDeployItemCondition(conditionType)

	if condition == nil {
		// Add a new condition
		readinessCondition := v1alpha1.Condition{
			Type:               v1alpha1.ConditionType(conditionType),
			Status:             v1alpha1.ConditionStatus(status),
			LastTransitionTime: now,
			LastUpdateTime:     now,
			Reason:             string(reason),
			Message:            message,
		}

		d.deployItem.Status.Conditions = append(d.deployItem.Status.Conditions, readinessCondition)
		return
	}

	if status == v1.ConditionFalse ||
		(status == v1.ConditionUnknown && condition.Status == v1alpha1.ConditionUnknown) ||
		condition.Status == v1alpha1.ConditionTrue {
		// status is worse or equal condition.Status
		condition.Status = v1alpha1.ConditionStatus(status)
		condition.LastTransitionTime = now
		condition.Reason = string(reason)
		condition.Message = message
		condition.LastUpdateTime = now
	}
}

func (d *DeployData) ReplaceDeployItemCondition(conditionType hubv1.HubDeploymentConditionType,
	status v1.ConditionStatus, now metav1.Time, reason hubv1.HubDeploymentConditionReason, message string) {
	condition := d.GetDeployItemCondition(conditionType)

	if condition == nil {
		// Add a new condition
		readinessCondition := v1alpha1.Condition{
			Type:               v1alpha1.ConditionType(conditionType),
			Status:             v1alpha1.ConditionStatus(status),
			LastTransitionTime: now,
			LastUpdateTime:     now,
			Reason:             string(reason),
			Message:            message,
		}

		d.deployItem.Status.Conditions = append(d.deployItem.Status.Conditions, readinessCondition)
	} else {
		// Adjust the existing condition
		if !isEqualHDCCondition(condition, status, reason, message) {
			condition.Status = v1alpha1.ConditionStatus(status)
			condition.LastTransitionTime = now
			condition.Reason = string(reason)
			condition.Message = message
		}

		condition.LastUpdateTime = now
	}
}

func isEqualHDCCondition(condition *v1alpha1.Condition, status v1.ConditionStatus,
	reason hubv1.HubDeploymentConditionReason, message string) bool {
	return string(condition.Status) == string(status) && condition.Reason == string(reason) && condition.Message == message
}

func (d *DeployData) GetDeployItemCondition(conditionType hubv1.HubDeploymentConditionType) *v1alpha1.Condition {
	for i := range d.deployItem.Status.Conditions {
		condition := &d.deployItem.Status.Conditions[i]
		if string(condition.Type) == string(conditionType) {
			return condition
		}
	}
	return nil
}

func (d *DeployData) IsConditionTrue(conditionType hubv1.HubDeploymentConditionType) bool {
	condition := d.GetDeployItemCondition(conditionType)
	return condition != nil && condition.Status == v1alpha1.ConditionTrue
}

func (d *DeployData) ComputeReadiness(ctx context.Context, basicKubernetesObjects []BasicKubernetesObject,
	targetClient client.Client, dynamicClient *DynamicTargetClient, namespace string) string {
	log := ctx.Value(util.LoggerKey{}).(logr.Logger)

	resultReadiness := util.StateOk

	for i := range basicKubernetesObjects {
		obj := &basicKubernetesObjects[i]

		loggerForObject := log.WithValues("object", obj)
		contextForObject := context.WithValue(ctx, util.LoggerKey{}, log)

		if obj.ObjectMeta.Namespace == "" {
			obj.ObjectMeta.Namespace = namespace
		}

		resource, err := readFromTargetCluster(contextForObject, targetClient, obj)
		if err != nil {
			loggerForObject.Error(err, "Error reading resource from target cluster")
			resultReadiness = WorseState(resultReadiness, util.StateUnknown)
		} else if !isResourceReady(contextForObject, resource) {
			loggerForObject.Info("Resource not ready: " + obj.ObjectMeta.String() + " of kind " + obj.Kind)
			resultReadiness = WorseState(resultReadiness, util.StatePending)
		}
	}

	resultReadiness = ComputeReadinessForResourceReadyRequirements(d.Configuration.DeploymentConfig.ReadyRequirements.Resources,
		resultReadiness, dynamicClient, log)

	for j := range d.Configuration.DeploymentConfig.ReadyRequirements.Jobs {
		job := &d.Configuration.DeploymentConfig.ReadyRequirements.Jobs[j]

		namespacedName := types.NamespacedName{
			Namespace: job.Namespace,
			Name:      job.Name,
		}

		basicObject := BasicKubernetesObject{
			APIVersion: util.APIVersionBatchV1,
			Kind:       util.KindJob,
			ObjectMeta: namespacedName,
		}

		loggerForObject := log.WithValues("object", namespacedName)
		contextForObject := context.WithValue(ctx, util.LoggerKey{}, log)

		resource, err := readFromTargetCluster(contextForObject, targetClient, &basicObject)
		if err != nil {
			loggerForObject.Error(err, "Error reading resource from target cluster")
			resultReadiness = WorseState(resultReadiness, util.StateUnknown)
			continue
		}

		switch typedResource := resource.(type) {
		// Readiness for different versions of DaemonSet
		case *batchv1.Job:
			tmpState := util.StatePending
			for k := range typedResource.Status.Conditions {
				condition := &typedResource.Status.Conditions[k]
				if condition.Type == batchv1.JobComplete && condition.Status == v1.ConditionTrue {
					tmpState = util.StateOk
					break
				} else if condition.Type == batchv1.JobFailed && condition.Status == v1.ConditionTrue {
					tmpState = util.StateFinallyFailed
					LogApplicationFailure(ctx, ReasonFailedJob, "Job "+namespacedName.String()+" has finally failed")
					break
				}
			}
			resultReadiness = WorseState(resultReadiness, tmpState)
		default:
			log.Error(nil, "Unknown resource type for job")
			resultReadiness = WorseState(resultReadiness, util.StateUnknown)
		}
	}

	return resultReadiness
}

func (d *DeployData) GetDeployItem() *v1alpha1.DeployItem {
	return d.deployItem
}
