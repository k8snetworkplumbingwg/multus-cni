# Using Multus with Third Party Resources

Starting in Kubernetes 1.7, the use of Third Party Resources (TPR) was deprecated, and it is
no longer support in Kubernetes 1.8.  This document details how to use the Multus CNI application
with TPR objects.

## Creating TPR Network Objects

1.  Create a TPR, ```tprnetowrk.yaml``` for the network object:

```
apiVersion: extensions/v1beta1                                                              
kind: ThirdPartyResource                                                                    
metadata:                                                                                   
  name: network.kubernetes.com                                                              
description: "A specification of a Network obj in the kubernetes"                           
versions:                                                                                   
- name: v1 
```

2.  Create the resource

```
$ kubectl create -f ./tprnetwork.yaml
```

3. Run kubectl get command to check the Network TPR creation
```
$ kubectl get thirdpartyresource
NAME                     DESCRIPTION                                          VERSION(S)
network.kubernetes.com   A specification of a Network obj in the kubernetes   v1
```

## Create customer network objects in kubernetes

Network objects can be created after the TPR object is created.  A network object is defined
by a YAML/JSON file, indicatting plugin and network configuration details.  The particular
fields that are required depend on the TPR that was created.

In the following example, plugin and args fields are set to the object of kind ```Network```.
The ```Network``` type  Network is derived from the metadata.name of the TPR  object we created
above.

2. Create a flannel network object configuration to flannel-network.yaml

```
apiVersion: "kubernetes.com/v1"
kind: Network
metadata:
  name: flannel-conf
plugin: flannel
args: '[
        {
                "delegate": {
                        "isDefaultGateway": true
                }
        }
]'
```

2. Create the Flannel TPR network object

```
$ kubectl create -f ./flannel-network.yaml 
network "flannel-conf" created
```

3. Manage the Network objects using kubectl.

```
$ kubectl get network
NAME                         KIND
flannel-conf                 Network.v1.kubernetes.com
```

4. The plugin field should be the name of the CNI plugin binary  and the arguments should
have the flannel arguments.  The user is free to create additional network objects using
any other CNI plugin, such as Calico, Weave, Romana, etc.

5. Create a SRIOV network object

Save the following to sriov-network.yaml. 

```
apiVersion: "kubernetes.com/v1"
kind: Network
metadata:
  name: sriov-conf
plugin: sriov
args: '[
       {
                "if0": "enp12s0f1",
                "ipam": {
                        "type": "host-local",
                        "subnet": "10.56.217.0/24",
                        "rangeStart": "10.56.217.171",
                        "rangeEnd": "10.56.217.181",
                        "routes": [
                                { "dst": "0.0.0.0/0" }
                        ],
                        "gateway": "10.56.217.1"
                }
        }
]'
```
For more details on SRIOV, see [Intel SRIOV CNI](https://github.com/Intel-Corp/sriov-cni).

6. Save the below following YAML to sriov-vlanid-l2enable-network.yaml

```
apiVersion: "kubernetes.com/v1"
kind: Network
metadata:
  name: sriov-vlanid-l2enable-conf
plugin: sriov
args: '[
       {
                "if0": "enp2s0",
                "vlan": 210,
                "if0name": "north",
                "l2enable": true
        }
]'
```

7. Create the new network objects:

```
$ kubectl create -f sriov-vlanid-l2enable-network.yaml
$ kubectl create -f sriov-network.yaml
```

8. View the network objects using kubectl

```
$ kubectl get network
NAME                         KIND
flannel-conf                 Network.v1.kubernetes.com
sriov-vlanid-l2enable-conf   Network.v1.kubernetes.com
sriov-conf                   Network.v1.kubernetes.com
```

## Configuring Pod to use the TPR Network objects

1. Save the below following YAML to pod-multi-network.yaml. In this case flannel-conf network
  object act as the primary network. 

```
$ cat pod-multi-network.yaml 
apiVersion: v1
kind: Pod
metadata:
  name: multus-multi-net-poc
  annotations:
    networks: '[  
        { "name": "flannel-conf" },
        { "name": "sriov-conf"},
        { "name": "sriov-vlanid-l2enable-conf" } 
    ]'
spec:  # specification of the pod's contents
  containers:
  - name: multus-multi-net-poc
    image: "busybox"
    command: ["top"]
    stdin: true
    tty: true
```

2.  Create Multiple network based pod from the master node

```
$ kubectl create -f ./pod-multi-network.yaml
pod "multus-multi-net-poc" created
```

3.  Get the details of the running pod from the master

```
$ kubectl get pods
NAME                   READY     STATUS    RESTARTS   AGE
multus-multi-net-poc   1/1       Running   0          30s
```

## Verifying Pod network

1.  Run “ifconfig” command inside the container:
```
$ kubectl exec -it multus-multi-net-poc -- ifconfig
eth0      Link encap:Ethernet  HWaddr 06:21:91:2D:74:B9  
          inet addr:192.168.42.3  Bcast:0.0.0.0  Mask:255.255.255.0
          inet6 addr: fe80::421:91ff:fe2d:74b9/64 Scope:Link
          UP BROADCAST RUNNING MULTICAST  MTU:1450  Metric:1
          RX packets:0 errors:0 dropped:0 overruns:0 frame:0
          TX packets:8 errors:0 dropped:0 overruns:0 carrier:0
          collisions:0 txqueuelen:0 
          RX bytes:0 (0.0 B)  TX bytes:648 (648.0 B)

lo        Link encap:Local Loopback  
          inet addr:127.0.0.1  Mask:255.0.0.0
          inet6 addr: ::1/128 Scope:Host
          UP LOOPBACK RUNNING  MTU:65536  Metric:1
          RX packets:0 errors:0 dropped:0 overruns:0 frame:0
          TX packets:0 errors:0 dropped:0 overruns:0 carrier:0
          collisions:0 txqueuelen:1 
          RX bytes:0 (0.0 B)  TX bytes:0 (0.0 B)

net0      Link encap:Ethernet  HWaddr D2:94:98:82:00:00  
          inet addr:10.56.217.171  Bcast:0.0.0.0  Mask:255.255.255.0
          inet6 addr: fe80::d094:98ff:fe82:0/64 Scope:Link
          UP BROADCAST RUNNING MULTICAST  MTU:1500  Metric:1
          RX packets:2 errors:0 dropped:0 overruns:0 frame:0
          TX packets:8 errors:0 dropped:0 overruns:0 carrier:0
          collisions:0 txqueuelen:1000 
          RX bytes:120 (120.0 B)  TX bytes:648 (648.0 B)

north     Link encap:Ethernet  HWaddr BE:F2:48:42:83:12  
          inet6 addr: fe80::bcf2:48ff:fe42:8312/64 Scope:Link
          UP BROADCAST RUNNING MULTICAST  MTU:1500  Metric:1
          RX packets:1420 errors:0 dropped:0 overruns:0 frame:0
          TX packets:1276 errors:0 dropped:0 overruns:0 carrier:0
          collisions:0 txqueuelen:1000 
          RX bytes:95956 (93.7 KiB)  TX bytes:82200 (80.2 KiB)
```

Interface name | Description
------------ | -------------
lo | loopback
eth0@if41 | Flannel network tap interface
net0 | VF0 of NIC 1 assigned to the container by [Intel - SR-IOV CNI](https://github.com/Intel-Corp/sriov-cni) plugin
north | 	VF0 of NIC 2 assigned with VLAN ID 210 to the container by SR-IOV CNI plugin

2.  Check the vlan ID of the NIC 2 VFs

```
$ ip link show enp2s0
20: enp2s0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc mq state UP mode DEFAULT group default qlen 1000
    link/ether 24:8a:07:e8:7d:40 brd ff:ff:ff:ff:ff:ff
    vf 0 MAC 00:00:00:00:00:00, vlan 210, spoof checking off, link-state auto
    vf 1 MAC 00:00:00:00:00:00, vlan 4095, spoof checking off, link-state auto
    vf 2 MAC 00:00:00:00:00:00, vlan 4095, spoof checking off, link-state auto
    vf 3 MAC 00:00:00:00:00:00, vlan 4095, spoof checking off, link-state auto
```


