# URL of your K8s cluster
clusterURL=

# Name of the secret containing the service account token
secretName=

ca=$(kubectl get secret/$secretName -o jsonpath='{.data.ca\.crt}')
token=$(kubectl get secret/$secretName -o jsonpath='{.data.token}' | base64 --decode)
namespace=$(kubectl get secret/$secretName -o jsonpath='{.data.namespace}' | base64 --decode)

kubeconfig="
apiVersion: v1
kind: Config
clusters:
- name: default-cluster
  cluster:
    certificate-authority-data: ${ca}
    server: ${clusterURL}
contexts:
- name: default-context
  context:
    cluster: default-cluster
    namespace: default
    user: default-user
current-context: default-context
users:
- name: default-user
  user:
    token: ${token}
"

echo "$kubeconfig"