---
title: Docs
type: docs
sidebar: true
menu: sln
weight: 100
---

Welcome to the potter-controller docs! Here, you can find everything you always wanted to know about the controller part of project Potter. As a first time visitor, you might want to first read through the [Potter project overview](../../hub-docs/docs). If you are looking for specific potter-controller topics, please see the navigation on the left hand side.

  * [Working with Cluster-BoMs](#working-with-cluster-boms)
  * [Prerequisites](#prerequisites)
  * [Procedure](#procedure)
  * [Events](#events)

To fully automate cluster deployments, files describing the bill of material for a cluster, shortly *Cluster-BoMs*, are used instead of the Hub UI. Such a Cluster-BoM is mainly a list of references to deployments plus their parameters. Currently Helm charts and kapp deployments are supported, but more formats will be added later. 

Helm charts are usually referenced with the help of Helm chart Repositories. As an alternative, it’s also possible to directly provide a link (URL) to a Helm chart (see the [Cluster-BoM Helm example](./helm-example) for more details).

Referencing kapp deployments is described in the [Cluster-BoM kapp example](./kapp-example).

Helm and kapp deployments could be mixed in one Cluster-BoM.

### Working with Cluster-BoMs

A Cluster-BoM is a Custom Resource (CR) and can therefore be used like any other standard Kubernetes resource. For example, after specifying your deployments in a `yaml` file, you can use the following `kubectl` commands as usual:

| Command | Usage |
|:--------|:------|
| `kubectl apply -f myclusterbom.yaml` | Apply your Cluster-BoM to a cluster of your project. |
| `kubectl get clusterbom myclusterbom -o yaml` | Get information about the deployment status. |
| `kubectl get clusterboms` | List all available Cluster-BoMs of your Gardener project. |

### Prerequisites

* You have a running installation of Gardener and Potter and access to a Gardener project.
* You have already a cluster in your Gardener project
* You should use a personalized `kubeconfig` to access Gardener. In the Gardener dashboard, go to your account and from the Access widget download your personalized kubeconfig into `~/.kube/kubeconfig-garden[-myproject]`. Follow the instructions to setup kubelogin, and create an alias like this:
  ```
  alias kgarden="kubectl --kubeconfig ~/.kube/kubeconfig-garden-myproject.yaml"
  ```
* For an automated script or pipeline, you can use a robot service account for the Gardener project where your cluster is located. To create new service accounts on the Gardener dashboard, choose *Members* \> *Service Accounts* \> *+*.

  > Service accounts allow access to all project resources (including all shoot clusters). Deploying resources in a cluster using a Cluster-BoM is done in the context of a service account.

### Procedure

1. To use the service account locally from your computer, choose your project on the Gardener dashboard, then *MEMBERS* \> *Service Accounts*. Download the `kubeconfig` file of the service account you want to use.

1. Merge the downloaded `kubeconfig` of your service account to your existing `~/.kube/config` file or set your `KUBECONFIG` variable accordingly.
   
1. Create a new file `Cluster-BoM.yaml`. To safe you some time, you can copy the example of the next section and adjust it to your needs.
   
   > To find out which Helm charts are currently available, choose *CLUSTERS* \> \[YOUR-CLUSTER\] \> *External Tools* \> *K8s Applications and Services Hub* on the Gardener dashboard. On the Hub UI, you can browse the catalog of available Helm charts. The list of available charts depends on the specific landscape configuration.


1. Edit your Cluster-BoM. If you use the example, you must at least change `metadata.namespace` and `spec.secretRef`: 

    * Metadata for Cluster-BoM and `kubeconfig` of your target cluster:

      | Field | Description | Pattern |
      |:------|:--------|:------| 
      |`metadata.name`|Technical name of this Cluster-BoM| The name of a Cluster-BoM must consist of lower case alphanumeric characters or `.` or `-`, must start and end with an alphanumeric character, and must not be longer than 63 characters (for example, `testclusterbom.01`). |
      |`metadata.namespace`| The namespace of this Cluster-BoM| `garden-`\[PROJECT-NAME-IN-GARDENER\]|
      |`spec.secretRef`| Secret with the kubeconfig of your target cluster | \[TARGET-CLUSTER-NAME\].kubeconfig|

    * Mandatory Configuration Parameters for the application deployments (section `applicationConfigs`):

      | Field | Description |
      |:------|:--------| 
      |`id`|Unique ID of the application within this Cluster-BoM.<br>Pattern: `^[0-9a-z]{1,20}$`|
      |`configType`| Type of deployment. Currently only `helm` and `kapp` is supported. | 

    > More detailed information about the Cluster-BoM Structure can be found in the examples for [Helm](./helm-example) and [kapp](./kapp-example) applications.

1. After editing the Cluster-BoM file, use the Kubernetes context with your service account `kubeconfig` and execute the following command: 
   
   `kubectl apply -f [NAME-OF-YOUR-CLUSTERBOM].yaml`
   
   > If something is wrong within your Cluster-BoM, an error message is displayed.

1. To check if the deployments worked as expected:
   
   * In the context of your service account, get your deployed Cluster-BoM as `yaml` output:

      `kubectl get clusterbom [NAME-OF-YOUR-CLUSTERBOM] -o yaml`

      At the end of the `yaml` output, there’s now a status section that includes the status of each individual deployment and the aggregated status of the complete Cluster-BoM.

      More information: [Cluster BoM Status](./status)

   * In the context of your shoot cluster (switch your current context), check if new pods are being created for the deployments specified in your Cluster-BoM:

      `kubectl get pods`

1. After this initial deployment, you can also do changes to the Cluster-BoM and apply the modified Cluster-BoM again in the context of your service account to your cluster, for example: 
   
   * To change the version of a referenced chart.
   * To add or to delete individual deployments from the Cluster-BoM.
   * To change Helm chart values.

    > Some values of the Cluster-BoM cannot be changed, for example, `applicationConfigs.id`.

1. To delete **all deployments** of a Cluster-BoM, just delete the whole Cluster-BoM using the following command in the context of your service account: 
  
    `kubectl delete clusterbom [NAME-OF-YOUR-CLUSTERBOM] --wait=false` 

    > The Cluster-BoM is only marked for deletion using the Kubernetes finalizer concept. Only when all applications were successfully removed from the target cluster or when the target cluster is removed itself, the Cluster-BoM disappears as well. Use option `--wait=true` if you want to wait until this process is finished. 
    
### Events

With the following command you see events for Cluster-BoMs:

```
> kubectl describe clusterbom testbom

...
Events:
  Type    Reason             Age                From                          Message
  ----    ------             ----               ----                          -------
  Normal  SuccessDeployment  12m (x3 over 33m)  DeploymentController      Reconcile of deployment done
  Normal  SuccessDeployment  11m (x4 over 34m)  DeploymentController      Deployment ok
...
```