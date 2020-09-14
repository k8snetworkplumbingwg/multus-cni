#!/bin/sh
set -o errexit

export PATH=${PATH}:./bin

kubectl create -f simple-macvlan1.yml
kubectl wait --for=condition=ready -l app=macvlan --timeout=300s pod

echo "check macvlan1-worker1 interface: net1"
kubectl exec macvlan1-worker1 -- ip a show dev net1

echo "check macvlan1-worker1 interface address: net1"
ipaddr=$(kubectl exec macvlan1-worker1 -- ip -j a show  | jq -r \
	'.[]|select(.ifname =="net1")|.addr_info[]|select(.family=="inet").local')
if [ $ipaddr != "10.1.1.11" ]; then
	echo "macvlan1-worker1 IP address is different: ${ipaddr}"
fi

echo "check macvlan1-worker2 interface: net1"
kubectl exec macvlan1-worker2 -- ip a show dev net1

echo "check macvlan1-worker2 interface address: net1"
ipaddr=$(kubectl exec macvlan1-worker2 -- ip -j a show  | jq -r \
	'.[]|select(.ifname =="net1")|.addr_info[]|select(.family=="inet").local')
if [ $ipaddr != "10.1.1.12" ]; then
	echo "macvlan1-worker2 IP address is different: ${ipaddr}"
fi

echo "cleanup resources"
kubectl delete -f simple-macvlan1.yml
