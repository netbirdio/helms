# NetBird Helm chart

Deploys the NetBird dashboard and either the combined NetBird server or the split management, signal, and relay services.

Focused guides: [installation and backend modes](docs/installation.md), [configuration and secrets](docs/configuration.md), and [template development](docs/development.md).

## Requirements

- Kubernetes 1.24+
- Helm 3.12+
- An ingress controller when any ingress is enabled
- A TLS secret or certificate controller for HTTPS ingress

## Install

The chart uses the combined server by default. Set the public endpoints and an initial owner before installing:

```yaml
dashboard:
  config:
    api:
      httpEndpoint: https://netbird.example.com
      grpcEndpoint: https://netbird.example.com
    auth:
      authority: https://netbird.example.com/oauth2
  ingress:
    enabled: true
    className: nginx
    hosts:
      - host: dashboard.netbird.example.com
        paths:
          - path: /
            pathType: ImplementationSpecific

backend:
  mode: combined
  combined:
    config:
      exposedAddress: https://netbird.example.com:443
      auth:
        issuer: https://netbird.example.com/oauth2
        dashboardRedirectURIs:
          - https://dashboard.netbird.example.com/nb-auth
          - https://dashboard.netbird.example.com/nb-silent-auth
        owner:
          email: admin@example.com
          password: replace-me
    ingress:
      http:
        enabled: true
        className: nginx
      grpc:
        enabled: true
        className: nginx
        annotations:
          nginx.ingress.kubernetes.io/backend-protocol: GRPC
```

```bash
helm upgrade --install netbird ./charts/netbird \
  --namespace netbird \
  --create-namespace \
  --values values-production.yaml
```

The complete value surface, including probes, scheduling, persistence, services, and ingress routes, is documented inline in [`values.yaml`](values.yaml).

## Backend modes

### Combined

`backend.mode: combined` runs management, signal, relay, and STUN from `netbirdio/netbird-server`. The chart translates `backend.combined.config` to the server's `server:` YAML schema.

The HTTP and gRPC ingresses are separate because ingress controllers usually require different upstream protocols. The HTTP ingress serves `/api`, `/oauth2`, `/relay`, and `/ws-proxy`. The gRPC ingress serves management, proxy, and signal gRPC services.

Kubernetes Ingress does not expose UDP. Enable `backend.combined.stunService` and select an appropriate `LoadBalancer` or `NodePort` configuration when clients should use the embedded STUN server.

### Split

`backend.mode: split` deploys management, signal, and relay separately:

- `backend.split.management.config` is serialized as `management.json`.
- `backend.split.signal.config` is translated to the signal YAML configuration introduced by the shared service config loader.
- `backend.split.relay.config` is translated to the relay YAML configuration introduced by the shared service config loader.

Each service also supports `overrideConfig`. When set, the override is mounted verbatim and that service's structured `config` values are ignored. Supply secrets used by an override through `env`, `envRaw`, or `envFromSecret`.

The generated backend configurations target `feature/shared-service-config-loader`. In particular, the public `0.77.0` signal and relay images do not accept the `--config` flag. Until a release contains that loader, point the backend image values at images built from the feature branch.

The provider-specific examples under `examples/nginx-ingress`, `examples/traefik-ingress`, and `examples/istio` use split mode with the structured `dashboard.config` and `backend.split.*.config` values. Provider credentials remain Secret references under `envFromSecret`; the chart owns the relay and datastore credentials.

## Secrets

For generated configurations, the chart stores sensitive values in Kubernetes Secrets rather than ConfigMaps:

- relay authentication secret
- management datastore encryption key
- embedded IdP session-cookie encryption key
- optional store DSNs
- optional initial-owner password hash

An empty relay, datastore, or session-cookie secret is generated on install and retained on upgrade with Helm's `lookup`. Set the corresponding value explicitly for GitOps renderers that cannot query the target cluster.

`auth.owner.password` is a plaintext chart input. The chart stores only a bcrypt hash in the workload Secret because the server branch currently consumes a hash in the initial-owner field. The plaintext remains present in Helm release values, so provide it through your normal Helm secret-management workflow.

The initial owner is seeded only when the embedded IdP database is first created. Changing the value later does not reset an existing user's password.

## Environment values

Every component supports:

```yaml
env:
  NAME: value
envRaw:
  - name: NAME_FROM_FIELD_REF
    valueFrom:
      fieldRef:
        fieldPath: metadata.name
envFromSecret:
  NAME_FROM_SECRET: secret-name/secret-key
```

Dashboard values under `dashboard.config` are translated to the dashboard image's environment contract. Explicit entries in `dashboard.env` override those generated values.

## Persistence

Combined mode mounts one volume at `backend.combined.persistence.mountPath`. Split mode mounts the management volume at `backend.split.management.persistence.mountPath`. Keep that path aligned with the generated service data directory, or set it to the data directory used by `overrideConfig`. Set `persistence.existingClaim` to reuse a pre-created claim, or disable persistence only when every persistent store is external or data loss is acceptable.

SQLite does not support active replicas sharing one `ReadWriteOnce` claim. Keep the corresponding backend replica count at one when SQLite is selected.

## Local kind environment

The repository Taskfile creates a dedicated kind cluster, installs ingress-nginx, creates a local TLS secret, and deploys [`examples/kind/values.yaml`](examples/kind/values.yaml):

```bash
task dev:up
```

Open `https://dashboard.netbird.localhost` and sign in with:

- User: `admin@netbird.local`
- Password: `Netbird1!`

Useful commands:

```bash
task chart:lint
task chart:template -- --values charts/netbird/examples/kind/values.yaml
task dev:status
task dev:down
```

The kind credentials and cryptographic keys are development-only.

## Upgrade to 2.0

Version 2.0 is a clean values cutover. The top-level `management`, `signal`, and `relay` trees were replaced by `backend.mode`, `backend.combined`, and `backend.split`. Dashboard Kubernetes settings now use the same nested `image`, `pod`, and `container` layout as backend components. No compatibility aliases are rendered.
