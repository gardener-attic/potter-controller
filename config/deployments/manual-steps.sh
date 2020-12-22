#!/bin/bash

set -e
set -u

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

# TODO update these values
# path_gardener_kubeconfig="/Users/d056995/.kube/kubeconfig--hubforplay--cont.yaml"
path_gardener_kubeconfig="/Users/d056995/Desktop/kubeconfig-clusterbom-controller.yaml"
path_apprepo_kubeconfig="/Users/d056995/Desktop/apprepo-reader-secret.yaml"
path_gcr_secret="$(cat ~/Desktop/sap-gcp-k8s-wm-live-controller-read-2.json )"
path_image_pull_secret="/Users/d056995/Desktop/image-pull-secret.yaml"
gcr_email="controller-read-2@sap-gcp-k8s-wm-live.iam.gserviceaccount.com"

echo -e "\n#####################################################"
echo -e "### Create Gardener and Container Registry Secret ###"
echo -e "#####################################################\n"

kubectl create secret generic gardener-kubeconfig --from-file=kubeconfig=$path_gardener_kubeconfig
# Note: kubeconfig=... denotes the file name where this kubeconfig is named in mount dir, overrides file name
kubectl create secret docker-registry gcr \
  --docker-server=https://eu.gcr.io \
  --docker-username=_json_key \
  --docker-email="$gcr_email" \
  --docker-password="$path_gcr_secret"

echo -e "\n#####################################################"
echo -e "### Create Secret with image pull Credentials       ###"
echo -e "#####################################################\n"
kubectl create secret generic hubsec-image-pull-secrets-creds --from-file=hub=$path_image_pull_secret

echo -e "\n#####################################################"
echo -e "### Create Secret with Hub Credential for Apprepos  ###"
echo -e "#####################################################\n"
kubectl create secret generic apprepo-kubeconfig --from-file=apprepokubeconfig=$path_apprepo_kubeconfig

echo -e "\n#####################################################"
echo -e "### Apply ClusterBom and other CRDs ###"
echo -e "#####################################################\n"
kubectl apply -f config/crd/bases/

echo -e "\n#####################################################"
echo -e "### Apply the AppRepositories from Hub project   ###"
echo -e "#####################################################\n"
kubectl apply -f https://github.wdf.sap.corp/raw/kubernetes/hub/master/chart/kubeapps/crds/apprepository-crd.yaml
kubectl apply -f $DIR/apprepo-stable.yaml
kubectl apply -f $DIR/apprepo-incubator.yaml

echo -e "\n#####################################################"
echo -e "### Apply dummy secret like in Gardener             ###"
echo -e "#####################################################\n"
kubectl create secret generic sample-cluster.kubeconfig --from-file=kubeconfig=$path_gardener_kubeconfig

echo -e "\n##############################################################"
echo -e "### PLEASE START THE DEPLOYMENT by creating the deployment ###"
echo -e "##############################################################\n"
