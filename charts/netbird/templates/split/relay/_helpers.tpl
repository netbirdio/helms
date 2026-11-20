{{/* "true" when the relay runs one address per pod (config.exposedAddressTemplate set); validates the per-pod values. */}}
{{- define "netbird.relay.perPod" -}}
{{- $values := .Values.backend.split.relay -}}
{{- $template := toString (default "" $values.config.exposedAddressTemplate) -}}
{{- if not $template -}}
{{- if gt (int $values.replicaCount) 1 -}}
{{- fail "backend.split.relay.replicaCount > 1 needs backend.split.relay.config.exposedAddressTemplate: a relay only forwards between peers connected to the same pod, so each pod needs its own address" -}}
{{- end -}}
{{- else -}}
{{- if ne (len (regexFindAll "%d" $template -1)) 1 -}}
{{- fail (printf "backend.split.relay.config.exposedAddressTemplate must contain %%d exactly once, got %q" $template) -}}
{{- end -}}
{{- $url := urlParse (replace "%d" "0" $template) -}}
{{- if or (not (has $url.scheme (list "rel" "rels"))) (not $url.host) -}}
{{- fail (printf "backend.split.relay.config.exposedAddressTemplate must look like rels://relay-%%d.example.com:443/relay, got %q" $template) -}}
{{- end -}}
{{- /* Clients dial wss://<host>/relay whatever path the address has, and every pod serves the same port, so only the hostname tells pods apart. */ -}}
{{- if eq (regexReplaceAll ":[0-9]+$" $url.host "") (regexReplaceAll ":[0-9]+$" (urlParse (replace "%d" "1" $template)).host "") -}}
{{- fail (printf "backend.split.relay.config.exposedAddressTemplate must put %%d in the hostname, e.g. rels://relay-%%d.example.com:443/relay: clients ignore the path and every pod serves the same port, got %q" $template) -}}
{{- end -}}
{{- if and $values.service.externalIPs (gt (int $values.replicaCount) 1) -}}
{{- fail "backend.split.relay.service.externalIPs cannot be used with more than one per-pod relay: every per-pod Service would claim the same IP and port, so traffic could not reach a specific pod" -}}
{{- end -}}
{{- $name := include "netbird.componentName" (dict "root" . "component" "relay") -}}
{{- if gt (len $name) 52 -}}
{{- fail (printf "the relay StatefulSet name %q is longer than 52 characters, which its pod names cannot hold; shorten fullnameOverride" $name) -}}
{{- end -}}
true
{{- end -}}
{{- end -}}

{{/* Per-pod relay addresses as a JSON list of {ordinal, address, host}. */}}
{{- define "netbird.relay.podAddresses" -}}
{{- $values := .Values.backend.split.relay -}}
{{- $pods := list -}}
{{- range $ordinal := until (int $values.replicaCount) -}}
{{- $address := replace "%d" (toString $ordinal) $values.config.exposedAddressTemplate -}}
{{- $url := urlParse $address -}}
{{- $pods = append $pods (dict "ordinal" $ordinal "address" $address "host" (regexReplaceAll ":[0-9]+$" $url.host "")) -}}
{{- end -}}
{{- toJson $pods -}}
{{- end -}}

{{/* Context: root, name, podName (optional: select one StatefulSet pod). */}}
{{- define "netbird.relay.service" }}
{{- $values := .root.Values.backend.split.relay }}
apiVersion: v1
kind: Service
metadata:
  name: {{ .name }}
  namespace: {{ include "netbird.componentNamespace" (dict "root" .root "component" "relay") }}
  labels:
    {{- include "netbird.labels" (dict "root" .root "component" "relay") | nindent 4 }}
  {{- with $values.service.annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
spec:
  type: {{ $values.service.type }}
  ports:
    - name: http
      port: {{ $values.service.port }}
      targetPort: http
      protocol: TCP
    {{- if $values.config.metrics.enabled }}
    - name: metrics
      port: {{ $values.config.metrics.port }}
      targetPort: metrics
      protocol: TCP
    {{- end }}
    {{- if $values.config.health.enabled }}
    - name: health
      port: {{ $values.config.health.port }}
      targetPort: health
      protocol: TCP
    {{- end }}
  selector:
    {{- include "netbird.selectorLabels" (dict "root" .root "component" "relay") | nindent 4 }}
    {{- with .podName }}
    statefulset.kubernetes.io/pod-name: {{ . }}
    {{- end }}
  {{- with $values.service.externalIPs }}
  externalIPs:
    {{- toYaml . | nindent 4 }}
  {{- end }}
{{- end }}
