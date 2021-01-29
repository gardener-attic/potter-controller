package kapp

import (
	"context"
	"strconv"
	"time"

	"github.com/vmware-tanzu/carvel-kapp-controller/pkg/apis/kappctrl/v1alpha1"

	"github.com/gardener/potter-controller/pkg/util"
)

const (
	annotationKappPausedSince  = "potter.gardener.cloud/kapp-paused-since"
	annotationKappProblem      = "potter.gardener.cloud/kapp-problem"
	annotationKappProblemSince = "potter.gardener.cloud/kapp-problem-since"
)

type PauseStatus struct {
	Paused      bool
	PausedSince time.Time

	Problem      bool
	ProblemSince time.Time
}

func GetOldOrInitialPauseStatus(ctx context.Context, app *v1alpha1.App) (*PauseStatus, error) {
	log := util.GetLoggerFromContext(ctx)

	pauseStatus := PauseStatus{}

	pauseStatus.Paused = app.Spec.Paused

	value, ok := util.GetAnnotation(app, annotationKappPausedSince)
	if ok {
		timestamp, err := time.Parse(time.RFC3339, value)
		if err != nil {
			log.Error(err, "error parsing annotationKappPausedSince: "+value)
			return nil, err
		}
		pauseStatus.PausedSince = timestamp
	}

	value, ok = util.GetAnnotation(app, annotationKappProblem)
	if ok {
		problem, err := strconv.ParseBool(value)
		if err != nil {
			log.Error(err, "error parsing annotationKappProblem: "+value)
			return nil, err
		}
		pauseStatus.Problem = problem
	}

	value, ok = util.GetAnnotation(app, annotationKappProblemSince)
	if ok {
		timestamp, err := time.Parse(time.RFC3339, value)
		if err != nil {
			log.Error(err, "error parsing annotationKappProblemSince: "+value)
			return nil, err
		}
		pauseStatus.ProblemSince = timestamp
	}

	return &pauseStatus, nil
}

func SetPauseStatus(app *v1alpha1.App, pauseStatus *PauseStatus) {
	app.Spec.Paused = pauseStatus.Paused
	util.AddAnnotation(app, annotationKappPausedSince, pauseStatus.PausedSince.Format(time.RFC3339))

	util.AddAnnotation(app, annotationKappProblem, strconv.FormatBool(pauseStatus.Problem))
	util.AddAnnotation(app, annotationKappProblemSince, pauseStatus.ProblemSince.Format(time.RFC3339))
}
