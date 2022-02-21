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

### Deployment

There is a dedicated multus daemonset specification for users wanting to use
this thick plugin variant. This reference deployment spec of multus can be
deployed by following these commands:

```bash
kubectl apply -f deployments/multus-daemonset-thick-plugin.yml
```

### Command line parameters

Multus thick plugin variant accepts the same
[entrypoint arguments](https://github.com/k8snetworkplumbingwg/multus-cni/blob/master/docs/how-to-use.md#entrypoint-script-parameters)
its thin counterpart allows - with the following exceptions:

- `skip-multus-binary-copy`
- `restart-crio`
- `cleanup-config-on-exit`
- `rename-conf-file`

It is important to refer that these are command line parameters to the golang
binary; as such, they should be passed using a single dash ("-") e.g.
`-additional-bin-dir=/opt/multus/bin`, `-multus-log-level=debug`, etc.

Furthermore, it also accepts a new command line parameter, where the user
specifies the path to the server configuration:

- `config`: Defaults to `"/etc/cni/net.d/multus.d/daemon-config.json"`

### Server configuration

The server configuration is encoded in JSON, and allows the following keys:

- `"confDir"`: specifies the path to the CNI configuration directory.
- `"cniDir"`: specifies the path to the multus CNI cache.
- `"binDir"`: specifies the path to the CNI binary executables.
- `"logFile"`: specifies where the daemon log file will be persisted.
- `"logLevel"`: indicates the logging level of the multus daemon. 
- `"logToStderr"`: Whether or not to also log to stderr. Default to `true`.
- `"socketDir"`: Specify the location where the unix domain socket used for
client/server communication will be located. Defaults to `"/var/run/multus-cni/"`.

