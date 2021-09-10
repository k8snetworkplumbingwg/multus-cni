# Example Configurations & Pod Specs

In the `./examples` folder some example configurations are provided for using Multus, especially with CRDs, and doubly so in reference to their usage with the [defacto standard for CRDs](https://docs.google.com/document/d/1Ny03h6IDVy_e_vmElOqR7UdTPAG_RNydhVE1Kx54kFQ/edit) as proposed by the Network Plumbing Working Group.

## Examples overview

Generally, the examples here show a setup using Multus with CRD support. The examples here demonstrate a setup with Multus as the meta-plugin used by Kubernetes, and delegating to either Flannel (which will be the default pod network), or to macvlan. The CRDs are intended to be alignment with the defacto standard.

It is expected that aspects of your own setup will vary, at least in part, from some of what's demonstrated here. Namely, the IP address spaces, and likely the host ethernet interface names used in the macvlan part of the configuration.

More specifically, these examples show:

* Multus configured, using CNI a `.conf` file, with CRD support, specifying that we will use a "default network".
* A resource definition with a daemonset that places the `.conf` on each node in the cluster.
* A CRD defining the "networks" @ `network-attachment-definitions.k8s.cni.cncf.io` 
* CRD objects containing the configuration for both Flannel & macvlan.

## Quick-start instructions

* Compile Multus and place binaries into (typically) `/opt/cni/bin/`
    - Refer to the primary README.md for more details on compilation.
* Allow `system:node` access to enable Multus to pull CRD objects.
    - See "RBAC configuration section below for details."
* Create the Flannel + Multus setup with the daemonset provided
    - As in: `kubectl create -f multus-with-flannel.yml`
    - Optionally, verify that the `/etc/cni/net.d/*.conf` exists on each node.
* Create the CRDs
    - Create the CRD itself, `kubectl create -f crd.yml`
    - Create the network attachment configurations (i.e. CNI configurations packed into CRD objects)
        + `kubectl create -f flannel-conf.yml`
        + `kubectl create -f macvlan-conf.yml`
        + Verify the CRD objects are created with: `kubectl get networks`
* Spin up an sample pod
    - `kubectl create -f sample-pod.yml`
    - Verify that it has multiple interfaces with:
        + `kubectl exec -it samplepod -- ip a`

## RBAC configuration

You'll need to enable the `system:node` users access to the API endpoints that will deliver the CRD objects to Multus. 

Using these examples, you'll first create a cluster role with the provided sample:

```
kubectl create -f clusterrole.yml
```

You will then create a `clusterrolebinding` for each hostname in the Kubernetes cluster. Replace `HOSTNAME` below with the host name of a node, and then repeat for all hostnames in the cluster.

```
kubectl create clusterrolebinding multus-node-HOSTNAME \
    --clusterrole=multus-crd-overpowered \
    --user=system:node:HOSTNAME
```

## CNI Configuration

A sample `cni-configuration.conf` is provided, typically this file is placed in `/etc/cni/net.d/`. It must be the first file alphabetically in this folder in order for the Kubelet to honor its use. However, if you opt to use the provided Flannel + Multus YAML file, this will deploy a configuration (packed inside a daemonset therein) on each node in your Kubernetes cluster.

## Other considerations

Primarily in this setup one thing that one should consider are the aspects of the `macvlan-conf.yml`, which is likely specific to the configuration of the node on which this resides.

## Passing down device information
Some CNI plugins require specific device information which maybe pre-allocated by K8s device plugin. This could be indicated by providing `k8s.v1.cni.cncf.io/resourceName` annotation in its network attachment definition CRD. The file [`examples/sriov-net.yaml`](./sriov-net.yaml) shows an example on how to define a Network attachment definition with specific device allocation information. Multus will get allocated device information and make them available for CNI plugin to work on.

In this example (shown below), it is expected that an [SRIOV Device Plugin](https://github.com/intel/sriov-network-device-plugin/) making a pool of SRIOV VFs available to the K8s with `intel.com/sriov` as their resourceName. Any device allocated from this resource pool will be passed down by Multus to the [sriov-cni](https://github.com/intel/sriov-cni/tree/dev/k8s-deviceid-model) plugin in `deviceID` field. This is up to the sriov-cni plugin to capture this information and work with this specific device information.

```yaml
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: sriov-net-a
  annotations:
    k8s.v1.cni.cncf.io/resourceName: intel.com/sriov
spec:
  config: '{
  "type": "sriov",
  "vlan": 1000,
  "ipam": {
    "type": "host-local",
    "subnet": "10.56.217.0/24",
    "rangeStart": "10.56.217.171",
    "rangeEnd": "10.56.217.181",
    "routes": [{
      "dst": "0.0.0.0/0"
    }],
    "gateway": "10.56.217.1"
  }
}'
```
The [sriov-pod.yml](./sriov-pod.yml) is an example Pod manifest file that requesting a SRIOV device from a host which is then configured using the above network attachment definition.

>For further information on how to configure SRIOV Device Plugin and SRIOV-CNI please refer to the links given above.
