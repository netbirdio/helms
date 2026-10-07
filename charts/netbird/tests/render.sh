#!/usr/bin/env bash
# Render-only chart checks. Run: charts/netbird/tests/render.sh
set -euo pipefail
chart="$(cd "$(dirname "$0")/.." && pwd)"
render() { helm template t "$chart" --namespace netbird "$@"; }
fails_with() { local want="$1"; shift; local out; if out="$(render "$@" 2>&1)"; then echo "FAIL: render passed, want error: $want"; exit 1; fi; grep -qF -- "$want" <<<"$out" || { echo "FAIL: want error '$want', got: $out"; exit 1; }; }
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
fails_with "additional properties" "${required[@]}" --set management.enabled=true

# 10. One ServiceMonitor per component with metrics, selecting only that component.
out="$(render "${required[@]}" --set metrics.serviceMonitor.enabled=true --set backend.split.signal.metrics.enabled=true --show-only templates/monitoring/service-monitor.yaml)"
test "$(grep -c "^kind: ServiceMonitor" <<<"$out")" = 1 || { echo "FAIL: want 1 ServiceMonitor"; exit 1; }
has "app.kubernetes.io/name: netbird-signal" "$out"

echo "render checks passed"
