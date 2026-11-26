# High availability

`ha.enabled` runs management and signal as several replicas on the [enterprise edition](enterprise.md). The replicas share their state through services you host; the chart installs none of them and takes only their addresses:

| Service    | What for                                              | Requirement                                                   |
|------------|-------------------------------------------------------|---------------------------------------------------------------|
| PostgreSQL | The store, the activity events, the enterprise integrations and the embedded IdP | Every replica reaches the same databases                       |
| Redis      | Management's shared cache                             | One standalone server: no Redis Cluster or Sentinel            |
| NATS       | Network map updates between management replicas, and peer messages between signal replicas | At least 3 nodes. Plain NATS: no JetStream needed |

This follows NetBird's [self-hosted enterprise guide](https://docs.netbird.io/selfhosted/enterprise). This version runs the separate management and signal images; the combined `netbird-server-cloud` image comes later.

## Install

1. Create the databases: one for the store and one for the embedded IdP (Dex keeps its own tables), for example `netbird` and `idp`.

2. Put the connection details in a Secret, in the namespace of management and signal:

   ```bash
   kubectl --namespace netbird create secret generic netbird-ha \
     --from-literal=store-dsn='host=postgres.example.internal user=netbird password=... dbname=netbird port=5432 sslmode=require' \
     --from-literal=idp-dsn='postgres://netbird:...@postgres.example.internal:5432/idp?sslmode=require' \
     --from-literal=redis-url='rediss://:...@redis.example.internal:6379/0' \
     --from-literal=nats-urls='nats://nats-0.example.internal:4222,nats://nats-1.example.internal:4222,nats://nats-2.example.internal:4222'
   ```

   `redis-url` is one `redis://` or `rediss://` URL, password included. `nats-urls` lists the NATS servers, comma-separated.

3. Follow the [installation guide](installation.md), and layer [`examples/ha`](../examples/ha/values.yaml) over your values. It sets `edition: enterprise`, turns `ha` on, runs two replicas of management and signal on PostgreSQL, and two per-pod relays. Change `exposedAddressTemplate` to your relay hostnames:

   ```bash
   helm upgrade --install netbird charts/netbird --namespace netbird \
     --values values-production.yaml --values charts/netbird/examples/ha/values.yaml --wait
   ```

Every replica must use the same `dataStoreEncryptionKey` and IdP `sessionCookieEncryptionKey`. The chart already reads both from one place, `config` or its `*Ref`, so nothing changes there.

## Values

```yaml
edition: enterprise
ha:
  enabled: true
  redis:
    url: ""            # or urlRef: {name, key}
    urlRef: {}
  nats:
    urls: []           # or urlsRef: {name, key}, comma-separated
    urlsRef: {}
```

Inline `ha.redis.url` and `ha.nats.urls` go into the management credentials Secret, as the chart does with other inline credentials. `ha.nats.urlsRef` must exist in both management's and signal's namespaces.

## What it renders

The variables are the ones the combined enterprise server sets from its own `server.ha` settings, so both run management and signal the same way:

| Component  | Variable                          | From                                   |
|------------|-----------------------------------|----------------------------------------|
| management | `NB_SINGLE_INSTANCE_MODE=false`   | `ha.enabled` (`true` without HA)       |
| management | `NB_CACHE_REDIS_ADDRESS`          | `ha.redis.url` or `urlRef`             |
| management | `NB_NATS_ENDPOINTS`               | `ha.nats.urls` or `urlsRef`            |
| management | `NB_ACTIVITY_EVENT_STORE_ENGINE=postgres`, `NB_ACTIVITY_EVENT_POSTGRES_DSN` | the store DSN |
| management | `NETBIRD_CLOUD_POSTGRES_DSN`      | the store DSN; set on every enterprise release with a PostgreSQL store, HA or not |
| signal     | `SINGLE_NODE_MODE=false`          | `ha.enabled` (`true` without HA)       |
| signal     | `NATS_ENDPOINTS`                  | `ha.nats.urls` or `urlsRef`            |

Management runs without its volume: each replica keeps only the geolocation databases, downloaded at start into an `emptyDir` (about 460 MB while unpacking), so leave room for that in any `ephemeral-storage` limit.

The render fails, naming the value to change, when:

- `edition` is not `enterprise`;
- Redis or NATS is not set;
- `storeConfig.engine` is not `postgres`, or the embedded IdP's `storage.type` is not `postgres` (with its own `dsn` or `dsnRef`);
- `management.persistence.enabled` is on: a `ReadWriteOnce` volume cannot be shared by the replicas;
- `management.overrideConfig` is set.

Without `ha.enabled`, the enterprise edition still fails to render with `replicaCount` above 1 on management or signal.

## Relays and the load balancer

Relays share no state, so they scale differently: each pod gets its own address, never one address shared behind a load balancer. `examples/ha` runs two with `exposedAddressTemplate`; see [Multiple relays](configuration.md#multiple-relays).

Management's gRPC and HTTP and signal's gRPC go through your ingress or load balancer, which needs HTTP/2 and gRPC, as without HA. Peers reach any replica; Redis and NATS make the replicas agree.

## Limits

- **Rollouts drop connections.** Peers on a replaced management or signal pod reconnect to another replica; management does not drain its gRPC streams first.
- **Every management replica runs the IdP and EDR syncs.**
- **Traffic flow** (a receiver and an enricher next to each management replica) is not part of the chart.
- **Not yet in the chart:** PodDisruptionBudgets and spread constraints for management and signal, and a management readiness probe on `/oauth2/.well-known/openid-configuration`. Set `topologySpreadConstraints` or `affinity` per component meanwhile.

## Turn HA on for a running release

HA keeps nothing on the management volume, so move what is on it to PostgreSQL first, with [Migrate from SQLite to PostgreSQL](https://docs.netbird.io/selfhosted/maintenance/scaling/migrate-sqlite-to-postgresql): the store (`store.db`), the activity events (`events.db`) and the embedded IdP (`idp.db`). Activity events not migrated start fresh. Back up as for [the switch to enterprise](enterprise.md#before-you-start), then upgrade with `examples/ha`, which turns `persistence` off.

## Test

The end-to-end suite installs PostgreSQL, Redis and a 3-node NATS cluster next to the chart, then runs `examples/ha`: `task chart:e2e HA=true`. Peers enroll through random replicas, connect directly and through the relay, enroll again while a management and a signal replica are replaced, and stay listed after every management replica restarts.
