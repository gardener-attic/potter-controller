#!/usr/bin/env python3

import os
import subprocess
import sys

sys_args = sys.argv
root_path = os.getcwd()
hub_controller_path = os.environ['SOURCE_PATH']

# parse sys var(will be passed to subprocess calls)
for i in range(len(sys_args)):
    if '--garden-namespace ' in sys_args[i]:
        os.environ['GARDEN_NAMESPACE'] = sys_args[i].split(' ', 2)[1]
    if "--target-clustername" in sys_args[i]:
        os.environ['TARGET_CLUSTER'] = sys_args[i].split(' ', 2)[1]
    if "--target-cluster-ns1 " in sys_args[i]:
        os.environ['TARGET_CLUSTER_NS1'] = sys_args[i].split(' ', 2)[1]
    if "--target-cluster-ns2 " in sys_args[i]:
        os.environ['TARGET_CLUSTER_NS2'] = sys_args[i].split(' ', 2)[1]
    if "--test-prefix " in sys_args[i]:
        os.environ['TEST_PREFIX'] = sys_args[i].split(' ', 2)[1]
    if "--test-type " in sys_args[i]:
        os.environ['TEST_TYPE'] = sys_args[i].split(' ', 2)[1]

os.environ['HUB_CONTROLLER_PATH'] = os.environ['SOURCE_PATH']
os.environ['ROOT_PATH'] = root_path
os.environ['LANDSCAPE'] = "dev"
os.environ['NAMESPACE'] = "controller-release-test"


hub_kubeconfig = os.path.join(
    root_path, hub_controller_path,
    ".ci",
    "integration_test.py"
)

command = [hub_kubeconfig, "--namespace", "release-test"]

result = subprocess.run(command)
result.check_returncode()
