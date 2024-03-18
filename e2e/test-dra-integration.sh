#!/bin/sh
set -o errexit

export PATH=${PATH}:./bin

# This test is using an example implementation of a DRA driver. This driver is mocking GPU resources. At our test we
# don't care about what these resources are. We want to ensure that such resource is correctly passed in the Pod using
# Multus configurations. A couple of notes:
# - We explitictly don't pin the revision of the dra-example-driver to a specific commit to ensure that the integration
#   continues to work even when the dra-example-driver is updated (which may also indicate API changes on the DRA).
# - The chart and latest is image is not published somewhere, therefore we have to build locally. This leads to slower
#   e2e suite runs.
echo "installing dra-example-driver"
repo_path="repos/dra-example-driver"

rm -rf $repo_path || true
git clone https://github.com/kubernetes-sigs/dra-example-driver.git ${repo_path}
${repo_path}/demo/build-driver.sh
KIND_CLUSTER_NAME=kind ${repo_path}/demo/scripts/load-driver-image-into-kind.sh
helm install \
    -n dra-example-driver \
    --create-namespace \
    dra-example-driver \
    ${repo_path}/deployments/helm/dra-example-driver

echo "installing testing pods"
kubectl create -f yamls/dra-integration.yml
kubectl wait --for=condition=ready -l app=dra-integration --timeout=300s pod

echo "check dra-integration pod for DRA injected environment variable"

# We can validate that the resource is correctly injected by checking an environment variable this dra driver is injecting
# in the Pod.
# https://github.com/kubernetes-sigs/dra-example-driver/blob/be2b8b1db47b8c757440e955ce5ced88c23bfe86/cmd/dra-example-kubeletplugin/cdi.go#L71C20-L71C44
env_variable=$(kubectl exec dra-integration -- bash -c "echo \$DRA_RESOURCE_DRIVER_NAME | grep gpu.resource.example.com")
if [ $? -eq 0 ];then
	echo "dra-integration pod has DRA injected environment variable"
else
	echo "dra-integration pod doesn't have DRA injected environment variable"
	exit 1
fi

echo "cleanup resources"
kubectl delete -f yamls/dra-integration.yml
helm uninstall -n dra-example-driver dra-example-driver
