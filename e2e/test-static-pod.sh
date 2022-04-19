#!/usr/bin/env bash
set -o errexit

echo "Creating network attachment definition"
kubectl create -f static-pod-nad.yml

echo "Creating static pod config file"
docker cp simple-static-pod.yml kind-worker:/etc/kubernetes/manifests/static-web.yaml

echo "Waiting for static pod to start"
kubectl wait --for=condition=Ready --namespace=default pod/static-web-kind-worker

echo "Checking the pod annotation for net1 interface"
kubectl exec static-web-kind-worker --namespace=default -- ip a show dev net1

echo "Deleting static pod"
docker exec kind-worker /bin/bash -c "rm /etc/kubernetes/manifests/static-web.yaml"

echo "Deleting network attachment definition"
kubectl delete -f static-pod-nad.yml

echo "Test complete"
