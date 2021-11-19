## Dockerfile build

This is used for distribution of Multus in a Docker image.

Typically you'd build this from the root of your Multus clone, as such:

```
$ docker build -t dougbtv/multus -f images/Dockerfile .
```

---

## Daemonset deployment

You may wish to deploy Multus as a daemonset, you can do so by starting with the example Daemonset shown here:

```
$ kubectl create -f ./images/multus-daemonset.yml
```

Note: The likely best practice here is to build your own image given the Dockerfile, and then push it to your preferred registry, and change the `image` fields in the Daemonset YAML to reference that image.

---

### `entrypoint.sh` parameters

The entrypoint takes named parameters for the configuration

You can get get help with the `--help` flag.

```
$ ./entrypoint.sh --help

This is an entrypoint script for Multus CNI to overlay its binary and
configuration into locations in a filesystem. The configuration & binary file
will be copied to the corresponding configuration directory. When
`--multus-conf-file=auto` is used, 00-multus.conf will be automatically
generated from the CNI configuration file of the master plugin (the first file
in lexicographical order in cni-conf-dir).

./entrypoint.sh
    -h --help
    --cni-conf-dir=/host/etc/cni/net.d
    --multus-conf-file=/usr/src/multus-cni/images/70-multus.conf
    --multus-kubeconfig-file-host=/etc/cni/net.d/multus.d/multus.kubeconfig
```

You must use an `=` to delimit the parameter name and the value. For example you may set a custom `cni-conf-dir` like so:

```
./entrypoint.sh --cni-conf-dir=/special/path/to/cni/configs/
```

Note: You'll noticed that there's a `/host/...` directory from the root for the default for both the `cni-conf-dir` and `cni-bin-dir` as it's intended for the host volumes to be mounted specially under this directory to help in the semantics of which paths belong to the host or container.

---

### Development notes

Example docker run command:

```
$ docker run -it -v /opt/cni/bin/:/host/opt/cni/bin/ -v /etc/cni/net.d/:/host/etc/cni/net.d/ --entrypoint=/bin/bash dougbtv/multus
```

Originally inspired by and is a portmanteau of the [Flannel daemonset](https://github.com/coreos/flannel/blob/master/Documentation/kube-flannel.yml), the [Calico Daemonset](https://docs.projectcalico.org/manifests/calico.yaml), and the [Calico CNI install bash script](https://github.com/projectcalico/cni-plugin/blob/be4df4db2e47aa7378b1bdf6933724bac1f348d0/k8s-install/scripts/install-cni.sh#L104-L153).
