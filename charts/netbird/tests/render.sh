#!/usr/bin/env bash
# Render-only chart checks. Run: charts/netbird/tests/render.sh
set -euo pipefail
chart="$(cd "$(dirname "$0")/.." && pwd)"
render() { helm template t "$chart" --namespace netbird "$@"; }
fails_with() { local want="$1"; shift; local out; if out="$(render "$@" 2>&1)"; then echo "FAIL: render passed, want error: $want"; exit 1; fi; grep -qiF -- "$want" <<<"$out" || { echo "FAIL: want error '$want', got: $out"; exit 1; }; }
has() { grep -qF -- "$1" <<<"$2" || { echo "FAIL: missing '$1'"; exit 1; }; }
lacks() { ! grep -qF -- "$1" <<<"$2" || { echo "FAIL: unexpected '$1'"; exit 1; }; }

required=(
  --set backend.split.management.config.dataStoreEncryptionKey=datastore-key
  --set backend.split.management.config.embeddedIdp.sessionCookieEncryptionKey=cookie-key
  --set backend.split.relay.config.authSecret=relay-secret
)

# 1. No generation: every required credential needs a value or a ref.
fails_with "backend.split.relay.config.authSecretRef is required" "${required[@]:0:4}"
fails_with "dataStoreEncryptionKeyRef is required" "${required[@]:2:4}"
fails_with "sessionCookieEncryptionKeyRef is required" "${required[@]:0:2}" "${required[@]:4:2}"

# 2. Rendering is deterministic without a cluster.
a="$(render "${required[@]}" --set backend.split.management.config.embeddedIdp.owner.email=a@b.c --set backend.split.management.config.embeddedIdp.owner.password=pw)"
b="$(render "${required[@]}" --set backend.split.management.config.embeddedIdp.owner.email=a@b.c --set backend.split.management.config.embeddedIdp.owner.password=pw | sed '/owner-password-hash/d')"
test "$(sed '/owner-password-hash/d' <<<"$a")" = "$b" || { echo "FAIL: renders differ"; exit 1; }

# 3. The cookie key is not required when the embedded IdP is disabled.
out="$(render "${required[@]:0:2}" "${required[@]:4:2}" --set backend.split.management.config.embeddedIdp.enabled=false)"
lacks NB_IDP_SESSION_COOKIE_ENCRYPTION_KEY "$out"
lacks idp-session-cookie-encryption-key "$out"

# 4. Inline secret fields go to the Secret, never the ConfigMap.
out="$(render "${required[@]}" \
  --set backend.split.management.config.turnConfig.secret=turn-secret-value \
  --set backend.split.management.config.signal.password=signal-password-value \
  --set 'backend.split.management.config.stuns[0].uri=stun:x:3478' --set 'backend.split.management.config.stuns[0].password=stun-password-value' \
  --set 'backend.split.management.config.turnConfig.turns[0].uri=turn:x:3478' --set 'backend.split.management.config.turnConfig.turns[0].password=turn-password-value' \
  --set backend.split.management.config.embeddedIdp.storage.config.dsn=dsn-value)"
configmap="$(awk '/^kind: ConfigMap/,/^---/' <<<"$out")"
for v in turn-secret-value signal-password-value stun-password-value turn-password-value dsn-value; do
  lacks "$v" "$configmap"; has "$v" "$out"
done
for p in NB_MANAGEMENT_TURN_SECRET NB_MANAGEMENT_SIGNAL_PASSWORD NB_MANAGEMENT_STUN_PASSWORD_0 NB_MANAGEMENT_TURN_PASSWORD_0 NB_IDP_STORAGE_DSN; do
  has "{{ .$p }}" "$configmap"
done

# 5. Inline values that are already placeholders stay in the ConfigMap.
out="$(render "${required[@]}" --set-json 'backend.split.management.config.turnConfig.secret="{{ .MY_TURN }}"')"
has '"secret": "{{ .MY_TURN }}"' "$out"
lacks NB_MANAGEMENT_TURN_SECRET "$out"

# 6. References only: the chart renders no Secret.
out="$(render --values "$chart/tests/values/secret-refs.yaml")"
lacks "kind: Secret" "$out"

# 7. A user env var cannot shadow a chart-managed one.
fails_with "NB_AUTH_SECRET is set more than once" "${required[@]}" --set backend.split.relay.envFromSecret.NB_AUTH_SECRET=s/k
fails_with "NB_RELAY_AUTH_SECRET is set more than once" "${required[@]}" --set backend.split.management.env.NB_RELAY_AUTH_SECRET=x

# 8. Selectors keep the v1 form <chart>-<component>, for any release name.
out="$(render "${required[@]}")"
has "app.kubernetes.io/name: netbird-management" "$out"
lacks "app.kubernetes.io/name: t-netbird" "$out"

# 9. A v1 values file (top-level management/signal/relay) is rejected.
fails_with "additional propert" "${required[@]}" --set management.enabled=true

# 10. One ServiceMonitor per component with metrics, selecting only that component.
out="$(render "${required[@]}" --set metrics.serviceMonitor.enabled=true --set backend.split.signal.metrics.enabled=true --show-only templates/monitoring/service-monitor.yaml)"
test "$(grep -c "^kind: ServiceMonitor" <<<"$out")" = 1 || { echo "FAIL: want 1 ServiceMonitor"; exit 1; }
has "app.kubernetes.io/name: netbird-signal" "$out"

# 11. Embedded IdP disabled: no embeddedIdp block in management.json.
out="$(render "${required[@]:0:2}" "${required[@]:4:2}" --set backend.split.management.config.embeddedIdp.enabled=false --show-only templates/split/management/configmap.yaml)"
lacks '"embeddedIdp"' "$out"

# 12. Single-account mode is a typed value; "" turns it off.
out="$(render "${required[@]}" --set backend.split.management.server.singleAccountModeDomain= --show-only templates/split/management/deployment.yaml)"
has "--disable-single-account-mode=true" "$out"
lacks "--single-account-mode-domain" "$out"

# 13. postgres/mysql need a DSN, served from a Secret.
fails_with "storeConfig.dsnRef is required" "${required[@]}" --set backend.split.management.config.storeConfig.engine=postgres
out="$(render "${required[@]}" --set backend.split.management.config.storeConfig.engine=mysql --set backend.split.management.config.storeConfig.dsn=dsn-value)"
has NB_STORE_ENGINE_MYSQL_DSN "$out"
lacks dsn-value "$(awk '/^kind: ConfigMap/,/^---/' <<<"$out")"

# 14. Persistence: Recreate strategy and a kept PVC.
out="$(render "${required[@]}")"
has "type: Recreate" "$out"
has "helm.sh/resource-policy: keep" "$out"

# 15. Per-component enabled and namespace: a relay-only render needs only relay values.
out="$(render --set backend.split.relay.config.authSecret=relay-secret --set dashboard.enabled=false --set backend.split.management.enabled=false --set backend.split.signal.enabled=false --set backend.split.relay.namespace=edge)"
test "$(grep "app.kubernetes.io/component:" <<<"$out" | sort -u | tr -d ' ')" = "app.kubernetes.io/component:relay" || { echo "FAIL: relay-only render has other components"; exit 1; }
test "$(grep "^  namespace:" <<<"$out" | sort -u | tr -d ' ')" = "namespace:edge" || { echo "FAIL: relay namespace"; exit 1; }

# 16. Management keeps its own copy of the relay secret, so namespaces can differ.
out="$(render "${required[@]}" --set backend.split.relay.namespace=edge --show-only templates/split/management/secret.yaml)"
has "relay-auth-secret" "$out"

echo "render checks passed"
