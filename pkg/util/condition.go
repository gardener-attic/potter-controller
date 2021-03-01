package util

import (
	"github.com/gardener/landscaper/apis/core/v1alpha1"
	kapp "github.com/vmware-tanzu/carvel-kapp-controller/pkg/apis/kappctrl/v1alpha1"
	v1 "k8s.io/api/core/v1"

	hubv1 "github.com/gardener/potter-controller/api/v1"
)

func GetClusterBomCondition(clusterbom *hubv1.ClusterBom, conditionType hubv1.ClusterBomConditionType) *hubv1.ClusterBomCondition {
	return GetClusterBomConditionFromStatus(&clusterbom.Status, conditionType)
}

func GetClusterBomConditionFromStatus(clusterbomStatus *hubv1.ClusterBomStatus, conditionType hubv1.ClusterBomConditionType) *hubv1.ClusterBomCondition {
	for i := range clusterbomStatus.Conditions {
		condition := &clusterbomStatus.Conditions[i]
		if condition.Type == conditionType {
			return condition
		}
	}
	return nil
}

func IsEqualClusterBomConditionList(oldConditions, newConditions []hubv1.ClusterBomCondition) bool {
	if len(oldConditions) != len(newConditions) {
		return false
	}

	for oldIndex := range oldConditions {
		oldCondition := &oldConditions[oldIndex]

		found := false
		for newIndex := range newConditions {
			newCondition := &newConditions[newIndex]

			if oldCondition.Type == newCondition.Type {
				found = true

				if !IsEqualClusterBomCondition(oldCondition, newCondition) {
					return false
				}
			}
		}

		if !found {
			return false
		}
	}

	return true
}

func IsEqualClusterBomCondition(c, d *hubv1.ClusterBomCondition) bool {
	return c.Type == d.Type && c.Status == d.Status && c.Reason == d.Reason && c.Message == d.Message
}

func IsEqualHdcConditionList(oldConditions, newConditions []hubv1.HubDeploymentCondition) bool {
	if len(oldConditions) != len(newConditions) {
		return false
	}

	for oldIndex := range oldConditions {
		oldCondition := &oldConditions[oldIndex]

		found := false
		for newIndex := range newConditions {
			newCondition := &newConditions[newIndex]

			if oldCondition.Type == newCondition.Type {
				found = true

				if oldCondition != newCondition {
					return false
				}
			}
		}

		if !found {
			return false
		}
	}

	return true
}

func GetDeployItemCondition(deployItem *v1alpha1.DeployItem, conditionType hubv1.HubDeploymentConditionType) *v1alpha1.Condition {
	for i := range deployItem.Status.Conditions {
		condition := &deployItem.Status.Conditions[i]
		if string(condition.Type) == string(conditionType) {
			return condition
		}
	}
	return nil
}

func GetDeployItemConditionStatus(c *v1alpha1.Condition) v1.ConditionStatus {
	if c == nil {
		return v1.ConditionUnknown
	}
	if string(c.Status) == string(v1.ConditionTrue) {
		return v1.ConditionTrue
	}

	if string(c.Status) == string(v1.ConditionFalse) {
		return v1.ConditionFalse
	}

	return v1.ConditionUnknown
}

func WorseConditionStatus(status1, status2 v1.ConditionStatus) v1.ConditionStatus {
	if status1 == v1.ConditionFalse || status2 == v1.ConditionFalse {
		return v1.ConditionFalse
	} else if status1 == v1.ConditionUnknown || status2 == v1.ConditionUnknown {
		return v1.ConditionUnknown
	}

	return v1.ConditionTrue
}

func MapToHdcCondition(conditions []v1alpha1.Condition) []hubv1.HubDeploymentCondition {
	if conditions == nil {
		return nil
	}
	result := make([]hubv1.HubDeploymentCondition, len(conditions))

	for i := range conditions {
		condition := &conditions[i]
		result[i] = hubv1.HubDeploymentCondition{
			Type:               hubv1.HubDeploymentConditionType(condition.Type),
			Status:             v1.ConditionStatus(condition.Status),
			LastUpdateTime:     condition.LastUpdateTime,
			LastTransitionTime: condition.LastTransitionTime,
			Reason:             hubv1.HubDeploymentConditionReason(condition.Reason),
			Message:            condition.Message,
		}
	}

	return result
}

func GetAppCondition(app *kapp.App, conditionType kapp.AppConditionType) *kapp.AppCondition {
	for i := range app.Status.Conditions {
		condition := &app.Status.Conditions[i]
		if condition.Type == conditionType {
			return condition
		}
	}
	return nil
}

func GetAppConditionStatus(app *kapp.App, conditionType kapp.AppConditionType) v1.ConditionStatus {
	condition := GetAppCondition(app, conditionType)

	if condition == nil {
		return v1.ConditionUnknown
	}
	if string(condition.Status) == string(v1.ConditionTrue) {
		return v1.ConditionTrue
	}

	if string(condition.Status) == string(v1.ConditionFalse) {
		return v1.ConditionFalse
	}

	return v1.ConditionUnknown
}
