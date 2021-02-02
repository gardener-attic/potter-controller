package util

import (
	"context"
	"os"
	"strconv"
	"time"

	"github.com/gardener/potter-controller/pkg/kapp"
	"github.com/gardener/potter-controller/pkg/util"

	"github.com/vmware-tanzu/carvel-kapp-controller/pkg/apis/kappctrl/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetPauseStatus(ctx context.Context, app *v1alpha1.App) *kapp.PauseStatus {
	pauseStatus, err := kapp.GetOldOrInitialPauseStatus(ctx, app)
	if err != nil {
		Write(err, "Unable to get pause status")
		os.Exit(1)
	}

	return pauseStatus
}

func CheckPauseStatus(paused bool, pausedSince time.Time, problem bool, problemSince time.Time,
	pauseStatus *kapp.PauseStatus) {
	if paused != pauseStatus.Paused || pausedSince != pauseStatus.PausedSince ||
		problem != pauseStatus.Problem || problemSince != pauseStatus.ProblemSince {
		Write("pause status not correct")
		os.Exit(1)
	}
}

func UpdatePauseStatus(ctx context.Context, gardenClient client.Client, appKey types.NamespacedName,
	paused bool, pausedSince time.Time, problem bool, problemSince time.Time) {
	pauseStatus := &kapp.PauseStatus{
		Paused:       paused,
		PausedSince:  pausedSince,
		Problem:      problem,
		ProblemSince: problemSince,
	}

	op := func() bool {
		app := &v1alpha1.App{}
		err := gardenClient.Get(ctx, appKey, app)
		if err != nil {
			Write(err, "Unable to get kapp app")
			return false
		}

		kapp.SetPauseStatus(app, pauseStatus)

		err = gardenClient.Update(ctx, app)

		if err != nil {
			Write(err, "error updating kapp app")
			return false
		}

		return true
	}

	done := util.Repeat(op, 10, 1*time.Second)

	if !done {
		Write("Unable to update app")
		os.Exit(1)
	}

	Write("App update")
}

func GetAppWithProblem(ctx context.Context, gardenClient client.Client,
	appKey types.NamespacedName, problem bool) *v1alpha1.App {
	app := &v1alpha1.App{}

	op := func() bool {
		err := gardenClient.Get(ctx, appKey, app)
		if err != nil {
			Write(err, "Unable to get app")
			return false
		}

		pauseStatus, err := kapp.GetOldOrInitialPauseStatus(ctx, app)

		if err != nil {
			Write(err, "Unable to get kapp pause status")
			return false
		}

		if pauseStatus.Problem == problem {
			return true
		}

		return false
	}

	done := util.Repeat(op, 6, 10*time.Second)

	if !done {
		Write("Unable to get app with problem == " + strconv.FormatBool(problem))
		os.Exit(1)
	}

	Write("App fetched")

	return app
}

func GetAppWithNewPausedSince(ctx context.Context, gardenClient client.Client,
	appKey types.NamespacedName, oldPausedSince time.Time) *v1alpha1.App {
	app := &v1alpha1.App{}

	op := func() bool {
		err := gardenClient.Get(ctx, appKey, app)
		if err != nil {
			Write(err, "Unable to get app with paused")
			return false
		}

		if app.Generation != app.Status.ObservedGeneration {
			Write("Generation " + strconv.FormatInt(app.Generation, 10) + " - observed " +
				strconv.FormatInt(app.Status.ObservedGeneration, 10))
			return false
		}

		pausedStatus := GetPauseStatus(ctx, app)

		if pausedStatus.PausedSince.After(oldPausedSince) {
			return true
		}

		Write("no new paused since date")

		return false
	}

	done := util.Repeat(op, 6, 10*time.Second)

	if !done {
		Write("Unable to get app new paused since and last observed generation")
		os.Exit(1)
	}

	Write("App fetched")

	return app
}

func GetAppWithPaused(ctx context.Context, gardenClient client.Client,
	appKey types.NamespacedName, paused bool) *v1alpha1.App {
	app := &v1alpha1.App{}

	op := func() bool {
		err := gardenClient.Get(ctx, appKey, app)
		if err != nil {
			Write(err, "Unable to get app with paused")
			return false
		}

		if app.Spec.Paused == paused {
			return true
		}

		return false
	}

	done := util.Repeat(op, 6, 10*time.Second)

	if !done {
		Write("Unable to get app with paused == " + strconv.FormatBool(paused))
		os.Exit(1)
	}

	Write("App fetched")

	return app
}
