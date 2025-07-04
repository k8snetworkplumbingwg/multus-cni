#!/bin/bash
set -o errexit

NODE_NAME="kind-worker"
DAEMONSET_NAME="kube-multus-ds-amd64"
NAMESPACE="kube-system"
EXPECTED_BINARIES="${EXPECTED_BINARIES:-/opt/cni/bin/multus-shim}"
INSTALL_INIT_CONTAINER="${INSTALL_INIT_CONTAINER:-install-multus-shim}"

declare -A before_mtime

# Capture the mtimes before upgrade
echo "Capturing binary mtimes before upgrade on node ${NODE_NAME}..."

for bin in $EXPECTED_BINARIES; do
    echo "Getting mtime for ${bin}..."
    before_ts=$(docker exec "${NODE_NAME}" stat -c %Y "${bin}")
    before_mtime["${bin}"]=$before_ts
    echo "Before reboot: ${bin} mtime = ${before_ts}"
done


# Delete all Multus DaemonSet pods to simulate an upgrade.
echo "Deleting all Multus DaemonSet pods to simulate upgrade..."
kubectl delete pods -n ${NAMESPACE} -l name=multus

# Wait for the Multus DaemonSet pods to come back up.
echo "Waiting for Multus DaemonSet ${DAEMONSET_NAME} pods to be Ready after upgrade..."
kubectl rollout status daemonset/${DAEMONSET_NAME} -n ${NAMESPACE} --timeout=300s

source ./test-check-binaries.sh

echo "Upgrade test PASSED"
