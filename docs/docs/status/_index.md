---
title: Cluster-BoM Status Section explained
type: docs
weight: 70
---

* [Status Section Overview](#status-section-overview)
* [Cluster-BoM Status Section Example](#cluster-bom-status-section-example)
* [Conditions](#conditions)
* [Deployment State of each application](#deployment-state-of-each-application)
* [Overall Deployment State](#overall-deployment-state)

#### Status Section Overview

To check the deployment status, use `kubectl get clusterbom [NAME-OF-YOUR-CLUSTERBOM] -o yaml`. Kubernetes returns the current metadata of the BoM resource. This metadata includes information about the last applied configuration, your `applicationConfigs`, and a `status` section at the end.<br>

The status section consists of:

* the `conditions` describing if all configured apps are up and running in their last revision and if all removed applications have been deleted from the target cluster. This is the most important part in the status for the end user. Other parts of the status are more for detailed problem analysis.

* the `applicationStates` with a list of `detailedState` items containing a detailed description of the deployment and running state for every application specified by the Cluster-BoM. It reflects what currently is deployed on the target cluster in comparison to what is specified in the Cluster-BoM. This might also include detailed states of applications which are already removed from the Cluster-BoM but not already removed from the target cluster. 

* the `overallState` that is an aggregated state of all application states.

Execute `kubectl get clusterbom [NAME-OF-YOUR-CLUSTERBOM] -o yaml` several times to see the changes of your deployment status. 

> The explanatory comments are not part of the output.

#### Cluster-BoM Status Section Example

```yaml
status:

  conditions:
  - lastTransitionTime: "2020-04-07T08:38:57Z"
    lastUpdateTime: "2020-04-08T06:03:50Z"
    message: 'Pending applications: mongodb, servicecatalog. '
    reason: PendingApps
    status: Unknown
    type: Ready

  applicationStates:
  - detailedState:
      deletionTimestamp: "2020-03-13T09:11:28Z" # indicator that this app is marked for deletion
      generation: 1
      lastOperation:
        description: install successful
        observedGeneration: 1
        successGeneration: 1
        numberOfTries: 1
        operation: install
        state: ok                     # state of last install/remove
        time: "2020-03-13T09:11:22Z"
      reachability:
        reachable: true
        time: "2020-03-13T09:11:22Z"
      readiness:
        state: ok
        time: "2020-03-13T09:11:23Z"
    id: karydia
    state: ok                         # application state
  - detailedState:
      generation: 1
      lastOperation:
        description: install successful
        observedGeneration: 1
        successGeneration: 1
        numberOfTries: 1
        operation: install
        state: ok                     # state of last install/remove
        time: "2020-03-13T09:11:23Z"
      reachability:
        reachable: true
        time: "2020-03-13T09:11:23Z"
      readiness:
        state: pending
        time: "2020-03-13T09:11:23Z"
    id: mongodb
    state: ok                         # application state 
  overallState: pending               # overall state
  overallTime: "2020-04-08T06:03:50Z" # last update time of overall state
```

#### Conditions

The condition of type `Ready` describes if all configured apps are up and running in their last revision. 

| Section Field | Description |
  |:--------------|:--------|
  |`type`| `Ready` |
  |`status`| `True`: All configured apps are up and running in their last revision, the specified jobs in the `readyRequirements` sections are finished successfully and all removed applications have been deleted from the target cluster <br>`False`: Some application has failed without any chance to recover. <br>`Unknown`: Otherwise. |


Every Cluster-BoM contains a `generation` field in its `metadata` section which is automatically increased by the Kubernetes environment when a modification is done. In this sense it describes the revision of the complete `spec` section of the Cluster-BoM. 

In the `status` section there is a field `observedGeneration` which describes the revision or `generation` to which the condition refers. Only if the `observedGeneration` and the `generation` contain identical numbers, the condition describes the status of the current Cluster-BoM and not of a former state/revision.

The main advantage of the `Ready` condition compared to the overall deployment state is that it is computed with respect to a particular revision of a Cluster-BoM. The overall deployment state is computed based on an internal snapshot of what is currently deployed and what was internally seen as the last revision of particular applications.

The condition of type `ClusterReachable` describes if the target cluster is reachable. 

| Section Field | Description |
  |:--------------|:--------|
  |`type`| `ClusterReachable` |
  |`status`| `True`: Target Shoot Cluster is reachable. <br>`False`: Target Shoot Cluster is not reachable. <br>`Unknown`: No information available. |

#### Deployment State of Each Application

The `detailedState` of an application consists of:

* `generation`: Revision number for the application and its settings (for example, version, values), that **should be** deployed on the target shoot cluster. This number is automatically increased every time a user make changes to the application configuration. There might be a small delay until changes in the Cluster-BoM are reflected in changes of the `generation` of the affected application configurations. 

* `deletionTimestamp`: If this entry is set, the corresponding application is marked for deletion and will be removed after the application was uninstalled from the target cluster.

* `lastOperation`:<br> Describes the status of the last applied installation or removal operation with respect to the application config in the Cluster-BoM.

  | Section Field | Description |
  |:--------------|:--------|
  |`operation`| Operation that was triggered in this revision, for example, `install` or `remove`.|
  |`observedGeneration` | Revision number (`generation`) for the application and its settings (for example, version, values), that **was** installed or removed on the target shoot cluster. A lower number compared to `generation` signals that no the latest setting where applied until now. |
  |`numberOfTries`| The Application Hub doesn’t stop to try the deployment or removal if there are issues, but waits longer after each attempt before trying it again.  |
  |`state`|`ok`: the last try of the operation succeeded.<br>`failed`: The last try of the operation **didn't** succeed.|
  |`successGeneration` | The last revision number (`generation`) for which the operation succeeded. Due to an internal reconcile loop which re-executes the last operation from time to time, the `state` only informs about the success or failure of this. With the `successGeneration` you see which was the last successfully applied revision. |
  |`description`| More details about the operation result. |
  |`time`| Timestamp describing when the revision was applied. |
  |`errorHistory`| The first and up to the 4 last errors with respect to the last applied revision. |

* `reachability`:<br> Describes the availability of the target cluster. Operations aren't executed if the target cluster isn’t reachable, for example, if it’s hibernated. In such a case also section like `lastOperation` are not updated.
  
  | Section Field | Description |
  |:--------------|:--------|
  |`reachable`| `true`: The cluster could be reached.<br>`false`: The cluster *couldn't* be reached.  |
  |`time`| Time of the last check. Not reachable clusters are rechecked every couple of minutes. |

* `readiness`:<br> The `lastOperation` section describes the state with respect to the deployment and removal of k8s resources of the application. The `readiness` instead describes if the most important components of the installed applications are up and running. Currently all Deployments, DaemonSets and StatefulSets are checked for this. Furthermore the specified jobs of the `readyRequirements` section must be finished successfully. 
  
  | Section Field | Description |
  |:--------------|:--------|
  |`state`| `ok`: For the last deployed revision all relevant k8s components are up and running and all specified jobs of the `readyRequirements` section must be finished successfully. <br>`pending`: For the last deployed revision not all relevant k8s components are already up and running. <br>`failed`: The last deployment failed.<br>`finallyFailed`: One of the jobs specified in the `readyRequirements` section failed.  <br>`notRelevant`: For removal operations. <br>`unknown`: If something failed when finding out the readiness state, e.g. access to the target cluster timed out. |
  |`time`| Time of the last check. Not reachable clusters are rechecked every couple of minutes. |


* `detailedState.state`: Overall state for one application computed as follows:

  | Current Operation | Description |
  |:--------------|:--------|
  |remove| **ok**: The application was successfully uninstalled from the target cluster, i.e. `lastOperation.operation` is `remove` and `lastOperation.state` is `ok`.<br>**pending**: The uninstall operation was not executed until now, i.e. `lastOperation.operation` is not `remove`. <br>**failed**: The uninstall operation failed but will be retried, i.e. `lastOperation.operation` is `remove` and `lastOperation.state` is not `ok`.|
  |install| **ok**: The last application which was tried to install, was successfully installed on the target cluster and is ready. The last tried application might not be the latest specified in the Cluster-BoM. <br>**pending**: The last tried application was successfully installed but is not already up and running or there is newer revision of the application to be deployed. <br>**failed**: The installation of the last revision of the application failed or some components of the applications failed to succeed.<br>**unknown**: Something failed when finding out the state, e.g. access to the target cluster timed out. |

* `typeSpecificStatus`: Here you find additional status information depending on the config type (e.g. helm or kapp). Currently this is only used for kapp. More detailed information about the information provided here could be found [here](https://github.com/k14s/kapp-controller/blob/develop/docs/app-spec.md).

#### Overall Deployment State

Beside the detailed state, `status.overallState` provides an aggregated state of the application states (of `detailedState.state`) as the following table exemplifies:

|Single States: | | | | |
|:-|:-|:-|:-|:-|
|application state 1 | ok | ok | ok | ok |
|application state 2 | ok | ok | ok | failed |
|application state 3 | ok | ok | unknown | unknown |
|application state 4 | ok | pending | pending | pending |
|**Calculated Overall State:**| | | | |
|`status.overallState`| ok | pending | unknown | failed |


Besides this you find summarized information about the installation progress:
````yaml
overallNumOfDeployments: 4        # number of configured applications
overallNumOfReadyDeployments: 3   # number of successfully installed applications
overallProgress: 75               # percentage of successfully installed applications
````