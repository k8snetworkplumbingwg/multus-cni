# Metrics

Multus daemon serves Prometheus metrics at `/metrics` when `metricsPort` is configured.
Endpoint is not enabled by default.

| Metric                                                         | Meaning                                    |
| -------------------------------------------------------------- | ------------------------------------------ |
| `multus_cni_plugin_lookup_total{plugin,outcome}`               | CNI plugin lookup attempts                 |
| `multus_cni_plugin_execution_total{plugin,command,outcome}`    | CNI plugin execution attempts              |
| `multus_cni_plugin_execution_duration_seconds{plugin,command}` | CNI plugin execution duration              |
| `multus_server_request_total{handler,code,method}`             | Requests handled by the daemon HTTP server |
