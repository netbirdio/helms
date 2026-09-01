# Installation and backend modes

Run the commands in this guide from the repository root.

## Requirements

- Kubernetes 1.24 or newer
- Helm 3.12 or newer
- An ingress controller when ingress is enabled
- A TLS Secret or certificate controller for HTTPS ingress

## Choose a backend mode

The dashboard is always deployed. `backend.mode` selects the backend topology:

| Mode | Workloads | Use when |
| --- | --- | --- |
| `combined` | One `netbird-server` Deployment | You want the smallest self-hosted installation and the embedded identity provider. This is the default. |
| `split` | Separate management, signal, and relay Deployments | You need external identity-provider integration or independent service configuration and scaling. |

Generated backend configuration targets `feature/shared-service-config-loader`. Public `0.77.0` signal and relay images do not accept the generated `--config` argument. Until a release contains the loader, set the backend image values to images built from that branch.

## Create an installation values file

Start with the relevant example rather than copying the complete default values:

- Combined local installation: [`../examples/kind/values.yaml`](../examples/kind/values.yaml)
- Split external identity providers: [`../examples/nginx-ingress`](../examples/nginx-ingress), [`../examples/traefik-ingress`](../examples/traefik-ingress), and [`../examples/istio`](../examples/istio)

For combined mode, configure these public URLs together:

```yaml
dashboard:
  config:
    api:
      httpEndpoint: https://netbird.example.com
      grpcEndpoint: https://netbird.example.com
    auth:
      authority: https://netbird.example.com/oauth2

backend:
  mode: combined
  combined:
    config:
      exposedAddress: https://netbird.example.com:443
      auth:
        issuer: https://netbird.example.com/oauth2
        owner:
          email: admin@example.com
          password: replace-me
```

Also enable and configure the dashboard, HTTP, and gRPC ingresses for your ingress controller. The gRPC ingress usually needs a controller-specific backend-protocol annotation.

## Install or upgrade

```bash
helm upgrade --install netbird charts/netbird \
  --namespace netbird \
  --create-namespace \
  --values values-production.yaml \
  --wait
```

Use the same command for upgrades. Helm preserves chart-generated credentials by reading the existing Kubernetes Secrets during an upgrade.

## Local kind environment

The repository can create a dedicated kind cluster, ingress-nginx controller, local TLS certificate, and combined NetBird release:

```bash
task dev:up
```

Open `https://dashboard.netbird.localhost` and use the development-only credentials printed by:

```bash
task dev:urls
```

Inspect or delete the environment with:

```bash
task dev:status
task dev:down
```
