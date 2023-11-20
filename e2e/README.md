## Multus e2e test with kind

### Prerequisite

To run the e2e test, you need the following components:

- curl
- jinjanator
- docker

### How to test e2e

```
$ git clone https://github.com/k8snetworkplumbingwg/multus-cni.git
$ cd multus-cni/e2e
$ ./get_tools.sh
$ ./generate_yamls.sh
$ ./setup_cluster.sh
$ ./test-simple-macvlan1.sh
```

### How to teardown cluster

```
$ ./teardown.sh
```
