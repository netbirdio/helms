# Enterprise edition

`edition: enterprise` runs NetBird enterprise on the same chart. It changes only what the enterprise build needs:

| Component  | Image                                                    | Environment                                                         |
|------------|----------------------------------------------------------|---------------------------------------------------------------------|
| management | `ghcr.io/netbirdio/management-cloud:0.82.0-enterprise.1` | `NB_SINGLE_INSTANCE_MODE=true`, `NETBIRD_ENCRYPTION_KEY` ([see below](#encryption-key)) |
| signal     | `ghcr.io/netbirdio/signal-cloud:0.82.0-enterprise.1`     | `SINGLE_NODE_MODE=true`                                             |
| relay      | `netbirdio/relay:0.82.0` (no enterprise build)           | —                                                                   |
| dashboard  | `ghcr.io/netbirdio/dashboard-cloud:v2.96.0`              | —                                                                   |

The enterprise edition runs single instance: one replica of management and of signal, with no Redis, NATS or traffic-flow components. The chart fails to render with `replicaCount` above 1 on either.

Every other value works as in the community edition, so the [installation](installation.md) and [configuration](configuration.md) guides apply as written.

## Install

Follow the [installation guide](installation.md), credentials Secret included, and add one line to your values file:

```yaml
edition: enterprise
```

Then install with the guide's command:

```bash
helm upgrade --install netbird charts/netbird --namespace netbird --create-namespace --values values-production.yaml --wait
```

An `image.repository` or `image.tag` you set wins over the edition's default, for example to pull from a mirror. Leave both empty to follow the edition.

## Encryption key

Management encrypts the enterprise integrations store with `NETBIRD_ENCRYPTION_KEY`. It must equal the datastore encryption key, so the chart reads it from the same place: `config.dataStoreEncryptionKey` or `config.dataStoreEncryptionKeyRef`. Never rotate it.

With `overrideConfig` the chart sets no credentials, so set `NETBIRD_ENCRYPTION_KEY` yourself; the render fails without it:

```yaml
backend:
  split:
    management:
      envFromSecret:
        NETBIRD_ENCRYPTION_KEY: netbird-external-credentials/datastore-encryption-key
```

## License

Without a license, management runs the core features and logs `Failed to validate license`; the paid modules stay off. Put the key in a Secret and reference it:

```yaml
backend:
  split:
    management:
      envFromSecret:
        NB_LICENSE_KEY: netbird-license/license-key
```

Management validates the license against `https://license.netbird.io` every 12 hours and needs egress to it. `NETBIRD_LICENSE_SERVER_BASE_URL` overrides the address.

## Switch a community release to enterprise

A community release becomes enterprise with one `helm upgrade` of the same release. The chart renders the same resources under the same names and selectors, so Helm patches them in place: management keeps its volume or database, the credentials Secrets keep their values, and connected peers stay enrolled. The enterprise tables are created when management-cloud first starts. There is no separate schema migration.

The end-to-end suite runs this switch: `NB_E2E_FROM_EDITION=community task chart:e2e EDITION=enterprise`.

### Before you start

- **Same backend version.** Management and signal must run the version the enterprise build is based on: `0.82.0` for `0.82.0-enterprise.1`. If your release pins an older version, upgrade it on the community edition first. A pinned `image.digest` wins over the tag, so clear it too:

  ```bash
  helm upgrade netbird charts/netbird --namespace netbird --reuse-values \
    --set backend.split.management.image.tag=0.82.0 \
    --set backend.split.management.image.digest=null \
    --set backend.split.signal.image.tag=0.82.0 \
    --set backend.split.signal.image.digest=null
  kubectl get deployments --namespace netbird \
    -o custom-columns='NAME:.metadata.name,IMAGE:.spec.template.spec.containers[0].image'
  ```

  Management and signal must both show `0.82.0`. If your release runs a newer version than the enterprise default, wait for a chart release at that version; never move the store to an older management.
- **No MySQL.** NetBird 0.82.0 removed the MySQL store: management refuses to start with it, and the chart refuses to render it. On MySQL, [migrate to PostgreSQL](https://docs.netbird.io/selfhosted/migration/mysql-to-postgresql) while still on 0.81.x, with the tool from the [v0.81.5 release](https://github.com/netbirdio/netbird/releases/tag/v0.81.5), before the upgrade above.
- **Single instance.** Set `replicaCount` to 1 for management and signal.
- **Back up the store.** With the default SQLite store, snapshot the management volume (`<release>-management` PersistentVolumeClaim). With Postgres, dump the database. Going back to the community edition after the switch is not supported; restoring this backup is the way back.

### Switch

1. Export the release's values:

   ```bash
   helm get values netbird --namespace netbird --output yaml > netbird-values.yaml
   ```

2. Delete the `repository`, `tag` and `digest` of every `image` block under `dashboard` and `backend.split.management` and `signal`. Values you set win over the edition's, so a leftover `repository` or `tag` keeps the community image, and a leftover `digest` pins the enterprise repository to a community image that does not exist there. The relay's image stays as it is.

3. Keep the release name, the namespace and `dataStoreEncryptionKey` (or `dataStoreEncryptionKeyRef`) unchanged. `NETBIRD_ENCRYPTION_KEY` is read from that key, and changing it makes the existing store unreadable.

4. Check what changes. Expect new images and the enterprise environment, and no resource created or deleted:

   ```bash
   helm get manifest netbird --namespace netbird > before.yaml
   helm template netbird charts/netbird --namespace netbird -f netbird-values.yaml --set edition=enterprise > after.yaml
   diff before.yaml after.yaml
   ```

5. Upgrade the release:

   ```bash
   helm upgrade netbird charts/netbird --namespace netbird -f netbird-values.yaml --set edition=enterprise --wait --timeout 10m
   ```

   Management restarts; peers reconnect once it is ready.

### Verify

```bash
kubectl --namespace netbird get deployments -o custom-columns=NAME:.metadata.name,IMAGE:.spec.template.spec.containers[0].image
kubectl --namespace netbird logs deployment/netbird-management | grep "Starting CloudServer"
```

Management, signal and the dashboard run the `-cloud` images, the relay keeps `netbirdio/relay`, and management logs `Starting CloudServer`. Your peers are still listed in the dashboard.

To turn on the paid modules, add the license afterwards; see [License](#license).
