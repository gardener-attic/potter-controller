---
title: Ready Requirements
type: docs
---

# Resource Ready Requirements

This feature allows users of ClusterBoMs to specify properties from arbitrary K8s resources on a target cluster, that must match against a defined list of values in order for the "Ready" condition of the ClusterBoM to be "True". E.g. in the following simple example, the `Complete` condition of the K8s Job `my-job` is defined as a single resource ready requirement.

```yaml
apiVersion: hub.k8s.sap.com/v1
kind: ClusterBom
metadata:
  name: my-bom
  namespace: garden-hubtest
spec:
  applicationConfigs:
  - configType: helm
    id: my-app
    readyRequirements:
      resources:
      - name: my-job
        namespace: my-namespace
        apiVersion: batch/v1
        resource: jobs
        fieldPath: "{ .status.conditions[?(@.type == 'Complete')].status }"
        successValues:
        - value: "True"
    typeSpecificData:
      catalogAccess:
        chartName: my-chart
        chartVersion: 5.0.0
        repo: sap-incubator
      installName: my-app
      namespace: my-namespace
  secretRef: my-cluster.kubeconfig
```

Key Points:

- The list of resource ready requirements for one application config is defined via the property `applicationConfigs[].readyRequirements.resources`. It takes an arbitrary number of resource ready requirements that each must evaluate to true in order for the ClusterBom to be ready.
- Each single resource ready requirement has the properties `name`, `namespace`, `apiVersion`, and `resource`. They define the K8s resource on the target cluster which is used for evaluation.
- The field `resource` describes the resource kind as plural (Job --> jobs, Secret --> secrets, ...).
- The variable `fieldPath` addresses a property of the defined K8s resource which is extracted and used for evaluation. `fieldPath` therefore uses the [JSONPath](https://goessner.net/articles/JsonPath/) notation.
- If the resource itself or the property within the resource can't be found, the "Ready" condition of the ClusterBoM evaluates to "Unknown".
- `successValues` defines a list of values that the extracted value must match against. Each item in the list must be an object with the single key `value`. The value behind this key can be of any valid JSON/YAML type and gets used for comparison. Keep in mind that the value that is extracted via `fieldPath` and `successValues` must have the same type in order for the ready requirement to be fulfilled. The following example shows how resource ready requirements could be used on user-defined status fields using different data types.

```yaml
readyRequirements:
  resources:
  - name: foo-1
    namespace: namespace1
    apiVersion: foo.com/v1
    resource: foos
    fieldPath: "{ .status.overallState }"
    successValues:
    - value: "ok"
  - name: bar-1
    namespace: namespace1
    apiVersion: bar.com/v1
    resource: bars
    fieldPath: "{ .status.myStateObject }"
    successValues:
    - value:
        prop1: "val1"
        prop2: 42
    - value:
        prop1: "val2"
        prop2: 42
```

For the second resource from the above example, the extracted value from the defined resource is compared against each object

```json
{
    "prop1": "val1", 
    "prop2": 42
}
```

and 

```json
{
    "prop1": "val2",
    "prop2": 42
}
```

from `successValues`. The structure of the objects can be arbitrary. The keys and values of the extracted object and the "success" object must match in order for the ready requirement to be fulfilled.