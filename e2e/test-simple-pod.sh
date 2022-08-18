#!/bin/sh
set -o errexit

export PATH=${PATH}:./bin

kubectl create -f yamls/simple-pod.yml
kubectl wait --for=condition=ready -l app=simple --timeout=300s pod

echo "cleanup resources"
kubectl delete -f yamls/simple-pod.yml
