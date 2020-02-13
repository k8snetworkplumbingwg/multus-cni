#!/bin/sh
set -o errexit

export PATH=${PATH}:./bin

kind export kubeconfig
sudo koko -d kind-worker,eth1 -d kind-worker2,eth1
sleep 1
kubectl -n kube-system wait --for=condition=available deploy/coredns --timeout=300s
kubectl create -f https://raw.githubusercontent.com/intel/multus-cni/master/images/multus-daemonset.yml
sleep 1
kubectl -n kube-system wait --for=condition=ready -l name=multus pod --timeout=300s
kubectl create -f cni-install.yml
sleep 1
kubectl -n kube-system wait --for=condition=ready -l name=cni-plugins pod --timeout=300s
