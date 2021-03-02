---
title: Cluster-BoM Template
type: docs
weight: 71
---

Use the below yaml template to start creating your own Cluster BoM. You can also refer to the [Cluster BoM Helm Example](../helm-example) to see all available parameters.

```yaml
apiVersion: "hub.k8s.sap.com/v1"
kind: ClusterBom
metadata:
  name: <CLUSTER BOM NAME>         
  namespace: <CLUSTER BOM NAMESPACE>
spec:
  secretRef: <CLUSTER NAME>.kubeconfig
                                           
  applicationConfigs:                      

  - id: <UNIQUE-ID>
    configType: <TYPE>
    values:           

    typeSpecificData:

```