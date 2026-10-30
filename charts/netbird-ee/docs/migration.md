# Migrate from the netbird chart

A release of the [netbird chart](../../netbird) becomes a netbird-ee release with one `helm upgrade` of the same release. netbird-ee renders the same resources under the same names and selectors, so Helm patches them in place: management keeps its volume or database, the credentials Secrets keep their values, and connected peers stay enrolled. The enterprise tables are created when management-cloud first starts. There is no separate schema migration.

The end-to-end suite runs this migration: `NB_E2E_CHART=netbird-ee NB_E2E_FROM_CHART=netbird task chart:e2e`.

## Before you start

- **Same backend version.** Management, signal and relay must run the version netbird-ee ships (`0.76.2`, see [`values.yaml`](../values.yaml)). If your release is older, upgrade it within the netbird chart first. If it is newer, for example the netbird chart's default `0.77.0`, wait for a netbird-ee release at that version; never move the store to an older management.
- **Single instance.** netbird-ee runs one replica of management and of signal. Set `replicaCount` to 1 for both before migrating.
- **Back up the store.** With the default SQLite store, snapshot the management volume (`<release>-management` PersistentVolumeClaim). With Postgres or MySQL, dump the database. Going back to the netbird chart after the migration is not supported; restoring this backup is the way back.

## Migrate

1. Export the release's values:

   ```bash
   helm get values netbird --namespace netbird --output yaml > netbird-values.yaml
   ```

2. Delete every `image` block from `netbird-values.yaml`, under `dashboard` and under `backend.split.management`, `signal` and `relay`. Values you set win over netbird-ee's, so a leftover `image.repository` or `image.tag` keeps the community image.

3. Nest the values under `netbird:`:

   ```bash
   { echo 'netbird:'; sed 's/^\(.\)/  \1/' netbird-values.yaml; } > netbird-ee-values.yaml
   ```

4. Keep the release name, the namespace and `dataStoreEncryptionKey` (or `dataStoreEncryptionKeyRef`) unchanged. netbird-ee derives `NETBIRD_ENCRYPTION_KEY` from that key, and changing it makes the existing store unreadable.

5. Check what changes. Expect new images, the enterprise environment, and new `helm.sh/chart` labels, and no resource created or deleted:

   ```bash
   helm dependency update charts/netbird-ee
   helm template netbird charts/netbird --namespace netbird -f netbird-values.yaml > before.yaml
   helm template netbird charts/netbird-ee --namespace netbird -f netbird-ee-values.yaml > after.yaml
   diff before.yaml after.yaml
   ```

6. Upgrade the release:

   ```bash
   helm upgrade netbird charts/netbird-ee --namespace netbird -f netbird-ee-values.yaml --wait --timeout 10m
   ```

   Management restarts; peers reconnect once it is ready.

## Verify

```bash
kubectl --namespace netbird get deployments -o custom-columns=NAME:.metadata.name,IMAGE:.spec.template.spec.containers[0].image
kubectl --namespace netbird logs deployment/netbird-management | grep "Starting CloudServer"
```

Management, signal and the dashboard run the `-cloud` images, the relay keeps `netbirdio/relay`, and management logs `Starting CloudServer`. Your peers are still listed in the dashboard.

To turn on the paid modules, add the license afterwards; see [License](../README.md#license).
