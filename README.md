# Potter Controller
#### Please note: This repository and the related documentation is very much work-in-progress, so expect to find unfinished work ahead!

## Description
The Potter Controller - A Kubernetes extension to manage deployments in Kubernetes clusters. It’s a central component in a [Gardener](https://github.com/gardener/gardener) Kubernetes landscape which doesn’t require any additional components running in the managed cluster.

A complete Potter installation consists of a UI (maintain in [this repository](https://github.com/gardener/potter-hub), heavily based on the [kubeapps](https://github.com/kubeapps/kubeapps) project), which can be used to manually deploy [Helm Charts](https://github.com/helm/helm) and [Kapp resources](https://github.com/vmware-tanzu/carvel-kapp/blob/develop/README.md) to Clusters. The UI has been enhanced to work centrally, so that only one Potter installation is required to manage a multitude of Clusters in a remote fashion.

Apart from the UI, Potter introduces the concept of so-called "Cluster Bill-of-Materials" (in short: 'Cluster-BoMs'). These entities are YAML files, describing a list of Kubernetes Deployments which should run in a specific Cluster. Such a YAML file describes the "desired state" of all applications which should be running in a Cluster. Cluster-BoMs can easily be applied to a Cluster with kubectl. After applying such a Cluster-BoM, the Potter-Controller (located in this repository) will start deploying whatever is part of the Cluster-BoM. A Status Section at the end of the BoM provides the detailed deployment states.

Ideally, Cluster-BoMs are used to fully automate the management of deployments for a Kubernetes Cluster. By using Cluster-BoMs, not only Helm Charts, but also Kapp-Deployments can be managed.

The Potter Controller enables easy extensibility, so that further Kubernetes deployment types can be integrated, Helm Chart and Kapp support is provided out-of-the-box. The deployment itself is technically based on the [Landscaper Project](https://github.com/gardener/landscaper).

## Installation
The two main components of a Potter Installation (Potter-Hub and Potter-Controller) are distributed and installed via [Helm](https://github.com/helm/helm). For detailed installation instructions, visit the Potter Controller Helm Chart's [README.md](https://github.com/gardener/potter-controller/chart/hub/README.md) and the  [README.md](https://github.com/gardener/potter-controller/chart/hub/README.md) of the corresponding Potter-Hub.

## Limitations
The current version of the Potter Controller has the following limitations:
- Support for Potter installations outside of Gardener landscapes is untested.
  
## How to obtain support
If you encounter any problems using Potter or if you have feature requests, please don't hesitate to contact us via both GitHub repositories. We will offer further methods for getting in touch with us, which you will find here.

## Contributing
The Potter Controller is offered as Open Source, using the Apache 2.0 license model.<br>
If you would like to contribute, please check out the [Gardener contributor guide](https://gardener.cloud/documentation/contribute/).
