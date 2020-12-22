# Potter-Controller

TODO:
- Move install description to new website, reference it from helm chart
- the webhook must be exposed via an ingress because HTTPs is mandatory. It might necessary to add the CA of the ingress's tls certificate to the webhook config (TODO: check if this step is necessary with a canary cluster)

## Overview

The following figure shows the complete setup of the Project Potter, including the components of [potter-controller](https://github.com/gardener/potter-controller) and [potter-hub](https://github.com/gardener/potter-hub). This guide describes only the installation of the potter-controller. For the potter-hub, please refer to the corresponding docs.

![architecture overview](https://github.wdf.sap.corp/raw/kubernetes/hub/master/docs/docs/hub-overview-architecture-reworked.png?token=AAADTWH2DGD5UK737USSTRS74IJB4)

If you are installing this project into a [Gardener](https://gardener.cloud/) landscape, then the "resource" cluster will be the "garden" cluster from the Gardener landscape. The following guide mainly focuses on that scenario. In principle, it is also possible to store the ClusterBom in some other k8s cluster. Then also the access data to the target k8s clusters, on which the apps should be installed, must be stored in the same namespacs as the ClusterBoms for these target clusters. The access data must be stored as secrets containing the access data as a kubeconfig. The name of such a secret is used in the ClusterBom as the secretRef to the target cluster. 

## Installation Guide

**1. Clone the potter-controller repo and cd into the local repo directory**

```
git clone https://github.com/gardener/potter-controller
cd potter-controller
```

**2. Create the CRDs on the resource cluster**

Make sure you have set your kube context to point to the resource cluster, so that the following kubectl commands are performed within the context of the resource cluster.
```
kubectl apply -f ./config/crd/bases
```

**3. Create the RBAC primitives on the resource cluster**

Again, for the following kubectl command, use the kube context of the resource cluster.
```
kubectl apply -f ./config/deployments/gardener-roles
```

This service account in `clusterrole-clusterbom-controller.yaml` is used by the potter-controller to read the target cluster secrets (kubeconfigs) and the CRs (ClusterBoms) on the resource cluster. The cluster role defined in `clusterrole-project-members.yaml` is used to extend the priveledges of Gardener project members to maintain ClusterBoms in their Garden project namespaces.   

If you plan to store the ClusterBoms in a k8s cluster which is not a Garden cluster, e.g. some Garden shoot cluster, you also need to deploy the secrets containing the access data in form of kubeconfigs to the target shoot clusters (on which you want to install the applications) to that cluster. The secret with the access data of a target shoot cluster must be stored in the same namespace as the ClusterBoms for that cluster. In that situation deploying `clusterrole-project-members.yaml` is not needed but it is in the responsibility of the administrator to secure the access to the secrets and the ClusterBoms. 

**4. Create the kubeconfig for the service account**

Download the shell script [create-kubeconfig-for-serviceaccount.sh](./create-kubeconfig-for-serviceaccount.sh). Open the script and exchange the variables `clusterURL` and `secretName` according to the RBAC primitives you created in Step 3. Executing the script will generate a kubeconfig which uses the serviceaccount you just created and print it to the shell. This output must be set as a value in the potter-controller Helm Chart under the key `secretConfig.secretCluster`.

**5. Install the Webhooks on the resource cluster**

The potter-controller uses two admission webhooks. 

- The webhook configured in `./config/deployments/webhooks/clusterBomAdmissionHookConfig.yaml` checks and mutates Cluster-BoMs. You need to set the URL at `webhooks/clientConfig/url` to `https://hub.<ingress domain of hub controller cluster>/checkClusterBom` with the correct ingress domain. You can get the cluster domain from the kubeconfig by removing the `api` subdomain from the API server URL. **This webhook is mandatory.** 

- The webhook configured in `./config/deployments/webhooks/secretAdmissionHookConfig.yaml` ensures that secrets created and maintained under the control of the hub controller are not changed by others. This webhook is not mandatory. You need to set the URL at `webhooks/clientConfig/url` to `https://hub.<ingress domain of hub controller cluster>/checkSecret` with the correct ingress domain. You can get the cluster domain from the kubeconfig by removing the `api` subdomain from the API server URL.

**By default, these webhooks are not secured.** If you want to secure them, you need to configure the API server of the resource cluster as described [here](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/#authenticate-apiservers)
such that the requests to the webhook contain an authorization header with a JWT bearer token. Next, you need to enable the validation of these tokens by deploying the hub controller chart with the following additional values:

```
deploymentArgs:
  tokenReviewEnabled: true
  tokenIssuer: <issuer url>
```

Deploy the webhook configurations with the following command on the resource cluster:

```
kubectl apply -f ./config/deployments/webhooks
```

**6. Create the RBAC primitives on the Potter Cluster**

This service account is used for reading the apprepositories on the Potter cluster. By default it will be created in the `hub` namespace. You can change the namespace by modifying the Yaml files.

```
kubectl apply -f ./config/deployments/hub-roles
```

**7. Create the kubeconfig for the service account**

Download the shell script [create-kubeconfig-for-serviceaccount.sh](./create-kubeconfig-for-serviceaccount.sh). Open the script and exchange the variables `clusterURL` and `secretName` according to the RBAC primitives you just created. Executing the script will generate a kubeconfig which uses the serviceaccount you just created and print it to the shell. This output must be set as a value in the potter-controller Helm Chart under the key `secretConfig.apprepoCluster`.

**8. Install an Ingress Controller in the Potter cluster**

The endpoints that are called by the previously installed webhooks are exposed via an Ingress. To work correctly, you must have an ingress controller installed in the Potter cluster, such as [ingress-nginx](https://github.com/kubernetes/ingress-nginx) or [traefik](https://github.com/traefik/traefik). If this is not the case, please refer to the original projects for installation instructions.

**9. Install the potter-controller Helm Chart**

The following table includes all mandatory Chart parameters. For a list of ***all*** possible parameters, see the `Values.yaml`.

Parameter | Description | Type | Required
--- | --- | --- | --- | ---
`secretConfig.apprepoCluster` | kubeconfig of the serviceaccount to read the apprepositories | string  | yes
`secretConfig.secretCluster` | kubeconfig of the service account to read the kubeconfig secrets and CRs | string | yes

```
helm repo add potter <url>
helm install potter-controller potter/potter-controller -f values-override.yaml
```

**10. Create AppRepository CRs (optional)**

For using the `catalogAccess` in ClusterBoms, you must configure AppRepository CRs on your cluster. See the following example:

```
apiVersion: kubeapps.com/v1alpha1
kind: AppRepository
metadata:
  name: <repo-name>
  namespace: hub
spec:
  type: helm
  url: <repo-url>
```

For more information you can refer to the potter-hub Helm Chart.
