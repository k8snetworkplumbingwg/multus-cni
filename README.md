# MULTUS CNI plugin

Please read [CNI](https://github.com/containernetworking/cni) for more information on container networking.

Multus is the latin word for “Multi”

As name suggest, it act as the Multi plugin in the Kubernetes and provides the Multi interface support in pod

It is generic to run with other plugins like ptp, local-host, with flannel, with different IPAM and networks. 

It contact between the container runtime and other plugins, and it isn't having any of it own net configuration, it call other plugins like flannel/calico to do the real net conf job. Multus reuse the concept of invoking the delegates in the flannel, it group the multi plugins into delegates and invoke each other in the sequential order, according to the JSON scheme in the cni configuration.

## Build

This plugin requires Go 1.5+ to build.

Go 1.5 users will need to set `GO15VENDOREXPERIMENT=1` to get vendored dependencies. This flag is set by default in 1.6.

```
#./build
```
## Network configuration reference

* `name` (string, required): the name of the network
* `type` (string, required): "multus"
* `delegate` (([]map,required): number of delegate details in the Multus
* `masterplugin` (bool,required): master plugin to report back to container

## Usage

Given the following network configuration:

```
# tee /etc/cni/net.d/multus-cni.conf <<-'EOF'
{
    "name": "minion1-multus-demo-network",
    "type": "multus",
    "delegates": [
        {
                "type": "sriov",
                "if0": "enp12s0f0",
                "if0name": "north0",
                "createmac": true,
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
                "type": "sriov",
                "if0": "enp12s0f1",
                "if0name": "south0",
                "ipam": {
                        "type": "host-local",
                        "subnet": "10.56.217.0/24",
                        "rangeStart": "10.56.217.100",
                        "rangeEnd": "10.56.217.130",
                        "routes": [
                                { "dst": "0.0.0.0/0" }
                        ],
                        "gateway": "10.56.217.1"
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
Make sure that the multus, [sriov](https://github.com/Intel-Corp/sriov-cni) and [flannel](https://github.com/containernetworking/cni/blob/master/Documentation/flannel.md) binaries are in the /opt/cni/bin directories and follow the steps as mention in the [CNI](https://github.com/containernetworking/cni)
