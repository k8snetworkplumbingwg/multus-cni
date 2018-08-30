## Dockerfile build

This is used for distribution of Multus in a Docker image.

Typically you'd build this from the root of your Multus clone, as such:

```
$ docker build -t dougbtv/multus .
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

This is an entrypoint script for Multus CNI to overlay its
binary and configuration into locations in a filesystem.
The configuration & binary file will be copied to the 
corresponding configuration directory.

./entrypoint.sh
    -h --help
    --cni-conf-dir=/host/etc/cni/net.d
    --cni-bin-dir=/host/opt/cni/bin
    --multus-conf-file=/usr/src/multus-cni/images/70-multus.conf
    --multus-bin-file=/usr/src/multus-cni/bin/multus
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

Originally inspired by and is a portmanteau of the [Flannel daemonset](https://github.com/coreos/flannel/blob/master/Documentation/kube-flannel.yml), the [Calico Daemonset](https://github.com/projectcalico/calico/blob/master/v2.0/getting-started/kubernetes/installation/hosted/k8s-backend-addon-manager/calico-daemonset.yaml), and the [Calico CNI install bash script](https://github.com/projectcalico/cni-plugin/blob/be4df4db2e47aa7378b1bdf6933724bac1f348d0/k8s-install/scripts/install-cni.sh#L104-L153).