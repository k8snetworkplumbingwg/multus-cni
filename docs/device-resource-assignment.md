# Device resource assignment

This document describes how Multus builds the list of device IDs for a device resource, and how
those IDs are passed to delegate CNI plugins.

## Background

A NetworkAttachmentDefinition (NAD) can name a device resource with the
`k8s.v1.cni.cncf.io/resourceName` annotation:

```yaml
apiVersion: k8s.cni.cncf.io/v1
kind: NetworkAttachmentDefinition
metadata:
  name: sriov-net-a
  annotations:
    k8s.v1.cni.cncf.io/resourceName: intel.com/sriov_net_A
spec:
  config: '{ "cniVersion": "1.0.0", "name": "sriov-net-a", "type": "sriov" }'
```

Multus does not allocate devices. A device plugin advertises the resource, and kubelet assigns
devices **per container**:

```yaml
spec:
  containers:
  - name: appcntr1
    resources:
      requests:
        intel.com/sriov_net_A: '2'
  - name: appcntr2
    resources:
      requests:
        intel.com/sriov_net_A: '2'
```

Kubelet reports that assignment as `(pod, container, resource name) -> [device IDs]`. Multus reads
it from the kubelet pod-resources API, or from the kubelet checkpoint file if that API is
unavailable.

For each resource name, Multus concatenates those per-container lists into one pod-wide slice.
When a delegate network uses that resource name, Multus takes the next unused ID from the slice
and writes it into the delegate CNI configuration as `deviceID`.

## List order

The slice is built as follows:

1. Device IDs allocated to the **same container** for the **same resource name** are sorted.
2. Each container's sorted IDs are appended in **container order**. Devices from different
   containers are not mixed together.

Example: kubelet allocated `0000:03:02.3` and `0000:03:02.0` to `appcntr1`, and `0000:03:02.5` and
`0000:03:02.1` to `appcntr2`. The list for `intel.com/sriov_net_A` is:

```text
["0000:03:02.0", "0000:03:02.3", "0000:03:02.1", "0000:03:02.5"]
```

Sorting **within each container** makes the order of device IDs for that container deterministic.
Multus does not sort across containers, so kubelet's per-container grouping is unchanged.

## Related documentation

* [How to use](how-to-use.md) — device plugin and Dynamic Resource Allocation (DRA) examples.
* [Configuration](configuration.md) — Multus configuration reference.
