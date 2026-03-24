{{/*
Expand the name of the chart.
*/}}
{{- define "netbird-proxy.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "netbird-proxy.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "netbird-proxy.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels.
*/}}
{{- define "netbird-proxy.labels" -}}
helm.sh/chart: {{ include "netbird-proxy.chart" . }}
{{ include "netbird-proxy.selectorLabels" . }}
app.kubernetes.io/component: proxy
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels.
*/}}
{{- define "netbird-proxy.selectorLabels" -}}
app: {{ include "netbird-proxy.fullname" . }}
app.kubernetes.io/name: {{ include "netbird-proxy.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use.
*/}}
{{- define "netbird-proxy.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "netbird-proxy.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Name of the Secret containing the proxy token.
*/}}
{{- define "netbird-proxy.secretName" -}}
{{- if .Values.existingSecret }}
{{- .Values.existingSecret }}
{{- else }}
{{- include "netbird-proxy.fullname" . }}
{{- end }}
{{- end }}

{{/*
Extract the port number from an address string.
Handles ":8443", "0.0.0.0:8443", and "[::1]:8443".
*/}}
{{- define "netbird-proxy.port" -}}
{{- mustRegexFind "[0-9]+$" . -}}
{{- end -}}

{{/*
Name of the Secret containing the OIDC client secret.
*/}}
{{- define "netbird-proxy.oidcSecretName" -}}
{{- if .Values.oidc.existingOidcSecret }}
{{- .Values.oidc.existingOidcSecret }}
{{- else }}
{{- printf "%s-oidc" (include "netbird-proxy.fullname" .) }}
{{- end }}
{{- end }}
