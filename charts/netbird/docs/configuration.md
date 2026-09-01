# Configuration and secrets

The chart exposes Kubernetes settings and source-compatible NetBird configuration through [`../values.yaml`](../values.yaml). Supply a small environment-specific values file and inherit defaults for everything else.

## Opinionated configuration

Use the structured configuration trees whenever the chart supports the setting:

| Component | Values path | Generated configuration |
| --- | --- | --- |
| Dashboard | `dashboard.config` | Dashboard container environment variables |
| Combined server | `backend.combined.config` | `server:` YAML document |
| Management | `backend.split.management.config` | `management.json` |
| Signal | `backend.split.signal.config` | Signal YAML document |
| Relay | `backend.split.relay.config` | Relay YAML document |

Kubernetes settings sit beside each component's configuration. Common examples include `image`, `replicaCount`, `resources`, `pod`, `container`, `service`, `ingress`, probes, scheduling values, and persistence.

## Environment values

Every component accepts literal, structured, and Secret-backed environment entries:

```yaml
env:
  STATIC_NAME: static-value

envRaw:
  - name: POD_NAME
    valueFrom:
      fieldRef:
        fieldPath: metadata.name

envFromSecret:
  PROVIDER_CLIENT_ID: provider-credentials/client-id
```

`envFromSecret` references use `secret-name/secret-key` syntax. Provider examples use this mechanism for external identity-provider, STUN, TURN, and database credentials.

For the dashboard, `dashboard.env` overrides environment values generated from `dashboard.config`. Prefer `dashboard.config` for supported fields and reserve environment values for image settings that are not modeled by the chart.

## Chart-managed credentials

Generated configurations store sensitive runtime values in Kubernetes Secrets rather than ConfigMaps. Depending on the selected mode, the chart manages:

- relay authentication secret
- datastore encryption key
- embedded identity-provider session-cookie encryption key
- optional initial-owner password hash
- optional store DSNs

When a value is empty, Helm generates it during installation and reuses the existing Secret value during upgrades through `lookup`. Set explicit values when rendering through a GitOps system that cannot query the destination cluster.

A configured datastore encryption key must be a base64-encoded 32-byte value. Do not rotate datastore or session-cookie keys without planning for the data and sessions encrypted by the old key.

The initial-owner password is plaintext in Helm input and release values. Only its bcrypt hash is written to the workload Secret. The owner is seeded when the embedded identity-provider database is first created; changing the value later does not reset that account.

## Raw configuration overrides

Each backend service supports `overrideConfig`:

```yaml
backend:
  split:
    signal:
      overrideConfig: |-
        port: 80
        metricsPort: 9090
        logLevel: info
        logFile: console
```

An override is mounted verbatim and replaces that service's structured `config` output. The chart cannot safely infer Secret placeholders used by arbitrary content, so provide required variables through `env`, `envRaw`, or `envFromSecret`.

Use overrides only for source fields that the opinionated values do not expose. Structured values retain chart validation, generated credentials, and predictable upgrades.

## Persistence and network exposure

Combined mode persists data through `backend.combined.persistence`. Split mode persists management data through `backend.split.management.persistence`. Keep the mount path aligned with the generated data directory. SQLite installations should use one replica with a `ReadWriteOnce` claim.

HTTP and gRPC ingresses are separate because ingress controllers often require different upstream protocol settings. Kubernetes Ingress does not expose UDP; enable the combined or relay `stunService` and choose a suitable `LoadBalancer` or `NodePort` when clients need the chart's STUN endpoint.
