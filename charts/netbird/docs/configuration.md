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

### Management server flags

`backend.split.management.server` sets the management command-line flags:

| Value | Flag | Default |
| --- | --- | --- |
| `singleAccountModeDomain` | `--single-account-mode-domain`; when empty, `--disable-single-account-mode=true` | `netbird.selfhosted` |
| `dnsDomain` | `--dns-domain` | `netbird.selfhosted` |
| `disableLegacyPort` | `--disable-legacy-port=true` | `true` |

The management server uses single-account mode when no flag turns it off: every user joins one account. Set `singleAccountModeDomain: ""` for multi-account installations. `container.cmd.args` adds extra flags after these and is empty by default.

### Management JSON pass-through

Keys under `backend.split.management.config` that the chart does not know are copied unchanged into `management.json`. Use this for fields such as `httpConfig`, `idpManagerConfig` or `deviceAuthorizationFlow`.

At start, management replaces `{{ .NAME }}` placeholders in `management.json` with the environment variable `NAME`. Provide the variable through `env` or `envFromSecret`. A missing variable becomes the text `<no value>` and management gives no error, so check the variable names.

When `embeddedIdp.enabled` is `false`, the chart omits the `embeddedIdp` block from `management.json`.

### Dashboard redirect URIs

`dashboard.config.auth.redirectUri` and `silentRedirectUri` are empty by default, and the dashboard then uses `/#callback` and `/#silent-callback`. External identity providers expect these. The embedded IdP needs `/nb-auth` and `/nb-silent-auth`, as in [`../examples/kind/values.yaml`](../examples/kind/values.yaml).

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

## Typed Secret references

Each secret-bearing field in the chart's default typed values has an adjacent `*Ref` alternative. A reference selects one key from a Secret in the release namespace:

```yaml
dashboard:
  config:
    auth:
      clientSecretRef:
        name: netbird-external-credentials
        key: dashboard-client-secret

backend:
  split:
    management:
      config:
        dataStoreEncryptionKeyRef:
          name: netbird-external-credentials
          key: datastore-encryption-key
    relay:
      config:
        authSecretRef:
          name: netbird-external-credentials
          key: relay-auth-secret
```

Set either the literal value or its reference, never both. Helm rejects references without both `name` and `key`. The chart passes referenced management values through Secret-backed environment variables and expands them into `management.json` only inside the management process; the referenced data is not copied into a rendered ConfigMap or chart-managed Secret.

| Literal value | Secret reference |
| --- | --- |
| `dashboard.config.auth.clientSecret` | `dashboard.config.auth.clientSecretRef` |
| `backend.split.management.config.stuns[].password` | `backend.split.management.config.stuns[].passwordRef` |
| `backend.split.management.config.turnConfig.turns[].password` | `backend.split.management.config.turnConfig.turns[].passwordRef` |
| `backend.split.management.config.turnConfig.secret` | `backend.split.management.config.turnConfig.secretRef` |
| `backend.split.management.config.signal.password` | `backend.split.management.config.signal.passwordRef` |
| `backend.split.management.config.dataStoreEncryptionKey` | `backend.split.management.config.dataStoreEncryptionKeyRef` |
| `backend.split.management.config.storeConfig.dsn` | `backend.split.management.config.storeConfig.dsnRef` |
| `backend.split.management.config.embeddedIdp.storage.config.dsn` | `backend.split.management.config.embeddedIdp.storage.config.dsnRef` |
| `backend.split.management.config.embeddedIdp.sessionCookieEncryptionKey` | `backend.split.management.config.embeddedIdp.sessionCookieEncryptionKeyRef` |
| `backend.split.management.config.embeddedIdp.owner.password` | `backend.split.management.config.embeddedIdp.owner.passwordRef` |
| `backend.split.relay.config.authSecret` | `backend.split.relay.config.authSecretRef` |

The initial-owner `passwordRef` must contain the bcrypt hash consumed by management, not plaintext. Inline `owner.password` remains a convenience input that Helm hashes. Management always receives the relay reference configured under `backend.split.relay.config`, so management and relay cannot drift to different authentication secrets.

Changing the data in an external Secret does not change the rendered Pod template. Restart the affected Deployment, or use a Secret-reload controller, after rotating an externally managed value.

## Chart-managed credentials

The chart manages two Secret identities:

- `<fullname>-management-credentials`
  - `datastore-encryption-key`
  - `idp-session-cookie-encryption-key`, when the embedded IdP is enabled
  - `store-dsn`, when `storeConfig.engine` is `postgres` or `mysql`
  - optional `stun-password-<index>`, `turn-password-<index>`, `turn-secret`, `signal-password` and `idp-storage-dsn`
  - optional `owner-password-checksum`
  - optional `owner-password-hash`
- `<fullname>-relay-credentials`
  - `relay-auth-secret`

Management and relay both read the same `relay-auth-secret`, keeping advertised credentials aligned with relay runtime authentication.

The chart does not generate credentials. The datastore encryption key, the relay authentication secret and, when the embedded IdP is enabled, the session-cookie key each need an inline value or a reference; otherwise the render fails. Rendering is therefore the same with `helm install`, `helm template`, Tanka, Argo CD and Flux.

Inline values are written to the chart-managed Secret and replaced by an environment placeholder in `management.json`; they never appear in the ConfigMap. An inline value that is already a placeholder, such as `"{{ .TURN_SERVER_PASSWORD }}"`, is left in `management.json` unchanged. A configured reference is used directly, and the corresponding key is omitted from the chart-managed Secret. If every key for one of the chart-managed Secrets is external, that Secret is not rendered. For GitOps, set references and keep the values in your secret store.

Environment variables in `env`, `envRaw` and `envFromSecret` must not repeat each other or a variable that the chart sets, such as `NB_RELAY_AUTH_SECRET`; the render fails on a collision.

A configured datastore encryption key must be a base64-encoded 32-byte value. Do not rotate datastore or session-cookie keys without planning for data and sessions encrypted by the old key.

The inline initial-owner password is plaintext in Helm input and release values. Only its bcrypt hash is written to the workload Secret. Without cluster access (for example `helm template`), bcrypt makes a new salt on each render, so the stored hash changes while it still matches the same password. Use `owner.passwordRef` for a stable render. `owner.passwordRef` instead references a precomputed bcrypt hash, keeping the plaintext out of Helm values. The owner is seeded when the embedded identity-provider database is first created; changing either value later does not reset that account.

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

When persistence is enabled, management uses the `Recreate` strategy, because a `ReadWriteOnce` volume cannot attach to the old and the new pod at the same time. Set `backend.split.management.strategy` to change it. The claim has `helm.sh/resource-policy: keep`, so `helm uninstall` does not delete the data; delete the claim manually when you no longer need it.

For PostgreSQL or MySQL, set `storeConfig.engine` and `storeConfig.dsnRef` (or the inline `dsn`). The chart provides the DSN as `NB_STORE_ENGINE_POSTGRES_DSN` or `NB_STORE_ENGINE_MYSQL_DSN`; the render fails when it is missing.

HTTP and gRPC ingresses are separate because ingress controllers often require different upstream protocol settings. Kubernetes Ingress does not expose UDP; enable both relay `config.stun.enabled` and `stunService.enabled`, then choose a suitable `LoadBalancer` or `NodePort` when clients need the chart's STUN endpoint.
