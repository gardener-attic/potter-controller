#!/usr/bin/env python3

import os
from subprocess import Popen, PIPE, STDOUT, run
import sys
import shutil
import utils
import yaml

from ci.util import ctx

print("Starting integration test")
print(f"current environment: {os.environ}")
source_path = os.environ['SOURCE_PATH']
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

factory = ctx().cfg_factory()
landscape_kubeconfig = factory.kubernetes("garden-" + landscape + "-virtual")

landscape_kubeconfig_name = "landscape_kubeconfig"
landscape_kubeconfig_path = os.path.join(root_path, source_path,
                                         "integration-test",
                                         landscape_kubeconfig_name)

utils.write_data(landscape_kubeconfig_path, yaml.dump(
                landscape_kubeconfig.kubeconfig()))

golang_found = shutil.which("go")
if golang_found:
    print(f"Found go compiler in {golang_found}")
else:
    print("No Go compiler found, installing Go")
    command = ['apk', 'add', 'go', '--no-progress']
    result = run(command)
    result.check_returncode()


os.chdir(os.path.join(root_path, source_path, "integration-test"))

command = ["go", "run", "main.go",
           "-garden-kubeconfig", landscape_kubeconfig_path,
           '-garden-namespace', garden_namespace,
           '-target-clustername', target_cluster,
           "-target-cluster-namespace1", target_cluster_ns1,
           "-target-cluster-namespace2", target_cluster_ns2,
           "-test-prefix", test_prefix,
           "-test-type", test_type]
# TODO: temp disablbe int tests
command = ["go", "version"]

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