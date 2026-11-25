# End-to-end tests

These tests install the repository's charts into Kind clusters and check them the way users run them. Today that is the netbird chart, in its community and enterprise editions, with real NetBird peers connecting through them.

```bash
task chart:e2e                  # or, from this directory: go test -tags e2e ./netbird/... -v -count 1 -timeout 30m
task chart:e2e EDITION=enterprise # go test -tags e2e ./enterprise/... ...
```

You need Docker, Go and helm. A run takes about three minutes and removes everything it created.

## Layout

```text
e2e/
├── go.mod, go.sum        one module for every chart's tests
├── framework/            plumbing every chart shares; knows no chart
│   └── netbird/          plumbing for the netbird chart, both editions
├── shared/               the steps and checks the netbird and enterprise tests share
├── netbird/              the community edition's tests
└── enterprise/           the enterprise edition's tests, and the switch to it
```

The rule: a chart's package, such as `netbird/`, holds only `_test.go` files and decides which steps run. `shared/` holds the steps and checks the two netbird packages share; it asserts, so it is not under `framework/`. Everything under `framework/` sets things up and reports state, and never asserts. Each chart's package brings up its own Kind cluster, named after it, so packages can run side by side.

## Where to start

Read [`netbird/chart_test.go`](netbird/chart_test.go). `TestChart` lists the steps in the order they run; [`enterprise/chart_test.go`](enterprise/chart_test.go) is the same for the enterprise edition, with its own steps added.

| File | What it holds |
| --- | --- |
| [`netbird/chart_test.go`](netbird/chart_test.go), [`enterprise/chart_test.go`](enterprise/chart_test.go) | The tests: `TestChart` and the steps each edition runs |
| [`netbird/main_test.go`](netbird/main_test.go), [`enterprise/main_test.go`](enterprise/main_test.go) | `TestMain`: which edition and cluster the package brings up |
| [`shared/lifecycle.go`](shared/lifecycle.go) | The shared steps: `Lifecycle`'s methods, and `Run` |
| [`shared/assertions.go`](shared/assertions.go) | What the steps check: `RequireConnected`, `RequireHTTPRoutes`, `RequirePeersListed` |
| [`shared/main.go`](shared/main.go) | `Main`: brings the environment up once for every test, then tears it down |

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

## What happens in a run

1. **Bootstrap** (`netbird.Bootstrap`):
   - Create a one-node Kind cluster and install ingress-nginx on it.
   - Install the chart at the first edition (the package's, or community when enterprise runs with `NB_E2E_FROM_EDITION=community`) from `examples/kind`, with its hostnames swapped for `netbird.e2e.test`, at backend version `NB_E2E_FROM_VERSION`.
   - Activate the account through `POST /api/setup` to get an API token.
2. **TestChart** (`netbird/chart_test.go` or `enterprise/chart_test.go`; `ha/chart_test.go` is described below):
   1. HTTP routes: the dashboard, the API and the IdP answer through the ingress.
   2. Two peers connect directly.
   3. The release is upgraded to the edition under test (community to enterprise, when switching) and its backends to the chart's defaults, and the peers and the credential Secrets survive it.
   4. enterprise only: the enterprise images run, and management starts the enterprise server.
   5. Two peers forced onto the relay connect through it.
   6. Management's pod is deleted, and every peer is still listed and reconnects.
   7. community only: the relay moves to per-pod mode with two pods. Two relay-only peers, each resolving only its own relay until that becomes its home, connect across the pods.
3. **Teardown**: remove the peers and the cluster.

`ha` (`task chart:e2e HA=true`) first installs PostgreSQL, Redis and a 3-node NATS cluster from `framework/netbird/testdata/ha-deps` in their own namespace, with the `netbird-ha` Secret, then the enterprise edition with `examples/ha`: two replicas of management and signal. Its steps: both replicas run the enterprise server; HTTP routes; peers connect directly and through the relay; one management and one signal pod are deleted and two new peers connect before the replacements are up; every management pod restarts and the peers stay listed; per-pod relays.

The peers run in Docker containers on Kind's network, outside the cluster. They resolve `netbird.e2e.test` and the per-pod relay names `relay-<n>.netbird.e2e.test` to the Kind node, where ingress-nginx listens on 443, so they reach management, signal and the relay the way users' peers do. The tests reach the same ingress through a port-forward.

## Knobs

| Variable | Effect |
| --- | --- |
| `NB_E2E_FROM_EDITION` | enterprise: `community` installs the community edition first and tests the switch in `charts/netbird/docs/enterprise.md` |
| `NB_E2E_KIND_NODE_IMAGE` | Kind node image, which picks the Kubernetes version |
| `NB_E2E_KEEP` | Leave the cluster running. The kubeconfig path is logged |
| `NB_E2E_ARTIFACTS_DIR` | On failure, write the cluster, chart and client logs here |
| `NB_E2E_FROM_VERSION` | netbird: backend version installed first (default `0.74.0` for community, the edition's default for enterprise). Setting it to the chart's default skips the upgrade |
| `NB_E2E_ROOTLESS` | netbird: layer `examples/rootless` into a namespace that enforces the `restricted` Pod Security Standard |
| `NB_E2E_CLIENT_IMAGE` | netbird: client the peers run (default: the release of `NB_E2E_FROM_VERSION`) |

## Adding a chart

A new netbird edition gets its own package that reuses `framework/netbird` and `shared`: a `TestMain` calling `shared.Main` with its edition and cluster name, and a `TestChart` listing the shared steps plus its own. It also needs a `netbird.Edition*` constant and a value in the chart's `edition` enum.

Any other chart:

1. Add its plumbing under `framework/<chart>/`, built on `framework.Cluster`, with a Kind cluster name of its own so packages can run side by side.
2. Add its tests in `<chart>/`, `_test.go` files only, with a `TestMain` that brings the environment up.
3. Copy `.github/workflows/e2e-chart-netbird.yaml`, with its paths filter on `charts/<chart>/**`, `e2e/framework/**` and `e2e/<chart>/**`, and `go test` on `./<chart>/...`.

## Editor setup

The files carry the `e2e` build tag, so gopls needs it to load them: `"gopls": {"buildFlags": ["-tags=e2e"]}`.
