#!/bin/sh
set -o errexit

export PATH=${PATH}:./bin

kubectl create -f default-route1.yml
kubectl wait --for=condition=ready -l app=default-route1 --timeout=300s pod

echo "check default-route-worker1 interface: net1"
kubectl exec default-route-worker1 -- ip a show dev net1

echo "check default-route-worker1 interface address: net1"
ipaddr=$(kubectl exec default-route-worker1 -- ip -j a show  | jq -r \
	'.[]|select(.ifname =="net1")|.addr_info[]|select(.family=="inet").local')
if [ $ipaddr != "10.1.1.21" ]; then
	echo "default-route-worker1 IP address is different: ${ipaddr}"
fi

echo "check default-route-worker1 default route"
ipaddr=$(kubectl exec default-route-worker1 -- ip -j route | jq -r \
	'.[]|select(.dst=="default")|.gateway')
if [ $ipaddr != "10.1.1.254" ]; then
	echo "default-route-worker1 default route is different: ${ipaddr}"
fi

echo "check default-route-worker2 interface: net1"
kubectl exec default-route-worker2 -- ip a show dev net1

echo "check default-route-worker2 interface address: net1"
ipaddr=$(kubectl exec default-route-worker2 -- ip -j a show  | jq -r \
	'.[]|select(.ifname =="net1")|.addr_info[]|select(.family=="inet").local')
if [ $ipaddr != "10.1.1.22" ]; then
	echo "default-route-worker2 IP address is different: ${ipaddr}"
fi

echo "check default-route-worker2 default route"
ipaddr=$(kubectl exec default-route-worker2 -- ip -j route | jq -r \
	'.[]|select(.dst=="default")|.gateway')
if [ $ipaddr != "10.244.1.1" ]; then
	echo "default-route-worker2 default route is different: ${ipaddr}"
fi

echo "cleanup resources"
kubectl delete -f default-route1.yml
