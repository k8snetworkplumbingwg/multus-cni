#!/bin/sh
set -o errexit

export PATH=${PATH}:./bin

TEST_POD_NAME="sysctl-modified"

# Deploy the daemonset that will lay down the chained CNI config
kubectl apply -f yamls/subdirectory-chaining.yml

# Wait for the daemonset pods to be ready (we need the config to be laid down)
kubectl rollout status daemonset/cni-setup-daemonset

# Deploy a test pod that will get chained CNI applied
kubectl apply -f yamls/subdirectory-chaining-pod.yml

# Wait for the pod to be Ready
kubectl wait --for=condition=ready pod/sysctl-modified --timeout=300s

# Check that the sysctl got set properly inside the pod's eth0 interface
echo "Verifying sysctl arp_filter is set to 1 on eth0"

SYSCTL_VALUE=$(kubectl exec sysctl-modified -- sysctl -n net.ipv4.conf.eth0.arp_filter)

if [ "$SYSCTL_VALUE" != "1" ]; then
  echo "FAIL: net.ipv4.conf.eth0.arp_filter is not set to 1, got ${SYSCTL_VALUE}" >&2
  exit 1
else
  echo "SUCCESS: net.ipv4.conf.eth0.arp_filter is set correctly."
fi

# 6. Clean up
echo "Cleaning up test resources"
kubectl delete -f yamls/subdirectory-chaining-pod.yml
kubectl delete -f yamls/subdirectory-chaining.yml

exit 0
