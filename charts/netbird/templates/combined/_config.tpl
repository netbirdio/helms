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
