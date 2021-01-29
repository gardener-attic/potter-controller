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
	"bytes"
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakeK8s "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/gardener/potter-controller/pkg/util"

	"github.com/arschles/assert"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/repo"

	appRepov1 "github.com/gardener/potter-controller/api/external/apprepository/v1alpha1"
)

func Test_resolveChartURL(t *testing.T) {
	tests := []struct {
		name      string
		baseURL   string
		chartURL  string
		wantedURL string
	}{
		{
			"absolute url",
			"http://www.google.com",
			"http://charts.example.com/repo/wordpress-0.1.0.tgz",
			"http://charts.example.com/repo/wordpress-0.1.0.tgz",
		},
		{
			"relative, repo url",
			"http://charts.example.com/repo/",
			"wordpress-0.1.0.tgz",
			"http://charts.example.com/repo/wordpress-0.1.0.tgz",
		},
		{
			"relative, repo index url",
			"http://charts.example.com/repo/index.yaml",
			"wordpress-0.1.0.tgz",
			"http://charts.example.com/repo/wordpress-0.1.0.tgz",
		},
		{
			"relative, repo url - no trailing slash",
			"http://charts.example.com/repo",
			"wordpress-0.1.0.tgz",
			"http://charts.example.com/wordpress-0.1.0.tgz",
		},
		{
			"invalid base url",
			":foo",
			"wordpress-0.1.0.tgz",
			"",
		},
		{
			"invalid chart name",
			"http://charts.example.com/repo",
			":foo",
			"",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			chartURL, err := resolveChartURL(tt.baseURL, tt.chartURL)
			if tt.wantedURL == "" {
				assert.ExistsErr(t, err, "url parse error")
			} else {
				assert.NoErr(t, err)
			}
			assert.Equal(t, chartURL, tt.wantedURL, "url")
		})
	}
}

func TestFindChartInRepoIndex(t *testing.T) {
	name := "foo"
	version := "v1.0.0"
	chartURL := "wordpress-0.1.0.tgz"
	repoURL := "http://charts.example.com/repo/"
	expectedURL := fmt.Sprintf("%s%s", repoURL, chartURL)

	chartMeta := chart.Metadata{Name: name, Version: version}
	chartVersion := repo.ChartVersion{URLs: []string{chartURL}}
	chartVersion.Metadata = &chartMeta
	chartVersions := []*repo.ChartVersion{&chartVersion}
	entries := map[string]repo.ChartVersions{}
	entries[name] = chartVersions
	index := &repo.IndexFile{APIVersion: "v1", Generated: time.Now(), Entries: entries}

	res, err := findChartInRepoIndex(index, repoURL, name, version)
	if err != nil {
		t.Errorf("Unexpected error %v", err)
	}
	if res != expectedURL {
		t.Errorf("Expecting %s to be resolved as %s", res, expectedURL)
	}
}

const pemCert = `
-----BEGIN CERTIFICATE-----
MIIDETCCAfkCFEY03BjOJGqOuIMoBewOEDORMewfMA0GCSqGSIb3DQEBCwUAMEUx
CzAJBgNVBAYTAkRFMRMwEQYDVQQIDApTb21lLVN0YXRlMSEwHwYDVQQKDBhJbnRl
cm5ldCBXaWRnaXRzIFB0eSBMdGQwHhcNMTkwODE5MDQxNzU5WhcNMTkxMDA4MDQx
NzU5WjBFMQswCQYDVQQGEwJERTETMBEGA1UECAwKU29tZS1TdGF0ZTEhMB8GA1UE
CgwYSW50ZXJuZXQgV2lkZ2l0cyBQdHkgTHRkMIIBIjANBgkqhkiG9w0BAQEFAAOC
AQ8AMIIBCgKCAQEAzA+X6HcScuHxqxCc5gs68weW8i72qMjvcWvBG064SvpTuNDK
ECEGvug6f8SFJjpA+hWjlqR5+UPMdfjMKPUEg1CI8JZm6lyNiB54iY50qvhv+qQg
1STdAWNTzvqUXUMGIImzeXFnErxlq8WwwLGwPNT4eFxF8V8fzIhR8sqQKFLOqvpS
7sCQwF5QOhziGfS+zParDLFsBoXQpWyDKqxb/yBSPwqijKkuW7kF4jGfPHD0Re3+
rspXiq8+jWSwSJIPSIbya8DQqrMwFeLCAxABidPnlrwS0UUion557ylaBK6Cv0UB
MojA4SMfjm5xRdzrOcoE8EcabxqoQD5rCIBgFQIDAQABMA0GCSqGSIb3DQEBCwUA
A4IBAQCped08LTojPejkPqmp1edZa9rWWrCMviY5cvqb6t3P3erse+jVcBi9NOYz
8ewtDbR0JWYvSW6p3+/nwyDG4oVfG5TiooAZHYHmgg4x9+5h90xsnmgLhIsyopPc
Rltj86tRCl1YiuRpkWrOfRBGdYfkGEG4ihJzLHWRMCd1SmMwnmLliBctD7IeqBKw
UKt8wcroO8/sj/Xd1/LCtNZ79/FdQFa4l3HnzhOJOrlQyh4gyK05EKdg6vv3un17
l6NEPfiXd7dZvsWi9uY/PGBhu9EY/bdvuIOWDNNK262azk1A56HINpMrYBUcfti1
YrvYQHgOtHsqCB/hFHWfZp1lg2Sx
-----END CERTIFICATE-----
`

func TestInitNetClientForRawURL(t *testing.T) {
	systemCertPool, err := x509.SystemCertPool()
	if err != nil {
		t.Fatalf("%+v", err)
	}

	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte("username:password"))

	testCases := []struct {
		name             string
		customCAData     string
		errorExpected    bool
		numCertsExpected int
		authHeader       string
	}{
		{
			name:             "no custom CA and no auth header",
			numCertsExpected: len(systemCertPool.Subjects()),
		},
		{
			name:             "custom CA and auth header",
			customCAData:     pemCert,
			authHeader:       authHeader,
			numCertsExpected: len(systemCertPool.Subjects()) + 1,
		},
		{
			name:             "no custom CA and auth header",
			customCAData:     "",
			authHeader:       authHeader,
			numCertsExpected: len(systemCertPool.Subjects()),
		},
		{
			name:          "errors if custom CA cannot be parsed",
			authHeader:    authHeader,
			customCAData:  "not a valid cert",
			errorExpected: true,
		},
	}

	for _, tc := range testCases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			httpClient, err := InitNetClientForRawURL(tc.customCAData, tc.authHeader)

			if err != nil {
				if tc.errorExpected {
					return
				}
				t.Fatalf("%+v", err)
			}

			if err == nil && tc.errorExpected {
				t.Fatalf("got: nil, want: error")
			}

			clientWithDefaultHeaders, ok := httpClient.(*clientWithDefaultHeaders)
			if !ok {
				t.Fatalf("unable to assert expected type")
			}
			client, ok := clientWithDefaultHeaders.client.(*http.Client)
			if !ok {
				t.Fatalf("unable to assert expected type")
			}
			transport, ok := client.Transport.(*http.Transport)
			if !ok {
				t.Fatalf("unable to assert expected type")
			}
			certPool := transport.TLSClientConfig.RootCAs

			if got, want := len(certPool.Subjects()), tc.numCertsExpected; got != want {
				t.Errorf("got: %d, want: %d", got, want)
			}

			// If the Auth header was set, the default Authorization header should be set from the secret.
			if tc.authHeader != "" {
				_, ok := clientWithDefaultHeaders.defaultHeaders["Authorization"]
				if !ok {
					t.Fatalf("expected Authorization header but found none")
				}
				if got, want := clientWithDefaultHeaders.defaultHeaders.Get("Authorization"), authHeader; got != want {
					t.Errorf("got: %q, want: %q", got, want)
				}
			}
		})
	}
}

func TestInitNetClientForCatalogChart(t *testing.T) {
	systemCertPool, err := x509.SystemCertPool()
	if err != nil {
		t.Fatalf("%+v", err)
	}

	const (
		authHeaderSecretName = "auth-header-secret-name"
		authHeaderSecretData = "really-secret-stuff"
		customCASecretName   = "custom-ca-secret-name"
	)

	testCases := []struct {
		name             string
		customCAData     string
		appRepoSpec      appRepov1.AppRepositorySpec
		errorExpected    bool
		numCertsExpected int
	}{
		{
			name:             "default cert pool without auth",
			numCertsExpected: len(systemCertPool.Subjects()),
		},
		{
			name: "custom CA added when passed an AppRepository CR",
			appRepoSpec: appRepov1.AppRepositorySpec{
				Auth: appRepov1.AppRepositoryAuth{
					CustomCA: &appRepov1.AppRepositoryCustomCA{
						SecretKeyRef: corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: customCASecretName},
							Key:                  "custom-secret-key",
							Optional:             nil,
						},
					},
				},
			},
			customCAData:     pemCert,
			numCertsExpected: len(systemCertPool.Subjects()) + 1,
		},
		{
			name: "errors if secret for custom CA secret cannot be found",
			appRepoSpec: appRepov1.AppRepositorySpec{
				Auth: appRepov1.AppRepositoryAuth{
					CustomCA: &appRepov1.AppRepositoryCustomCA{
						SecretKeyRef: corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: "other-secret-name"},
							Key:                  "custom-secret-key",
							Optional:             nil,
						},
					},
				},
			},
			customCAData:  pemCert,
			errorExpected: true,
		},
		{
			name: "errors if custom CA key cannot be found in secret",
			appRepoSpec: appRepov1.AppRepositorySpec{
				Auth: appRepov1.AppRepositoryAuth{
					CustomCA: &appRepov1.AppRepositoryCustomCA{
						SecretKeyRef: corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: customCASecretName},
							Key:                  "some-other-secret-key",
							Optional:             nil,
						},
					},
				},
			},
			customCAData:  pemCert,
			errorExpected: true,
		},
		{
			name: "errors if custom CA cannot be parsed",
			appRepoSpec: appRepov1.AppRepositorySpec{
				Auth: appRepov1.AppRepositoryAuth{
					CustomCA: &appRepov1.AppRepositoryCustomCA{
						SecretKeyRef: corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: customCASecretName},
							Key:                  "custom-secret-key",
							Optional:             nil,
						},
					},
				},
			},
			customCAData:  "not a valid cert",
			errorExpected: true,
		},
		{
			name: "authorization header added when passed an AppRepository CR",
			appRepoSpec: appRepov1.AppRepositorySpec{
				Auth: appRepov1.AppRepositoryAuth{
					Header: &appRepov1.AppRepositoryAuthHeader{
						SecretKeyRef: corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: authHeaderSecretName},
							Key:                  "custom-secret-key",
							Optional:             nil,
						},
					},
				},
			},
			numCertsExpected: len(systemCertPool.Subjects()),
		},
		{
			name: "errors if auth secret cannot be found",
			appRepoSpec: appRepov1.AppRepositorySpec{
				Auth: appRepov1.AppRepositoryAuth{
					CustomCA: &appRepov1.AppRepositoryCustomCA{
						SecretKeyRef: corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: "other-secret-name"},
							Key:                  "custom-secret-key",
							Optional:             nil,
						},
					},
				},
			},
			errorExpected: true,
		},
	}

	for _, tc := range testCases {
		tc := tc

		// The fake k8s  will contain secret for the CA and header respectively.
		fakeK8sClient := fakeK8s.NewFakeClient(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      customCASecretName,
				Namespace: util.GetApprepoNamespace(),
			},
			Data: map[string][]byte{
				"custom-secret-key": []byte(tc.customCAData),
			},
		}, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      authHeaderSecretName,
				Namespace: util.GetApprepoNamespace(),
			},
			Data: map[string][]byte{
				"custom-secret-key": []byte(authHeaderSecretData),
			},
		})

		t.Run(tc.name, func(t *testing.T) {
			ctx := context.TODO()

			appRepo := &appRepov1.AppRepository{
				Spec: tc.appRepoSpec,
			}

			httpClient, err := InitNetClientForCatalogChart(ctx, appRepo, fakeK8sClient)

			if err != nil {
				if tc.errorExpected {
					return
				}
				t.Fatalf("%+v", err)
			}

			if err == nil && tc.errorExpected {
				t.Fatalf("got: nil, want: error")
			}

			clientWithDefaultHeaders, ok := httpClient.(*clientWithDefaultHeaders)
			if !ok {
				t.Fatalf("unable to assert expected type")
			}
			client, ok := clientWithDefaultHeaders.client.(*http.Client)
			if !ok {
				t.Fatalf("unable to assert expected type")
			}
			transport, ok := client.Transport.(*http.Transport)
			if !ok {
				t.Fatalf("unable to assert expected type")
			}
			certPool := transport.TLSClientConfig.RootCAs

			if got, want := len(certPool.Subjects()), tc.numCertsExpected; got != want {
				t.Errorf("got: %d, want: %d", got, want)
			}

			// If the Auth header was set, the default Authorization header should be set from the secret.
			if tc.appRepoSpec.Auth.Header != nil {
				_, ok := clientWithDefaultHeaders.defaultHeaders["Authorization"]
				if !ok {
					t.Fatalf("expected Authorization header but found none")
				}
				if got, want := clientWithDefaultHeaders.defaultHeaders.Get("Authorization"), authHeaderSecretData; got != want {
					t.Errorf("got: %q, want: %q", got, want)
				}
			}
		})
	}
}

// fakeReadChart implements ReadChart interface.
func fakeReadChart(in io.Reader) (*chart.Chart, error) {
	data, err := ioutil.ReadAll(in)
	if err != nil {
		panic(err)
	}

	var chartProps map[string]string
	err = json.Unmarshal(data, &chartProps)
	if err != nil {
		panic(err)
	}

	return &chart.Chart{
		Metadata: &chart.Metadata{
			Name:    chartProps["chartName"],
			Version: chartProps["chartVersion"],
		},
	}, nil
}

func TestLoadRawURL(t *testing.T) {
	const (
		chartName    = "test-chart"
		chartVersion = "0.0.1"
	)

	chartfilePath := fmt.Sprintf("/%s-%s.tgz", chartName, chartVersion)

	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		println(r.URL.Path)
		if r.URL.Path == chartfilePath {
			chartProps := map[string]string{
				"chartName":    chartName,
				"chartVersion": chartVersion,
			}

			data, err := json.Marshal(chartProps)
			assert.NoErr(t, err)

			_, err = w.Write(data)
			assert.NoErr(t, err)
		}
	}))
	defer testServer.Close()

	chartURL := testServer.URL + chartfilePath
	loaderFunc := LoadRawURL(context.Background(), "", "", chartURL, fakeReadChart)

	// Execute loader func -> this should load the chart data from the test http server
	ch, err := loaderFunc()

	assert.NoErr(t, err)
	assert.Equal(t, ch.Metadata.Name, chartName, "chart name")
	assert.Equal(t, ch.Metadata.Version, chartVersion, "chart version")
}

func TestLoadCatalogChart(t *testing.T) {
	const (
		chartName    = "test-chart"
		chartVersion = "0.0.1"
		repoName     = "foo-repo"
	)

	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chartfilePath := fmt.Sprintf("/%s-%s.tgz", chartName, chartVersion)
		if r.URL.Path == chartfilePath {
			chartProps := map[string]string{
				"chartName":    chartName,
				"chartVersion": chartVersion,
			}

			data, err := json.Marshal(chartProps)
			assert.NoErr(t, err)

			_, err = w.Write(data)
			assert.NoErr(t, err)
		} else if r.URL.Path == "/index.yaml" {
			chartData := &chart.Metadata{
				Name:    chartName,
				Version: chartVersion,
			}
			repoIndex := generateRepoIndex("//"+r.Host, []*chart.Metadata{chartData})

			data, err := json.Marshal(repoIndex)
			assert.NoErr(t, err)

			_, err = w.Write(data)
			assert.NoErr(t, err)
		}
	}))
	defer testServer.Close()

	apprepo := &appRepov1.AppRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      repoName,
			Namespace: "dummy-namespace",
		},
		Spec: appRepov1.AppRepositorySpec{
			URL: testServer.URL,
		},
	}

	fakeK8sClient := fakeK8s.NewFakeClient()
	ctx := context.Background()

	loaderFunc := LoadCatalogChart(ctx, apprepo, chartName, chartVersion, fakeReadChart, fakeK8sClient)

	// Execute loader func -> this should load the index.yaml and chart data from the test http server
	ch, err := loaderFunc()

	assert.NoErr(t, err)
	assert.Equal(t, ch.Metadata.Name, chartName, "chart name")
	assert.Equal(t, ch.Metadata.Version, chartVersion, "chart version")
}

func generateRepoIndex(repoURL string, charts []*chart.Metadata) *repo.IndexFile {
	entries := map[string]repo.ChartVersions{}
	for _, ch := range charts {
		chartMeta := chart.Metadata{Name: ch.Name, Version: ch.Version}
		chartURL := fmt.Sprintf("%s/%s-%s.tgz", repoURL, ch.Name, ch.Version)
		chartVersion := repo.ChartVersion{Metadata: &chartMeta, URLs: []string{chartURL}}
		chartVersions := []*repo.ChartVersion{&chartVersion}
		entries[ch.Name] = chartVersions
	}
	index := &repo.IndexFile{APIVersion: "v1", Generated: time.Now(), Entries: entries}
	return index
}

func TestGetIndexFromCache(t *testing.T) {
	repoURL := "https://test.com"
	data := []byte("foo")
	index, sha := getIndexFromCache(repoURL, data)
	if index != nil {
		t.Error("Index should be empty since it's not in the cache yet")
	}
	fakeIndex := &repo.IndexFile{}
	storeIndexInCache(repoURL, fakeIndex, sha)
	index, _ = getIndexFromCache(repoURL, data)
	if index != fakeIndex {
		t.Error("It should return the stored index")
	}
}

type fakeHTTPClient struct {
	repoURL   string
	chartURLs []string
	index     *repo.IndexFile
	userAgent string
	// TODO(absoludity): perhaps switch to use httptest instead of our own fake?
	requests []*http.Request
}

func (f *fakeHTTPClient) Do(h *http.Request) (*http.Response, error) {
	// Record the request for later test assertions.
	f.requests = append(f.requests, h)
	if f.userAgent != "" && h.Header.Get("User-Agent") != f.userAgent {
		return nil, fmt.Errorf("Wrong user agent: %s", h.Header.Get("User-Agent"))
	}
	if h.URL.String() == fmt.Sprintf("%sindex.yaml", f.repoURL) {
		// Return fake chart index
		body, err := json.Marshal(*f.index)
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: 200, Body: ioutil.NopCloser(bytes.NewReader(body))}, nil
	}
	for _, chartURL := range f.chartURLs {
		if h.URL.String() == chartURL {
			// Fake chart response
			return &http.Response{StatusCode: 200, Body: ioutil.NopCloser(bytes.NewReader([]byte{}))}, nil
		}
	}
	// Unexpected path
	return &http.Response{StatusCode: 404}, fmt.Errorf("Unexpected path")
}

// getFakeClientRequests returns the requests which were issued to the fake test client.
func getFakeClientRequests(t *testing.T, c HTTPClient) []*http.Request {
	clientWithDefaultUA, ok := c.(*clientWithDefaultHeaders)
	if !ok {
		t.Fatalf("client was not a clientWithDefaultUA")
	}
	fakeClient, ok := clientWithDefaultUA.client.(*fakeHTTPClient)
	if !ok {
		t.Fatalf("client was not a fakeHTTPClient")
	}
	return fakeClient.requests
}

func TestClientWithDefaultHeaders(t *testing.T) {
	testCases := []struct {
		name            string
		requestHeaders  http.Header
		defaultHeaders  http.Header
		expectedHeaders http.Header
	}{
		{
			name:            "no headers added when none set",
			defaultHeaders:  http.Header{},
			expectedHeaders: http.Header{},
		},
		{
			name:            "existing headers in the request remain present",
			requestHeaders:  http.Header{"Some-Other": []string{"value"}},
			defaultHeaders:  http.Header{},
			expectedHeaders: http.Header{"Some-Other": []string{"value"}},
		},
		{
			name: "headers are set when present",
			defaultHeaders: http.Header{
				"User-Agent":    []string{"foo/devel"},
				"Authorization": []string{"some-token"},
			},
			expectedHeaders: http.Header{
				"User-Agent":    []string{"foo/devel"},
				"Authorization": []string{"some-token"},
			},
		},
		{
			name: "headers can have multiple Values",
			defaultHeaders: http.Header{
				"Authorization": []string{"some-token", "some-other-token"},
			},
			expectedHeaders: http.Header{
				"Authorization": []string{"some-token", "some-other-token"},
			},
		},
		{
			name: "default headers do not overwrite request headers",
			requestHeaders: http.Header{
				"Authorization":        []string{"request-auth-token"},
				"Other-Request-Header": []string{"other-request-header"},
			},
			defaultHeaders: http.Header{
				"Authorization":        []string{"default-auth-token"},
				"Other-Default-Header": []string{"other-default-header"},
			},
			expectedHeaders: http.Header{
				"Authorization":        []string{"request-auth-token"},
				"Other-Request-Header": []string{"other-request-header"},
				"Other-Default-Header": []string{"other-default-header"},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			client := &clientWithDefaultHeaders{
				client:         &fakeHTTPClient{},
				defaultHeaders: tc.defaultHeaders,
			}

			request, err := http.NewRequest("GET", "http://example.com/foo", nil)
			if err != nil {
				t.Fatalf("%+v", err)
			}
			for k, v := range tc.requestHeaders {
				request.Header[k] = v
			}
			//nolint
			client.Do(request)

			requestsWithHeaders := getFakeClientRequests(t, client)
			if got, want := len(requestsWithHeaders), 1; got != want {
				t.Fatalf("got: %d, want: %d", got, want)
			}

			requestWithHeader := requestsWithHeaders[0]

			assert.True(t, reflect.DeepEqual(requestWithHeader.Header, tc.expectedHeaders), "request headers")
		})
	}
}
