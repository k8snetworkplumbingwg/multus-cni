# Quickstart Guide

This guide is intended as a way to get you off the ground, using Multus CNI to create Kubernetes pods with multiple interfaces. If you're already using Multus and need more detail, see the [comprehensive usage guide](how-to-use.md). This document is a quickstart and a getting started guide in one, intended for your first run-through of Multus CNI.

We'll first install Multus CNI, and then we'll setup some configurations so that you can see how multiple interfaces are created for pods.

## Key Concepts

Two things we'll refer to a number of times through this document are:

* "Default network" -- This is your pod-to-pod network. This is how pods communicate among one another in your cluster, how they have connectivity. Generally speaking, this is presented as the interface named `eth0`. This interface is always attached to your pods, so that they can have connectivity among themselves. We'll add interfaces in addition to this.
* "CRDs" -- Custom Resource Definitions. Custom Resources are a way that the Kubernetes API is extended. We use these here to store some information that Multus can read. Primarily, we use these to store the configurations for each of the additional interfaces that are attached to your pods.

## Prerequisites

Our installation method requires that you first have installed Kubernetes and have configured a default network -- that is, a CNI plugin that's used for your pod-to-pod connectivity. 

We recommend Kubernetes 1.16 or later.

To install Kubernetes, you may decide to use [kubeadm](https://kubernetes.io/docs/setup/independent/create-cluster-kubeadm/), or potentially [kubespray](https://github.com/kubernetes-sigs/kubespray).

After installing Kubernetes, you must install a default network CNI plugin. If you're using kubeadm, refer to the "[Installing a pod network add-on](https://kubernetes.io/docs/setup/independent/create-cluster-kubeadm/#pod-network)" section in the kubeadm documentation. If it's your first time, we generally recommend using Flannel for the sake of simplicity.

Alternatively, for advanced use cases, for installing Multus and a default network plugin at the same time, you may refer to the [Kubernetes Network Plumbing Group's Reference Deployments](https://github.com/k8snetworkplumbingwg/reference-deployment).

To verify that you default network is ready, you may list your Kubernetes nodes with:

```
kubectl get nodes
```

In the case that your default network is ready you will see the `STATUS` column also switch to `Ready` for each node.

```
NAME                  STATUS   ROLES           AGE    VERSION
master-0              Ready    master          1h     v1.17.1
master-1              Ready    master          1h     v1.17.1
master-2              Ready    master          1h     v1.17.1
```

## Installation

Our recommended quickstart method to deploy Multus is to deploy using a Daemonset (a method of running pods on each nodes in your cluster), this spins up pods which install a Multus binary and configure Multus for usage.

Firstly, clone this GitHub repository. 

```
git clone https://github.com/intel/multus-cni.git && cd multus-cni
```

We'll apply a YAML file with `kubectl` from this repo.

```
$ cat ./images/multus-daemonset.yml | kubectl apply -f -
```

### What the Multus daemonset does

* Starts a Multus daemonset, this runs a pod on each node which places a Multus binary on each node in `/opt/cni/bin`
* Reads the lexicographically (alphabetically) first configuration file in `/etc/cni/net.d`, and creates a new configuration file for Multus as `/etc/cni/net.d/00-multus.conf`, this configuration is auto-generated and is based on the default network configuration (which is assumed to be the alphabetically first configuration)
* Creates a `/etc/cni/net.d/multus.d` directory on each node with authentication information for Multus to access the Kubernetes API.


### Validating your installation

Generally, the first step in validating your installation is to ensure that the Multus pods have run without error, you may see an overview of those by looking at:

```
$ kubectl get pods --all-namespaces | grep -i multus
```

You may further validate that it has ran by looking at the `/etc/cni/net.d/` directory and ensure that the auto-generated `/etc/cni/net.d/00-multus.conf` exists corresponding to the alphabetically first configuration file.

## Creating additional interfaces

The first thing we'll do is create configurations for each of the additional interfaces that we attach to pods. We'll do this by creating Custom Resources. Part of the quickstart installation creates a "CRD" -- a custom resource definition that is the home where we keep these custom resources -- we'll store our configurations for each interface in these.

### CNI Configurations

Each configuration we'll add is a CNI configuration. If you're not familiar with them, let's break them down quickly. Here's an example CNI configuration:

```
{
  "cniVersion": "0.3.0",
  "type": "loopback",
  "additional": "information"
}
```

CNI configurations are JSON, and we have a structure here that has a few things we're interested in:

1. `cniVersion`: Tells each CNI plugin which version is being used and can give the plugin information if it's using a too late (or too early) version.
2. `type`: This tells CNI which binary to call on disk. Each CNI plugin is a binary that's called. Typically, these binaries are stored in `/opt/cni/bin` on each node, and CNI executes this binary. In this case we've specified the `loopback` binary (which create a loopback-type network interface). If this is your first time installing Multus, you might want to verify that the plugins that are in the "type" field are actually on disk in the `/opt/cni/bin` directory.
3. `additional`: This field is put here as an example, each CNI plugin can specify whatever configuration parameters they'd like in JSON. These are specific to the binary you're calling in the `type` field.

For an even further example -- take a look at the [bridge CNI plugin README](https://github.com/containernetworking/plugins/tree/master/plugins/main/bridge) which shows additional details.

If you'd like more information about CNI configuration, you can read [the entire CNI specification](https://github.com/containernetworking/cni/blob/master/SPEC.md). It might also be useful to look at the [CNI reference plugins](https://github.com/containernetworking/plugins) and see how they're configured.

You do not need to reload or refresh the Kubelets when CNI configurations change. These are read on each creation & deletion of pods. So if you change a configuration, it'll apply the next time a pod is created. Existing pods may need to be restarted if they need the new configuration.

### Storing a configuration as a Custom Resource

So, we want to create an additional interface. Let's create a macvlan interface for pods to use. We'll create a custom resource that defines the CNI configuration for interfaces.

Note in the following command that there's a `kind: NetworkAttachmentDefinition`. This is our fancy name for our configuration -- it's a custom extension of Kubernetes that defines how we attach networks to our pods. 

Secondarily, note the `config` field. You'll see that this is a CNI configuration just like we explained earlier.

Lastly but *very* importantly, note under `metadata` the `name` field -- here's where we give this configuration a name, and it's how we tell pods to use this configuration. The name here is `macvlan-conf` -- as we're creating a configuration for macvlan.

Here's the command to create this example configuration:

```
cat <<EOF | kubectl create -f -
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: macvlan-conf
spec:
  config: '{
      "cniVersion": "0.3.0",
      "type": "macvlan",
      "master": "eth0",
      "mode": "bridge",
      "ipam": {
        "type": "host-local",
        "subnet": "192.168.1.0/24",
        "rangeStart": "192.168.1.200",
        "rangeEnd": "192.168.1.216",
        "routes": [
          { "dst": "0.0.0.0/0" }
        ],
        "gateway": "192.168.1.1"
      }
    }'
EOF
```

*NOTE*: This example uses `eth0` as the `master` parameter, this master parameter should match the interface name on the hosts in your cluster.

You can see which configurations you've created using `kubectl` here's how you can do that:

```
kubectl get network-attachment-definitions
```

You can get more detail by describing them:

```
kubectl describe network-attachment-definitions macvlan-conf
```

### Creating a pod that attaches an additional interface

We're going to create a pod. This will look familiar as any pod you might have created before, but, we'll have a special `annotations` field -- in this case we'll have an annotation called `k8s.v1.cni.cncf.io/networks`. This field takes a comma delimited list of the names of your `NetworkAttachmentDefinition`s as we created above. Note in the command below that we have the annotation of `k8s.v1.cni.cncf.io/networks: macvlan-conf` where `macvlan-conf` is the name we used above when we created our configuration.

Let's go ahead and create a pod (that just sleeps for a really long time) with this command:

```
cat <<EOF | kubectl create -f -
apiVersion: v1
kind: Pod
metadata:
  name: samplepod
  annotations:
    k8s.v1.cni.cncf.io/networks: macvlan-conf
spec:
  containers:
  - name: samplepod
    command: ["/bin/ash", "-c", "trap : TERM INT; sleep infinity & wait"]
    image: alpine
EOF
```

You may now inspect the pod and see what interfaces are attached, like so:

```
$ kubectl exec -it samplepod -- ip a
```

You should note that there are 3 interfaces:

* `lo` a loopback interface
* `eth0` our default network
* `net1` the new interface we created with the macvlan configuration.

### Network Status Annotations

For additional confirmation, use `kubectl describe pod samplepod` and there will be an annotations section, similar to the following:

```
Annotations:        k8s.v1.cni.cncf.io/networks: macvlan-conf
                    k8s.v1.cni.cncf.io/networks-status:
                      [{
                          "name": "cbr0",
                          "ips": [
                              "10.244.1.73"
                          ],
                          "default": true,
                          "dns": {}
                      },{
                          "name": "macvlan-conf",
                          "interface": "net1",
                          "ips": [
                              "192.168.1.205"
                          ],
                          "mac": "86:1d:96:ff:55:0d",
                          "dns": {}
                      }]
```

This metadata tells us that we have two CNI plugins running successfully.

### What if I want more interfaces?

You can add more interfaces to a pod by creating more custom resources and then referring to them in pod's annotation. You can also reuse configurations, so for example, to attach two macvlan interfaces to a pod, you could create a pod like so:

```
cat <<EOF | kubectl create -f -
apiVersion: v1
kind: Pod
metadata:
  name: samplepod
  annotations:
    k8s.v1.cni.cncf.io/networks: macvlan-conf,macvlan-conf
spec:
  containers:
  - name: samplepod
    command: ["/bin/ash", "-c", "trap : TERM INT; sleep infinity & wait"]
    image: alpine
EOF
```

Note that the annotation now reads `k8s.v1.cni.cncf.io/networks: macvlan-conf,macvlan-conf`. Where we have the same configuration used twice, separated by a comma. 

If you were to create another custom resource with the name `foo` you could use that such as: `k8s.v1.cni.cncf.io/networks: foo,macvlan-conf`, and use any number of attachments.
