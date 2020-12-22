# Hub Controller

## Configuration and setup

### cluster setup

In the oidc cluster(e.g garden cluster) apply the following resources:

1. [clusterbom crd](../../config/crd/bases/hub.k8s.sap.com_clusterboms.yaml) 
1. [clusterbomsync crd](../../config/crd/bases/hub.k8s.sap.com_clusterbomsyncs.yaml)
1. [apps crd](../../config/crd/bases/kappctrl.k14s.io_app.yaml)
1. [deployitem crd](../../config/crd/bases/landscaper.gardener.cloud_deployitems.yaml)
1. the [clusterrole-clusterbom-controller.yaml](../../config/deployments/gardener-roles/clusterrole-clusterbom-controller.yaml) to create a clusterrole, clusterrolebinding and a serviceaccount.

    > **Note:** The controller needs the path to the persisted service account kubeconfig in order to run.

1. the [clusterrole-project-members.yaml](../../config/deployments/gardener-roles/clusterrole-project-members.yaml) which grants all project users to manage the clusterboms in their owned projects (Gardener specific)
1. register the mutating webhook at the oidc cluster api server [clusterBomAdmissionHookConfig.yaml](../../config/deployments/clusterBomAdmissionHookConfig.yaml). The mutating webhook checks whether the applied clusterbom customresources are valid.

In hub cluster apply these resources:

1. create a hub namespace
2. create an image pull secret in the hub ns. The default name of the secret is `gcr` but it can be configured in the helm values entry `imagePullSecrets`
3. the [role-apprepo.yaml](../../config/deployments/hub-roles/role-apprepo.yaml).This will create a service account, a role and a role binding which is needed for the controller to read the apprepositories custom resources.
    > **Note:** The controller needs the path to the persisted service account kubeconfig file in order to run.

### configuration

> **Note:** in order to run properly please expose the controller with an ingress.

Parameter | Description | Default | Type | Required
--- | --- | --- | --- | ---
`deploymentArgs.reconcileIntervalMinutes` | reconcile interval in minutes | `30` | int | no
`deploymentArgs.logLevel` | log level  | `warning` | string | no
`deploymentArgs.configTypes` | supported deployment types  | `helm,kapp` | string | no
`image.registry` | image registry  | `eu.gcr.io` | string | yes
`image.repository` | image repository  | `none` | string | yes
`image.tag` | image tag  | `none` | string | yes
`image.pullPolicy` | image pull policy  | `IfNotPresent` | string | no
`imagePullSecrets` | image pull secrets | `gcr` | list | yes
`ingress.gardenerCertManager` | use the cert manager provided by Gardener to retrieve certificates from Let's Encrypt | `false` | bool | no
`kappImage.registry` | kapp-controller image registry  | `eu.gcr.io` | string | yes
`kappImage.repository` | kapp-controller image image repository  | `none` | string | yes
`kappImage.tag` | kapp-controller image image tag  | `none` | string | yes
`kappImage.pullPolicy` | kapp-controller image image pull policy  | `IfNotPresent` | string | no
`namespaces.appRepo` | namespace of app repositories | `hub` | string | yes
`secretConfig.apprepoCluster` | kubeconfig of a serviceaccount to read the apprepositories | `nil` | multiline-string  | yes
`secretConfig.hubImagePullSecret` | read image pull secret of hub | `nil` | multiline-string | yes
`secretConfig.secretCluster` | kubeconfig of a service account to read the kubeconfig secrets | `nil` | multiline-string | yes
`tokenIssuer` | URL for the validation of bearer tokens of requests to the admission webhook | "" | string | no | 
`tokenReviewEnabled` | flag to switch validation of bearer tokens on/off of requests to the admission webhook | false | bool | no | 
`threads.deploymentController` | thread count of deploymentController | `30` | int | no
`threads.clusterBomController` | thread count of clusterBomController | `10` | int | no
`threads.clusterBomStateController` | thread count of clusterBomStateController | `10` | int | no
