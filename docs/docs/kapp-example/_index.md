---
title: Cluster-BoM Example with kapp Applications
type: docs
weight: 50
---

In this example, kapp applications are deployed. For kapp deployments the [kapp controller](https://github.com/k14s/kapp-controller/blob/develop/docs/app-spec.md) is used, providing a large variety of different fetch, and template mechanisms. Therefore the format of `typeSpecificData` is similar to the specification used there. 

The section `cluster.kubeconfigSecretRef` is optional and if missing `secretRef` of the Cluster-BoM is used for `cluster.kubeconfigSecretRef.name` with key `kubeconfig`. If `cluster.kubeconfigSecretRef.name` is specified it must be equal to `secretRef` of the Cluster-BoM.

Of course it is also possible to add more than one kapp application to a Cluster-BoM or mix it with Helm chart deployments. 

The currently deployed version of the kapp controller could be found under `kappImage.tag` in the [values](https://github.com/gardener/potter-controller/blob/master/chart/hub-controller/values.yaml) file. 


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

  - id: kappexample1                       # ID of the application within this Cluster-BoM
    configType: kapp                       # Deployment via kapp controller           
    typeSpecificData:                      # Spec of a KAPP App, see
                                           # https://github.com/k14s/kapp-controller/blob/develop/docs/app-spec.md
      fetch:
      - git:
          ref: origin/develop
          url: https://github.com/k14s/k8s-simple-app-example
      template:
      - ytt:
          paths:
          - config-step-2-template
      deploy:
      - kapp: {}

  - id: kappexample2                      
    configType: kapp                                  
    typeSpecificData:                      
      fetch:
      - http:
          url: https://storage.googleapis.com/hub-tarballs/kapp/success/http-zip-yml/config/simple.zip
      template:
      - ytt: {}
      deploy:
      - kapp:
          intoNs: testnamespace


# more application deployments can go here
```

Typical pitfalls we found so far with the specification of the `fetch` section using http (might be resolved in future versions of the kapp controller):
  * Yaml files referenced via http fetch must be packaged e.g. in a zip file.
  * Be careful if you create zip files on a Mac. A folder `__MAXOSX` is added to the archive automatically and must be removed before usage e.g. via the command 
  ```bash
  zip -d example.zip "__MAXOSX*"`.
  ```
  * The yaml files in the archive must have the file ending `yml` and not `yaml`.

For kapp applications, in the type specific data of the Cluster-BoM, you have the possibility to reference secrets via `secretRef` entries. The secrets must be stored in the same cluster and namespace as the corresponding Cluster-BoM. More details can be found [here](../special-topics/fetching-resources-from-private-github-repo/).

Currently the `values` section of application configs is not considered for kapp deployments.