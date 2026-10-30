# NetBird chart documentation

These guides cover installing, configuring, and changing the NetBird Helm chart:

- [Installation](installation.md) — install the split dashboard, management, signal, and relay topology.
- [Configuration and secrets](configuration.md) — use typed values, Secret references, management persistence, relay STUN, and management's raw JSON override.
- [Template development](development.md) — find component templates and safely change generated manifests.

Chart `2.0.0` tests NetBird backend images from `0.74.0` through the default `0.77.0`. Its dashboard is independently pinned to `netbirdio/dashboard:v2.91.1`.

The complete value surface is documented inline in [`../values.yaml`](../values.yaml). Runnable configurations are under [`../examples`](../examples), including the local kind environment and external identity-provider examples.

Return to the [chart README](../README.md).
