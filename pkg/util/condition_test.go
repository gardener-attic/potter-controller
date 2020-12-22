package util

import (
	"testing"

	hubv1 "github.wdf.sap.corp/kubernetes/hub-controller/api/v1"

	"github.com/arschles/assert"
)

func TestEqualClusterBomCondition(t *testing.T) {
	condition1 := hubv1.ClusterBomCondition{Type: "Ready", Status: "True", Reason: "R", Message: "M"}
	condition2 := hubv1.ClusterBomCondition{Type: "Ready", Status: "True", Reason: "RRRRR", Message: "M"}
	condition3 := hubv1.ClusterBomCondition{Type: "Ready", Status: "False", Reason: "R", Message: "M"}
	condition4 := hubv1.ClusterBomCondition{Type: "Complete", Status: "Unknown", Reason: "R", Message: "M"}

	assert.True(t, IsEqualClusterBomCondition(&condition1, &condition1), "equality of conditions (1)")
	assert.False(t, IsEqualClusterBomCondition(&condition1, &condition2), "equality of conditions (2)")
	assert.False(t, IsEqualClusterBomCondition(&condition1, &condition3), "equality of conditions (3)")
	assert.False(t, IsEqualClusterBomCondition(&condition1, &condition4), "equality of conditions (4)")

	list1 := []hubv1.ClusterBomCondition{condition1, condition4}
	list2 := []hubv1.ClusterBomCondition{condition4, condition1}
	list3 := []hubv1.ClusterBomCondition{condition4, condition3}
	list4 := []hubv1.ClusterBomCondition{condition1}

	assert.True(t, IsEqualClusterBomConditionList(nil, nil), "equality of condition lists (1)")
	assert.True(t, IsEqualClusterBomConditionList(list1, list2), "equality of condition lists (2)")
	assert.False(t, IsEqualClusterBomConditionList(list1, list4), "equality of condition lists (3)")
	assert.False(t, IsEqualClusterBomConditionList(list4, list1), "equality of condition lists (4)")
	assert.False(t, IsEqualClusterBomConditionList(list2, list3), "equality of condition lists (5)")
}
