---
title: Cluster-BoM Example with Helm Applications
type: docs
weight: 40
---

In this example two Helm applications are deployed, one from the SAP-incubator Helm repository and one using a direct chart link. This example uses the project name `apphubdemo` in Gardener and the shoot cluster named `my-cluster`.

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

  - id: karydia                            # Unique id for application for this BoM-Cluster
    noReconcile: true                      # Exclude application from reconcilation loop 
                                           #    (optional with default false)

    readyRequirements:                     # (optional)
      jobs:                                # (optional) Jobs which must succeed as a precondition for a  
      - name: testJob1                     # successfully deployed and running application
        namespace: testNamespace

    configType: helm                       # Helm deployment, more types are planned
    values:                                # The value section, Helm chart values in this example
      config:
        cloudProvider: "GCP"               # Helm chart specific value 

    typeSpecificData:                      # Details about what exactly to deploy
      installName: "securityconfig"        # Name of the deployment (arbitrary)
      namespace: "karydia"                 # Namespace in target cluster where to install the application
      installTimeout: 10                   # Timeout in minutes for the Helm install command (optional default=5)
      upgradeTimeout: 10                   # Timeout in minutes for the Helm upgrade command (optional default=5)
      uninstallTimeout: 10                 # Timeout in minutes for the Helm uninstall command (optional default=5)
      installArguments:                    # (optional) Arguments used for helm install.
      - atomic                             # Currently only atomic is supported. 
      updateArguments:                     # (optional) Arguments used for helm upgrade.
      - atomic                             # Currently only atomic is supported. 
      catalogAccess:                       # Catalog specification where the Helm chart can be found:
        chartName: "karydia"               # Name of the Helm chart
        repo: "sap-incubator"              # Name of the Helm chart repository
        chartVersion: "0.3.1"              # Helm chart version to be deployed

  - id: mongodb                            # The second application within this Cluster-BoM
    configType: helm
    typeSpecificData:                      # Helm settings (see above)
      installName: "mongodb"               
      namespace: "default"   
      tarballAccess:                       # Allows specifying an url pointing to the packaged chart
        url: "https://kubernetes-charts.storage.googleapis.com/mongodb-7.8.4.tgz" 

        customCAData:                      # (optional) This property allows you to add a custom CA, 
                                           # which is useful if your server speaks HTTPS with a self-
                                           # signed certificate. The added certificate must be 
                                           # in PEM format and base64 encoded.
                                           
        authHeader: Basic dX...3dvcmQ=     # (optional) The value of this property will be set 
                                           # in the "Authorization" header when fetching the Chart. 
                                           # For Basic Authentication, the credentials can also 
                                           # be stored as part of the url instead of using the
                                           # "authHeader" property, for example, 
                                           # https://username:password@example.com/my-chart.tgz.

        secretRef:                         # (optional) Reference to a secret in the same namespace as the 
          name: someSecretName             # clusterbom, containing an entry with key "authHeader". 
                                           # The value of this entry will be set in the "Authorization" 
                                           # header when fetching the Chart (e.g. "Basic dX...3dvcmQ="). 
                                           # You could also reference a named secret here (see 
                                           # https://github.wdf.sap.corp/kubernetes/hub/wiki/Named-Secrets).

# more application deployments can go here
```