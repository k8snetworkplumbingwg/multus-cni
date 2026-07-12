# Multus CNI Helm Chart

Helm chart for deploying [Multus CNI](https://github.com/k8snetworkplumbingwg/multus-cni) on Kubernetes. Multus is a meta-CNI plugin that lets pods attach multiple network interfaces.

## Prerequisites

- Kubernetes `>= 1.21`
- Helm 3
- A default CNI plugin already installed on the cluster (for example Calico, Cilium, or Flannel)
- Nodes configured with standard CNI paths:
  - `/etc/cni/net.d` for CNI configuration
  - `/opt/cni/bin` for CNI binaries
- For metrics scraping with `metrics.enabled=true`: [Prometheus Operator](https://github.com/prometheus-operator/prometheus-operator) with the `PodMonitor` CRD installed

## Quick start

Install the chart into `kube-system` with the default **thin** plugin:

```bash
helm upgrade --install multus-cni ./deploy/charts/multus-cni \
  --namespace kube-system
```

Install with the **thick** plugin (recommended for most environments):

```bash
helm upgrade --install multus-cni ./deploy/charts/multus-cni \
  --namespace kube-system \
  --set pluginType=thick
```

## Plugin types

| Value | Description |
|-------|-------------|
| `thin` | Classic deployment. Lower resource usage. Default. |
| `thick` | Client/server deployment with `multus-daemon` and `multus-shim`. Supports metrics and additional features. See [thick plugin docs](../../../docs/thick-plugin.md). |

Image tags are selected automatically when `image.tag` is empty:

- `thin`: `<appVersion>` (for example `v4.3.0`)
- `thick`: `<appVersion>-thick` (for example `v4.3.0-thick`)

## What gets installed

| Resource | Notes |
|----------|-------|
| DaemonSet `multus-cni` | Runs on every node (`thin` or `thick` depending on `pluginType`) |
| ServiceAccount | Created when `serviceAccount.create` is `true` |
| ClusterRole / ClusterRoleBinding | Created when `rbac.create` is `true` |
| ConfigMap `multus-daemon-config` | Created only for `pluginType=thick` |
| PodMonitor | Created when `metrics.enabled=true` and `pluginType=thick` |
| CRD `network-attachment-definitions.k8s.cni.cncf.io` | Installed from `crds/` on first install |

## CRDs

The `NetworkAttachmentDefinition` CRD is shipped in the chart `crds/` directory. Helm installs CRDs before other resources on the initial install.

- CRDs are **not** removed on `helm uninstall`
- CRD upgrades may require a manual apply if Helm cannot update an existing CRD in place

## Verify the installation

```bash
kubectl -n kube-system get daemonset multus-cni
kubectl -n kube-system get pods -l app=multus-cni
kubectl get crd network-attachment-definitions.k8s.cni.cncf.io
```

Check that Multus CNI config was written on a node:

```bash
kubectl -n kube-system logs -l app=multus-cni --tail=50
```

When metrics are enabled:

```bash
kubectl -n kube-system get podmonitor
```

## Configuration

### Common examples

Use a custom image tag:

```bash
helm upgrade --install multus-cni ./deploy/charts/multus-cni \
  --namespace kube-system \
  --set image.tag=4.3.0-thick
```

Deploy to a different namespace:

```bash
helm upgrade --install multus-cni ./deploy/charts/multus-cni \
  --namespace multus \
  --create-namespace \
  --set namespaceOverride=multus
```

Use custom CNI host paths (for distributions with non-standard layouts):

```bash
helm upgrade --install multus-cni ./deploy/charts/multus-cni \
  --namespace kube-system \
  --set cniConfig.hostPath=/etc/cni/net.d \
  --set cniBin.hostPath=/opt/cni/bin
```

Set log level for thin or thick deployments:

```bash
helm upgrade --install multus-cni ./deploy/charts/multus-cni \
  --namespace kube-system \
  --set logLevel=debug
```

Enable Prometheus metrics (thick plugin only):

```bash
helm upgrade --install multus-cni ./deploy/charts/multus-cni \
  --namespace kube-system \
  --set pluginType=thick \
  --set metrics.enabled=true
```

Add extra environment variables or volumes:

```bash
helm upgrade --install multus-cni ./deploy/charts/multus-cni \
  --namespace kube-system \
  --set-json 'additionalEnv=[{"name":"EXAMPLE","value":"true"}]'
```

Override thin plugin container arguments:

```yaml
thin:
  container:
    args:
      - --multus-conf-file=auto
      - --multus-cni-conf-dir={{ .Values.cniConfig.mountPath }}
      - --multus-log-level={{ .Values.logLevel }}
```

### Values reference

| Key | Description | Default |
|-----|-------------|---------|
| `pluginType` | Deployment mode: `thin` or `thick` | `thin` |
| `namespaceOverride` | Override the release namespace for chart resources | `""` |
| `nameOverride` | Override the chart name used in labels | `""` |
| `fullnameOverride` | Override the generated full name | `""` |
| `image.registry` | Container image registry | `ghcr.io` |
| `image.repository` | Container image repository | `k8snetworkplumbingwg/multus-cni` |
| `image.tag` | Image tag. Empty uses the default tag for `pluginType` | `""` |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `cniVersion` | CNI spec version | `0.3.1` |
| `logLevel` | Log level for thin plugin args and thick daemon config | `verbose` |
| `thin.container.command` | Thin plugin main container command | `/thin_entrypoint` |
| `thin.container.args` | Thin plugin main container args (Helm-templated) | see `values.yaml` |
| `thin.container.resources` | Thin plugin main container CPU/memory | see `values.yaml` |
| `thin.initContainer.command` | Thin plugin init container command | `/install_multus` |
| `thin.initContainer.args` | Thin plugin init container args (Helm-templated) | see `values.yaml` |
| `thin.initContainer.resources` | Thin plugin init container CPU/memory | see `values.yaml` |
| `thick.container.command` | Thick plugin main container command | `/usr/src/multus-cni/bin/multus-daemon` |
| `thick.container.args` | Thick plugin main container args (Helm-templated) | `[]` |
| `thick.container.resources` | Thick plugin main container CPU/memory | see `values.yaml` |
| `thick.initContainer.command` | Thick plugin init container command | `/usr/src/multus-cni/bin/install_multus` |
| `thick.initContainer.args` | Thick plugin init container args (Helm-templated) | see `values.yaml` |
| `thick.initContainer.resources` | Thick plugin init container CPU/memory | see `values.yaml` |
| `thick.daemonConfig` | Helm-templated JSON for the thick daemon ConfigMap | see `values.yaml` |
| `metrics.enabled` | Expose metrics port and create a PodMonitor (thick only) | `false` |
| `metrics.port` | Metrics listen port | `38080` |
| `metrics.podMonitor.interval` | Prometheus scrape interval | `60s` |
| `metrics.podMonitor.scrapeTimeout` | Prometheus scrape timeout | `10s` |
| `metrics.podMonitor.relabelings` | Prometheus relabeling rules | see `values.yaml` |
| `metrics.podMonitor.labels` | Extra labels on the PodMonitor | `release: multus-cni` |
| `cniConfig.hostPath` | Host path for CNI config directory | `/etc/cni/net.d` |
| `cniConfig.mountPath` | Mount path inside the pod for CNI config | `/host/etc/cni/net.d` |
| `cniBin.hostPath` | Host path for CNI binaries | `/opt/cni/bin` |
| `cniBin.mountPath` | Mount path inside the pod for CNI binaries | `/host/opt/cni/bin` |
| `additionalEnv` | Extra container environment variables | `[]` |
| `additionalVolumes` | Extra pod volumes | `[]` |
| `additionalVolumeMounts` | Extra container volume mounts | `[]` |
| `rbac.create` | Create ClusterRole and ClusterRoleBinding | `true` |
| `serviceAccount.create` | Create a ServiceAccount | `true` |
| `serviceAccount.name` | ServiceAccount name | `multus-cni` |
| `updateStrategy.type` | DaemonSet update strategy | `RollingUpdate` |
| `securityContext` | Security context for Multus containers | see `values.yaml` |
| `tolerations` | Pod tolerations | tolerate all `NoSchedule` and `NoExecute` taints |
| `nodeSelector` | Pod node selector | `{}` |
| `affinity` | Pod affinity rules | `{}` |
| `priorityClassName` | Pod priority class | `""` |

### Container commands and arguments

Each plugin type defines its own container and init container settings under `thin.*` or `thick.*`:

| Key | Description |
|-----|-------------|
| `command` | Container entrypoint binary |
| `args` | Container arguments, rendered with Helm `tpl` |
| `resources` | CPU and memory requests/limits for that container |

Default thin plugin args reference `cniConfig.mountPath`, `cniBin.mountPath`, and `logLevel`. Default thick init container args reference `cniBin.mountPath`.

Args entries that contain `{{` must be quoted in `values.yaml` to remain valid YAML:

```yaml
thin:
  container:
    args:
      - --multus-conf-file=auto
      - "--multus-cni-conf-dir={{ .Values.cniConfig.mountPath }}"
      - "--multus-log-level={{ .Values.logLevel }}"
```

### Thick daemon config

`thick.daemonConfig` is a JSON string rendered with Helm `tpl`. The default template references other values such as `cniVersion`, `logLevel`, `metrics.port`, and `cniConfig.mountPath`, so most changes can be made through those top-level keys.

To override the full daemon config, set `thick.daemonConfig` in a values file:

```yaml
thick:
  daemonConfig: |
    {
      "chrootDir": "/hostroot",
      "cniVersion": "{{ .Values.cniVersion }}",
      "logLevel": "{{ .Values.logLevel }}",
      "logToStderr": true,
      "metricsPort": {{ .Values.metrics.port }},
      "cniConfigDir": "{{ .Values.cniConfig.mountPath }}",
      "multusAutoconfigDir": "{{ .Values.cniConfig.mountPath }}",
      "multusConfigFile": "auto",
      "socketDir": "{{ print .Values.cniConfig.mountPath "/run/multus/" }}"
    }
```

The rendered config is stored in ConfigMap `multus-daemon-config` as `daemon-config.json`.

### Metrics

Metrics are only available with `pluginType=thick`. When `metrics.enabled=true`, the chart:

1. Exposes port `metrics` on the `multus-cni` container (default `38080`)
2. Sets `metricsPort` in the thick daemon config
3. Creates a `PodMonitor` for Prometheus Operator

Match the PodMonitor to your Prometheus instance by setting `metrics.podMonitor.labels` (for example `release: kube-prometheus-stack`).

## Upgrade

```bash
helm upgrade multus-cni ./deploy/charts/multus-cni \
  --namespace kube-system
```

When switching plugin types, the chart replaces the DaemonSet template. Expect a rolling restart of Multus pods on all nodes.

Config changes to `thick.daemonConfig` trigger a rolling restart via a `checksum/configmap` pod annotation.

## Uninstall

```bash
helm uninstall multus-cni --namespace kube-system
```

This removes the DaemonSet, RBAC, ServiceAccount, thick-plugin ConfigMap, and PodMonitor. It does **not** remove:

- The `NetworkAttachmentDefinition` CRD
- CNI binaries or config files already written to nodes

## Further reading

- [Multus quickstart](../../../docs/quickstart.md)
- [How to use Multus](../../../docs/how-to-use.md)
- [Configuration reference](../../../docs/configuration.md)
- [Thick plugin](../../../docs/thick-plugin.md)
