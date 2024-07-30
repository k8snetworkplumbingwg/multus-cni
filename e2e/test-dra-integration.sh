#!/bin/sh
set -o errexit

export PATH=${PATH}:./bin

# This test is using an example implementation of a DRA driver. This driver is mocking GPU resources. At our test we
# don't care about what these resources are. We want to ensure that such resource is correctly passed in the Pod using
# Multus configurations. A couple of notes:
# - We explitictly pin the revision of the dra-example-driver to the branch `classic-dra` to indicate that the
#   integration continues to work even when the dra-example-driver is updated. We know that classic-dra is supported
#   in Kubernetes versions 1.26 to 1.30. Multus supports DRA in the aforementioned Kubernetes versions.
# - The chart and latest is image is not published somewhere, therefore we have to build locally. This leads to slower
#   e2e suite runs.
echo "installing dra-example-driver"
repo_path="repos/dra-example-driver"

rm -rf $repo_path || true
git clone --branch classic-dra https://github.com/kubernetes-sigs/dra-example-driver.git ${repo_path}
${repo_path}/demo/build-driver.sh
KIND_CLUSTER_NAME=kind ${repo_path}/demo/scripts/load-driver-image-into-kind.sh
chart_path=${repo_path}/deployments/helm/dra-example-driver/
overriden_values_path=${chart_path}/overriden_values.yaml

# With the thick plugin, in kind, the primary network on the control plane is not always working as expected. The pods
# sometimes are not able to communicate with the control plane and the error looks like this:
# failed to list *v1alpha2.PodSchedulingContext: Get "https://10.96.0.1:443/apis/resource.k8s.io/v1alpha2/podschedulingcontexts?limit=500&resourceVersion=0": dial tcp 10.96.0.1:443: connect: no route to host
# We override the values here to schedule the controller on the worker nodes where the network is working as expected.
cat <<EOF >> ${overriden_values_path}
controller:
  nodeSelector: null
  tolerations: null
EOF

helm install \
    -n dra-example-driver \
    --create-namespace \
    -f ${overriden_values_path} \
    dra-example-driver \
    ${chart_path}

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
