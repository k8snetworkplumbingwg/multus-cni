# Multus-CNI

![multus-cni Logo](https://github.com/k8snetworkplumbingwg/multus-cni/blob/master/docs/images/Multus.png)

[![Build](https://github.com/k8snetworkplumbingwg/multus-cni/actions/workflows/build.yml/badge.svg)](https://github.com/k8snetworkplumbingwg/multus-cni/actions/workflows/build.yml)[![Test](https://github.com/k8snetworkplumbingwg/multus-cni/actions/workflows/test.yml/badge.svg)](https://github.com/k8snetworkplumbingwg/multus-cni/actions/workflows/test.yml)[![Go Report Card](https://goreportcard.com/badge/github.com/k8snetworkplumbingwg/multus-cni)](https://goreportcard.com/report/github.com/k8snetworkplumbingwg/multus-cni)[![Coverage Status](https://coveralls.io/repos/github/k8snetworkplumbingwg/multus-cni/badge.svg)](https://coveralls.io/github/k8snetworkplumbingwg/multus-cni)

Multus CNI enables attaching multiple network interfaces to pods in Kubernetes.

## How it works

Multus CNI is a container network interface (CNI) plugin for Kubernetes that enables attaching multiple network interfaces to pods. Typically, in Kubernetes each pod only has one network interface (apart from a loopback) -- with Multus you can create a multi-homed pod that has multiple interfaces. This is accomplished by Multus acting as a "meta-plugin", a CNI plugin that can call multiple other CNI plugins.

Multus CNI follows the [Kubernetes Network Custom Resource Definition De-facto Standard](https://docs.google.com/document/d/1Ny03h6IDVy_e_vmElOqR7UdTPAG_RNydhVE1Kx54kFQ/edit) to provide a standardized method by which to specify the configurations for additional network interfaces. This standard is put forward by the Kubernetes [Network Plumbing Working Group](https://docs.google.com/document/d/1oE93V3SgOGWJ4O1zeD1UmpeToa0ZiiO6LqRAmZBPFWM/edit).

Multus is one of the projects in the [Baremetal Container Experience kit](https://networkbuilders.intel.com/network-technologies/container-experience-kits)

### Multi-Homed pod

Here's an illustration of the network interfaces attached to a pod, as provisioned by Multus CNI. The diagram shows the pod with three interfaces: `eth0`, `net0` and `net1`. `eth0` connects kubernetes cluster network to connect with kubernetes server/services (e.g. kubernetes api-server, kubelet and so on). `net0` and `net1` are additional network attachments and connect to other networks by using [other CNI plugins](https://kubernetes.io/docs/concepts/extend-kubernetes/compute-storage-net/network-plugins/) (e.g. vlan/vxlan/ptp).

![multus-pod-image](docs/images/multus-pod-image.svg)

## Quickstart Installation Guide

The quickstart installation method for Multus requires that you have first installed a Kubernetes CNI plugin to serve as your pod-to-pod network, which we refer to as your "default network" (a network interface that every pod will be created with). Each network attachment created by Multus will be in addition to this default network interface. For more detail on installing a default network CNI plugin, refer to our [quick-start guide](docs/quickstart.md).

Clone this GitHub repository, and apply a daemonset which installs Multus using `kubectl`. From the root directory of the clone, apply the daemonset YAML file:

```
cat ./deployments/multus-daemonset-thick.yml | kubectl apply -f -
```

This will configure your systems to be ready to use Multus CNI, but, to get started with adding additional interfaces to your pods, refer to our complete [quick-start guide](docs/quickstart.md)

## Thin Plugin v.s Thick Plugin

With the multus 4.0 release, we introduce a new client/server-style plugin deployment. This new deployment is called ['thick plugin'](docs/thick-plugin.md), in contrast to deployments in previous versions, which is now called a 'thin plugin'. The new thick plugin consists of two binaries, multus-daemon and multus-shim CNI plugin. The 'multus-daemon' will be deployed to all nodes as a local agent and supports additional features, such as metrics, which were not available with the 'thin plugin' deployment before. Due to these additional features, the 'thick plugin' comes with the trade-off of consuming more resources than the 'thin plugin'.

We recommend using the thick plugin in most environments, but if you wish to run the thin plugin, or are in a resource-constrained environment, you may do so with:

```
cat ./deployments/multus-daemonset.yml | kubectl apply -f -
```

## Additional Installation Options

In addition to the [quick-start guide](docs/quickstart.md), you may:

- Download binaries from [release page](https://github.com/k8snetworkplumbingwg/multus-cni/releases)
- By Docker image from [GitHub Container Registry](https://github.com/orgs/k8snetworkplumbingwg/packages/container/package/multus-cni)
- Or, roll-your-own and build from source
  - See [Development](docs/development.md)

## Comprehensive Documentation

- [How to use](docs/how-to-use.md)
- [Quick Start Guide](docs/quickstart.md)
- [Configuration](docs/configuration.md)
- [Development and Support Information](docs/development.md)
- [Thick Plugin](docs/thick-plugin.md)

## Contact Us

For any questions about Multus CNI, open up a GitHub issue or feel free to ask a question in #general in the [NPWG Slack](https://npwg-team.slack.com/).

To be invited, use [this slack invite link](https://join.slack.com/t/npwg-team/shared_invite/zt-1u2vmsn2b-tKdOokdPY73zn9B32JoAOg).
