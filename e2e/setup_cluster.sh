#!/bin/sh
set -o errexit

export PATH=${PATH}:./bin

# define the OCI binary to be used. Acceptable values are `docker`, `podman`.
# Defaults to `docker`.
OCI_BIN="${OCI_BIN:-docker}"

# define the deployment spec to use when deploying multus.
# Acceptable values are `multus-daemonset.yml`. `multus-daemonset-thick.yml`.
# Defaults to `multus-daemonset-thick.yml`.
MULTUS_MANIFEST="${MULTUS_MANIFEST:-multus-daemonset-thick.yml}"
# define the dockerfile to build multus.
# Acceptable values are `Dockerfile`. `Dockerfile.thick`.
# Defaults to `Dockerfile.thick`.
MULTUS_DOCKERFILE="${MULTUS_DOCKERFILE:-Dockerfile.thick}"

kind_network='kind'
if [ "${MULTUS_DOCKERFILE}" != "none" ]; then
	$OCI_BIN build -t localhost:5000/multus:e2e -f ../images/${MULTUS_DOCKERFILE} ..
fi

# deploy cluster with kind
cat <<EOF | kind create cluster --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
  - role: worker
    kubeadmConfigPatches:
    - |
      kind: InitConfiguration
      nodeRegistration:
        kubeletExtraArgs:
          pod-manifest-path: "/etc/kubernetes/manifests/"
          feature-gates: "DynamicResourceAllocation=true,DRAResourceClaimDeviceStatus=true,KubeletPodResourcesDynamicResources=true"
  - role: worker
# Required by DRA Integration
##
featureGates:
  DynamicResourceAllocation: true
  DRAResourceClaimDeviceStatus: true
  KubeletPodResourcesDynamicResources: true
runtimeConfig:
  "api/beta": "true"
containerdConfigPatches:
# Enable CDI as described in
# https://github.com/container-orchestrated-devices/container-device-interface#containerd-configuration
- |-
  [plugins."io.containerd.grpc.v1.cri"]
      enable_cdi = true
##
EOF

# load multus image from container host to kind node
kind load docker-image localhost:5000/multus:e2e

worker1_pid=$($OCI_BIN inspect --format "{{ .State.Pid }}" kind-worker)
worker2_pid=$($OCI_BIN inspect --format "{{ .State.Pid }}" kind-worker2)

kind export kubeconfig
sudo env PATH=${PATH} koko -p "$worker1_pid,eth1" -p "$worker2_pid,eth1"
sleep 1
kubectl -n kube-system wait --for=condition=available deploy/coredns --timeout=300s
kubectl create -f yamls/$MULTUS_MANIFEST
sleep 1
kubectl -n kube-system wait --for=condition=ready -l name=multus pod --timeout=300s
kubectl create -f yamls/cni-install.yml
sleep 1
kubectl -n kube-system wait --for=condition=ready -l name=cni-plugins pod --timeout=300s
