# NetBird Helm chart

Deploys the NetBird dashboard and separate management, signal, and relay services.

Focused guides: [installation](docs/installation.md), [configuration and secrets](docs/configuration.md), and [template development](docs/development.md).

## Requirements

- Kubernetes 1.24+
- Helm 3.12+
- An ingress controller when any ingress is enabled
- A TLS Secret or certificate controller for HTTPS ingress

## Version compatibility

| Chart | Tested backend versions | Default backend | Dashboard |
| --- | --- | --- | --- |
| `2.0.0` | `0.74.0`–`0.77.0` | `0.77.0` | `v2.91.1` |

Backend versions apply to the management, signal, and relay images. The dashboard has an independent compatibility pin and is not derived from the chart `appVersion`.

## Install

Configure the public endpoints, embedded identity provider, and service-specific ingresses:

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

backend:
  split:
    management:
      config:
        relay:
          addresses:
            - rels://netbird.example.com:443/relay
        signal:
          proto: https
          uri: netbird.example.com:443
        embeddedIdp:
          issuer: https://netbird.example.com/oauth2
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
    signal:
      ingress:
        enabled: true
        className: nginx
        annotations:
          nginx.ingress.kubernetes.io/backend-protocol: GRPC
    relay:
      config:
        exposedAddress: rels://netbird.example.com:443/relay
      ingress:
        enabled: true
        className: nginx
```

```bash
helm upgrade --install netbird ./charts/netbird \
  --namespace netbird \
  --create-namespace \
  --values values-production.yaml \
  --wait
```

The complete value surface, including probes, scheduling, persistence, services, and ingress routes, is documented inline in [`values.yaml`](values.yaml).

## Backend configuration

The chart always renders separate management, signal, and relay workloads:

- `backend.split.management.config` is serialized as `management.json` and mounted at `/etc/netbird/management.json`.
- `backend.split.signal.config` is translated to signal command-line flags supported by NetBird 0.74.0 through 0.77.0.
- `backend.split.relay.config` is translated to relay command-line flags and Secret-backed environment supported by NetBird 0.74.0 through 0.77.0.

Only management supports `overrideConfig`. It replaces the generated `management.json`; provide any variables needed by the override through `env`, `envRaw`, or `envFromSecret`. Signal and relay deliberately expose typed values and portable runtime arguments instead of raw config-file overrides.

These additive settings require NetBird 0.77.0 or newer and are harmlessly ignored by 0.74.0:

- `backend.split.management.config.agentNetwork.pricingDefaultsFile`
- `backend.split.signal.config.pprofAddress`
- `backend.split.relay.config.trustedProxies`

Provider-specific examples under `examples/nginx-ingress`, `examples/traefik-ingress`, and `examples/istio` use the same split layout.

## Secrets

The chart stores generated sensitive values in Kubernetes Secrets rather than ConfigMaps:

- `<fullname>-management-credentials`: datastore encryption key, embedded IdP session-cookie encryption key, and optional initial-owner password hash
- `<fullname>-relay-credentials`: relay authentication secret used by both management and relay

Empty relay, datastore, and session-cookie secrets are generated on install and retained on upgrade with Helm's `lookup`. Set them explicitly for GitOps renderers that cannot query the target cluster.

`backend.split.management.config.embeddedIdp.owner.password` is a plaintext Helm input. The chart stores only its bcrypt hash in the workload Secret, but the plaintext remains in Helm release values. The owner is seeded only when the embedded IdP database is first created; changing the value later does not reset that account.

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

Dashboard values under `dashboard.config` are translated to the dashboard image's environment contract. Explicit entries in `dashboard.env` override generated values.

## Persistence and STUN

Management data is mounted at `backend.split.management.persistence.mountPath`. Keep the path aligned with `backend.split.management.config.datadir` or the data directory used by a management `overrideConfig`. Set `persistence.existingClaim` to reuse a pre-created claim. SQLite requires one management replica with its default `ReadWriteOnce` claim.

Kubernetes Ingress does not expose UDP. To publish the relay's STUN endpoint, enable both `backend.split.relay.config.stun.enabled` and `backend.split.relay.stunService.enabled`, then select a suitable `LoadBalancer` or `NodePort` configuration.

## Local kind environment

The repository Taskfile creates a dedicated kind cluster, installs ingress-nginx, creates a local TLS Secret, and deploys [`examples/kind/values.yaml`](examples/kind/values.yaml):

```bash
task dev:up
```

Open `https://dashboard.netbird.localhost` and sign in with:

- User: `admin@netbird.local`
- Password: `Netbird1!`

Useful commands:

```bash
task chart:verify
task chart:template -- --values charts/netbird/examples/kind/values.yaml
task dev:status
task dev:down
```

The kind credentials and cryptographic keys are development-only.
