# Rootless NetBird

This example runs every component under the [restricted Pod Security Standard](https://kubernetes.io/docs/concepts/security/pod-security-standards/#restricted). Each component has a `rootless` value, `false` by default. When it is `true`, the chart sets:

| Component | User | Container port |
| --- | --- | --- |
| dashboard | `10001:0` from the image (needs a `*-rootless` tag) | `8080` |
| management | `65532:65532`, `fsGroup: 65532` | `8080` |
| signal | `65532:65532`, `fsGroup: 65532` | `10000` |
| relay | `65532:65532`, `fsGroup: 65532` | `33080` |

Every rootless pod also gets `runAsNonRoot` and the `RuntimeDefault` seccomp profile, and every rootless container drops all capabilities and disallows privilege escalation. The Service ports do not change, so Ingress and Route definitions stay the same.

## Usage

Layer [`values.yaml`](values.yaml) over your own values:

```bash
kubectl create namespace netbird
kubectl label namespace netbird \
  pod-security.kubernetes.io/enforce=restricted \
  pod-security.kubernetes.io/enforce-version=latest

helm upgrade --install netbird ./charts/netbird \
  --namespace netbird \
  --values my-values.yaml \
  --values ./charts/netbird/examples/rootless/values.yaml
```

The label is optional: it makes Kubernetes reject any pod that does not meet the standard.

The dashboard tag must be a `*-rootless` variant (published from `v2.95.0`). The chart does not rewrite the tag, so set it whenever you change `dashboard.image.tag` or `dashboard.image.digest`.

`rootless` can be set per component, for example to keep a custom root dashboard image while the backend runs rootless.

## Overriding the defaults

Keys under `pod.securityContext` and `container.securityContext` replace the rootless default of the same name, and `null` removes one:

```yaml
backend:
  split:
    signal:
      rootless: true
      pod:
        securityContext:
          runAsUser: 1000    # replaces 65532
          fsGroup: null      # removes fsGroup
```

`container.port` set to a number wins over the rootless port.

## OpenShift

OpenShift's `restricted-v2` SCC assigns the user and group from the namespace range and rejects fixed IDs. With `global.compatibility.openshift.adaptSecurityContext: auto` (the default) the chart drops `runAsUser`, `runAsGroup` and `fsGroup` when the cluster serves `security.openshift.io/v1`. `helm template` cannot detect the cluster, so set it to `force` for GitOps renders.

`restricted-v2` also runs the stock images as a non-root user, so they cannot bind port 80: use this example on OpenShift.

## Storage

Management's data directory is owned by root in the image. With persistence, `fsGroup` makes the volume writable for user `65532`; a volume whose files were written by a root management keeps root ownership on storage that ignores `fsGroup`, and needs `chown -R 65532:65532`. Without persistence, management mounts an `emptyDir` at the data directory.
