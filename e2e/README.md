## Multus e2e test with kind

### How to test e2e


```
$ git clone https://github.com/intel/multus-cni.git
$ cd multus-cni/e2e
$ ./get_tools.sh
$ ./setup_cluster.sh
$ ./test-simple-macvlan1.sh
```

### How to teardown cluster

```
$ ./teardown.sh
```
