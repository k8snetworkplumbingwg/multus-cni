![multus-cni Logo](https://github.com/Intel-Corp/multus-cni/blob/master/doc/images/Multus.png)

Table of Contents
=================

   * [MULTUS CNI plugin](#multus-cni-plugin)
      * [Multus additional plugins](#multus-additional-plugins)
      * [NFV based Networking in Kubernetes](#nfv-based-networking-in-kubernetes)
      * [Multi-Homed pod](#multi-homed-pod)
      * [Build](#build)
      * [Work flow](#work-flow)
      * [Usage with Kubernetes CRD/TPR based Network Objects](#usage-with-kubernetes-crdtpr-based-network-objects)
         * [Creating “Network” third party resource in kubernetes](#creating-network-third-party-resource-in-kubernetes)
            * [CRD based Network objects](#crd-based-network-objects)
            * [TPR based Network objects](#tpr-based-network-objects)
               * [Creating “Custom Network objects” third party resource in kubernetes](#creating-custom-network-objects-third-party-resource-in-kubernetes)
         * [Configuring Multus to use the kubeconfig](#configuring-multus-to-use-the-kubeconfig)
         * [Configuring Multus to use the kubeconfig and also default networks](#configuring-multus-to-use-the-kubeconfig-and-also-default-networks)
         * [Configuring Pod to use the TPR Network objects](#configuring-pod-to-use-the-tpr-network-objects)
         * [Verifying Pod network](#verifying-pod-network)
      * [Using Multus Conf file](#using-multus-conf-file)
      * [Testing the Multus CNI](#testing-the-multus-cni)
         * [Multiple Flannel Network](#multiple-flannel-network)
         * [docker](#docker)
         * [Kubernetes](#kubernetes)
            * [Launching workloads in Kubernetes](#launching-workloads-in-kubernetes)
      * [Contacts](#contacts)

Created by [gh-md-toc](https://github.com/ekalinin/github-markdown-toc)

### For more info on Multus - Sign In for KubeCon 2017 Salon: [Enabling NFV Features in Kubernetes. ](https://kccncna17.sched.com/event/Cvnw/enabling-nfv-features-in-kubernetes-hosted-by-kuralamudhan-ramakrishnan-ivan-coughlan-intel)

# MULTUS CNI plugin

- *Multus* is the latin word for “Multi”

- As the name suggests, it acts as the Multi plugin in Kubernetes and provides the Multi interface support in a pod

- Multus supports all [reference plugins](https://github.com/containernetworking/plugins) (eg. [Flannel](https://github.com/containernetworking/plugins/tree/master/plugins/meta/flannel), [DHCP](https://github.com/containernetworking/plugins/tree/master/plugins/ipam/dhcp), [Macvlan](https://github.com/containernetworking/plugins/tree/master/plugins/main/macvlan)) that implement the CNI specification and all 3rd party plugins (eg. [Calico](https://github.com/projectcalico/cni-plugin), [Weave](https://github.com/weaveworks/weave), [Cilium](https://github.com/cilium/cilium), [Contiv](https://github.com/contiv/netplugin)). In addition to it, Multus supports [SRIOV](https://github.com/hustcat/sriov-cni), [DPDK](https://github.com/Intel-Corp/sriov-cni), [OVS-DPDK & VPP](https://github.com/intel/vhost-user-net-plugin) workloads in Kubernetes with both cloud native and NFV based applications in Kubernetes. 

- It is a contact between the container runtime and other plugins, and it doesn't have any of its own net configuration, it calls other plugins like flannel/calico to do the real net conf job. 

- Multus reuses the concept of invoking the delegates in flannel, it groups the multi plugins into delegates and invoke each other in sequential order, according to the JSON scheme in the cni configuration.

- No. of plugins supported is dependent upon the number of delegates in the conf file.

- Master plugin invokes "eth0" interface in the pod, rest of plugins(Mininon plugins eg: sriov,ipam) invoke interfaces as "net0", "net1".. "netn"

- The "masterplugin" is the only net conf option of multus cni, it identifies the primary network. The default route will point to the primary network 

Please read [CNI](https://github.com/containernetworking/cni) for more information on container networking.

## Multus additional plugins
- [DPDK -SR-IOV CNI](https://github.com/Intel-Corp/sriov-cni)
- [Vhostuser CNI - a Dataplane network plugin - Supports OVS-DPDK & VPP](https://github.com/intel/vhost-user-net-plugin)

## NFV based Networking in Kubernetes
- Feature Brief -[Multiple Network Interface Support in Kubernetes](https://builders.intel.com/docs/networkbuilders/multiple-network-interfaces-support-in-kubernetes-feature-brief.pdf)
- White Paper - [Enabling New Features with Kubernetes for NFV](https://builders.intel.com/docs/networkbuilders/enabling_new_features_in_kubernetes_for_NFV.pdf)

## Multi-Homed pod
<p align="center">
   <img src="doc/images/multus_cni_pod.png" width="1008" />
</p>

## Work flow
<p align="center">
   <img src="doc/images/workflow.png" width="1008" />
</p>


# Getting Started

## Requirements to build

 * [go 1.8](https://golang.org)

## Setup the environment

 1. Define GOPATH

   ```bash
   $ export GOPATH=$HOME/go
   ```

 2. Create GOPATH directory

   ```bash
    $ mkdir -p $GOPATH
   ```

 3. Get the code

   ```bash
   $ go get -d github.com/intel-corp/multus-cni
   ```

## Build and install plugin

   ```bash
    $ cd $GOPATH/src/github.com/intel-corp/multus-cni
    $ ./build
    $ sudo cp ./bin/multus /opt/cni/bin/
   ```
# Multus Usage Models

Multus can be configured using a CNI configuration file as well as by making use of custom 
resource definitions in Kubernetes (CRD).  These are not mutually exclusive - the user can setup a default
configuration via CNI configuration file while also creating network objects which pods can make use of.
We'll introduce a simple CNI configuration file and custom resource usage in the next couple of sections.

## Using Multus with a CNI configuration file

When making use of just a CNI configuration file, each pod created in the cluster will  include
a network interface per network delegate described. A minimal example is provided as follows:

```
{
    "type": "multus",
    "log_level": "debug",
    "kubeconfig": "/etc/kubernetes/admin.conf",
    "delegates": [{
        "name": "flannel",
        "type": "flannel",
        "masterplugin": true
    } ]
}
```

In this simple case, we're just using the Multus CNI to add a single interface using the CNI flannel plugin.  Available fields
include: 
* `name` (string, required): the name of the network
* `type` (string, required): "multus"
* `kubeconfig` (string, optional): kubeconfig file for the out of cluster communication with kube-apiserver, Refer the doc
* `delegates` (([]map,required): number of delegate details in the Multus, ignored in case kubeconfig is added.
* `masterplugin` (bool,required): master plugin to report back the IP address and DNS to the container

Additional interfaces using potentially differnet plugins are created by adding more delegates to the Multus CNI configuration. 

A ```masterplugin``` needs to be defined.  In the first example, you see the ```"masterplugin": true``` provided.  In the case that multiple delegates are created, one of these must be marked as the ```masterplugin.``` This annotation declares that this plugin will serve as the primary interface, providing the pod's IP address and DNS information back to Kubernetes.  No other interfaces will be visible to Kubernetes.  A  simple multi-homed network configuration is provided below:

```
{
        "type": "multus",
        "log_level": "debug",
        "kubeconfig": "/etc/kubernetes/admin.conf",
        "delegates":
        [
        {
                "name": "flannel",
                "type": "flannel",
                "masterplugin": true
        },
        {
                "type": "ptp",
                "ipam": {
                        "type": "host-local",
                        "subnet": "10.0.0.0/24",
                        "routes": [
                                { "dst": "0.0.0.0/0" }
                        ],
                        "gateway": "10.0.0.1"
                }
        },
        {
                "type": "bridge",
                "ipam": {
                        "type": "host-local",
                        "subnet": "11.0.0.0/24",
                        "rangeStart": "11.0.0.10",
                        "rangeEnd": "11.0.0.20",
                        "routes": [
                                { "dst": "0.0.0.0/0" }
                        ],
                        "gateway": "11.0.0.1"
                }
        }
        ]
}
 ```

Once you have a kubernertes cluster up making use of this configuration, if you deploy a pod you should see the relevant 
network interfaces created.

### Try it out

Bring up the Kubernetes cluster, using our sample three delegate Multus configuration:
```
$ sudo curl  https://raw.githubusercontent.com/egernst/multus-cni/readme-updates/doc/sample/01-multus-cni_fl_ptp_br.conf -o /etc/cni/net.d/01-multus-cni.conf
$ sudo -E kubeadm init --pod-network-cidr 10.244.0.0/16
$ export KUBECONFIG=/etc/kubernetes/admin.conf
```
Taint the master so we can schedule a pod on it:
```
$ master=$(hostname)
$ sudo -E kubectl taint nodes "$master" node-role.kubernetes.io/master:NoSchedule-
```
Start a simple pod and observe the network interfaces provided:
```
$ curl https://raw.githubusercontent.com/egernst/multus-cni/readme-updates/doc/sample/ubuntu-pod.yaml -o ubuntu-pod.yaml
$ sudo -E kubectl create -f ubuntu-pod.yaml
```

You should see the following output, corresponding to flannel, PTP and bridge network plugins:

```
$ sudo -E kubectl exec -it ubuntu-pod -- bash -c "ip a" 
1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UNKNOWN group default qlen 1
    link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00
    inet 127.0.0.1/8 scope host lo
       valid_lft forever preferred_lft forever
    inet6 ::1/128 scope host 
       valid_lft forever preferred_lft forever
3: eth0@if243: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1450 qdisc noqueue state UP group default 
    link/ether 0a:58:0a:f4:00:5e brd ff:ff:ff:ff:ff:ff
    inet 10.244.0.94/24 scope global eth0
       valid_lft forever preferred_lft forever
    inet6 fe80::e063:c3ff:fe3d:69ea/64 scope link 
       valid_lft forever preferred_lft forever
5: net0@if244: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc noqueue state UP group default 
    link/ether 0a:58:0a:00:00:05 brd ff:ff:ff:ff:ff:ff
    inet 10.0.0.5/24 scope global net0
       valid_lft forever preferred_lft forever
    inet6 fe80::d023:b1ff:fe86:4b87/64 scope link 
       valid_lft forever preferred_lft forever
7: net1@if245: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc noqueue state UP group default 
    link/ether 0a:58:0b:00:00:0d brd ff:ff:ff:ff:ff:ff
    inet 11.0.0.13/24 scope global net1
       valid_lft forever preferred_lft forever
    inet6 fe80::c0cb:4cff:feea:9df4/64 scope link 
       valid_lft forever preferred_lft forever
 ```

### An example using the SRIOV CNI plugin

Another example of using a network configuration file can be found [here](https://github.com/egernst/multus-cni/blob/readme-updates/doc/sriov-conf-example.md)


Having a CNI configuration file for Multus helps provide a default option for a cluster's networking.
In the next section, we'll introduce CRD, an alternative option for creating and assigning networks to pods.

## Using Multus with Kubernete's CRD

[CRDs](https://kubernetes.io/docs/concepts/api-extension/custom-resources) are an extension of the Kubernetes 
API, providing enhanced features which are custom to the particular Kubernetes installation.                                                   
                                                                                          
For Multus, a networking CRD can be created which contains configuration details and plugin details
for a given network.  Making use of this, multiple networks could be created with varying CNI
plugins.  Pods can then be created to make use of one or multiple of these network CRDs.  

Multus is also compatible with Third Party Resources (TPR), which became deprecated in Kubernetets
1.7 and removed in version 1.8.  Details on TPR support can be found on the [Multus TPR readme](https://github.com/egernst/multus-cni/blob/readme-updates/doc/tpr-overview.md).
                                                                                        
When Multus is invoked by Kubelet, it recovers custom pod annotations and uses these to obtain
the relevant CRD.  This CRD tells Multus which CNI plugin to invoke and what networking configuration
should be used.             
                                                                                          
In the event that a multi-homed pod is created, the order of CNI plugin invocation is determined
by the order the network annotation is provided in the pod description. Notably, the first
network in the pod configuration will act as the ```masterplugin``` for the pod.  It is this interface
which will serve as the primary interface, providing the pod's IP address and DNS information back to 
Kubernetes.  No other interfaces will be visible to Kubernetes. 
                                                             
For more details, please refer to the [Kubernetes Network SIG Multiple Network Proposal](https://docs.google.com/document/d/1TW3P4c8auWwYy-w_5afIPDcGNLK3LZf0m14943eVfVg/edit)

<p align="center">
   <img src="doc/images/multus_crd_usage_diagram.JPG" width="1008" />
</p>    
    
### Simple example of Multus with Kubernetes CRD based Network Objects

#### Create CRD based Network objects

1. Create a CRD resource “crdnetwork.yaml” for the network object as shown below

```
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  # name must match the spec fields below, and be in the form: <plural>.<group>
  name: networks.kubernetes.com
spec:
  # group name to use for REST API: /apis/<group>/<version>
  group: kubernetes.com
  # version name to use for REST API: /apis/<group>/<version>
  version: v1
  # either Namespaced or Cluster
  scope: Namespaced
  names:
    # plural name to be used in the URL: /apis/<group>/<version>/<plural>
    plural: networks
    # singular name to be used as an alias on the CLI and for display
    singular: network
    # kind is normally the CamelCased singular type. Your resource manifests use this.
    kind: Network
    # shortNames allow shorter string to match your resource on the CLI
    shortNames:
    - net
```

2. Create the network CRD

```
# kubectl create -f ./crdnetwork.yaml
customresourcedefinition "network.kubernetes.com" created
```

3. Verify the network CRD's creation

```
# kubectl get CustomResourceDefinition
NAME                      KIND
networks.kubernetes.com   CustomResourceDefinition.v1beta1.apiextensions.k8s.io
```

4. Create a description of the network objects to make

Save the following to example-networks.yaml to create network definitions.  Note, you can store
these in their own files as well.  For convenience we placed a few arbitrary bridge and PTP
networks here.

```
---- 
apiVersion: "kubernetes.com/v1" 
kind: Network 
metadata: 
  name: ptp-net 
plugin: ptp 
args: '[ 
    { 
        "name": "ptp-net", 
        "type": "ptp", 
        "ipam": { 
                  "type": "host-local", 
                  "subnet": "10.248.246.144/28", 
                  "routes": [ { "dst": "0.0.0.0/0" } ] 
        } 
    } 
]' 
 
--- 
apiVersion: "kubernetes.com/v1" 
kind: Network 
metadata: 
  name: br-net-1 
plugin: bridge 
args: '[ 
    { 
        "name": "br-net-1", 
        "type": "bridge", 
        "bridge": "br-net-1", 
        "ipam": { 
            "type": "host-local", 
            "subnet": "10.1.10.0/24" 
        } 
    } 
]' 
 
--- 
apiVersion: "kubernetes.com/v1" 
kind: Network 
metadata: 
  name: br-net-2 
plugin: bridge 
args: '[ 
    { 
        "name": "br-net-2", 
        "type": "bridge", 
        "bridge": "br-net-2", 
        "ipam": { 
            "type": "host-local", 
            "subnet": "11.1.1.0/24" 
        } 
    } 
]'

```

5. Create the custom resource defined networks

```
$ kubectl create -f example-networks.yaml

network "ptp-net" created
network "br-net-1" created
network "br-net-2" created
```

#### Start a pod to make use of the CRD based networks

1. Create the multus enabled pod configuration

An example pod configuration, example-pod.yaml, is given below.  Note that you needn't
use every network-object that was created, and that you may also create multiple interfaces
from a single network object.
```
---
apiVersion: v1
kind: Pod
metadata:
  name: crd-network-test
  annotations:
    networks: '[
        { "name": "br-net-1" },
        { "name": "br-net-2" },
        { "name": "br-net-2" },
        { "name": "ptp-net" }
    ]'
spec:
  containers:
  - name: ubuntu
    image: "ubuntu:14.04"
    stdin: true
    tty: true
```

2. Launch the pod

```
$ kubectl create -f example-pod.yaml
```

3. Verify the newly created interfaces

We expect to see a primary 10.1.10.x, two 11.1.1.x and a 10.248.246.x based network interface
in the pod.  Use kubectl exec as follows to verify:

```
$ sudo -E kubectl exec -it crd-network-test -- bash -c " ip a"
1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UNKNOWN group default qlen 1
    link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00
    inet 127.0.0.1/8 scope host lo
       valid_lft forever preferred_lft forever
    inet6 ::1/128 scope host 
       valid_lft forever preferred_lft forever
3: eth0@if255: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc noqueue state UP group default 
    link/ether 0a:58:0a:01:0a:19 brd ff:ff:ff:ff:ff:ff
    inet 10.1.10.25/24 scope global eth0
       valid_lft forever preferred_lft forever
    inet6 fe80::f0ce:a3ff:feaf:1cd1/64 scope link 
       valid_lft forever preferred_lft forever
5: net0@if256: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc noqueue state UP group default 
    link/ether 0a:58:0b:01:01:25 brd ff:ff:ff:ff:ff:ff
    inet 11.1.1.37/24 scope global net0
       valid_lft forever preferred_lft forever
    inet6 fe80::68ea:c3ff:fe95:f114/64 scope link 
       valid_lft forever preferred_lft forever
7: net1@if257: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc noqueue state UP group default 
    link/ether 0a:58:0b:01:01:26 brd ff:ff:ff:ff:ff:ff
    inet 11.1.1.38/24 scope global net1
       valid_lft forever preferred_lft forever
    inet6 fe80::641a:50ff:feb0:5ada/64 scope link 
       valid_lft forever preferred_lft forever
9: net2@if258: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc noqueue state UP group default 
    link/ether 0a:58:0a:f8:f6:98 brd ff:ff:ff:ff:ff:ff
    inet 10.248.246.152/28 scope global net2
       valid_lft forever preferred_lft forever
    inet6 fe80::b417:57ff:fe52:c091/64 scope link 
       valid_lft forever preferred_lft forever
```


### Configuring Multus to use the kubeconfig

1.	Create Multus CNI configuration file /etc/cni/net.d/multus-cni.conf with below content in minions. Use only the absolute path to point to the kubeconfig file (it may change depending upon your cluster env) and make sure all CNI binary files are in `\opt\cni\bin` dir
```
{
    "name": "minion-cni-network",
    "type": "multus",
    "kubeconfig": "/etc/kubernetes/node-kubeconfig.yaml"
}
```
2.	Restart kubelet service
```
# systemctl restart kubelet
```
### Configuring Multus to use the kubeconfig and also default networks
1.	Many user want default networking feature along with Network object. Refer [#14](https://github.com/Intel-Corp/multus-cni/issues/14) & [#17](https://github.com/Intel-Corp/multus-cni/issues/17) issues for more information. In this following config, Weave act as the default network in the absence of network field in the pod metadata annotation.
```
{
    "name": "minion-cni-network",
    "type": "multus",
    "kubeconfig": "/etc/kubernetes/node-kubeconfig.yaml",
    "delegates": [{
        "type": "weave-net",
        "hairpinMode": true,
        "masterplugin": true
    }]
}
```
2.	Restart kubelet service
```
# systemctl restart kubelet
```

## Testing Multus CNI ##

### Multiple Flannel Network

Github user [YYGCui](https://github.com/YYGCui) has used Multiple flannel network to work
with Multus CNI plugin. Please refer this [closed issue](https://github.com/Intel-Corp/multus-cni/issues/7)
for details on multiple overlay network support with Multus CNI.

### docker

Make sure that the multus, [sriov](https://github.com/Intel-Corp/sriov-cni), [flannel](https://github.com/containernetworking/cni/blob/master/Documentation/flannel.md),
and [ptp](https://github.com/containernetworking/cni/blob/master/Documentation/ptp.md) binaries
are in the ```/opt/cni/bin``` directories and follow the steps as mention in the
[CNI](https://github.com/containernetworking/cni/#running-a-docker-container-with-network-namespace-set-up-by-cni-plugins)

### Kubernetes

Refer to the Kubernetes user guides and network plugins:
* [Single Node](https://kubernetes.io/docs/getting-started-guides/fedora/fedora_manual_config/)
* [Multi Node](https://kubernetes.io/docs/getting-started-guides/fedora/flannel_multi_node_cluster/)
* [Network Plugin](https://kubernetes.io/docs/admin/network-plugins/)

Kubelet must be configured to run with the CNI `--network-plugin`, with the following configuration
information.  Edit `/etc/default/kubelet` file and add `KUBELET_OPTS`:
```
KUBELET_OPTS="...
--network-plugin-dir=/etc/cni/net.d
--network-plugin=cni
"
```
Restart the kubelet
```
# systemctl restart kubelet.service
```

## Contacts
For any questions about Multus CNI, please reach out on github issue or feel free to contact
the developer @kural in our [Intel-Corp Slack](https://intel-corp.herokuapp.com/)

