#!/bin/sh
set -o errexit

export PATH=${PATH}:./bin

kubectl create -f yamls/simple-macvlan1.yml
kubectl wait --for=condition=ready -l app=macvlan --timeout=300s pod

echo "check macvlan1-worker1 interface: net1"
net=$(kubectl exec macvlan1-worker1 -- ip a show dev net1)
if [ $? -eq 0 ];then
	echo "macvlan1-worker1 pod has net1 card"
else
	echo "macvlan1-worker1 pod has no net1 card"
	exit 1
fi

echo "check macvlan1-worker1 interface address: net1"
ipaddr=$(kubectl exec macvlan1-worker1 -- ip -j a show  | jq -r \
	'.[]|select(.ifname =="net1")|.addr_info[]|select(.family=="inet").local')
if [ $ipaddr != "10.1.1.11" ]; then
	echo "macvlan1-worker1 IP address is different: ${ipaddr}"
fi

echo "check macvlan1-worker2 interface: net1"
net2=$(kubectl exec macvlan1-worker2 -- ip a show dev net1)
if [ $? -eq 0  ];then
	echo "macvlan1-worker2 pod has net1 card"
else
	echo "macvlan1-worker2 pod has no net1 card"
	exit 1
fi

echo "check macvlan1-worker2 interface address: net1"
ipaddr=$(kubectl exec macvlan1-worker2 -- ip -j a show  | jq -r \
	'.[]|select(.ifname =="net1")|.addr_info[]|select(.family=="inet").local')
if [ $ipaddr != "10.1.1.12" ]; then
	echo "macvlan1-worker2 IP address is different: ${ipaddr}"
fi

echo "check eventual connectivity of macvlan1-control-plane Pod to the Kubernetes API server"
for i in `seq 1 10`;
do
    if [ $(kubectl exec macvlan1-control-plane -- nc -zvw1 kubernetes 443 >/dev/null && echo $? || echo $?) -eq 0 ]; then
        echo "macvlan1-control-plane reached the Kubernetes API server"
        break
    fi

    if [ $i -eq 10 ]; then
        echo "macvlan1-control-plane couldn't connect to the Kubernetes API server"
        exit 1
    fi

    sleep 1
done

echo "cleanup resources"
kubectl delete -f yamls/simple-macvlan1.yml
