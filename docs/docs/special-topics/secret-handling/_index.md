---
title: Secret Handling
type: docs
---

#### Please note: The feature described here is deprecated and will be removed end of April 2021. For a more flexible way to create and use secrets see [Named Secrets](../named-secrets).

This section is only relevant for Helm deployments. 

In kubernetes, credentials and other secret data should only be stored in secrets, but not in other kinds of resources like clusterboms. Therefore the Application Hub provides a way to handle secret data during deployments. 

You can include secret data in a section `secretValues` in every application config of the Cluster-BoM.

```yaml
spec:
  applicationConfigs:
  - id:               ...
    configType:       ...
    typeSpecificData: ...
    values:           ...
    secretValues:
      data:
        credentials:
        - username: <some name>
          password: <some password>
```

Before the Cluster-BoM is saved, the secret data are moved into a secret resource, one secret for every application config with secret values. Only a reference to the secret remains in the clusterbom. If you fetch the Cluster-BoM using `kubectl get clusterbom ...` you get the following:

```yaml
    ...
    secretValues:
      internalSecretName: <secret name>
    ...
```

The secret is stored in the same cluster (garden cluster) and namespace as the Cluster-BoM. 

Hub managed secrets cannot be modified or deleted by the user. The user can modify secret values only by changing the Cluster-BoM. If the user changes secret values, a **new** secret is created and the internal secret name in the clusterbom is replaced by the new secret name.

During the helm deployment operation, the secret values are merged into the (normal) values section. Thereby keys are added and values replaced as in the example below. Given the following part of a Cluster-BoM:

```yaml
spec:
  applicationConfigs:
  - id:               ...
    configType:       ...
    typeSpecificData: ...
    values: 
      test:
        key1: val11
        key2: 
        - val21
        - val22
        - val23
        key4: val41
    secretValues:
      data:
        test:
          key1: val12
          key2: 
          - val24
          - val25
          key3: val31
```

This results in the following merged values file provided to the helm deploy operation:

```yaml
test:
  key1: val12
  key2: 
  - val24
  - val25
  key3: val31
  key4: val41
```

## Update Secret Values

To **keep** the secret values unchanged, there are several possibilities how to specify this in the Cluster-BoM. Either there is no secretValues section at all or you are using one of the following alternatives:

```yaml
    secretValues:
      operation: keep
```

or

```yaml
    secretValues:
      operation: replace
      data: 
        ... # same data as before
```

or

```yaml
    secretValues:
      internalSecretName: ... # same secret name as before
```

To **replace** the secret values use the `replace` operation:

```yaml
    secretValues:
      operation: replace
      data: 
        ... # new data
```

It is also possible to just provide new data without any operation to replace the secret values. It is not allowed to delete the secrets values this way by just providing an empty data section.

```yaml
    secretValues:
      data: 
        ... # new data
```

## Delete Secret Values

To **delete** the secret values use the `delete` operation:

```yaml
    secretValues:
      operation: delete
``` 