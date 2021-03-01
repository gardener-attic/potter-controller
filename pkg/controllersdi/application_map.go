package controllersdi

import (
	ls "github.com/gardener/landscaper/apis/core/v1alpha1"

	"github.com/gardener/potter-controller/pkg/util"
)

type applicationItem struct {
	deployItem   *ls.DeployItem
	installation *ls.Installation
}

type applicationMap map[string]*applicationItem

func newApplicationMap(installationList *ls.InstallationList, deployItemList *ls.DeployItemList) applicationMap {
	m := applicationMap{}
	m.addInstallations(installationList.Items)
	m.addDeployItems(deployItemList.Items)
	return m
}

func (m applicationMap) addInstallations(installations []ls.Installation) {
	for i := range installations {
		m.addInstallation(&installations[i])
	}
}

func (m applicationMap) addInstallation(installation *ls.Installation) {
	if installation == nil {
		return
	}

	appID := util.GetAppConfigIDFromInstallation(installation)

	item := m[appID]
	if item == nil {
		item = &applicationItem{}
		m[appID] = item
	}

	item.installation = installation
}

func (m applicationMap) addDeployItems(deployItems []ls.DeployItem) {
	for i := range deployItems {
		m.addDeployItem(&deployItems[i])
	}
}

func (m applicationMap) addDeployItem(deployItem *ls.DeployItem) {
	if deployItem == nil {
		return
	}

	appID := util.GetAppConfigIDFromDeployItem(deployItem)

	item := m[appID]
	if item == nil {
		item = &applicationItem{}
		m[appID] = item
	}

	item.deployItem = deployItem
}
