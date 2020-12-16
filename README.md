# Multus-CNI

![multus-cni Logo](https://github.com/intel/multus-cni/blob/master/docs/images/Multus.png)

[![Travis CI](https://travis-ci.org/intel/multus-cni.svg?branch=master)](https://travis-ci.org/intel/multus-cni/builds)[![Go Report Card](https://goreportcard.com/badge/github.com/intel/multus-cni)](https://goreportcard.com/report/github.com/intel/multus-cni)[![Coverage Status](https://coveralls.io/repos/github/intel/multus-cni/badge.svg)](https://coveralls.io/github/intel/multus-cni)

Multus CNI enables attaching multiple network interfaces to pods in Kubernetes.

## How it works

Multus CNI is a container network interface (CNI) plugin for Kubernetes that enables attaching multiple network interfaces to pods. Typically, in Kubernetes each pod only has one network interface (apart from a loopback) -- with Multus you can create a multi-homed pod that has multiple interfaces. This is accomplished by Multus acting as a "meta-plugin", a CNI plugin that can call multiple other CNI plugins.

Multus CNI follows the [Kubernetes Network Custom Resource Definition De-facto Standard](https://docs.google.com/document/d/1Ny03h6IDVy_e_vmElOqR7UdTPAG_RNydhVE1Kx54kFQ/edit) to provide a standardized method by which to specify the configurations for additional network interfaces. This standard is put forward by the Kubernetes [Network Plumbing Working Group](https://docs.google.com/document/d/1oE93V3SgOGWJ4O1zeD1UmpeToa0ZiiO6LqRAmZBPFWM/edit).

Multus is one of the projects in the [Baremetal Container Experience kit](https://networkbuilders.intel.com/network-technologies/container-experience-kits)

### Multi-Homed pod

Here's an illustration of the network interfaces attached to a pod, as provisioned by Multus CNI. The diagram shows the pod with three interfaces: `eth0`, `net0` and `net1`. `eth0` connects kubernetes cluster network to connect with kubernetes server/services (e.g. kubernetes api-server, kubelet and so on). `net0` and `net1` are additional network attachments and connect to other networks by using [other CNI plugins](https://kubernetes.io/docs/concepts/extend-kubernetes/compute-storage-net/network-plugins/) (e.g. vlan/vxlan/ptp).

![multus-pod-image](docs/images/multus-pod-image.svg)

## Quickstart Installation Guide

The quickstart installation method for Multus requires that you have first installed a Kubernetes CNI plugin to serve as your pod-to-pod network, which we refer to as your "default network" (a network interface that every pod will be creatd with). Each network attachment created by Multus will be in addition to this default network interface. For more detail on installing a default network CNI plugins, refer to our [quick-start guide](docs/quickstart.md).

Clone this GitHub repository, we'll apply a daemonset which installs Multus using to `kubectl` from this repo. From the root directory of the clone, apply the daemonset YAML file:

```
$ cat ./images/multus-daemonset.yml | kubectl apply -f -
```

This will configure your systems to be ready to use Multus CNI, but, to get started with adding additional interfaces to your pods, refer to our complete [quick-start guide](docs/quickstart.md)

## Additional installation Options

- Install via daemonset using the quick-start guide, above.
- Download binaries from [release page](https://github.com/intel/multus-cni/releases)
- By Docker image from [Docker Hub](https://hub.docker.com/r/nfvpe/multus/tags/)
- Or, roll-your-own and build from source
  - See [Development](docs/development.md)

## Comprehensive Documentation

- [How to use](docs/how-to-use.md)
- [Configuration](docs/configuration.md)
- [Development](docs/development.md)

## Contact Us

For any questions about Multus CNI, feel free to ask a question in #general in the [Intel-Corp Slack](https://intel-corp.herokuapp.com/), or open up a GitHub issue.
