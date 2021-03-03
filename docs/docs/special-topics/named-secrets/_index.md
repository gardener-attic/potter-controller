---
title: Named Secrets
type: docs
---

## Basics and Example

Named secret values allow to specify secret values in a Cluster-BoM which are automatically moved to secrets before the Cluster-BoM is stored or updated. These secrets could be referenced in kapp deployments via their logical names. In helm deployments the secret values stored in the secrets are added as additional values files for templating.

The following example shows a Cluster-BoM with a section `namedSecretValues` with two named secret values `potter-examples-access` and `otherSecrets`. Make sure that the values of the top level map entries under `data` are strings.


````yaml
apiVersion: hub.k8s.sap.com/v1
kind: ClusterBom
metadata:
  name: pot-namedsecretvalues-kapp
  namespace: <some-garden-namespace>
spec:
  applicationConfigs:
  - configType: kapp
    id: appId1
    namedSecretValues:
      potter-examples-access:          # logical secret name
        data:
          username: YOUR_USER          # moved to secret
          password: YOUR_ACCESS_TOKEN  # moved to secret
      otherSecrets:        
        data:
          otherSecret1: |
            secret1: s11
            secret2: s21
          otherSecret2: |
            secret3: 
              user: users3
              password: pw3
    typeSpecificData:
      fetch:
      - git:
          ref: origin/master
          subPath: namedsecretvalues/kapp/resources
          url: https://github.com/your-path/potter-examples
          secretRef:
            name: potter-examples-access
      template:
      - ytt: {}
      deploy:
      - kapp: {}
  secretRef: <clusterName>.kubeconfig
````

When this Cluster-BoM is deployed, the data of `potter-examples-access` and `otherSecrets` are stored in two automatically generated secrets in the following format:

````yaml
apiVersion: v1
kind: Secret
metadata:
  labels:
  # internal secret name, not the logical secret name  
  name: pot-namedsecretvalues-kapp-appId1-<someGuid1>
  namespace: <some-garden-namespace>
type: Opaque
data:
  username: YOUR_USER        
  password: YOUR_ACCESS_TOKEN
````

````yaml
apiVersion: v1
kind: Secret
metadata:
  labels:
  # internal secret name, not the logical secret name  
  name: pot-namedsecretvalues-kapp-appId1-<someGuid2>
  namespace: <some-garden-namespace>
type: Opaque
data:
  otherSecret1:
    secret1: s11
    secret2: s21
  otherSecret2:
    secret3: 
      user: users3
      password: pw3
````

The stored Cluster-BoM itself only contains references to these secrets instead of the data: 

````yaml
apiVersion: hub.k8s.sap.com/v1
kind: ClusterBom
metadata:
  name: pot-namedsecretvalues-kapp
  namespace: <some-garden-namespace>
spec:
  applicationConfigs:
  - configType: kapp
    id: appId1
    namedSecretValues:
      potter-examples-access:
        internalSecretName: pot-namedsecretvalues-kapp-appId1-<someGuid1>
      otherSecrets:        
        internalSecretName: pot-namedsecretvalues-kapp-appId1-<someGuid2>
    typeSpecificData:
      fetch:
      - git:
          ref: origin/master
          subPath: namedsecretvalues/kapp/resources
          url: https://github.com/your-path/potter-examples
          secretRef:
            name: potter-examples-access
      template:
      - ytt: {}
      deploy:
      - kapp: {}
  secretRef: <clusterName>.kubeconfig
````

## Usage of Named Secrets in kapp deployments

In the section `typeSpecificData` you see an example how named secret values could be used in a kapp deployment. Here the logical secret name `potter-examples-access` is specified as the name of a secret ref. During the deployment the secret connected with this logical name is used to access a private git repository. You could enter logical secret names everywhere in the type specific data section of a kapp deployment where a secret ref could be used.

## Usage of Named Secrets in helm deployments

In helm deployments, the data of every named secret is provided as an additional values file during templating. These template files are applied after the values specified in the secret values section. There is no predefined order in which the data of the different named secrets are applied.

A named secret could be also referenced in a tarball access as in the following example:

````yaml
apiVersion: hub.k8s.sap.com/v1
kind: ClusterBom
metadata:
  name: pot-namedsecretvalues-authheader
  namespace: <some-garden-namespace>
spec:
  applicationConfigs:
  - configType: helm
    id: echoserver
    namedSecretValues:
      logical-name-1:
        data:
          authHeader: "Basic dX...3dvcmQ="
    typeSpecificData:
      tarballAccess:
        url: "https://yourpath/echo-server-1.0.5.tgz" 
        secretRef:
          name: logical-name-1
      installName: echoserver
      namespace: <some-target-namespace>
  secretRef: <your-clustername>.kubeconfig
````

With this a secret containing the authheader data is automatically created and used for accessing the tgz file. This secret it automatically excluded from being merged as an additional values file during helm template.

## Update Secret Values

To **keep** the values of a named secret value with logical name `X` unchanged, there are several possibilities how to specify this in a Cluster-BoM. Either there is no `namedSecretValues` section at all or it does not contain `X`. You could also provide the named secret with identical data or no data.

To **replace** the secret values just provide new data. 

````yaml
    namedSecretValues:
      X:
        data: ... # new data
````

It is not allowed to delete named secrets values by just providing an empty data section to prevent accidental deletions.

## Delete Secret Values

To **delete** the secret values use the `delete` operation:

````yaml
    namedSecretValues:
      X:
        operation: delete
````