# Multus Thick plugin

Multus CNI can also be deployed using a thick plugin architecture, which is
characterized by a client/server architecture.

The client - which will be referred to as "shim" - is a binary executable
located on the Kubernetes node's file-system that
[speaks CNI](https://github.com/containernetworking/cni/blob/master/SPEC.md#section-2-execution-protocol):
the runtime - Kubernetes - passes parameters to the plugin via environment
variables and configuration - which is passed via stdin.
The plugin returns a result on stdout on success, or an error on stderr if the
operation fails. Configuration and results are a JSON encoded string.

Once the shim is invoked by the runtime (Kubernetes) it will contact the
multus-daemon (server) via a unix domain socket which is bind mounted to the
host's file-system; the multus-daemon is the one that will do all the
heavy-pulling: fetch the delegate CNI configuration from the corresponding
`net-attach-def`, compute the `RuntimeConfig`, and finally, invoke the delegate.

It will then return the result of the operation back to the client.

Please refer to the diagram below for a visual representation of the flow
described above:

```
┌─────────┐             ┌───────┐           ┌────────┐             ┌──────────┐
│         │ cni ADD/DEL │       │ REST POST │        │ cni ADD/DEL │          │
│ runtime ├────────────►│ shim  │===========│ daemon ├────────────►│ delegate │
│         │<------------│       │           │        │<------------│          │
└─────────┘             └───────┘           └────────┘             └──────────┘
```

## How to use it

### Configure Deployment

If your delegate CNI plugin requires some files which is in container host, please update
update `deployments/multus-daemonset-thick.yml` to add directory into multus-daemon pod.
For example, flannel requires `/run/flannel/subnet.env`, so you need to mount this directory
into the multus-daemon pod.

Required directory/files are different for each CNI plugin, so please refer your CNI plugin.

### Deployment

There is a dedicated multus daemonset specification for users wanting to use
this thick plugin variant. This reference deployment spec of multus can be
deployed by following these commands:

```bash
kubectl apply -f deployments/multus-daemonset-thick.yml
```

### Command line parameters

The available command line parameters are:

- `config`: Defaults to `"/etc/cni/net.d/multus.d/daemon-config.json"`
- `version`: Prints the daemon config version and exits

### Server / Daemon configuration

The server configuration is encoded in JSON, and allows the following keys:

- `"chrootDir"`: Specify the directory which points to host root from the pod. See 'Chroot configuration' section for the details.
- `"socketDir"`: Specify the location where the unix domain socket used
for client/server communication will be located. This is the location where the
**Daemon** will read the configuration from. Defaults to `"/run/multus"`.
- `"metricsPort"`: Metrics port (of multus' metric exporter); by default, no port
is provided.
- `"logFile"`: the path to where the daemon logs will be persisted.
- `"logLevel"`: the logging level for the multus daemon logs.
- `"logToStderr"`: enable this to have the daemon multus logs echoed to stderr
as well. By default, it is disabled.
- `"auxiliaryCNIChainName"`: set a value to execute chained cni configurations from disk in an auxiliary CNI chain (see details in [configuration.md](configuration.md))

In addition, you can add any configuration which is in [configuration reference](https://github.com/k8snetworkplumbingwg/multus-cni/blob/master/docs/configuration.md#multus-cni-configuration-reference). Server configuration override multus CNI configuration (e.g. `/etc/cni/net.d/00-multus.conf`)

Below you can see an example of the daemon configuration:
```json
{
        "chrootDir": "/hostroot",
        "confDir": "/host/etc/cni/net.d",
        "logToStderr": true,
        "logLevel": "verbose",
        "logFile": "/tmp/multus.log",
        "binDir": "/opt/cni/bin",
        "cniDir": "/var/lib/cni/multus",
        "socketDir": "/host/run/multus/",
        "cniVersion": "0.3.1",
        "cniConfigDir": "/host/etc/cni/net.d",
        "multusConfigFile": "auto",
        "multusAutoconfigDir": "/host/etc/cni/net.d"
    }
```

### Client / Shim configuration

The multus shim configuration is encoded in JSON, and essentially is just a
regular CNI configuration, usually available in `/etc/cni/net.d/00-multus.conf`.

It allows the following keys:

- `"cniVersion"`: the CNI version for the Multus CNI plugin.
- `"logFile"`:  the path to where the daemon logs will be persisted.
- `"logLevel"`: the logging level for the multus daemon logs.
- `"logToStderr"`: enable this to have the daemon multus logs echoed to stderr
  as well. By default, it is disabled.

#### Chroot configuration

In thick plugin case, delegate CNI plugin is executed by multus-daemon from Pod, hence if the delegate CNI requires resources in container host, for example unix socket or even file, then CNI plugin is failed to execute because multus-daemon runs in Pod. Multus-daemon supports "chrootDir" option which executes delegate CNI under chroot (to container host).

This configuration is enabled in deployments/multus-daemonset-thick.yml as default.
