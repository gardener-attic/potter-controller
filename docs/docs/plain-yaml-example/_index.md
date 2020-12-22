---
title: Cluster-BoM Example with plain yaml K8s resources
type: docs
weight: 60
---

In this example, kubernetes resources specified by plain yaml files are deployed. This is a special case of kapp deployments described [here](../kapp-example/).

The provided kubernetes yaml resources are deployed according to the rules specified [here](https://github.com/k14s/kapp/blob/develop/docs/apply-ordering.md), e.g. CRDs and namspaces are deployed before namespaced resources.

The first example uses a zip file containing one yml file with different kubernetes resources, which is accessible via the URL https://storage.googleapis.com/hub-tarballs/integration-tests/zip-http-plain-yml/config.zip. 

Of course the zip might contain more than one yml file. For pitfalls with respect to the zip creation see the [kapp example](../kapp-example/).

```yaml
apiVersion: "hub.k8s.sap.com/v1"
kind: ClusterBom
metadata:
  name: demo                               # Cluster-BoM name.
  namespace: garden-apphubdemo             # Cluster-BoM namespace. Pattern: garden-<projectname in Gardener>
spec:
  secretRef: my-cluster.kubeconfig         # Reference to kubeconfig of target cluster 
                                           # Pattern: <name of Kubernetes cluster in gardener>.kubeconfig

  autoDelete:                              # Auto delete of Cluster-BoM (optional)
    clusterBomAge: 120                     # Custer-BoM age in minutes
                                           # Cluster-BoM is deleted automatically if the
                                           # corresponding shoot cluster is removed and the 
                                           # Clustere-BoM is older than the specified clusterBomAge. 

  applicationConfigs:                      # List of applications to be deployed in target cluster

  - id: yamlexample1                       # ID of the application within this Cluster-BoM
    configType: kapp                       # Deployment via kapp controller           
    typeSpecificData:                      # Spec of a YAML/KAPP App, see
                                           # https://github.com/k14s/kapp-controller/blob/develop/docs/app-spec.md
      fetch:
      - http:
          url: https://storage.googleapis.com/hub-tarballs/integration-tests/zip-http-plain-yml/config.zip
      template:                            # No templating
      - ytt: {}
      deploy:
      - kapp: {}

# more application deployments can go here
```

The second example fetches the yml files from a git repository. Of course the specified folder in the Git repository might contain more than one yml file.

```yaml
apiVersion: "hub.k8s.sap.com/v1"
kind: ClusterBom
metadata:
  name: demo                               # Cluster-BoM name.
  namespace: garden-apphubdemo             # Cluster-BoM namespace. Pattern: garden-<projectname in Gardener>
spec:
  secretRef: my-cluster.kubeconfig         # Reference to kubeconfig of target cluster 
                                           # Pattern: <name of Kubernetes cluster in gardener>.kubeconfig

  autoDelete:                              # Auto delete of Cluster-BoM (optional)
    clusterBomAge: 120                     # Custer-BoM age in minutes
                                           # Cluster-BoM is deleted automatically if the
                                           # corresponding shoot cluster is removed and the 
                                           # Clustere-BoM is older than the specified clusterBomAge. 

  applicationConfigs:                      # List of applications to be deployed in target cluster

  - id: yamlexample2                       # ID of the application within this Cluster-BoM
    configType: kapp                       # Deployment via kapp controller           
    typeSpecificData:                      # Spec of a YAML/KAPP App, see
                                           # https://github.com/k14s/kapp-controller/blob/develop/docs/app-spec.md
      fetch:
      - git:
          ref: origin/develop
          url: https://github.com/k14s/k8s-simple-app-example
          subPath: config-step-1-minimal
      template:
      - ytt: {}          
      deploy:
      - kapp: {}

# more application deployments can go here
```