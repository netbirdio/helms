{{/* Chart naming. */}}
{{- define "netbird.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "netbird.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "netbird.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "netbird.namespace" -}}
{{- default .Release.Namespace .Values.global.namespace -}}
{{- end -}}

{{/* Validate the clean-cutover backend selector once from an always-rendered resource. */}}
{{- define "netbird.validate" -}}
{{- if not (has .Values.backend.mode (list "combined" "split")) -}}
{{- fail "backend.mode must be either combined or split" -}}
{{- end -}}
{{- end -}}

{{/* Generic component metadata. Context: root, component. */}}
{{- define "netbird.componentName" -}}
{{- printf "%s-%s" (include "netbird.fullname" .root) .component | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "netbird.selectorLabels" -}}
app.kubernetes.io/name: {{ include "netbird.componentName" . }}
app.kubernetes.io/instance: {{ .root.Release.Name }}
{{- end -}}

{{- define "netbird.labels" -}}
helm.sh/chart: {{ include "netbird.chart" .root }}
{{ include "netbird.selectorLabels" . }}
{{- if .root.Chart.AppVersion }}
app.kubernetes.io/version: {{ .root.Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .root.Release.Service }}
app.kubernetes.io/component: {{ .component }}
{{- end -}}

{{/* Context: root, component, values. */}}
{{- define "netbird.serviceAccountName" -}}
{{- if .values.serviceAccount.create -}}
{{- default (include "netbird.componentName" (dict "root" .root "component" .component)) .values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{/* User-supplied environment entries. Context: values. */}}
{{- define "netbird.env" -}}
{{- range $key := keys .values.env | sortAlpha }}
- name: {{ $key }}
  value: {{ index $.values.env $key | quote }}
{{- end }}
{{- with .values.envRaw }}
{{ toYaml . }}
{{- end }}
{{- range $key := keys .values.envFromSecret | sortAlpha }}
{{- $reference := index $.values.envFromSecret $key -}}
{{- $parts := splitList "/" $reference -}}
{{- if ne (len $parts) 2 -}}
{{- fail (printf "%s envFromSecret reference must use secretName/secretKey" $key) -}}
{{- end }}
- name: {{ $key }}
  valueFrom:
    secretKeyRef:
      name: {{ index $parts 0 }}
      key: {{ index $parts 1 }}
{{- end }}
{{- end -}}

{{/* Preserve generated credentials across upgrades unless a value is configured. */}}
{{- define "netbird.persistedSecret" -}}
{{- $configured := toString (default "" .value) -}}
{{- if $configured -}}
{{- $configured -}}
{{- else -}}
{{- $existing := lookup "v1" "Secret" (include "netbird.namespace" .root) .name -}}
{{- if and $existing (hasKey $existing.data .key) -}}
{{- index $existing.data .key | b64dec -}}
{{- else -}}
{{- if .base64 -}}{{- randBytes (default 32 .length) -}}{{- else -}}{{- randAlphaNum (default 32 .length) -}}{{- end -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/* The branch's combined owner field currently consumes a bcrypt hash. */}}
{{- define "netbird.passwordHash" -}}
{{- $password := toString (default "" .password) -}}
{{- if $password -}}
{{- $checksum := sha256sum $password -}}
{{- $existing := lookup "v1" "Secret" (include "netbird.namespace" .root) .name -}}
{{- if and $existing (hasKey $existing.data .hashKey) (hasKey $existing.data .checksumKey) (eq ((index $existing.data .checksumKey) | b64dec) $checksum) -}}
{{- index $existing.data .hashKey | b64dec -}}
{{- else -}}
{{- htpasswd "netbird" $password | trimPrefix "netbird:" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/* Dashboard's typed values become the container's supported environment contract. */}}
{{- define "netbird.dashboardEnv" -}}
{{- $api := .Values.dashboard.config.api -}}
{{- $auth := .Values.dashboard.config.auth -}}
{{- $generated := dict
  "NETBIRD_MGMT_API_ENDPOINT" $api.httpEndpoint
  "NETBIRD_MGMT_GRPC_API_ENDPOINT" $api.grpcEndpoint
  "AUTH_AUDIENCE" $auth.audience
  "AUTH_CLIENT_ID" $auth.clientId
  "AUTH_CLIENT_SECRET" $auth.clientSecret
  "AUTH_AUTHORITY" $auth.authority
  "USE_AUTH0" (toString $auth.useAuth0)
  "AUTH_SUPPORTED_SCOPES" $auth.supportedScopes
  "AUTH_REDIRECT_URI" $auth.redirectUri
  "AUTH_SILENT_REDIRECT_URI" $auth.silentRedirectUri
  "NETBIRD_TOKEN_SOURCE" $auth.tokenSource
-}}
{{- $environment := mergeOverwrite $generated (deepCopy .Values.dashboard.env) -}}
{{- range $key := keys $environment | sortAlpha }}
- name: {{ $key }}
  value: {{ index $environment $key | quote }}
{{- end }}
{{- with .Values.dashboard.envRaw }}
{{ toYaml . }}
{{- end }}
{{- range $key := keys .Values.dashboard.envFromSecret | sortAlpha }}
{{- $reference := index $.Values.dashboard.envFromSecret $key -}}
{{- $parts := splitList "/" $reference -}}
{{- if ne (len $parts) 2 -}}
{{- fail (printf "%s envFromSecret reference must use secretName/secretKey" $key) -}}
{{- end }}
- name: {{ $key }}
  valueFrom:
    secretKeyRef:
      name: {{ index $parts 0 }}
      key: {{ index $parts 1 }}
{{- end }}
{{- end -}}

{{/* Build one source-compatible combined server YAML document. */}}
{{- define "netbird.combinedConfig" -}}
{{- $values := .Values.backend.combined -}}
{{- if $values.overrideConfig -}}
{{- $values.overrideConfig -}}
{{- else -}}
{{- $config := $values.config -}}
{{- $server := dict
  "listenAddress" (printf ":%v" $values.container.port)
  "exposedAddress" $config.exposedAddress
  "stunPorts" $config.stunPorts
  "metricsPort" $config.metrics.port
  "healthcheckAddress" (printf ":%v" $config.health.port)
  "logLevel" $config.log.level
  "logFile" $config.log.file
  "dataDir" $config.dataDir
  "disableAnonymousMetrics" $config.disableAnonymousMetrics
  "disableGeoliteUpdate" $config.disableGeoliteUpdate
  "stuns" $config.stuns
  "signalUri" $config.signalURI
  "reverseProxy" $config.reverseProxy
  "agentNetwork" $config.agentNetwork
-}}
{{- $relays := omit (deepCopy $config.relays) "secret" -}}
{{- $_ := set $server "relays" $relays -}}

{{- $stores := $config.store -}}
{{- $defaultStore := deepCopy $stores.default -}}
{{- $netbirdStore := mergeOverwrite (deepCopy $defaultStore) (deepCopy (default dict $stores.netbird)) -}}
{{- $authStore := mergeOverwrite (deepCopy $defaultStore) (deepCopy (default dict $stores.auth)) -}}
{{- $activityStore := mergeOverwrite (deepCopy $defaultStore) (deepCopy (default dict $stores.activity)) -}}

{{- $netbirdOutput := dict "engine" (default "sqlite" $netbirdStore.engine) -}}
{{- with $netbirdStore.file }}{{- $_ := set $netbirdOutput "file" . -}}{{- end -}}
{{- $authEngine := lower (default "sqlite" $authStore.engine) -}}
{{- if eq $authEngine "sqlite" }}{{- $authEngine = "sqlite3" -}}{{- end -}}
{{- $authOutput := dict "engine" $authEngine -}}
{{- with $authStore.file }}{{- $_ := set $authOutput "file" . -}}{{- end -}}
{{- $activityEngine := lower (default "sqlite" $activityStore.engine) -}}
{{- if eq $activityEngine "sqlite3" }}{{- $activityEngine = "sqlite" -}}{{- end -}}
{{- $activityOutput := dict "engine" $activityEngine -}}
{{- with $activityStore.file }}{{- $_ := set $activityOutput "file" . -}}{{- end -}}
{{- $_ := set $server "store" $netbirdOutput -}}
{{- $_ := set $server "authStore" $authOutput -}}
{{- $_ := set $server "activityStore" $activityOutput -}}

{{- $auth := omit (deepCopy $config.auth) "secret" "sessionCookieEncryptionKey" "owner" -}}
{{- $_ := set $server "auth" $auth -}}
{{- if ne $config.supportedSyncMessageVersions nil -}}
{{- $_ := set $server "supportedSyncMessageVersions" $config.supportedSyncMessageVersions -}}
{{- end -}}
{{- with $config.perAccountSupportedSyncMessageVersions -}}
{{- $_ := set $server "perAccountSupportedSyncMessageVersions" . -}}
{{- end -}}
{{- toYaml (dict "server" $server) -}}
{{- end -}}
{{- end -}}

{{/* Render management's established JSON schema with runtime secret expansion. */}}
{{- define "netbird.managementConfig" -}}
{{- $values := .Values.backend.split.management -}}
{{- if $values.overrideConfig -}}
{{- $values.overrideConfig -}}
{{- else -}}
{{- $config := deepCopy $values.config -}}
{{- $_ := set $config "dataStoreEncryptionKey" "{{ .NB_MANAGEMENT_DATASTORE_ENCRYPTION_KEY }}" -}}
{{- with $config.relay -}}
{{- $_ := set . "secret" "{{ .NB_RELAY_AUTH_SECRET }}" -}}
{{- end -}}
{{- with $config.embeddedIdp -}}
{{- $_ := set . "sessionCookieEncryptionKey" "{{ .NB_IDP_SESSION_COOKIE_ENCRYPTION_KEY }}" -}}
{{- $owner := deepCopy (default dict .owner) -}}
{{- $email := toString (default "" $owner.email) -}}
{{- $password := toString (default "" $owner.password) -}}
{{- if or $email $password -}}
{{- if not (and $email $password) -}}
{{- fail "backend.split.management.config.embeddedIdp.owner.email and password must be set together" -}}
{{- end -}}
{{- $_ := unset $owner "password" -}}
{{- $_ := set $owner "hash" "{{ .NB_OWNER_PASSWORD_HASH }}" -}}
{{- $_ := set . "owner" $owner -}}
{{- else -}}
{{- $_ := unset . "owner" -}}
{{- end -}}
{{- end -}}
{{- toPrettyJson $config -}}
{{- end -}}
{{- end -}}

{{/* Render the branch's strict signal YAML schema. */}}
{{- define "netbird.signalConfig" -}}
{{- $values := .Values.backend.split.signal -}}
{{- if $values.overrideConfig -}}
{{- $values.overrideConfig -}}
{{- else -}}
{{- $config := deepCopy $values.config -}}
{{- $_ := set $config "port" $values.container.port -}}
{{- $_ := set $config "metricsPort" $values.metrics.port -}}
{{- toYaml $config -}}
{{- end -}}
{{- end -}}

{{/* Render the branch's strict relay YAML schema. */}}
{{- define "netbird.relayConfig" -}}
{{- $values := .Values.backend.split.relay -}}
{{- if $values.overrideConfig -}}
{{- $values.overrideConfig -}}
{{- else -}}
{{- $config := $values.config -}}
{{- $output := dict
  "listenAddress" (printf ":%v" $values.container.port)
  "exposedAddress" $config.exposedAddress
  "metricsPort" $config.metrics.port
  "letsencryptEmail" $config.letsencrypt.email
  "letsencryptDataDir" $config.letsencrypt.dataDir
  "letsencryptDomains" $config.letsencrypt.domains
  "letsencryptAWSRoute53" $config.letsencrypt.awsRoute53
  "tlsCertFile" $config.tls.certFile
  "tlsKeyFile" $config.tls.keyFile
  "logLevel" $config.log.level
  "logFile" $config.log.file
  "healthcheckListenAddress" (printf ":%v" $config.health.port)
  "trustedProxies" $config.trustedProxies
  "enableSTUN" $config.stun.enabled
  "stunPorts" $config.stun.ports
  "stunLogLevel" $config.stun.logLevel
-}}
{{- toYaml $output -}}
{{- end -}}
{{- end -}}
