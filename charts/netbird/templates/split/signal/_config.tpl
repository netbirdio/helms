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
