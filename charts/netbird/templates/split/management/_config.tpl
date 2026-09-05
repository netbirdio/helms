{{/* Render management's established JSON schema with runtime secret expansion. */}}
{{- define "netbird.managementConfig" -}}
{{- $values := .Values.backend.split.management -}}
{{- if $values.overrideConfig -}}
{{- $values.overrideConfig -}}
{{- else -}}
{{- $config := deepCopy $values.config -}}
{{- $_ := unset $config "dataStoreEncryptionKeyRef" -}}
{{- $_ := set $config "dataStoreEncryptionKey" "{{ .NB_MANAGEMENT_DATASTORE_ENCRYPTION_KEY }}" -}}
{{- range $index, $stun := default list $config.stuns -}}
{{- $passwordRef := default dict (get $stun "passwordRef") -}}
{{- $_ := unset $stun "passwordRef" -}}
{{- if $passwordRef -}}
{{- $_ := set $stun "password" (printf "{{ .NB_MANAGEMENT_STUN_PASSWORD_%d }}" $index) -}}
{{- end -}}
{{- end -}}
{{- with $config.turnConfig -}}
{{- $secretRef := default dict (get . "secretRef") -}}
{{- $_ := unset . "secretRef" -}}
{{- if $secretRef -}}
{{- $_ := set . "secret" "{{ .NB_MANAGEMENT_TURN_SECRET }}" -}}
{{- end -}}
{{- range $index, $turn := default list .turns -}}
{{- $passwordRef := default dict (get $turn "passwordRef") -}}
{{- $_ := unset $turn "passwordRef" -}}
{{- if $passwordRef -}}
{{- $_ := set $turn "password" (printf "{{ .NB_MANAGEMENT_TURN_PASSWORD_%d }}" $index) -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- with $config.relay -}}
{{- $_ := unset . "secretRef" -}}
{{- $_ := set . "secret" "{{ .NB_RELAY_AUTH_SECRET }}" -}}
{{- end -}}
{{- with $config.signal -}}
{{- $passwordRef := default dict (get . "passwordRef") -}}
{{- $_ := unset . "passwordRef" -}}
{{- if $passwordRef -}}
{{- $_ := set . "password" "{{ .NB_MANAGEMENT_SIGNAL_PASSWORD }}" -}}
{{- end -}}
{{- end -}}
{{- with $config.embeddedIdp -}}
{{- $_ := unset . "sessionCookieEncryptionKeyRef" -}}
{{- with .storage.config -}}
{{- $dsnRef := default dict (get . "dsnRef") -}}
{{- $_ := unset . "dsnRef" -}}
{{- if $dsnRef -}}
{{- $_ := set . "dsn" "{{ .NB_IDP_STORAGE_DSN }}" -}}
{{- end -}}
{{- end -}}
{{- $_ := set . "sessionCookieEncryptionKey" "{{ .NB_IDP_SESSION_COOKIE_ENCRYPTION_KEY }}" -}}
{{- $owner := deepCopy (default dict .owner) -}}
{{- $email := toString (default "" $owner.email) -}}
{{- $password := toString (default "" $owner.password) -}}
{{- $passwordRef := default dict $owner.passwordRef -}}
{{- $_ := unset $owner "passwordRef" -}}
{{- if and $password $passwordRef -}}
{{- fail "backend.split.management.config.embeddedIdp.owner.password and passwordRef are mutually exclusive" -}}
{{- end -}}
{{- if or $email $password $passwordRef -}}
{{- if not (and $email (or $password $passwordRef)) -}}
{{- fail "backend.split.management.config.embeddedIdp.owner.email and one of password or passwordRef must be set together" -}}
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
