#!/usr/bin/env bash
# Lint and render-only checks for netbird-ee. Run: charts/netbird-ee/tests/render.sh
set -euo pipefail
chart="$(cd "$(dirname "$0")/.." && pwd)"
render() { helm template t "$chart" --namespace netbird "$@"; }
fails_with() { local want="$1"; shift; local out; if out="$(render "$@" 2>&1)"; then echo "FAIL: render passed, want error: $want"; exit 1; fi; grep -qiF -- "$want" <<<"$out" || { echo "FAIL: want error '$want', got: $out"; exit 1; }; }
has() { grep -qF -- "$1" <<<"$2" || { echo "FAIL: missing '$1'"; exit 1; }; }

helm dependency update "$chart" >/dev/null

required=(
  --set netbird.backend.split.management.config.dataStoreEncryptionKey=datastore-key
  --set netbird.backend.split.management.config.embeddedIdp.sessionCookieEncryptionKey=cookie-key
  --set netbird.backend.split.relay.config.authSecret=relay-secret
)

helm lint --strict "$chart" "${required[@]}"

# 1. Enterprise images; relay stays community.
out="$(render "${required[@]}")"
has 'image: "ghcr.io/netbirdio/management-cloud:0.82.0-enterprise.1"' "$out"
has 'image: "ghcr.io/netbirdio/signal-cloud:0.82.0-enterprise.1"' "$out"
has 'image: "ghcr.io/netbirdio/dashboard-cloud:v2.96.0"' "$out"
has 'image: "netbirdio/relay:0.76.2"' "$out"

# 2. Single-instance env, and the integrations key follows the datastore key.
grep -A1 "name: NB_SINGLE_INSTANCE_MODE" <<<"$out" | grep -qF '"true"' || { echo "FAIL: NB_SINGLE_INSTANCE_MODE"; exit 1; }
grep -A1 "name: SINGLE_NODE_MODE" <<<"$out" | grep -qF '"true"' || { echo "FAIL: SINGLE_NODE_MODE"; exit 1; }
grep -A1 "name: NETBIRD_ENCRYPTION_KEY" <<<"$out" | grep -qF '$(NB_MANAGEMENT_DATASTORE_ENCRYPTION_KEY)' || { echo "FAIL: NETBIRD_ENCRYPTION_KEY"; exit 1; }
# Kubernetes only expands $(VAR) from entries listed before it.
env_order="$(grep -oE 'name: (NB_MANAGEMENT_DATASTORE_ENCRYPTION_KEY|NETBIRD_ENCRYPTION_KEY)' <<<"$out" | tr '\n' ' ')"
test "$env_order" = "name: NB_MANAGEMENT_DATASTORE_ENCRYPTION_KEY name: NETBIRD_ENCRYPTION_KEY " || { echo "FAIL: env order: $env_order"; exit 1; }

# 3. NETBIRD_ENCRYPTION_KEY can come from a Secret, and other env entries stay.
out="$(render "${required[@]}" \
  --set netbird.backend.split.management.env.NETBIRD_ENCRYPTION_KEY.valueFrom.secretKeyRef.name=my-keys \
  --set netbird.backend.split.management.env.NETBIRD_ENCRYPTION_KEY.valueFrom.secretKeyRef.key=integrations)"
has "name: NB_SINGLE_INSTANCE_MODE" "$out"
grep -A4 "name: NETBIRD_ENCRYPTION_KEY" <<<"$out" | grep -qF "name: my-keys" || { echo "FAIL: NETBIRD_ENCRYPTION_KEY secretKeyRef"; exit 1; }
! grep -qF '$(NB_MANAGEMENT_DATASTORE_ENCRYPTION_KEY)' <<<"$out" || { echo "FAIL: default NETBIRD_ENCRYPTION_KEY left in"; exit 1; }

# 4. Single instance only.
fails_with "netbird.backend.split.management.replicaCount must be 1" "${required[@]}" --set netbird.backend.split.management.replicaCount=2
fails_with "netbird.backend.split.signal.replicaCount must be 1" "${required[@]}" --set netbird.backend.split.signal.replicaCount=2

# 5. examples/kind is the netbird chart's examples/kind under netbird:, and renders.
settings() { grep -vE '^\s*(#|$)' "$1"; }
diff <(settings "$chart/../netbird/examples/kind/values.yaml") <(settings "$chart/examples/kind/values.yaml" | sed '1{/^netbird:$/d}; s/^  //') \
  || { echo "FAIL: examples/kind drifted from the netbird chart's; see the diff above"; exit 1; }
render --values "$chart/examples/kind/values.yaml" >/dev/null

echo "netbird-ee render checks passed"
