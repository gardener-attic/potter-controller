package catalog

import "github.wdf.sap.corp/kubernetes/hub-controller/api/apitypes"

var ChartEcho1 = apitypes.CatalogAccess{ // nolint
	Repo:         "sap-incubator",
	ChartName:    "echo-server-private-image",
	ChartVersion: "1.0.4",
}

var ChartEcho2 = apitypes.CatalogAccess{ // nolint
	Repo:         "sap-incubator",
	ChartName:    "echo-server-private-image",
	ChartVersion: "1.0.5",
}

var ChartEchoInvalid = apitypes.CatalogAccess{ // nolint
	Repo:         "sap-incubator",
	ChartName:    "echo-server-private-image",
	ChartVersion: "1.0.11111111",
}

var TarballJob1 = apitypes.TarballAccess{ // nolint
	URL: "https://storage.googleapis.com/sap-hub-test/hub-inttest-job-0.1.0.tgz",
}

var TarballJob2 = apitypes.TarballAccess{ // nolint
	URL: "https://storage.googleapis.com/sap-hub-test/hub-inttest-job-0.2.0.tgz",
}

var TarballNginx1 = apitypes.TarballAccess{ // nolint
	URL: "https://charts.bitnami.com/bitnami/nginx-8.2.2.tgz",
}

var TarballNginx2 = apitypes.TarballAccess{ // nolint
	URL: "https://charts.bitnami.com/bitnami/nginx-8.2.3.tgz",
}

var TarballEcho = apitypes.TarballAccess{ // nolint
	URL: "https://storage.googleapis.com/hub-tarballs/echo-server-1.0.1.tgz",
}
