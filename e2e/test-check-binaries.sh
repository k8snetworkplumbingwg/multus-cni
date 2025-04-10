#!/bin/bash
set -o errexit

# Wait for init containers...
for pod in $(kubectl get pods -n ${NAMESPACE} -l name=multus -o jsonpath='{.items[*].metadata.name}'); do
    echo "Waiting for init container to complete in pod: ${pod}"

    # Timeout loop: 60 tries, 5 seconds sleep = 5 minutes max
    for i in {1..60}; do
        state=$(kubectl get pod ${pod} -n ${NAMESPACE} -o jsonpath="{.status.initContainerStatuses[?(@.name==\"${INSTALL_INIT_CONTAINER}\")].state.terminated.reason}" 2>/dev/null || true)

        if [ "$state" = "Completed" ]; then
            echo "SUCCESS: Init container completed in pod ${pod}"
            break
        fi

        echo "Still waiting for init container in pod ${pod} (current state: ${state})..."
        sleep 1
    done

    # After waiting, make sure it's done
    state=$(kubectl get pod ${pod} -n ${NAMESPACE} -o jsonpath="{.status.initContainerStatuses[?(@.name==\"${INSTALL_INIT_CONTAINER}\")].state.terminated.reason}" 2>/dev/null || true)
    if [ "$state" != "Completed" ]; then
        echo "FAIL: Init container did not complete in pod ${pod} after timeout."
        exit 1
    fi
done

echo "Sleeping for 5 seconds (for fs sync, possibly)..."
sleep 5

# verify binaries
for bin in $EXPECTED_BINARIES; do
    if ! docker exec "${NODE_NAME}" test -f "${bin}"; then
        echo "FAIL: Expected binary ${bin} not found on node ${NODE_NAME}"
        exit 1
    fi
    echo "SUCCESS: Binary ${bin} found."

    after_ts=$(docker exec "${NODE_NAME}" stat -c %Y "${bin}")
    echo "After reboot: ${bin} mtime = ${after_ts}"

    if [ "${after_ts}" -le "${before_mtime[${bin}]}" ]; then
        echo "FAIL: mtime for ${bin} did not update after reboot (before: ${before_mtime[${bin}]}, after: ${after_ts})"
        exit 1
    fi

    echo "SUCCESS: mtime for ${bin} updated correctly after reboot."
done
