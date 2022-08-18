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

kind_network='kind'
reg_name='kind-registry'
reg_port='5000'
running="$($OCI_BIN inspect -f '{{.State.Running}}' "${reg_name}" 2>/dev/null || true)"
if [ "${running}" != 'true' ]; then
  # run registry and push the multus image
  $OCI_BIN run -d --restart=always -p "${reg_port}:5000" --name "${reg_name}" registry:2
  $OCI_BIN build -t localhost:5000/multus:e2e -f ../images/Dockerfile ..
  $OCI_BIN push localhost:5000/multus:e2e
fi
reg_host="${reg_name}"
if [ "${kind_network}" = "bridge" ]; then
    reg_host="$($OCI_BIN inspect -f '{{.NetworkSettings.IPAddress}}' "${reg_name}")"
fi
echo "Registry Host: ${reg_host}"

# deploy cluster with kind
cat <<EOF | kind create cluster --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors."localhost:${reg_port}"]
    endpoint = ["http://${reg_host}:${reg_port}"]
nodes:
  - role: control-plane
  - role: worker
    kubeadmConfigPatches:
    - |
      kind: InitConfiguration
      nodeRegistration:
        kubeletExtraArgs:
          pod-manifest-path: "/etc/kubernetes/manifests/"
  - role: worker
EOF

cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: local-registry-hosting
  namespace: kube-public
data:
  localRegistryHosting.v1: |
    host: "localhost:${reg_port}"
    help: "https://kind.sigs.k8s.io/docs/user/local-registry/"
EOF

containers=$($OCI_BIN network inspect ${kind_network} -f "{{range .Containers}}{{.Name}} {{end}}")
needs_connect="true"
for c in $containers; do
  if [ "$c" = "${reg_name}" ]; then
    needs_connect="false"
  fi
done
if [ "${needs_connect}" = "true" ]; then
  $OCI_BIN network connect "${kind_network}" "${reg_name}" || true
fi

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
