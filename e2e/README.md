## Multus e2e test with kind

### Prerequisite

To run the e2e test, you need the following components:

- curl
- jinjanator (optional)
- docker

### How to test e2e

```
$ git clone https://github.com/k8snetworkplumbingwg/multus-cni.git
$ cd multus-cni/e2e
$ ./get_tools.sh
```

If you have `jinjanator` you can generate the YAML with:

```
$ ./generate_yamls.sh
```

Alternatively, if you have trouble with it, use the `sed` script.

```
$ ./e2e/sed_generate_yaml.sh
```

Then, setup the cluster

```
$ ./setup_cluster.sh
$ ./test-simple-macvlan1.sh
```

### How to teardown cluster

```
$ ./teardown.sh
```
