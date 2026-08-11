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

### Kube-burner node-density-cni (Multus)

After `setup_cluster.sh` with the thick Multus daemonset, you can run a CI-sized
Multus-aware node-density-cni workload (secondary macvlan on `eth1`):

```
$ ./test-node-density-cni.sh
```

Override scale with `PODS_PER_NODE` (default `64`).

The run enables kube-burner `podLatency` and writes metrics under
`perf-data/metrics/` (including `podLatencyMeasurement-node-density-cni-<cniVersion>.json`)
for the OVN-K-style [`performance-report`](../.github/workflows/performance-report.yml)
workflow. In GitHub Actions this density lane runs for all CNI versions on the
thick Multus matrix cells; on completion, `performance-report` posts a Pod Ready
Latency summary as a PR comment.

Pass `CNI_VERSION` to match the cluster CNI config (default `0.4.0`):

```
$ CNI_VERSION=0.3.1 ./test-node-density-cni.sh
```

See also [`e2e/perf/README.md`](perf/README.md).

### How to teardown cluster

```
$ ./teardown.sh
```
