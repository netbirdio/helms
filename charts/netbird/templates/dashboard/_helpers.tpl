{{/* Dashboard's typed values become the container's supported environment contract. */}}
{{- define "netbird.dashboardEnv" -}}
{{- $api := .Values.dashboard.config.api -}}
{{- $auth := .Values.dashboard.config.auth -}}
{{- $clientSecretRef := default dict $auth.clientSecretRef -}}
{{- $_ := include "netbird.validateSecretRef" (dict
  "ref" $clientSecretRef
  "value" $auth.clientSecret
  "path" "dashboard.config.auth.clientSecretRef"
) -}}
{{- $generated := dict
  "NETBIRD_MGMT_API_ENDPOINT" $api.httpEndpoint
  "NETBIRD_MGMT_GRPC_API_ENDPOINT" $api.grpcEndpoint
  "AUTH_AUDIENCE" $auth.audience
  "AUTH_CLIENT_ID" $auth.clientId
  "AUTH_AUTHORITY" $auth.authority
  "USE_AUTH0" (toString $auth.useAuth0)
  "AUTH_SUPPORTED_SCOPES" $auth.supportedScopes
  "AUTH_REDIRECT_URI" $auth.redirectUri
  "AUTH_SILENT_REDIRECT_URI" $auth.silentRedirectUri
  "NETBIRD_TOKEN_SOURCE" $auth.tokenSource
-}}
{{- if not $clientSecretRef -}}
{{- $_ := set $generated "AUTH_CLIENT_SECRET" $auth.clientSecret -}}
{{- end -}}
{{- $environment := mergeOverwrite $generated (deepCopy .Values.dashboard.env) -}}
{{- range $key := keys $environment | sortAlpha }}
- name: {{ $key }}
  value: {{ index $environment $key | quote }}
{{- end }}
{{- if and $clientSecretRef (not (hasKey .Values.dashboard.env "AUTH_CLIENT_SECRET")) (not (hasKey .Values.dashboard.envFromSecret "AUTH_CLIENT_SECRET")) }}
- name: AUTH_CLIENT_SECRET
  valueFrom:
    secretKeyRef:
      {{- include "netbird.secretKeyRef" (dict
        "ref" $clientSecretRef
        "value" $auth.clientSecret
        "path" "dashboard.config.auth.clientSecretRef"
      ) | nindent 6 }}
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
