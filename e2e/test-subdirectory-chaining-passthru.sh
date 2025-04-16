#!/bin/bash
set -o errexit
set -o nounset
set -o pipefail

export PATH=${PATH}:./bin

TEST_POD_NAME="sysctl-modified"
EXPECTED_BINARIES="${EXPECTED_BINARIES:-/opt/cni/bin/ptp /opt/cni/bin/portmap /opt/cni/bin/tuning}"
EXPECTED_CNI_DIR="/etc/cni/net.d"

# Reconfigure multus
echo "Applying subdirectory chain passthru config..."
kubectl apply -f yamls/subdirectory-chain-passthru-configupdate.yml

# Restart the multus daemonset to pick up the new config
echo "Restarting Multus DaemonSet..."
kubectl rollout restart daemonset kube-multus-ds-amd64 -n kube-system
kubectl rollout status daemonset/kube-multus-ds-amd64 -n kube-system

# Debug: show CNI configs and binaries inside each Kind node
echo "Checking CNI configs and binaries on nodes..."

for node in $(kubectl get nodes --no-headers | awk '{print $1}'); do
    container_name=$(docker ps --format '{{.Names}}' | grep "^${node}$")

    echo "------"
    echo "Node: ${node} (container: ${container_name})"
    echo "Listing /opt/cni/bin contents..."
    docker exec "${container_name}" ls -l /opt/cni/bin || echo "WARNING: /opt/cni/bin missing!"

    echo "Checking expected binaries..."
    for bin in $EXPECTED_BINARIES; do
        echo "Checking for ${bin}..."
        if docker exec "${container_name}" test -f "${bin}"; then
            echo "SUCCESS: ${bin} found."
        else
            echo "FAIL: ${bin} NOT found!"
        fi
    done

    echo "Listing /etc/cni/net.d configs..."
    docker exec "${container_name}" ls -l ${EXPECTED_CNI_DIR} || echo "WARNING: ${EXPECTED_CNI_DIR} missing!"
done
echo "------"

# Deploy the daemonset that will lay down the chained CNI config
echo "Applying CNI setup DaemonSet..."
kubectl apply -f yamls/subdirectory-chaining-passthru.yml

# Wait for the daemonset pods to be ready (make sure they set up CNI config)
echo "Waiting for CNI setup DaemonSet to be Ready..."
kubectl rollout status daemonset/cni-setup-daemonset --timeout=300s

# Deploy a test pod that will get chained CNI applied
echo "Applying test pod..."
kubectl apply -f yamls/subdirectory-chaining-pod.yml

# Wait for the pod to be Ready
echo "Waiting for test pod to be Ready..."
kubectl wait --for=condition=ready pod/${TEST_POD_NAME} --timeout=300s

# Check that the sysctl got set
echo "Verifying sysctl arp_filter is set to 1 on eth0..."

SYSCTL_VALUE=$(kubectl exec ${TEST_POD_NAME} -- sysctl -n net.ipv4.conf.eth0.arp_filter)

if [ "$SYSCTL_VALUE" != "1" ]; then
  echo "FAIL: net.ipv4.conf.eth0.arp_filter is not set to 1, got ${SYSCTL_VALUE}" >&2
  exit 1
else
  echo "SUCCESS: net.ipv4.conf.eth0.arp_filter is set correctly."
fi

# Cleanup
echo "Cleaning up test resources..."
kubectl delete -f yamls/subdirectory-chaining-pod.yml
kubectl delete -f yamls/subdirectory-chaining-passthru.yml

echo "Test completed successfully."
exit 0
