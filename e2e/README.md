## Multus e2e test with kind

### How to test e2e

### Pull the code repo to the local
```
$ git clone https://github.com/k8snetworkplumbingwg/multus-cni.git
```
### Download the tool installation package
```
$ cd multus-cni/e2e
$ ./get_tools.sh
```
### Before executing the following script, you need to install j2cil
```
$ ./generate_yamls.sh
```
### Before executing the following script, you need to install docker and kubectl
```
$ ./setup_cluster.sh
$ ./test-simple-macvlan1.sh
```

### How to teardown cluster
```
$ ./teardown.sh
```
