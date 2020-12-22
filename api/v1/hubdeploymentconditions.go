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

package v1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// HubDeploymentCondition describes the state of a hubdeployment at a certain point.
type HubDeploymentCondition struct {
	// Type of hubdeploymentconfig condition.
	Type HubDeploymentConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status v1.ConditionStatus `json:"status"`
	// The last time this condition was updated.
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`
	// Last time the condition transitioned from one status to another.
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// The reason for the condition's last transition.
	Reason HubDeploymentConditionReason `json:"reason,omitempty"`
	// A human readable message indicating details about the transition.
	Message string `json:"message,omitempty"`
}

type HubDeploymentConditionType string

// These are valid conditions of a hubdeploymentconfig.
const (
	HubDeploymentReady HubDeploymentConditionType = "Ready"
)

type HubDeploymentConditionReason string

const (
	ReasonClusterUnreachable   HubDeploymentConditionReason = "ClusterUnreachable"
	ReasonInitialState         HubDeploymentConditionReason = "InitialState"
	ReasonUpgradePending       HubDeploymentConditionReason = "UpgradePending"
	ReasonRemovePending        HubDeploymentConditionReason = "RemovePending"
	ReasonRunning              HubDeploymentConditionReason = "Running"
	ReasonRemoved              HubDeploymentConditionReason = "Removed"
	ReasonNotRunning           HubDeploymentConditionReason = "NotRunning"
	ReasonFinallyFailed        HubDeploymentConditionReason = "FinallyFailed"
	ReasonNotCurrentGeneration HubDeploymentConditionReason = "NotCurrentGeneration"
	ReasonCouldNotGetExport    HubDeploymentConditionReason = "CouldNotGetExport"
)
