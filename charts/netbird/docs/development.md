# Template development

The chart colocates each component's manifests and configuration helper so a change can be reviewed within one directory.

## Template layout

```text
templates/
├── _helpers.tpl
├── extra-manifests.yaml
├── dashboard/
│   ├── _helpers.tpl
│   └── *.yaml
├── combined/
│   ├── _config.tpl
│   └── *.yaml
├── split/
│   ├── management/
│   │   ├── _config.tpl
│   │   └── *.yaml
│   ├── signal/
│   │   ├── _config.tpl
│   │   └── *.yaml
│   └── relay/
│       ├── _config.tpl
│       └── *.yaml
└── monitoring/
    └── service-monitor.yaml
```

The root `_helpers.tpl` contains chart-wide naming, labels, environment rendering, and credential helpers. Component helpers build only that component's generated configuration or environment contract.

Helm loads helper files recursively. Files beginning with `_` are parsed but are not emitted as Kubernetes manifests. Named templates remain global across the chart, so keep names prefixed and unique even when definitions are in different directories:

```gotemplate
{{ include "netbird.combinedConfig" . }}
```

Do not rely on helper file load order or define the same named template in multiple files.

## Change a component

For a new or changed source configuration field:

1. Add the opinionated input under the component in [`../values.yaml`](../values.yaml).
2. Translate the value in the colocated `_config.tpl` or `_helpers.tpl` file.
3. Update the component Deployment, Secret, Service, or ingress only when the generated Kubernetes contract changes.
4. Update at least one runnable example when users need to configure the field.
5. Render both backend modes and the affected example.

Keep raw source configuration in `overrideConfig`; do not add parallel legacy aliases to the opinionated values.

## ConfigMap rollout checksums

Backend Deployments hash their rendered ConfigMap templates. If a ConfigMap is moved or renamed, update the explicit path used by the Deployment annotation:

```gotemplate
checksum/config: {{ include (print .Template.BasePath "/combined/configmap.yaml") . | sha256sum }}
```

A configuration change then updates the pod template and triggers a rollout. Folder names themselves have no Kubernetes ordering semantics.

## Render and lint

Run the supported mode checks:

```bash
task chart:verify
helm lint --strict charts/netbird
```

Render one example while developing:

```bash
helm template netbird charts/netbird \
  --namespace netbird \
  --values charts/netbird/examples/nginx-ingress/auth0/values.yaml
```

Render one source file when inspecting a focused change:

```bash
helm template netbird charts/netbird \
  --namespace netbird \
  --show-only templates/combined/configmap.yaml
```

For an end-to-end local installation, use `task dev:up`, inspect it with `task dev:status`, and remove the dedicated cluster with `task dev:down`.
