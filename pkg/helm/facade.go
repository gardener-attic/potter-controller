package helm

import (
	"context"

	"github.com/gardener/potter-controller/pkg/deployutil"
	"github.com/gardener/potter-controller/pkg/util"

	"github.com/go-logr/logr"
	"helm.sh/helm/v3/pkg/release"
)

type Facade interface {
	GetRelease(ctx context.Context, chartData *ChartData, namespace, targetKubeconfig string) (*release.Release, error)
	InstallOrUpdate(context.Context, *ChartData, string, string, *ReleaseMetadata) (*release.Release, error)
	Remove(context.Context, *ChartData, string, string) error
}

type FacadeImpl struct {
	Client Client
}

func (fi *FacadeImpl) GetRelease(ctx context.Context, chartData *ChartData, namespace, targetKubeconfig string) (*release.Release, error) {
	rel, err := fi.Client.GetRelease(ctx, chartData.InstallName, namespace, targetKubeconfig)
	if err != nil && IsClusterUnreachableErr(err) {
		return nil, &deployutil.ClusterUnreachableError{Err: err}
	} else if err != nil {
		return nil, err
	} else {
		return rel, nil
	}
}

func (fi *FacadeImpl) InstallOrUpdate(ctx context.Context, chartData *ChartData, namespace, targetKubeconfig string, metadata *ReleaseMetadata) (*release.Release, error) {
	log := util.GetLoggerFromContext(ctx)

	rel, err := fi.installOrUpdateInternal(ctx, chartData, namespace, targetKubeconfig, metadata)

	if err != nil {
		rel2, err2 := fi.Client.GetRelease(ctx, chartData.InstallName, namespace, targetKubeconfig)
		if err2 != nil {
			log.Error(err2, "Error fetching helm release")
		}

		return rel2, err
	}

	return rel, nil
}

func (fi *FacadeImpl) installOrUpdateInternal(ctx context.Context, chartData *ChartData, namespace, targetKubeconfig string, metadata *ReleaseMetadata) (*release.Release, error) {
	ch, err := chartData.Load()
	if err != nil {
		return nil, err
	}
	_, err = fi.Client.GetRelease(ctx, chartData.InstallName, namespace, targetKubeconfig)
	if err != nil && IsClusterUnreachableErr(err) {
		return nil, &deployutil.ClusterUnreachableError{Err: err}
	} else if err != nil && IsReleaseNotFoundErr(err) {
		return fi.Client.CreateRelease(ctx, chartData, chartData.InstallName, namespace, chartData.Values, metadata, chartData.InstallTimeout, ch, targetKubeconfig)
	} else if err != nil {
		return nil, err
	} else {
		return fi.Client.UpdateRelease(ctx, chartData, chartData.InstallName, namespace, chartData.Values, metadata, chartData.UpgradeTimeout, ch, targetKubeconfig)
	}
}

func (fi *FacadeImpl) Remove(ctx context.Context, chartData *ChartData, namespace, targetKubeconfig string) error {
	log := ctx.Value(util.LoggerKey{}).(logr.Logger)
	_, err := fi.Client.GetRelease(ctx, chartData.InstallName, namespace, targetKubeconfig)
	if err != nil && IsClusterUnreachableErr(err) {
		return &deployutil.ClusterUnreachableError{Err: err}
	} else if err != nil && IsReleaseNotFoundErr(err) {
		log.V(util.LogLevelWarning).Info("release could not be found")
		return nil
	} else if err != nil {
		log.Error(err, "unknown release state")
		return err
	}
	return fi.Client.DeleteRelease(ctx, chartData, chartData.InstallName, namespace, chartData.UninstallTimeout, false, targetKubeconfig)
}
