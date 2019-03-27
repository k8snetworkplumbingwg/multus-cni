## How to use multus-cni?

### Prerequisites

* Kubelet configured to use CNI 
* Kubernetes version with CRD support (generally )

Your Kubelet(s) must be configured to run with the CNI network plugin. Please see [Kubernetes document for CNI](https://kubernetes.io/docs/concepts/extend-kubernetes/compute-storage-net/network-plugins/#cni) for more details.

### Install multus

Generally we recommend two options: Manually place a Multus binary in your `/opt/cni/bin`, or use our [quick-start method](quickstart.md) -- which creates a daemonset that has an opinionated way of how to install & configure Multus CNI (recommended).

*Copy Multus Binary into place*

You may acquire the Multus binary via compilation (see the [developer guide](development.md)) or download the a binary from the [GitHub releases](https://github.com/intel/multus-cni/releases) page. Copy multus binary into CNI binary directory, usually `/opt/cni/bin`. Perform this on all nodes in your cluster (master and nodes).

    $ cp multus /opt/cni/bin

*Via Daemonset method*

As a [quickstart](quickstart.md), you may apply these YAML files (included in the clone of this repository). Run this command (typically you would run this on the master, or wherever you have access to the `kubectl` command to manage your cluster). 

    $ cat ./images/{multus-daemonset.yml,flannel-daemonset.yml} | kubectl apply -f -

If you need more comprehensive detail, continue along with this guide, otherwise, you may wish to either [follow the quickstart guide]() or skip to the ['Create network attachment definition'](#create-network-attachment-definition) section.

### Set up conf file in /etc/cni/net.d/ (Installed automatically by Daemonset)

**If you use daemonset to install multus, skip this section and go to "Create network attachment"**

You put CNI config file in `/etc/cni/net.d`. Kubernetes CNI runtime uses the alphabetically first file in the directory. (`"NOTE1"`, `"NOTE2"` are just comments, you can remove them at your configuration)

Execute following commands at all Kubernetes nodes (i.e. master and minions)

```
$ mkdir -p /etc/cni/net.d
$ cat >/etc/cni/net.d/30-multus.conf <<EOF
{
  "name": "multus-cni-network",
  "type": "multus",
  "readinessindicatorfile": "/var/run/flannel/subnet.env",
  "delegates": [
    {
      "NOTE1": "This is example, wrote your CNI config in delegates",
      "NOTE2": "If you use flannel, you also need to run flannel daemonset before!",
      "type": "flannel",
      "name": "flannel.1",
      "delegate": {
        "isDefaultGateway": true
      }
    }
  ],
  "kubeconfig": "/etc/cni/net.d/multus.d/multus.kubeconfig"
}
EOF
```

For the detail, please take a look into [Configuration Reference](configuration.md)

**NOTE: You can use "clusterNetwork"/"defaultNetworks" instead of "delegates", see []() for the detail**

As above config, you need to set `"kubeconfig"` in the config file for NetworkAttachmentDefinition(CRD).

##### Which network will be used for "Pod IP"?

In case of "delegates", the first delegates network will be used for "Pod IP". Otherwise, "clusterNetwork" will be used for "Pod IP".

#### Create ServiceAccount, ClusterRole and its binding

Create resources for multus to access CRD objects as following command:

```
# Execute following commands at Kubernetes master
$ cat <<EOF | kubectl create -f -
apiVersion: v1
kind: ServiceAccount
metadata:
  name: multus
  namespace: kube-system
---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: multus
rules:
  - apiGroups: ["k8s.cni.cncf.io"]
    resources:
      - '*'
    verbs:
      - '*'
  - apiGroups:
      - ""
    resources:
      - pods
      - pods/status
    verbs:
      - get
      - update
---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: multus
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: multus
subjects:
- kind: ServiceAccount
  name: multus
  namespace: kube-system
EOF
```

#### Set up kubeconfig file

Create kubeconfig at master node as following commands:

```
# Execute following command at Kubernetes master
$ mkdir -p /etc/cni/net.d/multus.d
$ SERVICEACCOUNT_CA=$(kubectl get secrets -n=kube-system -o json | jq -r '.items[]|select(.metadata.annotations."kubernetes.io/service-account.name"=="multus")| .data."ca.crt"')
$ SERVICEACCOUNT_TOKEN=$(kubectl get secrets -n=kube-system -o json | jq -r '.items[]|select(.metadata.annotations."kubernetes.io/service-account.name"=="multus")| .data.token' | base64 -d )
$ KUBERNETES_SERVICE_PROTO=$(kubectl get all -o json | jq -r .items[0].spec.ports[0].name)
$ KUBERNETES_SERVICE_HOST=$(kubectl get all -o json | jq -r .items[0].spec.clusterIP)
$ KUBERNETES_SERVICE_PORT=$(kubectl get all -o json | jq -r .items[0].spec.ports[0].port)
$ cat > /etc/cni/net.d/multus.d/multus.kubeconfig <<EOF
# Kubeconfig file for Multus CNI plugin.
apiVersion: v1
kind: Config
clusters:
- name: local
  cluster:
    server: ${KUBERNETES_SERVICE_PROTOCOL:-https}://${KUBERNETES_SERVICE_HOST}:${KUBERNETES_SERVICE_PORT}
    certificate-authority-data: ${SERVICEACCOUNT_CA}
users:
- name: multus
  user:
    token: "${SERVICEACCOUNT_TOKEN}"
contexts:
- name: multus-context
  context:
    cluster: local
    user: multus
current-context: multus-context
EOF
```

Copy `/etc/cni/net.d/multus.d/multus.kubeconfig` into other Kubernetes nodes
**NOTE: Recommend to exec 'chmod 600 /etc/cni/net.d/multus.d/multus.kubeconfig' to keep secure**

```
$ scp /etc/cni/net.d/multus.d/multus.kubeconfig ...
```

### Setup CRDs (daemonset automatically does)

**If you use daemonset to install multus, skip this section and go to "Create network attachment"**

Create CRD definition in Kubernetes as following command at master node:

```
# Execute following command at Kubernetes master
$ cat <<EOF | kubectl create -f -
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: network-attachment-definitions.k8s.cni.cncf.io
spec:
  group: k8s.cni.cncf.io
  version: v1
  scope: Namespaced
  names:
    plural: network-attachment-definitions
    singular: network-attachment-definition
    kind: NetworkAttachmentDefinition
    shortNames:
    - net-attach-def
  validation:
    openAPIV3Schema:
      properties:
        spec:
          properties:
            config:
                 type: string
EOF
```

### Create network attachment definition

The 'NetworkAttachmentDefinition' is used to setup the network attachment, i.e. secondary interface for the pod, There are two ways to configure the 'NetworkAttachmentDefinition' as following:

- NetworkAttachmentDefinition with json CNI config
- NetworkAttachmentDefinition with CNI config file

#### NetworkAttachmentDefinition with json CNI config:

Following command creates NetworkAttachmentDefinition. CNI config is in `config:` field.

```
# Execute following command at Kubernetes master
$ cat <<EOF | kubectl create -f -
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: macvlan-conf-1
spec:
  config: '{
            "cniVersion": "0.3.0",
            "type": "macvlan",
            "master": "eth1",
            "mode": "bridge",
            "ipam": {
                "type": "host-local",
                "ranges": [
                    [ {
                         "subnet": "10.10.0.0/16",
                         "rangeStart": "10.10.1.20",
                         "rangeEnd": "10.10.3.50",
                         "gateway": "10.10.0.254"
                    } ]
                ]
            }
        }'
EOF
```

#### NetworkAttachmentDefinition with CNI config file:

If NetworkAttachmentDefinition has no spec, multus find a file in defaultConfDir ('/etc/cni/multus/net.d', with same name in the 'name' field of CNI config.

```
# Execute following command at Kubernetes master
$ cat <<EOF | kubectl create -f -
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: macvlan-conf-2
EOF
```

```
# Execute following commands at all Kubernetes nodes (i.e. master and minions)
$ cat <<EOF > /etc/cni/multus/net.d/macvlan2.conf
{
  "cniVersion": "0.3.0",
  "type": "macvlan",
  "name": "macvlan-conf-2",
  "master": "eth1",
  "mode": "bridge",
  "ipam": {
      "type": "host-local",
      "ranges": [
          [ {
               "subnet": "11.10.0.0/16",
               "rangeStart": "11.10.1.20",
               "rangeEnd": "11.10.3.50"
          } ]
      ]
  }
}
```

### Run pod with network annotation

#### Lauch pod with text annotation

```
# Execute following command at Kubernetes master
$ cat <<EOF | kubectl create -f -
apiVersion: v1
kind: Pod
metadata:
  name: pod-case-01
  annotations:
    k8s.v1.cni.cncf.io/networks: macvlan-conf-1, macvlan-conf-2
spec:
  containers:
  - name: pod-case-01
    image: docker.io/centos/tools:latest
    command:
    - /sbin/init
EOF
```

#### Lauch pod with text annotation for NetworkAttachmentDefinition in different namespace

You can also specify NetworkAttachmentDefinition with its namespace as adding `<namespace>/`

```
# Execute following command at Kubernetes master
$ cat <<EOF | kubectl create -f -
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: macvlan-conf-3
  namespace: testns1
spec:
  config: '{
            "cniVersion": "0.3.0",
            "type": "macvlan",
            "master": "eth1",
            "mode": "bridge",
            "ipam": {
                "type": "host-local",
                "ranges": [
                    [ {
                         "subnet": "12.10.0.0/16",
                         "rangeStart": "12.10.1.20",
                         "rangeEnd": "12.10.3.50"
                    } ]
                ]
            }
        }'
EOF
$ cat <<EOF | kubectl create -f -
apiVersion: v1
kind: Pod
metadata:
  name: pod-case-02
  annotations:
    k8s.v1.cni.cncf.io/networks: testns1/macvlan-conf-3
spec:
  containers:
  - name: pod-case-02
    image: docker.io/centos/tools:latest
    command:
    - /sbin/init
EOF
```

#### Lauch pod with text annotation with interface name

You can also specify interface name as adding `@<ifname>`.

```
# Execute following command at Kubernetes master
$ cat <<EOF | kubectl create -f -
apiVersion: v1
kind: Pod
metadata:
  name: pod-case-03
  annotations:
    k8s.v1.cni.cncf.io/networks: macvlan-conf-1@macvlan1
spec:
  containers:
  - name: pod-case-03
    image: docker.io/centos/tools:latest
    command:
    - /sbin/init
EOF
```

#### Lauch pod with json annotation

```
# Execute following command at Kubernetes master
$ cat <<EOF | kubectl create -f -
apiVersion: v1
kind: Pod
metadata:
  name: pod-case-04
  annotations:
    k8s.v1.cni.cncf.io/networks: '[
            { "name" : "macvlan-conf-1" },
            { "name" : "macvlan-conf-2" }
    ]'
spec:
  containers:
  - name: pod-case-04
    image: docker.io/centos/tools:latest
    command:
    - /sbin/init
EOF
```

#### Lauch pod with json annotation for NetworkAttachmentDefinition in different namespace

You can also specify NetworkAttachmentDefinition with its namespace as adding `"namespace": "<namespace>"`.

```
# Execute following command at Kubernetes master
$ cat <<EOF | kubectl create -f -
apiVersion: v1
kind: Pod
metadata:
  name: pod-case-05
  annotations:
    k8s.v1.cni.cncf.io/networks: '[
            { "name" : "macvlan-conf-1",
              "namespace": "testns1" }
    ]'
spec:
  containers:
  - name: pod-case-05
    image: docker.io/centos/tools:latest
    command:
    - /sbin/init
EOF
```

#### Lauch pod with json annotation with interface

You can also specify interface name as adding `"interfaceRequest": "<ifname>"`.

```
# Execute following command at Kubernetes master
$ cat <<EOF | kubectl create -f -
apiVersion: v1
kind: Pod
metadata:
  name: pod-case-06
  annotations:
    k8s.v1.cni.cncf.io/networks: '[
            { "name" : "macvlan-conf-1",
              "interfaceRequest": "macvlan1" },
            { "name" : "macvlan-conf-2" }
    ]'
spec:
  containers:
  - name: pod-case-06
    image: docker.io/centos/tools:latest
    command:
    - /sbin/init
EOF
```

### Verifying pod network

Following the example of `ip -d address` output of above pod, "pod-case-06":

```
# Execute following command at Kubernetes master
$ kubectl exec -it pod-case-06 -- ip -d address
1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UNKNOWN group default qlen 1
    link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00 promiscuity 0 numtxqueues 1 numrxqueues 1 gso_max_size 65536 gso_max_segs 65535
    inet 127.0.0.1/8 scope host lo
       valid_lft forever preferred_lft forever
    inet6 ::1/128 scope host
       valid_lft forever preferred_lft forever
3: eth0@if11: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1450 qdisc noqueue state UP group default
    link/ether 0a:58:0a:f4:02:06 brd ff:ff:ff:ff:ff:ff link-netnsid 0 promiscuity 0
    veth numtxqueues 1 numrxqueues 1 gso_max_size 65536 gso_max_segs 65535
    inet 10.244.2.6/24 scope global eth0
       valid_lft forever preferred_lft forever
    inet6 fe80::ac66:45ff:fe7c:3a19/64 scope link
       valid_lft forever preferred_lft forever
4: macvlan1@if3: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc noqueue state UNKNOWN group default
    link/ether 4e:6d:7a:4e:14:87 brd ff:ff:ff:ff:ff:ff link-netnsid 0 promiscuity 0
    macvlan mode bridge numtxqueues 1 numrxqueues 1 gso_max_size 65536 gso_max_segs 65535
    inet 10.10.1.22/16 scope global macvlan1
       valid_lft forever preferred_lft forever
    inet6 fe80::4c6d:7aff:fe4e:1487/64 scope link
       valid_lft forever preferred_lft forever
5: net2@if3: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc noqueue state UNKNOWN group default
    link/ether 6e:e3:71:7f:86:f7 brd ff:ff:ff:ff:ff:ff link-netnsid 0 promiscuity 0
    macvlan mode bridge numtxqueues 1 numrxqueues 1 gso_max_size 65536 gso_max_segs 65535
    inet 11.10.1.22/16 scope global net2
       valid_lft forever preferred_lft forever
    inet6 fe80::6ce3:71ff:fe7f:86f7/64 scope link
       valid_lft forever preferred_lft forever
```

| Interface name | Description |
| --- | --- |
| lo | loopback |
| eth0 | Default network interface (flannel) |
| macvlan1 | macvlan interface (macvlan-conf-1) |
| net2 | macvlan interface (macvlan-conf-2) |
