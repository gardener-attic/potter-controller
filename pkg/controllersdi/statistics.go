package controllersdi

import (
	"strings"

	hubv1 "github.com/gardener/potter-controller/api/v1"
)

type statistics struct {
	successfulApps []string
	pendingApps    []string
	failedApps     []string
}

func (s *statistics) addSuccessfulApp(appConfigID string) {
	s.successfulApps = append(s.successfulApps, appConfigID)
}

func (s *statistics) addPendingApp(appConfigID string) {
	s.pendingApps = append(s.pendingApps, appConfigID)
}

func (s *statistics) addFailedApp(appConfigID string) {
	s.failedApps = append(s.failedApps, appConfigID)
}

func (s *statistics) getOverallNumOfDeployments() int {
	return len(s.failedApps) + len(s.pendingApps) + len(s.successfulApps)
}

func (s *statistics) getOverallNumOfReadyDeployments() int {
	return len(s.successfulApps)
}

func (s *statistics) getOverallProgress() int {
	all := s.getOverallNumOfDeployments()

	if all == 0 {
		return 100
	}

	return s.getOverallNumOfReadyDeployments() * 100 / all
}

func (s *statistics) getPendingMessage() string {
	return "Pending applications: " + strings.Join(s.pendingApps, ", ")
}

func (s *statistics) getFailedMessage() string {
	return "Failed applications: " + strings.Join(s.failedApps, ", ")
}

func (s *statistics) getReasonAndMessageForReadyCondition() (reason hubv1.ClusterBomConditionReason, message string) {
	if len(s.failedApps) > 0 && len(s.pendingApps) > 0 {
		reason = hubv1.ReasonFailedAndPendingApps
		message = s.getFailedMessage() + " " + s.getPendingMessage()
	} else if len(s.failedApps) > 0 {
		reason = hubv1.ReasonFailedApps
		message = s.getFailedMessage()
	} else if len(s.pendingApps) > 0 {
		reason = hubv1.ReasonPendingApps
		message = s.getPendingMessage()
	} else {
		reason = hubv1.ReasonAllAppsReady
		message = "All applications are ready. "
	}

	return reason, message
}
