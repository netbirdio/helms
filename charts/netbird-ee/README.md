# netbird-ee

NetBird enterprise on Kubernetes, single instance. The chart wraps the [netbird chart](../netbird) and changes only what the enterprise build needs:

| Component  | Image                                        | Environment                                                                  |
|------------|----------------------------------------------|------------------------------------------------------------------------------|
| management | `ghcr.io/netbirdio/management-cloud:0.76.2`  | `NB_SINGLE_INSTANCE_MODE=true`, `NETBIRD_ENCRYPTION_KEY` ([see below](#encryption-key)) |
| signal     | `ghcr.io/netbirdio/signal-cloud:0.76.2`      | `SINGLE_NODE_MODE=true`                                                      |
| relay      | `netbirdio/relay:0.76.2` (no enterprise build) | —                                                                          |
| dashboard  | `ghcr.io/netbirdio/dashboard-cloud:v2.96.0`  | —                                                                            |

Single instance means one replica of management and of signal, with no Redis, NATS or traffic-flow components. The chart fails to render with `replicaCount` above 1 on either.

## Install

Every value of the netbird chart goes under `netbird:`. Its [installation](../netbird/docs/installation.md) and [configuration](../netbird/docs/configuration.md) guides apply as written once you nest them.

```bash
helm dependency update charts/netbird-ee
helm upgrade --install netbird charts/netbird-ee --namespace netbird --create-namespace -f my-values.yaml
```

```yaml
netbird:
  backend:
    split:
      management:
        config:
          dataStoreEncryptionKeyRef:
            name: netbird-credentials
            key: datastore-encryption-key
```

## Encryption key

Management encrypts the enterprise integrations store with `NETBIRD_ENCRYPTION_KEY`. It must equal the datastore encryption key. Never rotate it.

By default the chart sets it to `$(NB_MANAGEMENT_DATASTORE_ENCRYPTION_KEY)`, the variable the netbird chart fills from `config.dataStoreEncryptionKey` or `config.dataStoreEncryptionKeyRef`. With a ref, the key stays in your Secret: the manifest holds only the `$(…)` reference, and the kubelet resolves it when the pod starts.

To read it from a Secret directly, for example with `overrideConfig`, where that variable is not set, override the entry:

```yaml
netbird:
  backend:
    split:
      management:
        env:
          NETBIRD_ENCRYPTION_KEY:
            valueFrom:
              secretKeyRef:
                name: netbird-credentials
                key: datastore-encryption-key
```

Helm logs that it ignores the chart's string default for that key; that is the override taking effect.

## License

Without a license, management runs the core features and logs `Failed to validate license`; the paid modules stay off. Put the key in a Secret and reference it:

```yaml
netbird:
  backend:
    split:
      management:
        envFromSecret:
          NB_LICENSE_KEY: netbird-license/license-key
```

Management validates the license against `https://license.netbird.io` every 12 hours and needs egress to it. `NETBIRD_LICENSE_SERVER_BASE_URL` overrides the address.

## Tests

`tests/render.sh` lints and renders the chart. The end-to-end suite in [`e2e/netbird`](../../e2e/netbird) runs against this chart with `NB_E2E_CHART=netbird-ee task chart:e2e`.
