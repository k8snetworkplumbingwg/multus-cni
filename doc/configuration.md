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
    
"Note1":"NOTE: you can set clusterNetwork+defaultNetworks OR delegates!!",
    "clusterNetwork": "defaultCRD",
    "defaultNetworks": ["sidecarCRD", "flannel"],
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
* `binDir` (string, optional): directory for CNI plugins which multus calls. default `/opt/cni/bin`
* `kubeconfig` (string, optional): kubeconfig file for the out of cluster communication with kube-apiserver. See the example [kubeconfig](https://github.com/intel/multus-cni/blob/master/doc/node-kubeconfig.yaml). If you would like to use CRD (i.e. network attachment definition), this is required
* `logFile` (string, optional): file path for log file. multus puts log in given file
* `logLevel` (string, optional): logging level ("debug", "error" or "panic")
* `capabilities` ({}list, optional): [capabilities](https://github.com/containernetworking/cni/blob/master/CONVENTIONS.md#dynamic-plugin-specific-fields-capabilities--runtime-configuration) supported by at least one of the delegates. (NOTE: Multus only supports portMappings capability for now). See the [example](https://github.com/intel/multus-cni/blob/master/examples/multus-ptp-portmap.conf).
* `readinessindicatorfile`: The path to a file whose existance denotes that the default network is ready

User should chose following parameters combination (`clusterNetwork`+`defaultNetworks` or `delegates`):

* `clusterNetwork` (string, required): default CNI network for pods, used in kubernetes cluster (Pod IP and so on): name of network-attachment-definition, CNI json file name (without extention, .conf/.conflist) or directory for CNI config file
* `defaultNetworks` ([]string, required): default CNI network attachment: name of network-attachment-definition, CNI json file name (without extention, .conf/.conflist) or directory for CNI config file
* `delegates` ([]map,required): number of delegate details in the Multus

### Network selection flow of clusterNetwork/defaultNetworks

Multus will find network for clusterNetwork/defaultNetworks as following sequences:

1. CRD object for given network name, in 'default' namespace
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
* `error`
* `panic`

You may configure the logging level by using the `LogLevel` option in your CNI configuration. For example:

```
    "LogLevel": "debug",
```

