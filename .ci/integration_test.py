#!/usr/bin/env python3

import os
from subprocess import Popen, PIPE, STDOUT, run
import sys
import shutil
import utils
import yaml

import secret_server
from util import ctx

print("Starting integration test")
print(f"current environment: {os.environ}")
controller_path = os.environ['HUB_CONTROLLER_PATH']
root_path = os.environ['ROOT_PATH']
landscape = os.environ['LANDSCAPE']
garden_namespace = os.environ['GARDEN_NAMESPACE']
target_cluster = os.environ['TARGET_CLUSTER']
target_cluster_ns1 = os.environ['TARGET_CLUSTER_NS1']
target_cluster_ns2 = os.environ['TARGET_CLUSTER_NS2']
test_prefix = os.environ['TEST_PREFIX']
test_type = os.environ.get('TEST_TYPE', None) # prevent key error if not present

try:
    # env var is implicitly set by the output dir in case of a release job
    integration_test_path = os.environ["INTEGRATION_TEST_PATH"]
except KeyError:
    print("Output dir env var not set. " +
          "The output of the integration test won't be saved in a file.")

# Init clients
secret_server_client = secret_server.SecretServer()
garden_cluster_k8s_client = secret_server_client.get_kube_client(
    f'garden-{landscape}-virtual'
)

# read and write kubeconfig of landscape cluster
garden_virtual_kubeconfig = secret_server_client.get_kubeconfig(
    f'garden-{landscape}-virtual'
)

garden_virtual_kubeconfig_path = os.path.join(
    root_path,
    "garden_virtual_kubeconfig.yaml"
)

landscape_kubeconfig = utils.get_kubecfg_from_serviceaccount(
    kubernetes_client=garden_cluster_k8s_client,
    namespace='default',
    name='app-hub-controller',
    sample_kubecfg_path=garden_virtual_kubeconfig_path
)

# factory = ctx().cfg_factory()
# garden_bom_dc_kubecfg = factory.kubernetes("hub-" + landscape)

landscape_kubeconfig_name = "landscape_kubeconfig"
landscape_kubeconfig_path = os.path.join(root_path, controller_path,
                                         "integration-test",
                                         landscape_kubeconfig_name)

utils.write_data(landscape_kubeconfig_path, yaml.dump(
                landscape_kubeconfig.kubeconfig()))

landscape_config = utils.get_landscape_config("hub-" + landscape)
int_test_config = landscape_config.raw["int-test"]["config"]
token = int_test_config["auth"]["token"]

golang_found = shutil.which("go")
if golang_found:
    print(f"Found go compiler in {golang_found}")
else:
    print("No Go compiler found, installing Go")
    command = ['apk', 'add', 'go', '--no-progress']
    result = run(command)
    result.check_returncode()


os.chdir(os.path.join(root_path, controller_path, "integration-test"))

command = ['go', 'run', 'main.go',
           '-garden-kubeconfig', landscape_kubeconfig_path,
           '-garden-namespace', garden_namespace,
           '-target-clustername', target_cluster,
           "-target-cluster-namespace1", target_cluster_ns1,
           "-target-cluster-namespace2", target_cluster_ns2,
           "-test-prefix", test_prefix,
           "-test-type", test_type]

print(f"Running integration test with command: {' '.join(command)}")
try:
    # check if path var is set
    integration_test_path
except NameError:
    run = run(command)
else:
    output_path = os.path.join(root_path, integration_test_path, "out")

    with Popen(command, stdout=PIPE, stderr=STDOUT, bufsize=1, universal_newlines=True) as run, open(output_path, 'w') as file:
        for line in run.stdout:
            sys.stdout.write(line)
            file.write(line)

if run.returncode != 0:
    raise EnvironmentError("Integration test exited with errors")