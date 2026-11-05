# Template development

The chart colocates each component's manifests so a change can be reviewed within one directory.

## Template layout

```text
templates/
├── _helpers.tpl
├── extra-manifests.yaml
├── dashboard/
│   ├── _helpers.tpl
│   └── *.yaml
├── split/
│   ├── management/
│   │   ├── _config.tpl
│   │   └── *.yaml
│   ├── signal/
│   │   └── *.yaml
│   └── relay/
│       └── *.yaml
└── monitoring/
    └── service-monitor.yaml
```

The root `_helpers.tpl` contains chart-wide naming, labels, environment rendering, and credential helpers. Management's `_config.tpl` builds its JSON document. Signal and relay Deployments translate typed values directly to stable command-line flags; relay also renders Secret-backed environment.

Helm loads helper files recursively. Files beginning with `_` are parsed but are not emitted as Kubernetes manifests. Named templates remain global across the chart, so keep names prefixed and unique.

## Change a component

For a new or changed source configuration field:

1. Add the typed input under the component in [`../values.yaml`](../values.yaml).
2. Translate management JSON in `split/management/_config.tpl`, or translate signal and relay settings in their Deployment arguments and environment.
3. Update the component Secret, Service, or ingress only when its Kubernetes contract changes.
4. Update at least one runnable example when users need to configure the field.
5. Render defaults, the 0.74.0 backend boundary, and the affected example.

Keep raw source configuration in management's `overrideConfig`. Signal and relay intentionally have no config-file override because their 0.74.0-compatible contract is command-line based.

The chart's supported backend range is `0.74.0` through the default `0.82.0`. `backend.split.signal.config.pprofAddress`, `backend.split.relay.config.trustedProxies`, and `backend.split.management.config.agentNetwork.pricingDefaultsFile` are 0.77.0+ fields that 0.74.0 ignores. The dashboard remains independently pinned to `v2.96.0`.

## ConfigMap rollout checksum

The management Deployment hashes its rendered ConfigMap:

```gotemplate
checksum/config: {{ include (print .Template.BasePath "/split/management/configmap.yaml") . | sha256sum }}
```

A management configuration change updates the pod template and triggers a rollout. Signal and relay have no chart-owned configuration ConfigMaps.

## Render and lint

Run all supported render checks:

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

Inspect management's generated JSON:

```bash
helm template netbird charts/netbird \
  --namespace netbird \
  --show-only templates/split/management/configmap.yaml
```

For an end-to-end local installation, use `task dev:up` (`task dev:up CHART=netbird-ee` for the enterprise chart), inspect it with `task dev:status`, redeploy it with `task dev:deploy`, and remove the dedicated cluster with `task dev:down`.

## End-to-end tests

[`e2e/netbird`](../../../e2e/netbird) installs the chart from `examples/kind` into its own Kind cluster behind ingress-nginx and connects real NetBird peers through it. It needs Docker, Go and helm:

```bash
task chart:e2e
```

The peers run in Docker containers on Kind's network, outside the cluster. The suite checks that:

- the dashboard, the API and the embedded IdP answer through the ingress;
- two peers connect directly, which needs management and signal;
- an upgrade from `NB_E2E_FROM_VERSION` (default `0.74.0`) to the chart's backends keeps the peers and their connection;
- two peers forced onto the relay connect through it. This runs after the upgrade: a 0.74.0 management sends peers no relay URLs in the updates it pushes when another peer joins, so relayed connections drop;
- management keeps every peer across a pod restart, so its store persisted.

The peers run the client release of `NB_E2E_FROM_VERSION` and keep it through the upgrade, as clients do when their server is upgraded.

`NB_E2E_ROOTLESS=true` layers `examples/rootless` into a namespace that enforces the `restricted` Pod Security Standard. `NB_E2E_KIND_NODE_IMAGE` picks the Kubernetes version, `NB_E2E_KEEP=true` leaves the cluster and peers running, and `NB_E2E_ARTIFACTS_DIR` collects logs on failure. [`e2e/README.md`](../../../e2e/README.md) explains the layout and lists every knob.

CI runs the newest Kubernetes with and without rootless on pull requests, and every supported version on `main`.
