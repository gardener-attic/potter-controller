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
	"fmt"
	"strings"
	"time"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/release"
)

type FakeHelmClient struct {
	Releases []release.Release
}

func (f *FakeHelmClient) GetReleaseStatus(ctx context.Context, namespace, relName, kubeconfig string) (release.Status, error) {
	return release.StatusDeployed, nil
}

func (f *FakeHelmClient) ResolveManifest(ctx context.Context, namespace string, values map[string]interface{}, ch *chart.Chart, kubeconfig string) (string, error) {
	return "", nil
}

func (f *FakeHelmClient) ResolveManifestFromRelease(ctx context.Context, namespace, releaseName string, revision int32, kubeconfig string) (string, error) {
	return "", nil
}

func (f *FakeHelmClient) ListReleases(ctx context.Context, namespace string, releaseListLimit int, status, kubeconfig string) ([]AppOverview, error) {
	var res []AppOverview
	for _, r := range f.Releases {
		relStatus := "DEPLOYED" // Default
		if r.Info != nil {
			relStatus = r.Info.Status.String()
		}
		if (namespace == "" || namespace == r.Namespace) &&
			len(res) <= releaseListLimit &&
			(r.Info == nil || strings.EqualFold(status, relStatus)) {
			res = append(res, AppOverview{
				ReleaseName: r.Name,
				Version:     "",
				Namespace:   r.Namespace,
				Icon:        "",
				Status:      relStatus,
			})
		}
	}
	return res, nil
}

func (f *FakeHelmClient) CreateRelease(ctx context.Context, chartData *ChartData, name, namespace string, values map[string]interface{}, metadata *ReleaseMetadata, timeout time.Duration, ch *chart.Chart, kubeconfig string) (*release.Release, error) {
	for _, r := range f.Releases {
		if r.Name == name {
			return nil, fmt.Errorf("release already exists")
		}
	}
	r := release.Release{
		Name:      name,
		Namespace: namespace,
	}
	f.Releases = append(f.Releases, r)
	return &r, nil
}

func (f *FakeHelmClient) UpdateRelease(ctx context.Context, chartData *ChartData, name, namespace string, values map[string]interface{}, metadata *ReleaseMetadata, timeout time.Duration, ch *chart.Chart, kubeconfig string) (*release.Release, error) {
	for _, r := range f.Releases {
		if r.Name == name {
			return &r, nil
		}
	}
	return nil, fmt.Errorf("release %s not found", name)
}

func (f *FakeHelmClient) RollbackRelease(ctx context.Context, name, namespace string, timeout time.Duration, revision int32, kubeconfig string) (*release.Release, error) {
	for _, r := range f.Releases {
		if r.Name == name {
			return &r, nil
		}
	}
	return nil, fmt.Errorf("release %s not found", name)
}

func (f *FakeHelmClient) GetRelease(ctx context.Context, name, namespace, kubeconfig string) (*release.Release, error) {
	for _, r := range f.Releases {
		if r.Name == name {
			return &r, nil
		}
	}
	return nil, fmt.Errorf("release: not found")
}

func (f *FakeHelmClient) DeleteRelease(ctx context.Context, chartData *ChartData, name, namespace string, timeout time.Duration, keepHistory bool, kubeconfig string) error {
	for i, r := range f.Releases {
		if r.Name == name {
			if !keepHistory {
				f.Releases[i] = f.Releases[len(f.Releases)-1]
				f.Releases = f.Releases[:len(f.Releases)-1]
			} else {
				r.Info = &release.Info{
					Status: release.StatusUninstalled,
				}
				f.Releases[i] = r
			}
			return nil
		}
	}
	return fmt.Errorf("release %s not found", name)
}
