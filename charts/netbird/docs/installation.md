# Installation

Run the commands in this guide from the repository root.

## Requirements

- Kubernetes 1.24 or newer
- Helm 3.12 or newer
- An ingress controller when ingress is enabled
- A TLS Secret or certificate controller for HTTPS ingress

## Workloads and supported versions

The dashboard is always deployed with separate management, signal, and relay Deployments. Chart `2.0.0` tests the three backend images from NetBird `0.74.0` through the default `0.77.0`. The dashboard is independently pinned to `netbirdio/dashboard:v2.91.1`.

Management uses its stable JSON configuration loader. Signal and relay use portable command-line flags; relay also receives its authentication secret through the environment. This runtime contract works at both tested backend boundaries without detecting the selected image tag.

## Create an installation values file

Start with the relevant example rather than copying the complete defaults:

- Embedded identity provider and local ingress: [`../examples/kind/values.yaml`](../examples/kind/values.yaml)
- External identity providers: [`../examples/nginx-ingress`](../examples/nginx-ingress), [`../examples/traefik-ingress`](../examples/traefik-ingress), and [`../examples/istio`](../examples/istio)

Configure the public endpoints together:

```yaml
dashboard:
  config:
    api:
      httpEndpoint: https://netbird.example.com
      grpcEndpoint: https://netbird.example.com
    auth:
      authority: https://netbird.example.com/oauth2

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
    relay:
      config:
        exposedAddress: rels://netbird.example.com:443/relay
```

To keep credentials out of Helm values, create or provision a Secret in the release namespace and replace inline sensitive values with their adjacent `*Ref` selectors:

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
        embeddedIdp:
          sessionCookieEncryptionKeyRef:
            name: netbird-external-credentials
            key: idp-session-cookie-encryption-key
    relay:
      config:
        authSecretRef:
          name: netbird-external-credentials
          key: relay-auth-secret
```

The referenced Secret must exist in the Helm release namespace. Set `name` to the Secret name and `key` to the key containing the value. Do not set the corresponding inline value at the same time. See [configuration and secrets](configuration.md#typed-secret-references) for every supported pair, including TURN/STUN passwords, embedded-IDP storage, and the initial-owner bcrypt hash.

Enable dashboard, management HTTP and gRPC, signal gRPC, and relay ingresses for your ingress controller. Management and signal gRPC ingresses usually need a controller-specific backend-protocol annotation.

## Install or upgrade

```bash
helm upgrade --install netbird charts/netbird \
  --namespace netbird \
  --create-namespace \
  --values values-production.yaml \
  --wait
```

Use the same command for upgrades. Helm preserves chart-generated credentials by reading existing Kubernetes Secrets.

The 0.77.0-only `pprofAddress`, `trustedProxies`, and `agentNetwork.pricingDefaultsFile` values are ignored by 0.74.0. Keep them empty unless their runtime requirements are configured.

## Local kind environment

The repository can create a dedicated kind cluster, ingress-nginx controller, local TLS certificate, and split NetBird release:

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
