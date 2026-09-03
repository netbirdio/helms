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

{{/* The split management embedded-IDP owner field consumes a bcrypt hash. */}}
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
