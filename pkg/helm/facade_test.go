package helm

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/zapr"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/release"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/gardener/potter-controller/pkg/util"
)

// nolint
const namespace = "test-ns"

// nolint
const brokenLoaderFuncErr = "some random error"

// nolint
var (
	fakeHelmClient    FakeHelmClient
	dummyChart        *ChartData
	updatedDummyChart *ChartData
	brokenChart       *ChartData
	facade            FacadeImpl
)

// nolint
func init() {
	dummyChart = &ChartData{
		InstallName: "dummy-chart",
		Values: map[string]interface{}{
			"foo": "bar",
		},
		Load: dummyLoaderFunc,
	}

	updatedDummyChart = &ChartData{
		InstallName: "dummy-chart",
		Values: map[string]interface{}{
			"bar": "foo",
		},
		Load: dummyLoaderFunc,
	}

	brokenChart = &ChartData{
		InstallName: "broken-chart",
		Values: map[string]interface{}{
			"foo": "bar",
		},
		Load: brokenLoaderFunc,
	}
}

func TestInstallFail(t *testing.T) {
	RegisterFailHandler(Fail)
	NewGomegaWithT(t)

	fakeHelmClient = FakeHelmClient{Releases: []release.Release{}}
	facade = FacadeImpl{Client: &fakeHelmClient}

	log := ctrl.Log.WithName("controllers").WithName("facade-test")
	ctx := context.Background()
	ctx = context.WithValue(ctx, util.LoggerKey{}, log)

	_, err := facade.InstallOrUpdate(ctx, brokenChart, namespace, "thisIsNoKubeconfig", nil)
	Expect(err).ToNot(BeNil())
	Expect(err.Error()).To(Equal(brokenLoaderFuncErr))
}

func TestInstallOk(t *testing.T) {
	RegisterFailHandler(Fail)
	NewGomegaWithT(t)

	fakeHelmClient = FakeHelmClient{Releases: []release.Release{}}
	facade = FacadeImpl{Client: &fakeHelmClient}

	log := ctrl.Log.WithName("controllers").WithName("facade-test")
	ctx := context.Background()
	ctx = context.WithValue(ctx, util.LoggerKey{}, log)

	_, err := facade.InstallOrUpdate(ctx, dummyChart, namespace, "thisIsNoKubeconfig", nil)
	Expect(err).To(BeNil())

	releases := facade.Client.(*FakeHelmClient).Releases

	installedRelease, err := checkForInstalledRelease(releases, dummyChart.InstallName)
	Expect(err).To(BeNil())
	Expect(installedRelease).ToNot(BeNil())
}

func TestUpdate(t *testing.T) {
	RegisterFailHandler(Fail)
	NewGomegaWithT(t)

	fakeHelmClient = FakeHelmClient{Releases: []release.Release{
		release.Release{Name: dummyChart.InstallName},
	}}
	facade = FacadeImpl{Client: &fakeHelmClient}

	log := ctrl.Log.WithName("controllers").WithName("facade-test")
	ctx := context.Background()
	ctx = context.WithValue(ctx, util.LoggerKey{}, log)

	_, err := facade.InstallOrUpdate(ctx, updatedDummyChart, namespace, "thisIsNoKubeconfigButItWorks", nil)
	Expect(err).To(BeNil())

	releases := facade.Client.(*FakeHelmClient).Releases

	installedRelease, err := checkForInstalledRelease(releases, dummyChart.InstallName)
	Expect(err).To(BeNil())
	Expect(installedRelease).ToNot(BeNil())
}

func TestRemoveOk(t *testing.T) {
	RegisterFailHandler(Fail)
	NewGomegaWithT(t)

	fakeHelmClient = FakeHelmClient{Releases: []release.Release{
		release.Release{Name: dummyChart.InstallName},
	}}
	facade = FacadeImpl{Client: &fakeHelmClient}

	ctx := context.TODO()
	ctx = context.WithValue(ctx, util.LoggerKey{}, zapr.NewLogger(zap.NewNop()))

	err := facade.Remove(ctx, dummyChart, namespace, "thisIsNoKubeconfigButItWorks")
	Expect(err).To(BeNil())
}

func TestRemoveOfNonExistentRelease(t *testing.T) {
	RegisterFailHandler(Fail)
	NewGomegaWithT(t)

	fakeHelmClient = FakeHelmClient{}
	facade = FacadeImpl{Client: &fakeHelmClient}

	ctx := context.TODO()
	ctx = context.WithValue(ctx, util.LoggerKey{}, zapr.NewLogger(zap.NewNop()))

	err := facade.Remove(ctx, dummyChart, namespace, "thisIsNoKubeconfigButItWorks")
	Expect(err).To(BeNil())
}

func checkForInstalledRelease(releases []release.Release, name string) (*release.Release, error) {
	for index := range releases {
		if releases[index].Name == name {
			return &releases[index], nil
		}
	}
	return nil, errors.New("release not found")
}

func dummyLoaderFunc() (*chart.Chart, error) {
	return &chart.Chart{}, nil
}

func brokenLoaderFunc() (*chart.Chart, error) {
	return &chart.Chart{}, errors.New(brokenLoaderFuncErr)
}
