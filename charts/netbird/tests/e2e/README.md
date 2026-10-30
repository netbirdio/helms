# End-to-end tests

These tests install the chart into a Kind cluster and connect real NetBird peers through it.

```bash
task chart:e2e     # or: go test -tags e2e ./... -v -count 1 -timeout 30m
```

You need Docker, Go and helm. A run takes about three minutes and removes everything it created.

## Where to start

Read [`chart_test.go`](chart_test.go). `TestChart` lists the steps in the order they run, and each step is a short function below it.

| File | What it holds |
| --- | --- |
| [`chart_test.go`](chart_test.go) | The test: `TestChart` and its steps |
| [`assertions_test.go`](assertions_test.go) | What the steps check: `requireConnected`, `requireHTTPRoutes`, `requirePeersListed` |
| [`main_test.go`](main_test.go) | `TestMain`: brings the environment up once for every test, then tears it down |
| [`framework/`](framework) | The plumbing. It sets things up and reports state, and never decides whether a test passes |

The `framework` package has one file per concern:

| File | Concern |
| --- | --- |
| `env.go` | `Env`, `Bootstrap` and `Teardown`: the order things come up in |
| `cluster.go` | The Kind cluster, ingress-nginx, the namespace and its TLS Secret |
| `chart.go` | Installing and upgrading the chart, waiting for rollouts, restarting management |
| `peer.go` | NetBird clients in Docker: start them, read `netbird status`, ping |
| `api.go` | The few management API calls the suite makes |
| `forward.go` | The port-forward and HTTP client that reach the ingress from the host |
| `tls.go` | The CA that signs the ingress certificate |
| `diagnostics.go` | Logs printed and saved when something fails |
| `exec.go` | Running CLIs and reading the knobs |

## What happens in a run

1. **Bootstrap** (`framework.Bootstrap`):
   - Create a one-node Kind cluster and install ingress-nginx on it.
   - Install the chart from `examples/kind`, with its hostnames swapped for `netbird.e2e.test`, at backend version `NB_E2E_FROM_VERSION`.
   - Activate the account through `POST /api/setup` to get an API token.
2. **TestChart** (`chart_test.go`):
   1. HTTP routes: the dashboard, the API and the IdP answer through the ingress.
   2. Two peers connect directly.
   3. The backends are upgraded to the chart's defaults, and the peers stay connected.
   4. Two peers forced onto the relay connect through it.
   5. Management's pod is deleted, and every peer is still listed and reconnects.
3. **Teardown**: remove the peers and the cluster.

The peers run in Docker containers on Kind's network, outside the cluster. They resolve `netbird.e2e.test` to the Kind node, where ingress-nginx listens on 443, so they reach management, signal and the relay the way users' peers do. The suite itself reaches the same ingress through a port-forward.

## Knobs

| Variable | Effect |
| --- | --- |
| `NB_E2E_KIND_NODE_IMAGE` | Kind node image, which picks the Kubernetes version |
| `NB_E2E_FROM_VERSION` | Backend version installed first (default `0.74.0`). Setting it to the chart's default skips the upgrade |
| `NB_E2E_ROOTLESS` | Layer `examples/rootless` into a namespace that enforces the `restricted` Pod Security Standard |
| `NB_E2E_CLIENT_IMAGE` | Client the peers run (default: the release of `NB_E2E_FROM_VERSION`) |
| `NB_E2E_KEEP` | Leave the cluster and peers running. The kubeconfig path is logged |
| `NB_E2E_ARTIFACTS_DIR` | On failure, write the cluster, chart and peer logs here |
