#!/bin/bash
set -o errexit

NODE_NAME="kind-worker"
DAEMONSET_NAME="kube-multus-ds-amd64"
NAMESPACE="kube-system"
EXPECTED_BINARIES="${EXPECTED_BINARIES:-/opt/cni/bin/multus-shim}"
INSTALL_INIT_CONTAINER="${INSTALL_INIT_CONTAINER:-install-multus-shim}"

declare -A before_mtime

for bin in $EXPECTED_BINARIES; do
    before_ts=$(docker exec "${NODE_NAME}" stat -c %Y "${bin}")
    before_mtime["${bin}"]=$before_ts
    echo "Before reboot: ${bin} mtime = ${before_ts}"
done

echo "Rebooting node..."
docker restart "${NODE_NAME}"
sleep 2
docker start "${NODE_NAME}"

kubectl wait --for=condition=Ready node/${NODE_NAME} --timeout=300s
kubectl rollout status daemonset/${DAEMONSET_NAME} -n ${NAMESPACE} --timeout=300s

source ./test-check-binaries.sh

echo "SUCCESS: reboot test passed"
