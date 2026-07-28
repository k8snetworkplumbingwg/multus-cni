#!/bin/sh
set -o errexit

export PATH=${PATH}:./bin

kubectl create -f yamls/many-pods.yml
kubectl wait --for=condition=ready -l app=many --timeout=300s pod

echo "cleanup resources"
kubectl delete -f yamls/many-pods.yml
