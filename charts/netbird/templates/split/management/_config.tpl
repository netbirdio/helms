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
