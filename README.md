![multus-cni Logo](https://github.com/Intel-Corp/multus-cni/blob/dev/doc/doc/images/Multus.png)
# MULTUS CNI plugin

- *Multus* is the latin word for “Multi”

- As the name suggests, it acts as the Multi plugin in Kubernetes and provides the Multi interface support in a pod

- It is generic to run with other plugins like ptp, local-host, calico and flannel, with different IPAM and networks. 

- It is a contact between the container runtime and other plugins, and it doesn't have any of its own net configuration, it calls other plugins like flannel/calico to do the real net conf job. 

- Multus reuses the concept of invoking the delegates in flannel, it groups the multi plugins into delegates and invoke each other in sequential order, according to the JSON scheme in the cni configuration.

- No. of plugins supported is dependent upon the number of delegates in the conf file.

- Master plugin invokes "eth0" interface in the pod, rest of plugins(Mininon plugins eg: sriov,ipam) invoke interfaces as "net0", "net1".. "netn"

- The "masterplugin" is the only net conf option of multus cni, it identifies the primary network. The default route will point to the primary network 

Please read [CNI](https://github.com/containernetworking/cni) for more information on container networking.

## Multi-Homed pod
<p align="center">
   <img src="doc/images/multus_cni_pod.png" width="1008" />
</p>

## Build

This plugin requires Go 1.5+ to build.

Go 1.5 users will need to set `GO15VENDOREXPERIMENT=1` to get vendored dependencies. This flag is set by default in 1.6.

```
#./build
```
## Work flow
<p align="center">
   <img src="doc/images/workflow.png" width="1008" />
</p>
## Network configuration reference

* `name` (string, required): the name of the network
* `type` (string, required): "multus"
* `delegates` (([]map,required): number of delegate details in the Multus
* `masterplugin` (bool,required): master plugin to report back the IP address and DNS to the container

## Usage

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
## Testing the Multus CNI with docker
Make sure that the multus, [sriov](https://github.com/Intel-Corp/sriov-cni), [flannel](https://github.com/containernetworking/cni/blob/master/Documentation/flannel.md), and [ptp](https://github.com/containernetworking/cni/blob/master/Documentation/ptp.md) binaries are in the `/opt/cni/bin` directories and follow the steps as mention in the [CNI](https://github.com/containernetworking/cni/#running-a-docker-container-with-network-namespace-set-up-by-cni-plugins)

## Testing the Multus CNI with Kubernetes
Refer the Kubernetes User Guide and network plugin
* [Single Node](https://kubernetes.io/docs/getting-started-guides/fedora/fedora_manual_config/)
* [Multi Node](https://kubernetes.io/docs/getting-started-guides/fedora/flannel_multi_node_cluster/)
* [Network Plugin](https://kubernetes.io/docs/admin/network-plugins/)

Kubelet must be configured to run with the CNI `--network-plugin`, with the following configuration information. 
Edit `/etc/default/kubelet` file and add `KUBELET_OPTS`:
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
### Launching workloads in Kubernetes
Launch the workload using yaml file in the kubernetes master, with above configuration in the multus CNI, each pod should have multiple interfaces.
> Note:	To verify whether Multus CNI plugin is working fine create a pod containing one “busybox” container and execute “ip link” command to check if interfaces management follows configuration.

1. Create “multus-test.yaml” file containing below configuration. Created pod will consist of one “busybox” container running “top” command.
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
3. Run “ip link” command inside the container:
```
# 1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue qlen 1
    link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00
3: eth0@if41: <BROADCAST,MULTICAST,UP,LOWER_UP,M-DOWN> mtu 1500 qdisc noqueue
    link/ether 26:52:6b:d8:44:2d brd ff:ff:ff:ff:ff:ff
20: net0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc mq qlen 1000
    link/ether f6:fb:21:4f:1d:63 brd ff:ff:ff:ff:ff:ff
21: net1: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc mq qlen 1000
    link/ether 76:13:b1:60:00:00 brd ff:ff:ff:ff:ff:ff As seen in the above output 3 interfaces are created.
```
Interface name | Description
------------ | -------------
lo | loopback
eth0@if41 | Flannel network tap interface
net0 | VF assigned to the container by [SR_IOV CNI](https://github.com/Intel-Corp/sriov-cni) plugin
net1 | ptp localhost interface

### Contacts
For any questions about Multus CNI, please reach out on github issue or contact the developer

- Kuralamudhan Ramakrishnan <kuralamudhan.ramakrishnan@intel.com>
- David M O Neill <david.m.oneill@intel.com>
