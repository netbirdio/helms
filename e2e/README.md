# End-to-end tests

These tests install the repository's charts into Kind clusters and check them the way users run them. Today that is the netbird chart, with real NetBird peers connecting through it.

```bash
task chart:e2e     # or, from this directory: go test -tags e2e ./netbird/... -v -count 1 -timeout 30m
```

You need Docker, Go and helm. A run takes about three minutes and removes everything it created.

## Layout

```text
e2e/
├── go.mod, go.sum        one module for every chart's tests
├── framework/            plumbing every chart shares; knows no chart
│   └── netbird/          plumbing for the netbird chart
└── netbird/              the netbird chart's tests
```

The rule: a top-level package such as `netbird/` holds only `_test.go` files and decides what passes. Everything under `framework/` sets things up and reports state, and never asserts.

## Where to start

Read [`netbird/chart_test.go`](netbird/chart_test.go). `TestChart` lists the steps in the order they run, and each step is a short function below it.

| File | What it holds |
| --- | --- |
| [`netbird/chart_test.go`](netbird/chart_test.go) | The test: `TestChart` and its steps |
| [`netbird/assertions_test.go`](netbird/assertions_test.go) | What the steps check: `requireConnected`, `requireHTTPRoutes`, `requirePeersListed` |
| [`netbird/main_test.go`](netbird/main_test.go) | `TestMain`: brings the environment up once for every test, then tears it down |

[`framework/`](framework) is shared, one file per concern:

| File | Concern |
| --- | --- |
| `cluster.go` | `Cluster`: create and delete the Kind cluster, install ingress-nginx, create namespaces and TLS Secrets, run helm |
| `workloads.go` | Wait for a Deployment's rollout, restart pods |
| `forward.go` | The port-forward and HTTP client that reach the ingress from the host |
| `tls.go` | The CA that signs the ingress certificates |
| `diagnostics.go` | Pod logs and events, for when something fails |
| `exec.go` | Running CLIs, finding a chart, reading the shared knobs |

[`framework/netbird/`](framework/netbird) is the netbird chart's:

| File | Concern |
| --- | --- |
| `env.go` | `Env`, `Bootstrap` and `Teardown`: the order things come up in |
| `chart.go` | Installing and upgrading the chart, restarting management |
| `peer.go` | NetBird clients in Docker: start them, read `netbird status`, ping |
| `api.go` | The few management API calls the tests make |
| `diagnostics.go` | What to log and save for this chart when something fails |

## What happens in a netbird run

1. **Bootstrap** (`netbird.Bootstrap`):
   - Create a one-node Kind cluster and install ingress-nginx on it.
   - Install the chart (`NB_E2E_CHART`) from the netbird chart's `examples/kind`, nested under `netbird:` for netbird-ee, with its hostnames swapped for `netbird.e2e.test`, at backend version `NB_E2E_FROM_VERSION`.
   - Activate the account through `POST /api/setup` to get an API token.
2. **TestChart** (`netbird/chart_test.go`):
   1. HTTP routes: the dashboard, the API and the IdP answer through the ingress.
   2. netbird-ee only: the enterprise images run.
   3. Two peers connect directly.
   4. The backends are upgraded to the chart's defaults, and the peers and the credential Secrets survive it.
   5. Two peers forced onto the relay connect through it.
   6. Management's pod is deleted, and every peer is still listed and reconnects.
3. **Teardown**: remove the peers and the cluster.

The peers run in Docker containers on Kind's network, outside the cluster. They resolve `netbird.e2e.test` to the Kind node, where ingress-nginx listens on 443, so they reach management, signal and the relay the way users' peers do. The tests reach the same ingress through a port-forward.

## Knobs

| Variable | Effect |
| --- | --- |
| `NB_E2E_CHART` | netbird: the chart under test, `netbird` (default) or `netbird-ee` |
| `NB_E2E_KIND_NODE_IMAGE` | Kind node image, which picks the Kubernetes version |
| `NB_E2E_KEEP` | Leave the cluster running. The kubeconfig path is logged |
| `NB_E2E_ARTIFACTS_DIR` | On failure, write the cluster, chart and client logs here |
| `NB_E2E_FROM_VERSION` | netbird: backend version installed first (default `0.74.0` for netbird, the chart's default for netbird-ee). Setting it to the chart's default skips the upgrade |
| `NB_E2E_ROOTLESS` | netbird: layer `examples/rootless` into a namespace that enforces the `restricted` Pod Security Standard |
| `NB_E2E_CLIENT_IMAGE` | netbird: client the peers run (default: the release of `NB_E2E_FROM_VERSION`) |

## Adding a chart

A chart that wraps the netbird chart, like netbird-ee, is a value of `NB_E2E_CHART` instead: it reuses `framework/netbird` and the tests in `netbird/`, which skip what does not apply to it.

Any other chart:

1. Add its plumbing under `framework/<chart>/`, built on `framework.Cluster`, with a Kind cluster name of its own so packages can run side by side.
2. Add its tests in `<chart>/`, `_test.go` files only, with a `TestMain` that brings the environment up.
3. Copy `.github/workflows/e2e-chart-netbird.yaml`, with its paths filter on `charts/<chart>/**`, `e2e/framework/**` and `e2e/<chart>/**`, and `go test` on `./<chart>/...`.

## Editor setup

The files carry the `e2e` build tag, so gopls needs it to load them: `"gopls": {"buildFlags": ["-tags=e2e"]}`.
