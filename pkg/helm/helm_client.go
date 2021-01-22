/*
Copyright (c) 2018 Bitnami

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

package helm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	grpcStatus "google.golang.org/grpc/status"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/release"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/gardener/potter-controller/pkg/util"
)

// AppOverview represents the basics of a release
type AppOverview struct {
	ReleaseName   string         `json:"releaseName"`
	Version       string         `json:"version"`
	Namespace     string         `json:"namespace"`
	Icon          string         `json:"icon,omitempty"`
	Status        string         `json:"status"`
	Chart         string         `json:"chart"`
	ChartMetadata chart.Metadata `json:"chartMetadata"`
}

type clientImpl struct{}

func NewDefaultClient() Client {
	return &clientImpl{}
}

func (p *clientImpl) getRelease(ctx context.Context, kubeconfig, name, namespace string) (*release.Release, error) {
	config, err := initActionConfig(ctx, kubeconfig, namespace)
	if err != nil {
		return nil, err
	}

	rls, err := action.NewGet(config).Run(name)

	if err != nil {
		return nil, errors.New(prettyError(err).Error())
	}

	// We check that the release found is from the provided namespace.
	// If `namespace` is an empty string we do not do that check
	// This check check is to prevent users of for example updating releases that might be
	// in namespaces that they do not have access to.
	if namespace != "" && rls.Namespace != namespace {
		return nil, errors.Errorf("release %q not found in namespace %q", name, namespace)
	}

	return rls, err
}

// GetReleaseStatus prints the status of the given release if exists
func (p *clientImpl) GetReleaseStatus(ctx context.Context, namespace, relName, kubeconfig string) (release.Status, error) {
	config, err := initActionConfig(ctx, kubeconfig, namespace)
	if err != nil {
		return "", err
	}

	stats, err := action.NewStatus(config).Run(relName)

	if err == nil {
		if stats.Info != nil {
			return stats.Info.Status, nil
		}
	}
	return release.StatusUnknown, errors.Wrapf(err, "unable to fetch release status for %s", relName)
}

// ResolveManifest returns a manifest given the chart parameters
func (p *clientImpl) ResolveManifest(ctx context.Context, namespace string, values map[string]interface{}, ch *chart.Chart, kubeconfig string) (string, error) {
	// We use the release returned after running a dry-run to know the elements to install

	config, err := initActionConfig(ctx, kubeconfig, namespace)
	if err != nil {
		return "", err
	}

	install := action.NewInstall(config)
	install.DryRun = true
	install.ReleaseName = "dummyrlsname"
	install.Namespace = namespace

	resDry, err := install.Run(ch, values)

	if err != nil {
		return "", errors.Wrap(err, "could not run install dry run")
	}
	// The manifest returned has some extra new lines at the beginning
	return strings.TrimLeft(resDry.Manifest, "\n"), nil
}

// ResolveManifestFromRelease returns a manifest given the release name and revision
func (p *clientImpl) ResolveManifestFromRelease(ctx context.Context, namespace, releaseName string, revision int32, kubeconfig string) (string, error) {
	rel, err := p.GetRelease(ctx, releaseName, namespace, kubeconfig)

	if err != nil {
		return "", err
	}
	// The manifest returned has some extra new lines at the beginning
	return strings.TrimLeft(rel.Manifest, "\n"), nil
}

// Apply the same filtering than helm CLI
// Ref: https://github.com/helm/helm/blob/d3b69c1fc1ac62f1cc40f93fcd0cba275c0596de/cmd/helm/list.go#L173
func filterList(rels []*release.Release) []*release.Release {
	idx := map[string]int{}

	for _, r := range rels {
		name, version := r.Name, r.Version
		if max, ok := idx[name]; ok {
			// check if we have a greater version already
			if max > version {
				continue
			}
		}
		idx[name] = version
	}

	uniq := make([]*release.Release, 0, len(idx))
	for _, r := range rels {
		if idx[r.Name] == r.Version {
			uniq = append(uniq, r)
		}
	}
	return uniq
}

// ListReleases list releases in a specific namespace if given
func (p *clientImpl) ListReleases(ctx context.Context, namespace string, releaseListLimit int, status, kubeconfig string) ([]AppOverview, error) {
	config, err := initActionConfig(ctx, kubeconfig, namespace)
	if err != nil {
		return nil, err
	}

	listCommand := action.NewList(config)
	if status == "all" {
		listCommand.All = true
		listCommand.SetStateMask()
	}

	releases, err := listCommand.Run()

	if err != nil {
		return []AppOverview{}, errors.Wrapf(err, "unable to list helm releases")
	}
	appList := []AppOverview{}
	if releases != nil {
		filteredReleases := filterList(releases)
		for _, r := range filteredReleases {
			if namespace == "" || namespace == r.Namespace {
				appList = append(appList, AppOverview{
					ReleaseName:   r.Name,
					Version:       r.Chart.Metadata.Version,
					Namespace:     r.Namespace,
					Icon:          r.Chart.Metadata.Icon,
					Status:        r.Info.Status.String(),
					Chart:         r.Chart.Metadata.Name,
					ChartMetadata: *r.Chart.Metadata,
				})
			}
		}
	}
	return appList, nil
}

// CreateRelease creates a helm release
func (p *clientImpl) CreateRelease(ctx context.Context, chartData *ChartData, name, namespace string, values map[string]interface{},
	metadata *ReleaseMetadata, timeout time.Duration, ch *chart.Chart, kubeconfig string) (*release.Release, error) {
	log := ctx.Value(util.LoggerKey{}).(logr.Logger)

	log.V(util.LogLevelDebug).Info(fmt.Sprintf("Installing release %s into namespace %s", name, namespace))

	err := ensureNamespace(ctx, kubeconfig, namespace)
	if err != nil {
		return nil, err
	}

	config, err := initActionConfig(ctx, kubeconfig, namespace)
	if err != nil {
		return nil, err
	}

	install := action.NewInstall(config)
	install.Namespace = namespace
	install.ReleaseName = name

	install.Timeout = timeout

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, errors.Wrap(err, "cannot marshal release metadata")
	}
	install.Description = string(metadataJSON)

	if util.ContainsString(InstallArgAtomic, chartData.InstallArguments) {
		install.Atomic = true
	}

	log.V(util.LogLevelDebug).Info(fmt.Sprintf("Installing chart %s", name))
	rel, err := install.Run(ch, values)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to create the release")
	}

	if isHubSecretEnabled(rel) {
		var hubsec *string
		hubsec, err = readDockerconfigSecret()
		if err != nil {
			return nil, errors.New("Helmchart had hubSecret configured but cannot read image pull secret")
		}
		imageSecretManager := newImageSecretManager(ctx, kubeconfig)
		err = imageSecretManager.createOrUpdateImageSecret(ctx, rel, hubsec)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to create image pull secret")
		}
	}

	log.V(util.LogLevelDebug).Info(fmt.Sprintf("%s successfully installed in %s", name, namespace))

	return rel, err
}

// UpdateRelease upgrades a tiller release
func (p *clientImpl) UpdateRelease(ctx context.Context, chartData *ChartData, name, namespace string, values map[string]interface{},
	metadata *ReleaseMetadata, timeout time.Duration, ch *chart.Chart, kubeconfig string) (*release.Release, error) {
	log := ctx.Value(util.LoggerKey{}).(logr.Logger)

	log.V(util.LogLevelDebug).Info(fmt.Sprintf("Updating release %s", name))

	config, err := initActionConfig(ctx, kubeconfig, namespace)
	if err != nil {
		return nil, err
	}

	upgrade := action.NewUpgrade(config)
	upgrade.Namespace = namespace

	upgrade.Timeout = timeout

	if util.ContainsString(UpdateArgAtomic, chartData.UpdateArguments) {
		upgrade.Atomic = true
	}

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, errors.Wrap(err, "cannot marshal release metadata")
	}
	upgrade.Description = string(metadataJSON)

	upgrade.MaxHistory = 10

	rel, err := upgrade.Run(name, ch, values)

	if err != nil {
		return nil, errors.Wrap(err, "unable to update the release")
	}
	return rel, err
}

// RollbackRelease rolls back to a specific revision
func (p *clientImpl) RollbackRelease(ctx context.Context, name, namespace string, timeout time.Duration, revision int32, kubeconfig string) (*release.Release, error) {
	// Check if the release already exists
	config, err := initActionConfig(ctx, kubeconfig, namespace)
	if err != nil {
		return nil, err
	}

	_, err = p.getRelease(ctx, kubeconfig, name, namespace)
	if err != nil {
		return nil, err
	}

	rollbackCommand := action.NewRollback(config)
	rollbackCommand.Version = int(revision)

	rollbackCommand.Timeout = timeout

	err = rollbackCommand.Run(name)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to rollback the release")
	}
	return p.getRelease(ctx, kubeconfig, name, namespace)
}

// GetRelease returns the info of a release
func (p *clientImpl) GetRelease(ctx context.Context, name, namespace, kubeconfig string) (*release.Release, error) {
	return p.getRelease(ctx, kubeconfig, name, namespace)
}

// DeleteRelease deletes a release
func (p *clientImpl) DeleteRelease(ctx context.Context, chartData *ChartData, name, namespace string, timeout time.Duration, keepHistory bool, kubeconfig string) error {
	log := ctx.Value(util.LoggerKey{}).(logr.Logger)

	log.V(util.LogLevelDebug).Info(fmt.Sprintf("Deleting release %s in namespace %s", name, namespace))
	// Validate that the release actually belongs to the namespace
	_, err := p.getRelease(ctx, kubeconfig, name, namespace)
	if err != nil {
		return err
	}

	config, err := initActionConfig(ctx, kubeconfig, namespace)
	if err != nil {
		return err
	}
	uninstall := action.NewUninstall(config)
	uninstall.KeepHistory = keepHistory

	uninstall.Timeout = timeout

	rel, uninstallErr := uninstall.Run(name)
	if uninstallErr != nil {
		return errors.Wrap(uninstallErr, "unable to delete the release")
	}

	hubsec, enabled := os.LookupEnv("HUBSEC_DOCKERCONFIGJSON")
	log.V(util.LogLevelDebug).Info(fmt.Sprintf("Secret configured: %t", enabled))
	if enabled {
		log.V(util.LogLevelDebug).Info(fmt.Sprintf("Secret configured: %s", hubsec))
		releases, listErr := p.ListReleases(ctx, namespace, 0, "all", kubeconfig)
		if listErr != nil {
			return errors.Wrap(listErr, "unable to list release to check if imagepullsecret has to be deleted")
		}
		if len(releases) == 0 {
			imageSecretManager := newImageSecretManager(ctx, kubeconfig)
			err = imageSecretManager.deleteImageSecret(ctx, rel.Release)
			if err != nil {
				return errors.Wrap(err, "unable to delete image pull secret")
			}
		}
	}

	log.V(util.LogLevelDebug).Info(fmt.Sprintf("%s successfully deleted in %s", name, namespace))

	return err
}

// ensureNamespace make sure we create a namespace in the cluster in case it does not exist
func ensureNamespace(ctx context.Context, kubeconfig, namespace string) error {
	log := ctx.Value(util.LoggerKey{}).(logr.Logger)

	log.V(util.LogLevelDebug).Info(fmt.Sprintf("Ensuring namespace %s exists", namespace))
	// The namespace might not exist yet, so we create the clientset with the default namespace
	clientset, err := getClientSet(ctx, kubeconfig, "default")
	if err != nil {
		return errors.Wrapf(err, "Error creating kubernetes client for namespace %s.", namespace)
	}
	_, err = clientset.CoreV1().Namespaces().Get(context.TODO(), namespace, metav1.GetOptions{})
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			ns := corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}

			_, err = clientset.CoreV1().Namespaces().Create(context.TODO(), &ns, metav1.CreateOptions{})

			if err != nil {
				if k8sErrors.IsAlreadyExists(err) {
					log.V(util.LogLevelWarning).Info("could not create namespace %s, because it already exists", namespace)
					return nil
				}

				return errors.Wrapf(err, "could not create namespace %s.", namespace)
			}
			return nil
		}
		return errors.Wrapf(err, "unable to fetch namespace %s", namespace)
	}
	return nil
}

// extracted from https://github.com/helm/helm/blob/master/cmd/helm/helm.go#L227
// prettyError unwraps or rewrites certain errors to make them more user-friendly.
func prettyError(err error) error {
	// Add this check can prevent the object creation if err is nil.
	if err == nil {
		return nil
	}
	// If it's grpc's error, make it more user-friendly.
	if s, ok := grpcStatus.FromError(err); ok {
		return fmt.Errorf(s.Message())
	}
	// Else return the original error.
	return err
}

// Client for exposed funcs
type Client interface {
	GetReleaseStatus(ctx context.Context, namespace, relName, kubeconfig string) (release.Status, error)
	ResolveManifest(ctx context.Context, namespace string, values map[string]interface{}, ch *chart.Chart, kubeconfig string) (string, error)
	ResolveManifestFromRelease(ctx context.Context, namespace, releaseName string, revision int32, kubeconfig string) (string, error)
	ListReleases(ctx context.Context, namespace string, releaseListLimit int, status, kubeconfig string) ([]AppOverview, error)
	CreateRelease(ctx context.Context, chartData *ChartData, name, namespace string, values map[string]interface{}, metadata *ReleaseMetadata, timeout time.Duration, ch *chart.Chart, kubeconfig string) (*release.Release, error)
	UpdateRelease(ctx context.Context, chartData *ChartData, name, namespace string, values map[string]interface{}, metadata *ReleaseMetadata, timeout time.Duration, ch *chart.Chart, kubeconfig string) (*release.Release, error)
	RollbackRelease(ctx context.Context, name, namespace string, timeout time.Duration, revision int32, kubeconfig string) (*release.Release, error)
	GetRelease(ctx context.Context, name, namespace, kubeconfig string) (*release.Release, error)
	DeleteRelease(ctx context.Context, chartData *ChartData, name, namespace string, timeout time.Duration, keepHistory bool, kubeconfig string) error
}
