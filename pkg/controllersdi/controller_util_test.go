package controllersdi

import (
	"testing"

	"github.com/arschles/assert"

	hubv1 "github.com/gardener/potter-controller/api/v1"
	"github.com/gardener/potter-controller/pkg/deployutil"
	"github.com/gardener/potter-controller/pkg/util"
)

func TestWorseState(t *testing.T) {
	assert.Equal(t, deployutil.WorseState(util.StateOk, util.StateOk), util.StateOk, "state")
	assert.Equal(t, deployutil.WorseState(util.StateOk, util.StatePending), util.StatePending, "state")
	assert.Equal(t, deployutil.WorseState(util.StateOk, util.StateUnknown), util.StateUnknown, "state")
	assert.Equal(t, deployutil.WorseState(util.StateOk, util.StateFailed), util.StateFailed, "state")
	assert.Equal(t, deployutil.WorseState(util.StatePending, util.StateOk), util.StatePending, "state")
	assert.Equal(t, deployutil.WorseState(util.StatePending, util.StatePending), util.StatePending, "state")
	assert.Equal(t, deployutil.WorseState(util.StatePending, util.StateUnknown), util.StateUnknown, "state")
	assert.Equal(t, deployutil.WorseState(util.StatePending, util.StateFailed), util.StateFailed, "state")
	assert.Equal(t, deployutil.WorseState(util.StateUnknown, util.StateOk), util.StateUnknown, "state")
	assert.Equal(t, deployutil.WorseState(util.StateUnknown, util.StatePending), util.StateUnknown, "state")
	assert.Equal(t, deployutil.WorseState(util.StateUnknown, util.StateUnknown), util.StateUnknown, "state")
	assert.Equal(t, deployutil.WorseState(util.StateUnknown, util.StateFailed), util.StateFailed, "state")
	assert.Equal(t, deployutil.WorseState(util.StateFailed, util.StateOk), util.StateFailed, "state")
	assert.Equal(t, deployutil.WorseState(util.StateFailed, util.StatePending), util.StateFailed, "state")
	assert.Equal(t, deployutil.WorseState(util.StateFailed, util.StateUnknown), util.StateFailed, "state")
	assert.Equal(t, deployutil.WorseState(util.StateFailed, util.StateFailed), util.StateFailed, "state")
}

func TestComputeOverallState(t *testing.T) {
	clusterBomStateReconciler := ClusterBomStateReconciler{}

	applicationStates := []hubv1.ApplicationState{
		{State: util.StateOk},
		{State: util.StateOk},
		{State: util.StateOk},
	}
	overallState := clusterBomStateReconciler.computeOverallState(applicationStates)
	assert.Equal(t, overallState, util.StateOk, "overallState 1")

	applicationStates = []hubv1.ApplicationState{
		{State: util.StateOk},
		{State: util.StateFailed},
		{State: util.StateOk},
	}
	overallState = clusterBomStateReconciler.computeOverallState(applicationStates)
	assert.Equal(t, overallState, util.StateFailed, "overallState 2")

	applicationStates = []hubv1.ApplicationState{
		{State: util.StateOk},
		{State: util.StateUnknown},
		{State: util.StatePending},
	}
	overallState = clusterBomStateReconciler.computeOverallState(applicationStates)
	assert.Equal(t, overallState, util.StateUnknown, "overallState 3")

	applicationStates = []hubv1.ApplicationState{}
	overallState = clusterBomStateReconciler.computeOverallState(applicationStates)
	assert.Equal(t, overallState, util.StateOk, "overallState 4")

	overallState = clusterBomStateReconciler.computeOverallState(nil)
	assert.Equal(t, overallState, util.StateOk, "overallState 5")

	applicationStates = []hubv1.ApplicationState{
		{State: util.StateUnknown},
		{State: util.StateFailed},
	}
	overallState = clusterBomStateReconciler.computeOverallState(applicationStates)
	assert.Equal(t, overallState, util.StateFailed, "overallState 6")
}
