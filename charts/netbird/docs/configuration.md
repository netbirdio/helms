# Configuration and secrets

The chart exposes Kubernetes settings and source-compatible NetBird configuration through [`../values.yaml`](../values.yaml). Supply a small environment-specific values file and inherit defaults for everything else.

## Typed configuration

Use the structured configuration trees whenever the chart supports the setting:

| Component | Values path | Runtime contract |
| --- | --- | --- |
| Dashboard | `dashboard.config` | Dashboard container environment |
| Management | `backend.split.management.config` | Mounted `management.json` |
| Signal | `backend.split.signal.config` | Stable command-line flags |
| Relay | `backend.split.relay.config` | Stable command-line flags and Secret-backed environment |

The chart tests management, signal, and relay versions `0.74.0` through `0.77.0`; `0.77.0` is the default. Dashboard compatibility is independent and fixed at `netbirdio/dashboard:v2.91.1`.

Kubernetes settings sit beside each component's configuration. Common examples include `image`, `replicaCount`, `resources`, `pod`, `container`, `service`, `ingress`, probes, scheduling values, and management persistence.

## Version-specific values

The following additive fields are effective on NetBird 0.77.0 or newer and ignored by 0.74.0:

| Values path | Runtime behavior |
| --- | --- |
| `backend.split.management.config.agentNetwork.pricingDefaultsFile` | Added to `management.json`; a non-empty path must reference a file mounted by the operator |
| `backend.split.signal.config.pprofAddress` | Rendered as `NB_PPROF_ADDR` when non-empty |
| `backend.split.relay.config.trustedProxies` | Rendered as `NB_TRUSTED_PROXIES` when non-empty |

No image-tag checks are performed. Unknown environment variables and additive management JSON fields are harmless at the 0.74.0 boundary.

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

`envFromSecret` references use `secret-name/secret-key` syntax. Provider examples use this mechanism for identity-provider, STUN, TURN, and database credentials.

For the dashboard, `dashboard.env` overrides environment values generated from `dashboard.config`. Prefer `dashboard.config` for supported fields.

## Chart-managed credentials

The chart manages two Secret identities:

- `<fullname>-management-credentials`
  - `datastore-encryption-key`
  - `idp-session-cookie-encryption-key`
  - optional `owner-password-checksum`
  - optional `owner-password-hash`
- `<fullname>-relay-credentials`
  - `relay-auth-secret`

Management and relay both read the same `relay-auth-secret`, keeping advertised credentials aligned with relay runtime authentication.

When a value is empty, Helm generates it during installation and reuses the existing Secret value during upgrades through `lookup`. Set explicit values when rendering through a GitOps system that cannot query the destination cluster.

A configured datastore encryption key must be a base64-encoded 32-byte value. Do not rotate datastore or session-cookie keys without planning for data and sessions encrypted by the old key.

The initial-owner password is plaintext in Helm input and release values. Only its bcrypt hash is written to the workload Secret. The owner is seeded when the embedded identity-provider database is first created; changing the value later does not reset that account.

## Management raw configuration override

Only management supports a raw file override:

```yaml
backend:
  split:
    management:
      overrideConfig: |-
        {
          "datadir": "/var/lib/netbird",
          "storeConfig": {
            "engine": "sqlite"
          }
        }
```

The override is mounted verbatim as `management.json` and replaces the structured management output. The chart cannot infer Secret placeholders used by arbitrary content, so provide required variables through `env`, `envRaw`, or `envFromSecret`.

Signal and relay do not accept `overrideConfig`; use their typed settings and append extra portable arguments through `container.cmd.args`.

## Persistence and network exposure

Management persists data through `backend.split.management.persistence`. Keep its mount path aligned with `backend.split.management.config.datadir`. SQLite installations should use one replica with a `ReadWriteOnce` claim.

HTTP and gRPC ingresses are separate because ingress controllers often require different upstream protocol settings. Kubernetes Ingress does not expose UDP; enable both relay `config.stun.enabled` and `stunService.enabled`, then choose a suitable `LoadBalancer` or `NodePort` when clients need the chart's STUN endpoint.
