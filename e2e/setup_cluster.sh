#!/bin/sh
set -o errexit

export PATH=${PATH}:./bin

reg_name='kind-registry'
reg_port='5000'
running="$(docker inspect -f '{{.State.Running}}' "${reg_name}" 2>/dev/null || true)"
if [ "${running}" != 'true' ]; then
  docker run -d --restart=always -p "${reg_port}:5000" --name "${reg_name}" registry:2
fi
reg_ip="$(docker inspect -f '{{.NetworkSettings.IPAddress}}' "${reg_name}")"

cat <<EOF | kind create cluster --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors."localhost:${reg_port}"]
    endpoint = ["http://${reg_ip}:${reg_port}"]
nodes:
  - role: control-plane
  - role: worker
  - role: worker
EOF

kind export kubeconfig
sudo env PATH=${PATH} koko -d kind-worker,eth1 -d kind-worker2,eth1
sleep 1
kubectl -n kube-system wait --for=condition=available deploy/coredns --timeout=300s
kubectl create -f https://raw.githubusercontent.com/intel/multus-cni/master/images/multus-daemonset.yml
sleep 1
kubectl -n kube-system wait --for=condition=ready -l name=multus pod --timeout=300s
kubectl create -f cni-install.yml
sleep 1
kubectl -n kube-system wait --for=condition=ready -l name=cni-plugins pod --timeout=300s
