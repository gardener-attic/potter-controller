package helm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	appRepov1 "github.com/gardener/potter-controller/api/external/apprepository/v1alpha1"
	"github.com/gardener/potter-controller/pkg/util"

	"github.com/ghodss/yaml"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/repo"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultTimeoutSeconds = 180
)

type repoIndex struct {
	checksum string
	index    *repo.IndexFile
}

// nolint
var repoIndexes map[string]*repoIndex

// nolint
func init() {
	repoIndexes = map[string]*repoIndex{}
}

// HTTPClient Interface to perform HTTP requests
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// ReadChart should return a Chart struct from an IOReader
type ReadChart func(in io.Reader) (*chart.Chart, error)

// clientWithDefaultHeaders implements chart.HTTPClient interface
// and includes an override of the Do method which injects our default
// headers - User-Agent and Authorization (when present)
type clientWithDefaultHeaders struct {
	client         HTTPClient
	defaultHeaders http.Header
}

// fetchRepoIndex returns a Helm repository
func fetchRepoIndex(ctx context.Context, netClient HTTPClient, repoURL string) (*repo.IndexFile, error) {
	req, err := getReq(repoURL)
	if err != nil {
		return nil, err
	}

	res, err := (netClient).Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "request failed")
	}
	data, err := readResponseBody(ctx, res)
	if err != nil {
		return nil, err
	}

	index, sha := getIndexFromCache(repoURL, data)
	if index == nil {
		// index not found in the cache, parse it
		index, err = parseIndex(data)
		if err != nil {
			return nil, err
		}
		storeIndexInCache(repoURL, index, sha)
	}
	return index, nil
}

// Do HTTP request
func (c *clientWithDefaultHeaders) Do(req *http.Request) (*http.Response, error) {
	for k, v := range c.defaultHeaders {
		// Only add the default header if it's not already set in the request.
		if _, ok := req.Header[k]; !ok {
			req.Header[k] = v
		}
	}
	return c.client.Do(req)
}

func getReq(rawURL string) (*http.Request, error) {
	parsedURL, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return nil, errors.Wrap(err, "could not parse URL")
	}

	req, err := http.NewRequest("GET", parsedURL.String(), nil)
	if err != nil {
		return nil, errors.Wrap(err, "could not create request object")
	}

	return req, nil
}

// findChartInRepoIndex returns the URL of a chart given a Helm repository and its name and version
func findChartInRepoIndex(repoIndex *repo.IndexFile, repoURL, chartName, chartVersion string) (string, error) {
	errMsg := fmt.Sprintf("chart %q", chartName)
	if chartVersion != "" {
		errMsg = fmt.Sprintf("%s version %q", errMsg, chartVersion)
	}
	cv, err := repoIndex.Get(chartName, chartVersion)
	if err != nil {
		return "", errors.Errorf("%s not found in repository", errMsg)
	}
	if len(cv.URLs) == 0 {
		return "", errors.Errorf("%s has no downloadable URLs", errMsg)
	}
	return resolveChartURL(repoURL, cv.URLs[0])
}

func resolveChartURL(index, chartName string) (string, error) {
	indexURL, err := url.Parse(strings.TrimSpace(index))
	if err != nil {
		return "", errors.Wrap(err, "could not parse chart url")
	}
	chartURL, err := indexURL.Parse(strings.TrimSpace(chartName))
	if err != nil {
		return "", errors.Wrap(err, "could not parse chart url")
	}
	return chartURL.String(), nil
}

func readResponseBody(ctx context.Context, res *http.Response) ([]byte, error) {
	if res == nil {
		return nil, errors.New("response must not be nil")
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		logger := ctx.Value(util.LoggerKey{}).(logr.Logger)

		err := errors.New(fmt.Sprintf("chart download request failed with status code %v", res.StatusCode))

		if logger.V(util.LogLevelDebug).Enabled() {
			body, bodyReadErr := ioutil.ReadAll(res.Body)
			if bodyReadErr != nil {
				logger.Error(err, err.Error(), "response status code without body", res.StatusCode)
				return nil, err
			}

			logger.Error(err, err.Error(), "response status code with body", res.StatusCode,
				"response body", string(body))
		}

		return nil, err
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, errors.Wrap(err, "could not read response body")
	}
	return body, nil
}

// Cache the result of parsing the repo index since parsing this YAML
// is an expensive operation. See https://github.wdf.sap.corp/kubernetes/hub/issues/1052
func getIndexFromCache(repoURL string, data []byte) (*repo.IndexFile, string) {
	sha := checksum(data)
	if repoIndexes[repoURL] == nil || repoIndexes[repoURL].checksum != sha {
		// The repository is not in the cache or the content changed
		return nil, sha
	}
	return repoIndexes[repoURL].index, sha
}

func checksum(data []byte) string {
	hasher := sha256.New()
	_, _ = hasher.Write(data)
	return string(hasher.Sum(nil))
}

func parseIndex(data []byte) (*repo.IndexFile, error) {
	index := &repo.IndexFile{}
	err := yaml.Unmarshal(data, index)
	if err != nil {
		return index, errors.Wrap(err, "could not unmarshall helm chart repo index")
	}
	index.SortEntries()
	return index, nil
}

// fetchChart returns the Chart content given an URL
func fetchChart(ctx context.Context, netClient HTTPClient, chartURL string, load ReadChart) (*chart.Chart, error) {
	req, err := getReq(chartURL)
	if err != nil {
		return nil, err
	}

	res, err := (netClient).Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "request failed")
	}
	data, err := readResponseBody(ctx, res)
	if err != nil {
		return nil, err
	}

	unzippedChart, err := load(bytes.NewReader(data))
	if err != nil {
		return nil, errors.Wrap(err, "Could not extract chart archive")
	}

	return unzippedChart, err
}

func storeIndexInCache(repoURL string, index *repo.IndexFile, sha string) {
	repoIndexes[repoURL] = &repoIndex{sha, index}
}

func LoadCatalogChart(ctx context.Context, apprepo *appRepov1.AppRepository, chartName, chartVersion string, chartReader ReadChart,
	appRepoClient client.Client) ChartLoaderFunc {
	return func() (c *chart.Chart, err error) {
		netClient, err := InitNetClientForCatalogChart(ctx, apprepo, appRepoClient)
		if err != nil {
			return nil, err
		}

		return GetChartForCatalogChart(ctx, netClient, apprepo.Spec.URL, chartName, chartVersion, chartReader)
	}
}

// GetChart retrieves and loads a Chart from a registry
func GetChartForCatalogChart(ctx context.Context, netClient HTTPClient, repoURL, chartName, chartVersion string, load ReadChart) (*chart.Chart, error) {
	if repoURL == "" {
		return nil, errors.New("URL is empty")
	}

	repoURL = strings.TrimSuffix(strings.TrimSpace(repoURL), "/") + "/index.yaml"

	repoIndex, err := fetchRepoIndex(ctx, netClient, repoURL)
	if err != nil {
		return nil, err
	}

	chartURL, err := findChartInRepoIndex(repoIndex, repoURL, chartName, chartVersion)
	if err != nil {
		return nil, err
	}

	chartRequested, err := fetchChart(ctx, netClient, chartURL, load)
	if err != nil {
		return nil, err
	}
	return chartRequested, nil
}

// InitNetClient returns an HTTP FacaceImpl based on the chart details loading a
// custom CA if provided (as a secret)
func InitNetClientForCatalogChart(ctx context.Context, appRepo *appRepov1.AppRepository, appRepoClient client.Client) (HTTPClient, error) {
	// Require the SystemCertPool unless the env var is explicitly set.
	caCertPool, err := x509.SystemCertPool()
	if err != nil {
		if _, ok := os.LookupEnv("TILLER_PROXY_ALLOW_EMPTY_CERT_POOL"); !ok {
			return nil, errors.Wrap(err, "could not create system cert pool object")
		}
		caCertPool = x509.NewCertPool()
	}

	namespace := util.GetApprepoNamespace()

	auth := appRepo.Spec.Auth

	if auth.CustomCA != nil {
		var caCertSecret v1.Secret

		secretKey := types.NamespacedName{
			Namespace: namespace,
			Name:      auth.CustomCA.SecretKeyRef.Name,
		}

		err = appRepoClient.Get(ctx, secretKey, &caCertSecret)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to read caCertSecret %s in namespace %s", auth.CustomCA.SecretKeyRef.Name, namespace)
		}

		// Append our cert to the system pool
		customData, ok := caCertSecret.Data[auth.CustomCA.SecretKeyRef.Key]
		if !ok {
			return nil, errors.Errorf("secret %q did not contain key %q", auth.CustomCA.SecretKeyRef.Name, auth.CustomCA.SecretKeyRef.Key)
		}
		if ok := caCertPool.AppendCertsFromPEM(customData); !ok {
			return nil, errors.Errorf("failed to append %s to RootCAs", auth.CustomCA.SecretKeyRef.Name)
		}
	}

	defaultHeaders := http.Header{"User-Agent": []string{"hub-controller"}}

	if auth.Header != nil {
		var secret v1.Secret

		secretKey := types.NamespacedName{
			Namespace: namespace,
			Name:      auth.Header.SecretKeyRef.Name,
		}

		err = appRepoClient.Get(ctx, secretKey, &secret)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to read secret %s in namespace %s", auth.Header.SecretKeyRef.Name, namespace)
		}
		authHeader := string(secret.Data[auth.Header.SecretKeyRef.Key])

		if strings.HasPrefix(authHeader, "Basic ") {
			trimmedBasicHeader := strings.TrimPrefix(authHeader, "Basic ")
			username, password, err := util.DecodeBasicAuthCredentials(trimmedBasicHeader)
			if err != nil {
				return nil, err
			}
			if username == "_json_key" {
				accessToken, err := util.GetGCloudAccessToken(password)
				if err != nil {
					return nil, err
				}
				authHeader = "Bearer " + accessToken
			}
		}
		defaultHeaders.Set("Authorization", authHeader)
	}

	// Return Transport for testing purposes
	return &clientWithDefaultHeaders{
		client: &http.Client{
			Timeout: time.Second * defaultTimeoutSeconds,
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				TLSClientConfig: &tls.Config{
					RootCAs:    caCertPool,
					MinVersion: tls.VersionTLS12,
				},
			},
		},
		defaultHeaders: defaultHeaders,
	}, nil
}

func LoadRawURL(ctx context.Context, customCAData, authHeader, chartURL string, chartReader ReadChart) ChartLoaderFunc {
	return func() (*chart.Chart, error) {
		netClient, err := InitNetClientForRawURL(customCAData, authHeader)
		if err != nil {
			return nil, err
		}

		return fetchChart(ctx, netClient, chartURL, chartReader)
	}
}

// InitNetClient returns an HTTP FacaceImpl based on the chart details loading a
// custom CA if provided (as a secret)
func InitNetClientForRawURL(customCAData, authHeader string) (HTTPClient, error) {
	// Require the SystemCertPool unless the env var is explicitly set.
	caCertPool, err := x509.SystemCertPool()
	if err != nil {
		if _, ok := os.LookupEnv("TILLER_PROXY_ALLOW_EMPTY_CERT_POOL"); !ok {
			return nil, errors.Wrap(err, "could not create system cert pool object")
		}
		caCertPool = x509.NewCertPool()
	}

	if customCAData != "" {
		// Append our cert to the system pool
		if ok := caCertPool.AppendCertsFromPEM([]byte(customCAData)); !ok {
			return nil, errors.Errorf("failed to append customCA to system cert pool")
		}
	}

	defaultHeaders := http.Header{"User-Agent": []string{"hub-controller"}}

	if authHeader != "" {
		if strings.HasPrefix(authHeader, "Basic ") {
			trimmedBasicHeader := strings.TrimPrefix(authHeader, "Basic ")
			username, password, err := util.DecodeBasicAuthCredentials(trimmedBasicHeader)
			if err != nil {
				return nil, err
			}
			if username == "_json_key" {
				accessToken, err := util.GetGCloudAccessToken(password)
				if err != nil {
					return nil, err
				}
				authHeader = "Bearer " + accessToken
			}
		}
		defaultHeaders.Set("Authorization", authHeader)
		defaultHeaders.Set("Accept", "application/octet-stream, application/zip, "+
			"application/x-gzip")
	}

	return &clientWithDefaultHeaders{
		client: &http.Client{
			Timeout: time.Second * defaultTimeoutSeconds,
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				TLSClientConfig: &tls.Config{
					RootCAs:    caCertPool,
					MinVersion: tls.VersionTLS12,
				},
			},
		},
		defaultHeaders: defaultHeaders,
	}, nil
}
