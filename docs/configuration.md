# Multus-cni Configuration Reference

## Introduction

Aside from setting options for Multus, one of the goals of configuration is to set the configuration for your *default network*. The default network is also sometimes referred as the "primary CNI plugin", the "primary network", or a "default CNI plugin" and is the CNI plugin that is used to implement [the Kubernetes networking model](https://kubernetes.io/docs/concepts/services-networking/#the-kubernetes-network-model) in your cluster. Common examples include Flannel, Weave, Calico, Cillium, and OVN-Kubernetes, among others.

Here we will refer to this as your default CNI plugin or default network.

## Example configuration

Following is the example of multus config file, in `/etc/cni/net.d/`.

Example configuration using `clusterNetwork` (see also [using delegates](#using-delegates))

```
{
    "cniVersion": "0.3.1",
    "name": "node-cni-network",
    "type": "multus",
    "kubeconfig": "/etc/kubernetes/node-kubeconfig.yaml",
    "confDir": "/etc/cni/multus/net.d",
    "cniDir": "/var/lib/cni/multus",
    "binDir": "/opt/cni/bin",
    "logFile": "/var/log/multus.log",
    "logLevel": "debug",
    "logOptions": {
        "maxAge": 5,
        "maxSize": 100,
        "maxBackups": 5,
        "compress": true
    },
    "capabilities": {
        "portMappings": true
    },    
    "namespaceIsolation": false,
    "clusterNetwork": "/etc/cni/net.d/99-flannel.conf",
    "defaultNetworks": ["sidecarCRD", "exampleNetwork"],
    "systemNamespaces": ["kube-system", "admin"],
    "multusNamespace": "kube-system",
    allowTryDeleteOnErr: false
}
```

## Index of configuration options

This is a general index of options, however note that you must set either the `clusterNetwork` or `delegates` options, see the following sections after the index for details.

* `name` (string, required): The name of the network
* `type` (string, required): Must be set to the value of &quot;multus&quot;
* `confDir` (string, optional): directory for CNI config file that multus reads. default `/etc/cni/multus/net.d`
* `cniDir` (string, optional): Multus CNI data directory, default `/var/lib/cni/multus`
* `binDir` (string, optional): additional directory for CNI plugins which multus calls, in addition to the default (the default is typically set to `/opt/cni/bin`)
* `kubeconfig` (string, optional): kubeconfig file for the out of cluster communication with kube-apiserver. See the example [kubeconfig](https://github.com/k8snetworkplumbingwg/multus-cni/blob/master/docs/node-kubeconfig.yaml). If you would like to use CRD (i.e. network attachment definition), this is required
* [`logToStderr`](#Logging-via-STDERR) (bool, optional): Enable or disable logging to `STDERR`. Defaults to true.
* [`logFile`](#Writing-to-a-Log-File) (string, optional): file path for log file. multus puts log in given file
* [`logLevel`](#Logging-Level) (string, optional): logging level (values in decreasing order of verbosity: "debug", "error", "verbose", or "panic")
* [`logOptions`](#Logging-Options) (object, optional): logging option, More detailed log configuration
* [`namespaceIsolation`](#Namespace-Isolation) (boolean, optional): Enables a security feature where pods are only allowed to access `NetworkAttachmentDefinitions` in the namespace where the pod resides. Defaults to false.
* [`globalNamespaces`](#Allow-specific-namespaces-to-be-used-across-namespaces-when-using-namespace-isolation): (string, optional): Used only when `namespaceIsolation` is true, allows specification of comma-delimited list of namespaces which may be referred to outside of namespace isolation.
* `capabilities` ({}list, optional): [capabilities](https://github.com/containernetworking/cni/blob/master/CONVENTIONS.md#dynamic-plugin-specific-fields-capabilities--runtime-configuration) supported by at least one of the delegates. (NOTE: Multus only supports portMappings/Bandwidth capability for cluster networks).
* [`readinessindicatorfile`](#Default-Network-Readiness-Indicator): The path to a file whose existence denotes that the default network is ready
message to next when some missing error. Defaults to false.
* `systemNamespaces` ([]string, optional): list of namespaces for Kubernetes system (namespaces listed here will not have `defaultNetworks` added)
* `multusNamespace` (string, optional): namespace for `clusterNetwork`/`defaultNetworks` (the default value is `kube-system`)
* `retryDeleteOnError` (bool, optional): Enable or disable delegate DEL 

### Using `clusterNetwork`

Using the `clusterNetwork` option and the `delegates` are **mutually exclusive**. If `clusterNetwork` is set, the `delegates` field is *ignored*. 

You **must** set one or the other.

Therefore:

* Set `clusterNetwork` and if this is set, optionally set the `defaultNetworks`.
* OR you **must** set `delegates`.

Options:

* `clusterNetwork` (string, required if not using `delegates`): the default CNI plugin to be executed.
* `defaultNetworks` ([]string, optional): Additional / secondary network attachment that is always attached to each pod. 

The following values are valid for both `clusterNetwork` and `defaultNetworks` and are processed in the following order:

* The name of a `NetworkAttachmentDefinition` custom resource in the namespace specified  by the `multusNamespace` configuration option
* The `"name"` value in the contents of a CNI JSON configuration file in the CNI configuration directory, 
  * The given name for `clusterNetwork` should match the value for `name` key in the contents of the CNI JSON file (e.g. `"name": "test"` in `my.conf` when `"clusterNetwork": "test"`)
* A path to a directory containing CNI json configuration files. The alphabetically first file will be used.
* Absolute file path for CNI config file
* If none of the above are found using the value, Multus will raise an error.

If for example you have `defaultNetworks` set as:

```
"defaultNetworks": ["sidecarNetwork", "exampleNetwork"],
```

In this example, the values in the expression refer to `NetworkAttachmentDefinition` custom resource names. Therefore, there must be `NetworkAttachmentDefinitions` already created with the names `sidecarNetwork` and `exampleNetwork`.

This means that in addition to the cluster network, each pod would be assigned two additional networks by default, and the pod would present three interfaces, e.g. `eth0`, `net1`, and `net2`, with `net1` and `net2` being set by the above described `NetworkAttachmentDefinitions`. Additional attachments as made by setting `k8s.v1.cni.cncf.io/networks` on pods will be made in addition to those set in the `defaultNetworks` configuration option.

### Using `delegates`

If `clusterNetwork` is not set, you **must** use `delegates`.

* `delegates` ([]map, required if not using `clusterNetwork`). List of CNI configurations to be used as your default CNI plugin(s).

Example configuration using `delegates`:

```
{
    "cniVersion": "0.3.1",
    "name": "node-cni-network",
    "type": "multus",
    "kubeconfig": "/etc/kubernetes/node-kubeconfig.yaml",
    "confDir": "/etc/cni/multus/net.d",
    "cniDir": "/var/lib/cni/multus",
    "binDir": "/opt/cni/bin",
    "delegates": [{
        "type": "weave-net",
        "hairpinMode": true
    }, {
        "type": "macvlan",
        ... (snip)
    }]
}
```

## Configuration Option Details

### Default Network Readiness Indicator

You may desire that your default network becomes ready before attaching networks with Multus. This is disabled by default and not used unless you set the `readinessindicatorfile` option to a non-blank value.

For example, if you use Flannel as a default network, the recommended method for Flannel to be installed is via a daemonset that also drops a configuration file in `/etc/cni/net.d/`. This may apply to other plugins that place that configuration file upon their readiness, therefore, Multus uses their configuration filename as a semaphore and optionally waits to attach networks to pods until that file exists.

In this manner, you may prevent pods from crash looping, and instead wait for that default network to be ready.

Only one option is necessary to configure this functionality:

* `readinessindicatorfile`: The path to a file whose existence denotes that the default network is ready.

*NOTE*: If `readinessindicatorfile` is unset, or is an empty string, this functionality will be disabled, and is disabled by default.


### Logging

You may wish to enable some enhanced logging for Multus, especially during the process where you're configuring Multus and need to understand what is or isn't working with your particular configuration.

#### Logging via STDERR

By default, Multus will log via `STDERR`, which is the standard method by which CNI plugins communicate errors, and these errors are logged by the Kubelet.

Optionally, you may disable this method by setting the `logToStderr` option in your CNI configuration:

```
    "logToStderr": false,
```

#### Writing to a Log File

Optionally, you may have Multus log to a file on the filesystem. This file will be written locally on each node where Multus is executed. You may configure this via the `LogFile` option in the CNI configuration. By default this additional logging to a flat file is disabled.

For example in your CNI configuration, you may set:

```
    "logFile": "/var/log/multus.log",
```

#### Logging Level

The default logging level is set as `panic` -- this will log only the most critical errors, and is the least verbose logging level.

The available logging level values, in decreasing order of verbosity are:

* `debug`
* `verbose`
* `error`
* `panic`

You may configure the logging level by using the `LogLevel` option in your CNI configuration. For example:

```
    "logLevel": "debug",
```

#### Logging Options

If you want a more detailed configuration of the logging, This includes the following parameters:

* `maxAge` the maximum number of days to retain old log files in their filename
* `maxSize` the maximum size in megabytes of the log file before it gets rotated
* `maxBackups` the maximum number of days to retain old log files in their filename
* `compress` compress determines if the rotated log files should be compressed using gzip

For example in your CNI configuration, you may set:

```
    "logOptions": {
        "maxAge": 5,
        "maxSize": 100,
        "maxBackups": 5,
        "compress": true
    }
```

### Namespace Isolation

The functionality provided by the `namespaceIsolation` configuration option enables a mode where Multus only allows pods to access custom resources (the `NetworkAttachmentDefinitions`) within the namespace where that pod resides. In other words, the `NetworkAttachmentDefinitions` are isolated to usage within the namespace in which they're created. 

**NOTE**: The default namespace is special in this scenario. Even with namespace isolation enabled, any pod, in any namespace is allowed to refer to `NetworkAttachmentDefinitions` in the default namespace. This allows you to create commonly used unprivileged `NetworkAttachmentDefinitions` without having to put them in all namespaces. For example, if you had a `NetworkAttachmentDefinition` named `foo` the default namespace, you may reference it in an annotation with: `default/foo`.

**NOTE**: You can also add additional namespaces which can be referred to globally using the `global-namespaces` option (see next section).

For example, if a pod is created in the namespace called `development`, Multus will not allow networks to be attached when defined by custom resources created in a different namespace, say in the `default` network.

Consider the situation where you have a system that has users of different privilege levels -- as an example, a platform which has two administrators: a Senior Administrator and a Junior Administrator. The Senior Administrator may have access to all namespaces, and some network configurations as used by Multus are considered to be privileged in that they allow access to some protected resources available on the network. However, the Junior Administrator has access to only a subset of namespaces, and therefore it should be assumed that the Junior Administrator cannot create pods in their limited subset of namespaces. The `namespaceIsolation` feature provides for this isolation, allowing pods created in given namespaces to only access custom resources in the same namespace as the pod.

Namespace Isolation is disabled by default.

#### Configuration example

```
  "namespaceIsolation": true,
```

#### Usage example

Let's setup an example where we:

* Create a custom resource in a namespace called `privileged`
* Create a pod in a namespace called `development`, and have annotations that reference a custom resource in the `privileged` namespace. The creation of this pod should be disallowed by Multus (as we'll have the use of the custom resources limited only to those custom resources created within the same namespace as the pod).

Given the above scenario with a Junior & Senior Administrator. You may assume that the Senior Administrator has access to all namespaces, whereas the Junior Administrator has access only to the `development` namespace.

Firstly, we show that we have a number of namespaces available:

```
# List the available namespaces
[user@kube-master ~]$ kubectl get namespaces
NAME          STATUS   AGE
default       Active   7h27m
development   Active   3h
kube-public   Active   7h27m
kube-system   Active   7h27m
privileged    Active   4s
```

We'll create a `NetworkAttachmentDefinition` in the `privileged` namespace.

```
# Show the network attachment definition we're creating.
[user@kube-master ~]$ cat cr.yml
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

# Create that network attachment definition in the privileged namespace
[user@kube-master ~]$ kubectl create -f cr.yml -n privileged
networkattachmentdefinition.k8s.cni.cncf.io/macvlan-conf created

# List the available network attachment definitions in the privileged namespace.
[user@kube-master ~]$ kubectl get networkattachmentdefinition.k8s.cni.cncf.io -n privileged
NAME           AGE
macvlan-conf   11s
```

Next, we'll create a pod with an annotation that references the privileged namespace. Pay particular attention to the annotation that reads `k8s.v1.cni.cncf.io/networks: privileged/macvlan-conf` -- where it contains a reference to a `namespace/configuration-name` formatted network attachment name. In this case referring to the `macvlan-conf` in the namespace called `privileged`.

```
# Show the yaml for a pod.
[user@kube-master ~]$ cat example.pod.yml
apiVersion: v1
kind: Pod
metadata:
  name: samplepod
  annotations:
    k8s.v1.cni.cncf.io/networks: privileged/macvlan-conf
spec:
  containers:
  - name: samplepod
    command: ["/bin/bash", "-c", "sleep 2000000000000"]
    image: dougbtv/centos-network

# Create that pod.
[user@kube-master ~]$ kubectl create -f example.pod.yml -n development
pod/samplepod created
```

You'll note that pod fails to spawn successfully. If you check the Multus logs, you'll see an entry such as:

```
2018-12-18T21:41:32Z [error] GetNetworkDelegates: namespace isolation enabled, annotation violates permission, pod is in namespace development but refers to target namespace privileged
```

This error expresses that the pod resides in the namespace named `development` but refers to a `NetworkAttachmentDefinition` outside of that namespace, in this case, the namespace named `privileged`.

In a positive example, you'd instead create the `NetworkAttachmentDefinition` in the `development` namespace, and you'd have an annotation that either A. does not reference a namespace, or B. refers to the same annotation.

A positive example may be:

```
# Create the same NetworkAttachmentDefinition as above, however in the development namespace
[user@kube-master ~]$ kubectl create -f cr.yml -n development
networkattachmentdefinition.k8s.cni.cncf.io/macvlan-conf created

# Show the yaml for a sample pod which references macvlan-conf without a namspace/ format
[user@kube-master ~]$ cat positive.example.pod
apiVersion: v1
kind: Pod
metadata:
  name: samplepod
  annotations:
    k8s.v1.cni.cncf.io/networks: macvlan-conf
spec:
  containers:
  - name: samplepod
    command: ["/bin/bash", "-c", "sleep 2000000000000"]
    image: dougbtv/centos-network

# Create that pod.
[user@kube-master ~]$ kubectl create -f positive.example.pod -n development
pod/samplepod created

# We can see that this pod has been launched successfully.
[user@kube-master ~]$ kubectl get pods -n development
NAME        READY   STATUS    RESTARTS   AGE
samplepod   1/1     Running   0          31s
```

### Allow specific namespaces to be used across namespaces when using namespace isolation

The `globalNamespaces` configuration option is only used when `namespaceIsolation` is set to true. `globalNamespaces` specifies a comma-delimited list of namespaces which can be referred to from outside of any given namespace in which a pod resides.

```
  "globalNamespaces": "default,namespace-a,namespace-b",
```

Note that when using `globalNamespaces` the `default` namespace must be specified in the list if you wish to use that namespace, when `globalNamespaces` is not set, the `default` namespace is implied to be used across namespaces.

### Specify default cluster network in Pod annotations

Users may also specify the default network for any given pod (via annotation), for cases where there are multiple cluster networks available within a Kubernetes cluster.

Example use cases may include:

1. During a migration from one default network to another (e.g. from Flannel to Calico), it may be practical if both network solutions are able to operate in parallel. Users can then control which network a pod should attach to during the transition period.
2. Some users may deploy multiple cluster networks for the sake of their security considerations, and may desire to specify the default network for individual pods.

Follow these steps to specify the default network on a pod-by-pod basis:

1. First, you need to define all your cluster networks as network-attachment-definition objects.

2. Next, you can specify the network you want in pods with the `v1.multus-cni.io/default-network` annotation. Pods which do not specify this annotation will keep using the CNI as defined in the Multus config file.

```yaml
apiVersion: v1
kind: Pod
metadata:
name: pod-example
annotations:
 v1.multus-cni.io/default-network: calico-conf
...
```
