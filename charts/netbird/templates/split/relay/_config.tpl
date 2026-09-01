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
