package deployutil

import (
	"testing"
	"time"

	hubv1 "github.com/gardener/potter-controller/api/v1"
	"github.com/gardener/potter-controller/pkg/util"

	"github.com/gardener/landscaper/pkg/apis/core/v1alpha1"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_ErrorHistory(t *testing.T) {
	providerStatus := &hubv1.HubDeployItemProviderStatus{}

	deployData := DeployData{
		deployItem:     &v1alpha1.DeployItem{},
		Configuration:  nil,
		ProviderStatus: providerStatus,
	}

	error00 := "error00"

	time00 := createTimeFromString("220902 050316")

	deployData.SetStatus(util.StateFailed, error00, 1, time00)

	errorHistory := deployData.ProviderStatus.LastOperation.ErrorHistory

	assert.Equal(t, len(errorHistory.ErrorEntries), 1, len(errorHistory.ErrorEntries))
	assert.Equal(t, errorHistory.ErrorEntries[0].Description, error00, errorHistory.ErrorEntries[0].Description)
	assert.Equal(t, errorHistory.ErrorEntries[0].Time, time00, errorHistory.ErrorEntries[0].Time)

	// add 4 errors
	error01 := "error01"
	time01 := createTimeFromString("220902 050317")
	deployData.SetStatus(util.StateFailed, error01, 2, time01)

	error02 := "error02"
	time02 := createTimeFromString("220902 050318")
	deployData.SetStatus(util.StateFailed, error02, 3, time02)

	error03 := "error03"
	time03 := createTimeFromString("220902 050319")
	deployData.SetStatus(util.StateFailed, error03, 4, time03)

	error04 := "error04"
	time04 := createTimeFromString("220902 050320")
	deployData.SetStatus(util.StateFailed, error04, 5, time04)

	errorHistory = deployData.ProviderStatus.LastOperation.ErrorHistory
	assert.Equal(t, len(errorHistory.ErrorEntries), 5, len(errorHistory.ErrorEntries))
	assert.Equal(t, errorHistory.ErrorEntries[0].Description, error00, errorHistory.ErrorEntries[0].Description)
	assert.Equal(t, errorHistory.ErrorEntries[0].Time, time00, errorHistory.ErrorEntries[0].Time)

	assert.Equal(t, errorHistory.ErrorEntries[1].Description, error01, errorHistory.ErrorEntries[1].Description)
	assert.Equal(t, errorHistory.ErrorEntries[1].Time, time01, errorHistory.ErrorEntries[1].Time)

	assert.Equal(t, errorHistory.ErrorEntries[2].Description, error02, errorHistory.ErrorEntries[2].Description)
	assert.Equal(t, errorHistory.ErrorEntries[2].Time, time02, errorHistory.ErrorEntries[2].Time)

	assert.Equal(t, errorHistory.ErrorEntries[3].Description, error03, errorHistory.ErrorEntries[3].Description)
	assert.Equal(t, errorHistory.ErrorEntries[3].Time, time03, errorHistory.ErrorEntries[3].Time)

	assert.Equal(t, errorHistory.ErrorEntries[4].Description, error04, errorHistory.ErrorEntries[4].Description)
	assert.Equal(t, errorHistory.ErrorEntries[4].Time, time04, errorHistory.ErrorEntries[4].Time)

	// replace oldest but not first one
	error05 := "error05"
	time05 := createTimeFromString("220902 050321")
	deployData.SetStatus(util.StateFailed, error05, 6, time05)

	errorHistory = deployData.ProviderStatus.LastOperation.ErrorHistory
	assert.Equal(t, len(errorHistory.ErrorEntries), 5, len(errorHistory.ErrorEntries))
	assert.Equal(t, errorHistory.ErrorEntries[0].Description, error00, errorHistory.ErrorEntries[0].Description)
	assert.Equal(t, errorHistory.ErrorEntries[0].Time, time00, errorHistory.ErrorEntries[0].Time)

	assert.Equal(t, errorHistory.ErrorEntries[1].Description, error02, errorHistory.ErrorEntries[1].Description)
	assert.Equal(t, errorHistory.ErrorEntries[1].Time, time02, errorHistory.ErrorEntries[1].Time)

	assert.Equal(t, errorHistory.ErrorEntries[2].Description, error03, errorHistory.ErrorEntries[2].Description)
	assert.Equal(t, errorHistory.ErrorEntries[2].Time, time03, errorHistory.ErrorEntries[2].Time)

	assert.Equal(t, errorHistory.ErrorEntries[3].Description, error04, errorHistory.ErrorEntries[3].Description)
	assert.Equal(t, errorHistory.ErrorEntries[3].Time, time04, errorHistory.ErrorEntries[3].Time)

	assert.Equal(t, errorHistory.ErrorEntries[4].Description, error05, errorHistory.ErrorEntries[4].Description)
	assert.Equal(t, errorHistory.ErrorEntries[4].Time, time05, errorHistory.ErrorEntries[4].Time)

	// replace one with the same description
	error06 := "error02"
	time06 := createTimeFromString("220902 050322")
	deployData.SetStatus(util.StateFailed, error06, 6, time06)

	errorHistory = deployData.ProviderStatus.LastOperation.ErrorHistory
	assert.Equal(t, len(errorHistory.ErrorEntries), 5, len(errorHistory.ErrorEntries))
	assert.Equal(t, errorHistory.ErrorEntries[0].Description, error00, errorHistory.ErrorEntries[0].Description)
	assert.Equal(t, errorHistory.ErrorEntries[0].Time, time00, errorHistory.ErrorEntries[0].Time)

	assert.Equal(t, errorHistory.ErrorEntries[1].Description, error03, errorHistory.ErrorEntries[1].Description)
	assert.Equal(t, errorHistory.ErrorEntries[1].Time, time03, errorHistory.ErrorEntries[1].Time)

	assert.Equal(t, errorHistory.ErrorEntries[2].Description, error04, errorHistory.ErrorEntries[2].Description)
	assert.Equal(t, errorHistory.ErrorEntries[2].Time, time04, errorHistory.ErrorEntries[2].Time)

	assert.Equal(t, errorHistory.ErrorEntries[3].Description, error05, errorHistory.ErrorEntries[3].Description)
	assert.Equal(t, errorHistory.ErrorEntries[3].Time, time05, errorHistory.ErrorEntries[3].Time)

	assert.Equal(t, errorHistory.ErrorEntries[4].Description, error06, errorHistory.ErrorEntries[4].Description)
	assert.Equal(t, errorHistory.ErrorEntries[4].Time, time06, errorHistory.ErrorEntries[4].Time)

	// try to replace first one
	error07 := "error00"
	time07 := createTimeFromString("220902 050323")
	deployData.SetStatus(util.StateFailed, error07, 7, time07)

	errorHistory = deployData.ProviderStatus.LastOperation.ErrorHistory
	assert.Equal(t, len(errorHistory.ErrorEntries), 5, len(errorHistory.ErrorEntries))
	assert.Equal(t, errorHistory.ErrorEntries[0].Description, error00, errorHistory.ErrorEntries[0].Description)
	assert.Equal(t, errorHistory.ErrorEntries[0].Time, time00, errorHistory.ErrorEntries[0].Time)

	assert.Equal(t, errorHistory.ErrorEntries[1].Description, error04, errorHistory.ErrorEntries[1].Description)
	assert.Equal(t, errorHistory.ErrorEntries[1].Time, time04, errorHistory.ErrorEntries[1].Time)

	assert.Equal(t, errorHistory.ErrorEntries[2].Description, error05, errorHistory.ErrorEntries[2].Description)
	assert.Equal(t, errorHistory.ErrorEntries[2].Time, time05, errorHistory.ErrorEntries[2].Time)

	assert.Equal(t, errorHistory.ErrorEntries[3].Description, error06, errorHistory.ErrorEntries[3].Description)
	assert.Equal(t, errorHistory.ErrorEntries[3].Time, time06, errorHistory.ErrorEntries[3].Time)

	assert.Equal(t, errorHistory.ErrorEntries[4].Description, error07, errorHistory.ErrorEntries[4].Description)
	assert.Equal(t, errorHistory.ErrorEntries[4].Time, time07, errorHistory.ErrorEntries[4].Time)
}

func createTimeFromString(timeString string) v1.Time {
	layout := "020106 150405"
	timestamp, _ := time.Parse(layout, timeString)
	return v1.Time{Time: timestamp}
}
