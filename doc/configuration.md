## Multus-cni Configuration Reference

Following is the example of multus config file, in `/etc/cni/net.d/`.
(`"Note1"` and `"Note2"` are just comments, so you can remove them at your configuration)

```
{
    "name": "node-cni-network",
    "type": "multus",
    "kubeconfig": "/etc/kubernetes/node-kubeconfig.yaml",
    "confDir": "/etc/cni/multus/net.d",
    "cniDir": "/var/lib/cni/multus",
    "binDir": "/opt/cni/bin",
    "logFile": "/var/log/multus.log",
    "logLevel": "debug",
    "capabilities": {
        "portMappings": true
    },    
    "readinessindicatorfile": "",
    "namespaceIsolation": false,
"Note1":"NOTE: you can set clusterNetwork+defaultNetworks OR delegates!!",
    "clusterNetwork": "defaultCRD",
    "defaultNetworks": ["sidecarCRD", "flannel"],
    "systemNamespaces": ["kube-system", "admin"],
    "multusNamespace": "kube-system",
"Note2":"NOTE: If you use clusterNetwork/defaultNetworks, delegates is ignored",
    "delegates": [{
        "type": "weave-net",
        "hairpinMode": true
    }, {
        "type": "macvlan",
        ... (snip)
    }]
}
```

* `name` (string, required): the name of the network
* `type` (string, required): &quot;multus&quot;
* `confDir` (string, optional): directory for CNI config file that multus reads. default `/etc/cni/multus/net.d`
* `cniDir` (string, optional): Multus CNI data directory, default `/var/lib/cni/multus`
* `binDir` (string, optional): additional directory for CNI plugins which multus calls, in addition to the default (the default is typically set to `/opt/cni/bin`)
* `kubeconfig` (string, optional): kubeconfig file for the out of cluster communication with kube-apiserver. See the example [kubeconfig](https://github.com/intel/multus-cni/blob/master/doc/node-kubeconfig.yaml). If you would like to use CRD (i.e. network attachment definition), this is required
* `logFile` (string, optional): file path for log file. multus puts log in given file
* `logLevel` (string, optional): logging level ("debug", "error", "verbose", or "panic")
* `namespaceIsolation` (boolean, optional): Enables a security feature where pods are only allowed to access `NetworkAttachmentDefinitions` in the namespace where the pod resides. Defaults to false.
* `capabilities` ({}list, optional): [capabilities](https://github.com/containernetworking/cni/blob/master/CONVENTIONS.md#dynamic-plugin-specific-fields-capabilities--runtime-configuration) supported by at least one of the delegates. (NOTE: Multus only supports portMappings capability for now). See the [example](https://github.com/intel/multus-cni/blob/master/examples/multus-ptp-portmap.conf).
* `readinessindicatorfile`: The path to a file whose existance denotes that the default network is ready

User should chose following parameters combination (`clusterNetwork`+`defaultNetworks` or `delegates`):

* `clusterNetwork` (string, required): default CNI network for pods, used in kubernetes cluster (Pod IP and so on): name of network-attachment-definition, CNI json file name (without extention, .conf/.conflist) or directory for CNI config file
* `defaultNetworks` ([]string, required): default CNI network attachment: name of network-attachment-definition, CNI json file name (without extention, .conf/.conflist) or directory for CNI config file
* `systemNamespaces` ([]string, optional): list of namespaces for Kubernetes system (namespaces listed here will not have `defaultNetworks` added)
* `multusNamespace` (string, optional): namespace for `clusterNetwork`/`defaultNetworks`
* `delegates` ([]map,required): number of delegate details in the Multus

### Network selection flow of clusterNetwork/defaultNetworks

Multus will find network for clusterNetwork/defaultNetworks as following sequences:

1. CRD object for given network name, in 'kube-system' namespace
1. CNI json config file in `confDir`. Given name should be without extention, like .conf/.conflist. (e.g. "test" for "test.conf")
1. Directory for CNI json config file. Multus will find alphabetically first file for the network
1. Multus failed to find network. Multus raise error message

## Miscellaneous config

### Default Network Readiness Indicator

You may wish for your "default network" (that is, the CNI plugin & its configuration you specify as your default delegate) to become ready before you attach networks with Multus. This is disabled by default and not used unless you add the readiness check option(s) to your CNI configuration file.

For example, if you use Flannel as a default network, the recommended method for Flannel to be installed is via a daemonset that also drops a configuration file in `/etc/cni/net.d/`. This may apply to other plugins that place that configuration file upon their readiness, hence, Multus uses their configuration filename as a semaphore and optionally waits to attach networks to pods until that file exists.

In this manner, you may prevent pods from crash looping, and instead wait for that default network to be ready.

Only one option is necessary to configure this functionality:

* `readinessindicatorfile`: The path to a file whose existance denotes that the default network is ready.

*NOTE*: If `readinessindicatorfile` is unset, or is an empty string, this functionality will be disabled, and is disabled by default.


### Logging

You may wish to enable some enhanced logging for Multus, especially during the process where you're configuring Multus and need to understand what is or isn't working with your particular configuration.

Multus will always log via `STDERR`, which is the standard method by which CNI plugins communicate errors, and these errors are logged by the Kubelet. This method is always enabled.

#### Writing to a Log File

Optionally, you may have Multus log to a file on the filesystem. This file will be written locally on each node where Multus is executed. You may configure this via the `LogFile` option in the CNI configuration. By default this additional logging to a flat file is disabled.

For example in your CNI configuration, you may set:

```
    "LogFile": "/var/log/multus.log",
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
    "LogLevel": "debug",
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
