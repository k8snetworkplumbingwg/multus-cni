#!/bin/sh
set -o errexit

export PATH=${PATH}:./bin

kubectl create -f yamls/simple-pods.yml
kubectl wait --for=condition=ready -l app=simple --timeout=300s pod

echo "check eventual connectivity of simple-worker Pod to the Kubernetes API server"
for i in `seq 1 10`;
do
    if [ $(kubectl exec simple-worker -- nc -zvw1 kubernetes 443 >/dev/null && echo $? || echo $?) -eq 0 ]; then
        echo "simple-worker reached the Kubernetes API server"
        break
    fi

    if [ $i -eq 10 ]; then
        echo "simple-worker couldn't connect to the Kubernetes API server"
        exit 1
    fi

    sleep 1
done

echo "check eventual connectivity of simple-control-plane Pod to the Kubernetes API server"
for i in `seq 1 10`;
do
    if [ $(kubectl exec simple-control-plane -- nc -zvw1 kubernetes 443 >/dev/null && echo $? || echo $?) -eq 0 ]; then
        echo "simple-control-plane reached the Kubernetes API server"
        break
    fi

    if [ $i -eq 10 ]; then
        echo "simple-control-plane couldn't connect to the Kubernetes API server"
        exit 1
    fi

    sleep 1
done

echo "cleanup resources"
kubectl delete -f yamls/simple-pods.yml
