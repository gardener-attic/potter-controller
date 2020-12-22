---
title: GitOps
type: docs
---

In this chapter we show simple GitOps scenarios generating and updating Cluster-BoMs stored in a GitHub repository including templating.

## Simple Example Without Templating

Assume we want to deploy an Application-Cluster-BoM like the following:

```yaml
apiVersion: hub.k8s.sap.com/v1
kind: ClusterBom
metadata:
  name: simple-grafana
  namespace: garden-apphubdemo
spec:
  applicationConfigs:
  - configType: helm
    id: grafana
    typeSpecificData:
      catalogAccess:
        chartName: grafana
        chartVersion: 5.0.0
        repo: stable
      installName: grafana
      namespace: my-target-cluster-namespace
  secretRef: my-cluster.kubeconfig
```

Assume this Application-Cluster-BoM exists in a GitHub repository. We want that changes of this Application-Cluster-BoM in the git repository are applied automatically, i.e. without executing a command manually. We also want that this is done soon after the change in the git repository, let's say in about a minute afterwards. To achieve this, we do not create the Application-Cluster-BoM manually, but install it in our Gardener project via a second Cluster-BoM, called GitOps-Cluster-BoM. It looks as follows.

```yaml
apiVersion: hub.k8s.sap.com/v1
kind: ClusterBom
metadata:
  name: gitops-simple
  namespace: garden-apphubdemo               # Same namespace as for the Application-Cluster-BoM
spec:
  applicationConfigs:
  - configType: kapp
    id: gitops
    typeSpecificData:
      syncPeriod: 1m                         # Intervall for checking changes of the Application-Cluster-BoM
      cluster:
        namespace: garden-apphubdemo         # Must be the same namespace as in metadata.namespace
      fetch:
      - git:
          ref: origin/master
          url: <GITREPO-URL>                 # Git repository containing the Application-Cluster-BoM
          subPath: gitops/simple/application # Folder containing the Application-Cluster-BoM
      template:
      - ytt: {}
      deploy:
      - kapp: {}
  secretRef: my-robot-secret                 # Secret to create the Application-Cluster-BoM 
                                             # in Gardener namespace garden-apphubdemo.
                                             # See section "Secret to Create Application-Cluster-BoM" below
                                             # how to create this secret.
```

When you have deployed the GitOps-Cluster-BoM, the AppHub watches the specified folder in the git repositoy. It will fetch the Application-Cluster-BoM from there and deploy it in the Gardener namespace of the GitOps-Cluster-BoM. As soon as the Application-Cluster-BoM has been deployed, the specified application (in this example grafana) will be deployed on the target cluster. The sync period (`spec.applicationConfigs.typeSpecificData.syncPeriod`) defines how often the Application-Cluster-BoM is checked for changes in the git repository. For example, if you change the Application-Cluster-BoM by increasing the grafana chartVersion, then the AppHub recognizes this change within the sync period and will deploy the new version automatically.

You could store additional Application-Cluster-BoMs in the same folder as the first, and they will also be deployed by the same GitOps-Cluster-BoM.

Please note that in order to make this work, you need to create a secret in your Garden Project first, which is needed to create the Application-Cluster-BoM in the Garden Project. See [this section](#secret-to-create-an-application-cluster-bom) for more information.

To access private repositories, see [Fetching-Resources-From-a-Private-GitHub-Repository](../fetching-resources-from-private-github-repo/)

## Example With Templating and Multiple Clusters

In this example, we want to deploy the same application with different settings on a dev and a live cluster. For example, we want to deploy grafana in version 5.0.0 on a live cluster and in version 5.0.2 on a dev cluster. We use the following template `application_clusterbom.yml` for the Application-Cluster-BoM. The templating tool is ytt.

```yaml
#@ load("@ytt:data", "data")

---
apiVersion: hub.k8s.sap.com/v1
kind: ClusterBom
metadata:
  name: #@ data.values.clusterbom_name
  namespace: garden-apphubdemo
spec:
  applicationConfigs:
  - configType: helm
    id: grafana
    typeSpecificData:
      catalogAccess:
        chartName: grafana
        chartVersion: #@ data.values.chart_version
        repo: stable
      installName: grafana
      namespace: my-target-cluster-namespace
  secretRef: #@ data.values.secret_ref
```

For the dev cluster we fill the template with values from the following values `values_dev.yml`file:

```yaml
#@data/values
---
clusterbom_name: multi-grafana-dev
chart_version: 5.0.2
secret_ref: dev.kubeconfig
```

For the live cluster we fill the template with values from the following values `values_live.yml`file:

```yaml
#@data/values
---
clusterbom_name: multi-grafana-live
chart_version: 5.0.0
secret_ref: live.kubeconfig
```

The template and the values files are stored in a git repository with the following directory structure:

```
gitops
└── multi
    ├── base
    │   └── application_clusterbom.yml
    ├── dev
    │   └── values_dev.yml
    └── live
        └── values_live.yml
```

To deploy the Application-Cluster-Bom for the dev landscape, we use the following GitOps-Cluster-BoM, which combines the Cluster-Bom template with the dev values.

```yaml
apiVersion: hub.k8s.sap.com/v1
kind: ClusterBom
metadata:
  name: gitops-multi
  namespace: garden-apphubdemo
spec:
  applicationConfigs:
  - configType: kapp
    id: gitops
    typeSpecificData:
      syncPeriod: 1m
      cluster:
        namespace: garden-apphubdemo
      fetch:
      - git:
          ref: origin/master
          url: <GITREPO-URL>
          subPath: gitops/multi
      template:
      - ytt:
          paths:
          - base  # Folder gitops/multi/base in the git repo contains the file application_clusterbom.yml
          - dev   # Folder gitops/multi/dev  in the git repo contains the file values_dev.yml
      deploy:
      - kapp: {}
  secretRef: my-robot-secret
```

For the live landscape, use a second GitOps-Cluster-Bom which combines the Cluster-Bom template with the live values:

```yaml
      template:
      - ytt:
          paths:
          - base  # Contains the Cluster-BoM template
          - live  # Contains the live values
```

There are many more possibilities to template with ytt. For more details and further references, see [Cluster-BoM-kapp-Example](../../kapp-example/).

## Secret to Create an Application-Cluster-BoM

The Application-Cluster-BoMs are created on the Gardener cluster. Therefore we need a secret with these privileges. To create this secret proceed as follows.

1. Create a service account for your Gardener project in the *Members* section of the Gardener Dashboard:  
2. Download the kubeconfig for this service account.
3. Create a new secret using the service account kubeconfig file. Use the same service account kubeconfig as context, so that the secret will be created in the Gardener project.

Example:

```sh
kubectl create secret generic <SECRET-NAME> -n garden-<GARDEN-PROJECT-NAME> --from-file=kubeconfig=<PATH-TO-SERVICE-ACCOUNT-KUBECONFIG>
```
