---
title: Fetching Resources From a private GitHub Repository
type: docs
---

Section [Cluster BoM kapp Example](../../kapp-example/) describes how to deploy an application using the kapp controller. Now we explain the additional steps that are necessary if the application files are fetched from a **private** GitHub repository:

1. Create a Deploy Key for the GitHub repository
2. Create a secret in your Gardener project which contains the private key.
3. Create a Cluster BoM with a reference to the secret.

## Creating a Deploy Key

First, create a Deploy Key for your private GitHub repository. Basically, you you use the `ssh_keygen` command to create a pair of a private and a public key.

```zsh
▶ ssh-keygen -t rsa -b 4096 -C <email>
```

Next, you upload the public key in the section *Settings* > *Deploy keys* of the GitHub repository. There is a step-by-step description in the GitHub documentation:

- [Deploy Keys](https://docs.github.com/en/developers/overview/managing-deploy-keys#deploy-keys)  
- [Generating a New SSH Key](https://docs.github.com/en/github/authenticating-to-github/generating-a-new-ssh-key-and-adding-it-to-the-ssh-agent#generating-a-new-ssh-key)  

## Secret

Create a secret in your Gardener project containing the private key generated in the previous step. You can use the following command to create this secret (adjust secret name, namespace, and path):

```zsh
▶ kubectl create secret generic -n <namespace> <secret name> --from-file=ssh-privatekey=<path to private key>
```

The resulting secret contains the base64 encoded private key at `data.ssh-privatekey`:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: <secret name>
  namespace: <namespace>
data:
  ssh-privatekey: <base64 encoded content of the private key file>
type: Opaque
```

## Cluster BoM

Create a Cluster BoM for your your application. In the Cluster-Bom, add a reference to the secret created in the previous step (field `fetch.git.secretRef` in type specific data of the application config). Use the SSH URL of the repository (field `fetch.git.url`).

```yaml
apiVersion: "hub.k8s.sap.com/v1"
kind: ClusterBom
metadata:
  name: demo
  namespace: garden-apphubdemo
spec:
  secretRef: my-cluster.kubeconfig
  autoDelete:
    clusterBomAge: 120
  applicationConfigs:
  - id: kappexample
    configType: kapp
    typeSpecificData:
      fetch:
      - git:
          ref: origin/master
          url: git@github.com:demo/my-private-repo.git # SSH URL of the repository
          subPath: my/demo/application
          secretRef:
            name: <secret name> # name of the secret with private deploy key
      template:
      - ytt: {}          
      deploy:
      - kapp: {}
```