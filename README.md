![multus-cni Logo](https://github.com/Intel-Corp/multus-cni/blob/master/doc/images/Multus.png)

### Newest Multus Version v2.1(Kubernetes Network Plumbing working group- reference meta plugin) will be merged to master from dev branch(network-plumbing-working-group-crd-change). For more info - refer : [Multus v2.0 readme.md](https://github.com/intel/multus-cni/tree/dev/network-plumbing-working-group-crd-change#kubernetes-network-custom-resource-definition-de-facto-standard---reference-implementation)
   * [MULTUS CNI plugin](#multus-cni-plugin)
      * [Multi-Homed pod](#multi-homed-pod)
      * [Build](#build)
      * [Work flow](#work-flow)
      * [Usage with Kubernetes CRD/TPR based network objects](#usage-with-kubernetes-crdtpr-based-network-objects)
         * [Creating "Network" resources in Kubernetes](#creating-network-resources-in-kubernetes)
            * [<strong>CRD based Network objects</strong>](#crd-based-network-objects)
            * [TPR based Network objects(TPR deprecated in Kubernetes 1.7)](#tpr-based-network-objects)
         * [Creating network resources in Kubernetes](#creating-network-resources-in-kubernetes-1)
         * [Configuring Multus to use the kubeconfig](#configuring-multus-to-use-the-kubeconfig)
         * [Configuring Multus to use kubeconfig and a default network](#configuring-multus-to-use-kubeconfig-and-a-default-network)
         * [Configuring Pod to use the CRD/TPR network objects](#configuring-pod-to-use-the-crdtpr-network-objects)
         * [Verifying Pod network interfaces](#verifying-pod-network-interfaces)
      * [Using with Multus conf file](#using-with-multus-conf-file)
      * [Testing Multus CNI](#testing-multus-cni)
         * [Multiple flannel networks](#multiple-flannel-networks)
            * [Configure Kubernetes with CNI](#configure-kubernetes-with-cni)
            * [Launching workloads in Kubernetes](#launching-workloads-in-kubernetes)
      * [Multus additional plugins](#multus-additional-plugins)
      * [NFV based networking in Kubernetes](#nfv-based-networking-in-kubernetes)
      * [Need help](#need-help)
      * [Contacts](#contacts)

# MULTUS CNI plugin
- _Multus_ is a latin word for &quot;Multi&quot;
- As the name suggests, it acts as a Multi plugin in Kubernetes and provides the multiple network interface support in a pod
- Multus supports all [reference plugins](https://github.com/containernetworking/plugins) (eg. [Flannel](https://github.com/containernetworking/plugins/tree/master/plugins/meta/flannel), [DHCP](https://github.com/containernetworking/plugins/tree/master/plugins/ipam/dhcp), [Macvlan](https://github.com/containernetworking/plugins/tree/master/plugins/main/macvlan)) that implement the CNI specification and all 3rd party plugins (eg. [Calico](https://github.com/projectcalico/cni-plugin), [Weave](https://github.com/weaveworks/weave), [Cilium](https://github.com/cilium/cilium), [Contiv](https://github.com/contiv/netplugin)). In addition to it, Multus supports [SRIOV](https://github.com/hustcat/sriov-cni), [SRIOV-DPDK](https://github.com/Intel-Corp/sriov-cni), [OVS-DPDK &amp; VPP](https://github.com/intel/vhost-user-net-plugin) workloads in Kubernetes with both cloud native and NFV based applications in Kubernetes
- It is a contact between the container runtime and other plugins, and it doesn&#39;t have any of its own net configuration, it calls other plugins like flannel/calico to do the real net conf job.
- Multus reuses the concept of invoking delegates as used in flannel by grouping multiple plugins into delegates and invoking them in the sequential order of the CNI configuration file provided in json format
- Number of plugins supported is depends on the number of delegates in the configuration file.
- The "masterplugin" is the only net conf option of multus cni, it identifies the primary network. The default route will point to the primary network
- One of the plugin acts as a “Master” plugin and responsible for configuring k8s network with Pod interface “eth0”
- The “Master” plugin also responsible to set the default route for the Pod
- Any subsequent plugin gets Pod interface name as “net0”, “net1”,… “netX and so on
- Multus is one of the projects in the [Baremetal Container Experience kit](https://networkbuilders.intel.com/network-technologies/container-experience-kits).

Please check the [CNI](https://github.com/containernetworking/cni) documentation for more information on container networking.

## Multi-Homed pod
<p align="center">
   <img src="doc/images/multus_cni_pod.png" width="1008" />
</p>

## Build

**This plugin requires Go 1.8 to build.**

Go 1.5 users will need to set GO15VENDOREXPERIMENT=1 to get vendored dependencies. This flag is set by default in 1.6.
```
#./build
```
## Work flow
<p align="center">
   <img src="doc/images/workflow.png" width="1008" />
</p>
## Network configuration reference

- name (string, required): the name of the network
- type (string, required): &quot;multus&quot;
- kubeconfig (string, optional): kubeconfig file for the out of cluster communication with kube-apiserver. See the example [kubeconfig](https://github.com/Intel-Corp/multus-cni/blob/master/doc/node-kubeconfig.yaml)
- delegates (([]map,required): number of delegate details in the Multus, ignored in case kubeconfig is added.
- masterplugin (bool,required): master plugin to report back the IP address and DNS to the container

## Usage with Kubernetes CRD/TPR based network objects

Kubelet is responsible for establishing network interfaces for pods; it does this by invoking its configured CNI plugin. When Multus is invoked it retrieves network references from Pod annotation. Multus then uses these network references to get network configurations. Network configurations are defined as Kubernetes Custom Resource Object (CRD). These configurations describe which CNI plugins to invoke and what their configurations are. The order of plugin invocation is important as it identifies the primary plugin. This order is taken from network object references given in a Pod spec.

<p align="center">
   <img src="doc/images/multus_crd_usage_diagram.JPG" width="1008" />
</p>

### Creating &quot;Network&quot; resources in Kubernetes

Multus is compatible to work with both CRD/TPR. Both CRD/TPR based network object api self link is same.

##### **CRD based Network objects**

1. Create a Custom Resource Definition &quot;crdnetwork.yaml&quot; for the network object as shown below:

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
2. Run kubectl create command for the Custom Resource Definition
```
# kubectl create -f ./crdnetwork.yaml
customresourcedefinition "network.kubernetes.com" created
```
3. Run kubectl get command to check the Network CRD creation
```
# kubectl get CustomResourceDefinition
NAME                      KIND
networks.kubernetes.com   CustomResourceDefinition.v1beta1.apiextensions.k8s.io
```

4. Save the following YAML to flannel-network.yaml

```
apiVersion: "kubernetes.com/v1"
kind: Network
metadata:
  name: flannel-networkobj
plugin: flannel
args: '[
        {
                "delegate": {
                        "isDefaultGateway": true
                }
        }
]'
```

5. Create the custom resource definition

```
# kubectl create -f customCRD/flannel-network.yaml
network "flannel-networkobj" created
```
```
# kubectl get network
NAME                 KIND                        ARGS                                               PLUGIN
flannel-networkobj   Network.v1.kubernetes.com   [ { "delegate": { "isDefaultGateway": true } } ]   flannel
```


6. Get the custom network object details
```
# kubectl get network flannel-networkobj -o yaml
apiVersion: kubernetes.com/v1
args: '[ { "delegate": { "isDefaultGateway": true } } ]'
kind: Network
metadata:
  clusterName: ""
  creationTimestamp: 2017-07-11T21:46:52Z
  deletionGracePeriodSeconds: null
  deletionTimestamp: null
  name: flannel-networkobj
  namespace: default
  resourceVersion: "6848829"
  selfLink: /apis/kubernetes.com/v1/namespaces/default/networks/flannel-networkobj
  uid: 7311c965-6682-11e7-b0b9-408d5c537d27
plugin: flannel
```

For Kubernetes v1.7 and above use CRD to create network object. For version older than 1.7 use TPR based objects as shown below: 

Note: Both TPR and CRD will have same selfLink : 

*/apis/kubernetes.com/v1/namespaces/default/networks/*


#### TPR based Network objects

1. Create a Third Party Resource &quot;tprnetwork.yaml&quot; for the network object as shown below:

```
apiVersion: extensions/v1beta1
kind: ThirdPartyResource
metadata:
  name: network.kubernetes.com
description: "A specification of a Network obj in the kubernetes"
versions:
- name: v1
```

2. Run kubectl create command for the Third Party Resource

```
# kubectl create -f ./tprnetwork.yaml
thirdpartyresource "network.kubernetes.com" created
```
3. Run kubectl get command to check the Network TPR creation
```
# kubectl get thirdpartyresource
NAME                     DESCRIPTION                                          VERSION(S)
network.kubernetes.com   A specification of a Network obj in the kubernetes   v1
```
### Creating network resources in Kubernetes

1. After creating CRD/TPR network object you can create network resources in Kubernetes. These network resources may contain additional underlying CNI plugin parameters given in JSON format. In the following example shown below the args field contains parameters that will be passed into “flannel” plugin.

2. Save the following YAML to flannel-network.yaml

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
3. Run kubectl create command to create network object
```
# kubectl create -f ./flannel-network.yaml 
network "flannel-conf" created
```
4. Show network objects using kubectl:
```
# kubectl get network
NAME                         KIND
flannel-conf                 Network.v1.kubernetes.com
```
5. Show details of the network object:

```
# kubectl get network flannel-conf -o yaml
apiVersion: kubernetes.com/v1
args: '[ { "delegate": { "isDefaultGateway": true } } ]'
kind: Network
metadata:
  creationTimestamp: 2017-06-28T14:20:52Z
  name: flannel-conf
  namespace: default
  resourceVersion: "5422876"
  selfLink: /apis/kubernetes.com/v1/namespaces/default/networks/flannel-conf
  uid: fdcb94a2-5c0c-11e7-bbeb-408d5c537d27
plugin: flannel
```
6. Save the following YAML to sriov-network.yaml to creating sriov network object. ( Refer to [Intel - SR-IOV CNI](https://github.com/Intel-Corp/sriov-cni) or contact @kural in [Intel-Corp Slack](https://intel-corp.herokuapp.com/) for running the DPDK based workloads in Kubernetes)
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
7. Likewise save the following YAML to sriov-vlanid-l2enable-network.yaml to create another sriov based network object:

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
8. Follow step 3 above to create &quot;sriov-vlanid-l2enable-conf&quot; and &quot;sriov-conf&quot; network objects
9. View network objects using kubectl
```
# kubectl get network
NAME                         KIND
flannel-conf                 Network.v1.kubernetes.com
sriov-vlanid-l2enable-conf   Network.v1.kubernetes.com
sriov-conf                   Network.v1.kubernetes.com
```
### Configuring Multus to use the kubeconfig
1. Create a Mutlus CNI configuration file on each Kubernetes node. This file should be created in: /etc/cni/net.d/multus-cni.conf with the content shown below. Use only the absolute path to point to the kubeconfig file (as it may change depending upon your cluster env). We are assuming all CNI plugin binaries are default location (`\opt\cni\bin dir`)
```
{
    "name": "node-cni-network",
    "type": "multus",
    "kubeconfig": "/etc/kubernetes/node-kubeconfig.yaml"
}
```
2. Restart kubelet service
```
# systemctl restart kubelet
```
### Configuring Multus to use kubeconfig and a default network

1. Many users want Kubernetes default networking feature along with network objects. Refer to issues [#14](https://github.com/Intel-Corp/multus-cni/issues/14) &amp; [#17](https://github.com/Intel-Corp/multus-cni/issues/17) for more information. In the following Multus configuration, Weave act as the default network in the absence of network field in the pod metadata annotation.

```
{
    "name": "node-cni-network",
    "type": "multus",
    "kubeconfig": "/etc/kubernetes/node-kubeconfig.yaml",
    "delegates": [{
        "type": "weave-net",
        "hairpinMode": true,
        "masterplugin": true
    }]
}
```
2. Restart kubelet service

```
# systemctl restart kubelet
```

### Configuring Pod to use the CRD/TPR network objects

1. Save the following YAML to pod-multi-network.yaml. In this case flannel-conf network object acts as the primary network.
```
# cat pod-multi-network.yaml 
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

2. Create Multiple network based pod from the master node

```
# kubectl create -f ./pod-multi-network.yaml
pod "multus-multi-net-poc" created
```

3. Get the details of the running pod from the master

```
# kubectl get pods
NAME                   READY     STATUS    RESTARTS   AGE
multus-multi-net-poc   1/1       Running   0          30s
```

### Verifying Pod network interfaces

1. Run &quot;ifconfig&quot; command in Pod:
```
# kubectl exec -it multus-multi-net-poc -- ifconfig
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
| Interface name | Description |
| --- | --- |
| lo | loopback |
| eth0@if41 | Flannel network tap interface |
| net0 | VF0 of NIC 1 assigned to the container by [Intel - SR-IOV CNI](https://github.com/Intel-Corp/sriov-cni) plugin |
| north | VF0 of NIC 2 assigned with VLAN ID 210 to the container by SR-IOV CNI plugin |

2. Check the vlan ID of the NIC 2 VFs

```
# ip link show enp2s0
20: enp2s0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc mq state UP mode DEFAULT group default qlen 1000
    link/ether 24:8a:07:e8:7d:40 brd ff:ff:ff:ff:ff:ff
    vf 0 MAC 00:00:00:00:00:00, vlan 210, spoof checking off, link-state auto
    vf 1 MAC 00:00:00:00:00:00, vlan 4095, spoof checking off, link-state auto
    vf 2 MAC 00:00:00:00:00:00, vlan 4095, spoof checking off, link-state auto
    vf 3 MAC 00:00:00:00:00:00, vlan 4095, spoof checking off, link-state auto
```

## Using with Multus conf file

Given the following network configuration:

```
# tee /etc/cni/net.d/multus-cni.conf <<-'EOF'
{
    "name": "multus-demo-network",
    "type": "multus",
    "delegates": [
        {
                "type": "sriov",
                #part of sriov plugin conf
                "if0": "enp12s0f0", 
                "ipam": {
                        "type": "host-local",
                        "subnet": "10.56.217.0/24",
                        "rangeStart": "10.56.217.131",
                        "rangeEnd": "10.56.217.190",
                        "routes": [
                                { "dst": "0.0.0.0/0" }
                        ],
                        "gateway": "10.56.217.1"
                }
        },
        {
                "type": "ptp",
                "ipam": {
                        "type": "host-local",
                        "subnet": "10.168.1.0/24",
                        "rangeStart": "10.168.1.11",
                        "rangeEnd": "10.168.1.20",
                        "routes": [
                                { "dst": "0.0.0.0/0" }
                        ],
                        "gateway": "10.168.1.1"
                }
        },
        {
                "type": "flannel",
                "masterplugin": true,
                "delegate": {
                        "isDefaultGateway": true
                }
        }
    ]
}
EOF
```

## Testing Multus CNI

### Multiple flannel networks

Github user [YYGCui](https://github.com/YYGCui) has used multiple flannel network to work with Multus CNI plugin. Please refer to this [closed issue](https://github.com/Intel-Corp/multus-cni/issues/7) for ,multiple overlay network support with Multus CNI.

Make sure that the multus, [sriov](https://github.com/Intel-Corp/sriov-cni), [flannel](https://github.com/containernetworking/cni/blob/master/Documentation/flannel.md), and [ptp](https://github.com/containernetworking/cni/blob/master/Documentation/ptp.md) binaries are in the /opt/cni/bin directories and follow the steps as mentioned in the [CNI](https://github.com/containernetworking/cni/#running-a-docker-container-with-network-namespace-set-up-by-cni-plugins)

#### Configure Kubernetes with CNI
Kubelet must be configured to run with the CNI network plugin. Edit /etc/kubernetes/kubelet file and add "--network-plugin=cni" flags in KUBELET\_OPTS as shown below:

```
KUBELET_OPTS="...
--network-plugin-dir=/etc/cni/net.d
--network-plugin=cni
"
```
Refer to the Kubernetes User Guide and network plugin for more information.
- [Single Node](https://kubernetes.io/docs/getting-started-guides/fedora/fedora_manual_config/)
- [Multi Node](https://kubernetes.io/docs/getting-started-guides/fedora/flannel_multi_node_cluster/)
- [Network plugin](https://kubernetes.io/docs/admin/network-plugins/)

Restart kubelet:
```
# systemctl restart kubelet.service
```
#### Launching workloads in Kubernetes

With Multus CNI configured as described in sections above each workload launched via a Kubernetes Pod will have multiple network interfacesLaunch the workload using yaml file in the kubernetes master, with above configuration in the multus CNI, each pod should have multiple interfaces.

Note: To verify whether Multus CNI plugin is working correctly, create a pod containing one &quot;busybox&quot; container and execute &quot;ip link&quot; command to check if interfaces management follows configuration.

1. Create &quot;multus-test.yaml&quot; file containing below configuration. Created pod will consist of one &quot;busybox&quot; container running &quot;top&quot; command.

```
apiVersion: v1
kind: Pod
metadata:
  name: multus-test
spec:  # specification of the pod's contents
  restartPolicy: Never
  containers:
  - name: test1
    image: "busybox"
    command: ["top"]
    stdin: true
    tty: true

```
2. Create pod using command:

```
# kubectl create -f multus-test.yaml
pod "multus-test" created
```
3. Run &quot;ip link&quot; command inside the container:

```
# 1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue qlen 1
    link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00
3: eth0@if41: <BROADCAST,MULTICAST,UP,LOWER_UP,M-DOWN> mtu 1500 qdisc noqueue
    link/ether 26:52:6b:d8:44:2d brd ff:ff:ff:ff:ff:ff
20: net0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc mq qlen 1000
    link/ether f6:fb:21:4f:1d:63 brd ff:ff:ff:ff:ff:ff
21: net1: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc mq qlen 1000
    link/ether 76:13:b1:60:00:00 brd ff:ff:ff:ff:ff:ff 
```

| Interface name | Description |
| --- | --- |
| lo | loopback |
| eth0@if41 | Flannel network tap interface |
| net0 | VF assigned to the container by [SR-IOV CNI](https://github.com/Intel-Corp/sriov-cni) plugin |
| net1 | ptp localhost interface |

## Multus additional plugins

- [DPDK-SRIOV CNI](https://github.com/Intel-Corp/sriov-cni)
- [Vhostuser CNI](https://github.com/intel/vhost-user-net-plugin) - a Dataplane network plugin - Supports OVS-DPDK &amp; VPP
- [Bond CNI](https://github.com/Intel-Corp/bond-cni) - For fail-over and high availability of networking

## NFV based networking in Kubernetes

- KubeCon workshop on [&quot;Enabling NFV features in Kubernetes&quot;](https://kccncna17.sched.com/event/Cvnw/enabling-nfv-features-in-kubernetes-hosted-by-kuralamudhan-ramakrishnan-ivan-coughlan-intel) presentation [slide deck](https://www.slideshare.net/KuralamudhanRamakris/enabling-nfv-features-in-kubernetes-83923352)
- Feature brief
  - [Multiple Network Interface Support in Kubernetes](https://builders.intel.com/docs/networkbuilders/multiple-network-interfaces-support-in-kubernetes-feature-brief.pdf)
  - [Enhanced Platform Awareness in Kubernetes](https://builders.intel.com/docs/networkbuilders/enhanced-platform-awareness-feature-brief.pdf)
- Application note
  - [Multiple Network Interfaces in Kubernetes and Container Bare Metal](https://builders.intel.com/docs/networkbuilders/multiple-network-interfaces-in-kubernetes-application-note.pdf)
  - [Enhanced Platform Awareness Features in Kubernetes](https://builders.intel.com/docs/networkbuilders/enhanced-platform-awareness-in-kubernetes-application-note.pdf)
- White paper
  - [Enabling New Features with Kubernetes for NFV](https://builders.intel.com/docs/networkbuilders/enabling_new_features_in_kubernetes_for_NFV.pdf)
- Multus&#39;s related project github pages
  - [Multus](https://github.com/Intel-Corp/multus-cni)
  - [SRIOV - DPDK CNI](https://github.com/Intel-Corp/sriov-cni)
  - [Vhostuser - VPP &amp; OVS - DPDK CNI](https://github.com/intel/vhost-user-net-plugin)
  - [Bond CNI](https://github.com/Intel-Corp/bond-cni)
  - [Node Feature Discovery](https://github.com/kubernetes-incubator/node-feature-discovery)
  - [CPU Manager for Kubernetes](https://github.com/Intel-Corp/CPU-Manager-for-Kubernetes)


## Need help

- Read [Containers Experience Kits](https://networkbuilders.intel.com/network-technologies/container-experience-kits)
- Try our container exp kit demo - KubeCon workshop on [Enabling NFV Features in Kubernetes](https://github.com/intel/container-experience-kits-demo-area/)
- Join us on [#intel-sddsg-slack](https://intel-corp.herokuapp.com/) slack channel and ask question in [#general-discussion](https://intel-corp-team.slack.com/messages/C4C5RSEER)
- You can also [email](mailto:kuralamudhan.ramakrishnan@intel.com) us
- Feel free to [submit](https://github.com/Intel-Corp/multus-cni/issues/new) an issue

Please fill in the Questions/feedback - [google-form](https://goo.gl/forms/upBWyGs8Wmq69IEi2)!
## Contacts
For any questions about Multus CNI, please reach out on github issue or feel free to contact the developer @kural in our [Intel-Corp Slack](https://intel-corp.herokuapp.com/)
