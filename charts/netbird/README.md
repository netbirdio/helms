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

Create the `netbird-external-credentials` Secret first (see [Secrets](#secrets)). Then configure the public endpoints, embedded identity provider, credential references, and service-specific ingresses:

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
        dataStoreEncryptionKeyRef:
          name: netbird-external-credentials
          key: datastore-encryption-key
        embeddedIdp:
          issuer: https://netbird.example.com/oauth2
          sessionCookieEncryptionKeyRef:
            name: netbird-external-credentials
            key: idp-session-cookie-encryption-key
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
        authSecretRef:
          name: netbird-external-credentials
          key: relay-auth-secret
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

Sensitive typed values accept either an inline value or an adjacent `*Ref` selector:

```yaml
backend:
  split:
    relay:
      config:
        authSecretRef:
          name: netbird-external-credentials
          key: relay-auth-secret
```

References select a Secret key in the namespace of the component that uses them and are mutually exclusive with the corresponding inline value. Management expands referenced values into its JSON at process start, so their data is not copied into the management ConfigMap. See [configuration and secrets](docs/configuration.md) for every supported pair, including dashboard client credentials, TURN/STUN credentials, datastore and embedded-IdP values, and the initial owner hash.

The chart does not generate credentials. Create them once before the first install:

```bash
kubectl create namespace netbird
kubectl --namespace netbird create secret generic netbird-external-credentials \
  --from-literal=datastore-encryption-key="$(openssl rand -base64 32)" \
  --from-literal=idp-session-cookie-encryption-key="$(openssl rand -hex 16)" \
  --from-literal=relay-auth-secret="$(openssl rand -base64 32)"
```

Then reference them with `dataStoreEncryptionKeyRef`, `embeddedIdp.sessionCookieEncryptionKeyRef` (embedded IdP only) and `relay.config.authSecretRef`. The render fails if one of these has no value and no reference.

Inline values are stored in Kubernetes Secrets, never in the management ConfigMap:

- `<fullname>-management-credentials`: datastore and session-cookie keys, inline TURN/STUN/signal passwords, IdP storage DSN, and optional initial-owner password hash
- `<fullname>-relay-credentials`: relay authentication secret used by both management and relay

Referenced keys are omitted; a chart-managed Secret with no remaining keys is not rendered.

Inline `backend.split.management.config.embeddedIdp.owner.password` is plaintext in Helm input and release values, then stored only as a bcrypt hash in the workload Secret. `owner.passwordRef` must reference a precomputed bcrypt hash. The owner is seeded only when the embedded IdP database is first created; changing either value later does not reset that account.

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

## Upgrade from 1.x

Chart 2.0 keeps the 1.x resource names and selector labels (`app.kubernetes.io/name: <chart name>-<component>` and `app.kubernetes.io/instance: <release>`), so `helm upgrade` works for any release name. The values do not carry over:

1. Move the top-level `management`, `signal` and `relay` trees to `backend.split.management`, `backend.split.signal` and `backend.split.relay`, and rewrite them against [`values.yaml`](values.yaml). A 1.x values file fails schema validation, so nothing is ignored by mistake.
2. Point the credential references at the Secret that already holds your datastore key and relay secret (see [Secrets](#secrets)). A different datastore key makes the stored data unreadable.
3. Keep the management volume: set `backend.split.management.persistence.size: 10Mi` (the 1.x default), or a larger size only if the storage class allows volume expansion. The claim name does not change. `persistentVolume.existingPVName` has no replacement; bind the volume to a claim and set `persistence.existingClaim`.
4. Render with `helm template` and compare with the running objects before you upgrade.

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
