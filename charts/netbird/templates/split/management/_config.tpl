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
{{- if include "netbird.externalize" (dict "value" (get $stun "password") "ref" $passwordRef) -}}
{{- $_ := set $stun "password" (printf "{{ .NB_MANAGEMENT_STUN_PASSWORD_%d }}" $index) -}}
{{- end -}}
{{- end -}}
{{- with $config.turnConfig -}}
{{- $secretRef := default dict (get . "secretRef") -}}
{{- $_ := unset . "secretRef" -}}
{{- if include "netbird.externalize" (dict "value" (get . "secret") "ref" $secretRef) -}}
{{- $_ := set . "secret" "{{ .NB_MANAGEMENT_TURN_SECRET }}" -}}
{{- end -}}
{{- range $index, $turn := default list .turns -}}
{{- $passwordRef := default dict (get $turn "passwordRef") -}}
{{- $_ := unset $turn "passwordRef" -}}
{{- if include "netbird.externalize" (dict "value" (get $turn "password") "ref" $passwordRef) -}}
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
{{- if include "netbird.externalize" (dict "value" (get . "password") "ref" $passwordRef) -}}
{{- $_ := set . "password" "{{ .NB_MANAGEMENT_SIGNAL_PASSWORD }}" -}}
{{- end -}}
{{- end -}}
{{- with $config.embeddedIdp -}}
{{- $_ := unset . "sessionCookieEncryptionKeyRef" -}}
{{- with .storage.config -}}
{{- $dsnRef := default dict (get . "dsnRef") -}}
{{- $_ := unset . "dsnRef" -}}
{{- if include "netbird.externalize" (dict "value" (get . "dsn") "ref" $dsnRef) -}}
{{- $_ := set . "dsn" "{{ .NB_IDP_STORAGE_DSN }}" -}}
{{- end -}}
{{- end -}}
{{- if .enabled -}}
{{- $_ := set . "sessionCookieEncryptionKey" "{{ .NB_IDP_SESSION_COOKIE_ENCRYPTION_KEY }}" -}}
{{- else -}}
{{- $_ := unset . "sessionCookieEncryptionKey" -}}
{{- end -}}
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


{{/*
Management variables read from a Secret, as a JSON list of
{env, key, value, data, ref, path}. Entries without a ref are stored in the
chart-managed credentials Secret under key, with data (default value).
*/}}
{{- define "netbird.management.secretEnv" -}}
{{- $config := .Values.backend.split.management.config -}}
{{- $base := "backend.split.management.config" -}}
{{- $entries := list -}}
{{- include "netbird.requireSecret" (dict "value" $config.dataStoreEncryptionKey "ref" $config.dataStoreEncryptionKeyRef "path" (printf "%s.dataStoreEncryptionKey" $base)) -}}
{{- $entries = append $entries (dict "env" "NB_MANAGEMENT_DATASTORE_ENCRYPTION_KEY" "key" "datastore-encryption-key" "value" $config.dataStoreEncryptionKey "ref" $config.dataStoreEncryptionKeyRef "path" (printf "%s.dataStoreEncryptionKeyRef" $base)) -}}
{{- $idp := $config.embeddedIdp -}}
{{- if $idp.enabled -}}
{{- include "netbird.requireSecret" (dict "value" $idp.sessionCookieEncryptionKey "ref" $idp.sessionCookieEncryptionKeyRef "path" (printf "%s.embeddedIdp.sessionCookieEncryptionKey" $base)) -}}
{{- $entries = append $entries (dict "env" "NB_IDP_SESSION_COOKIE_ENCRYPTION_KEY" "key" "idp-session-cookie-encryption-key" "value" $idp.sessionCookieEncryptionKey "ref" $idp.sessionCookieEncryptionKeyRef "path" (printf "%s.embeddedIdp.sessionCookieEncryptionKeyRef" $base)) -}}
{{- end -}}
{{- range $index, $stun := default list $config.stuns -}}
{{- if include "netbird.externalize" (dict "value" (get $stun "password") "ref" (get $stun "passwordRef")) -}}
{{- $entries = append $entries (dict "env" (printf "NB_MANAGEMENT_STUN_PASSWORD_%d" $index) "key" (printf "stun-password-%d" $index) "value" (get $stun "password") "ref" (get $stun "passwordRef") "path" (printf "%s.stuns[%d].passwordRef" $base $index)) -}}
{{- end -}}
{{- end -}}
{{- with $config.turnConfig -}}
{{- if include "netbird.externalize" (dict "value" .secret "ref" .secretRef) -}}
{{- $entries = append $entries (dict "env" "NB_MANAGEMENT_TURN_SECRET" "key" "turn-secret" "value" .secret "ref" .secretRef "path" (printf "%s.turnConfig.secretRef" $base)) -}}
{{- end -}}
{{- range $index, $turn := default list .turns -}}
{{- if include "netbird.externalize" (dict "value" (get $turn "password") "ref" (get $turn "passwordRef")) -}}
{{- $entries = append $entries (dict "env" (printf "NB_MANAGEMENT_TURN_PASSWORD_%d" $index) "key" (printf "turn-password-%d" $index) "value" (get $turn "password") "ref" (get $turn "passwordRef") "path" (printf "%s.turnConfig.turns[%d].passwordRef" $base $index)) -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- with $config.signal -}}
{{- if include "netbird.externalize" (dict "value" .password "ref" .passwordRef) -}}
{{- $entries = append $entries (dict "env" "NB_MANAGEMENT_SIGNAL_PASSWORD" "key" "signal-password" "value" .password "ref" .passwordRef "path" (printf "%s.signal.passwordRef" $base)) -}}
{{- end -}}
{{- end -}}
{{- with $idp.storage.config -}}
{{- if include "netbird.externalize" (dict "value" .dsn "ref" .dsnRef) -}}
{{- $entries = append $entries (dict "env" "NB_IDP_STORAGE_DSN" "key" "idp-storage-dsn" "value" .dsn "ref" .dsnRef "path" (printf "%s.embeddedIdp.storage.config.dsnRef" $base)) -}}
{{- end -}}
{{- end -}}
{{- $owner := default dict $idp.owner -}}
{{- if and $owner.email (or $owner.password $owner.passwordRef) -}}
{{- $credentialsName := printf "%s-credentials" (include "netbird.componentName" (dict "root" . "component" "management")) -}}
{{- $hash := include "netbird.passwordHash" (dict "root" . "name" $credentialsName "hashKey" "owner-password-hash" "checksumKey" "owner-password-checksum" "password" $owner.password) -}}
{{- $entries = append $entries (dict "env" "NB_OWNER_PASSWORD_HASH" "key" "owner-password-hash" "value" $owner.password "data" $hash "ref" $owner.passwordRef "path" (printf "%s.embeddedIdp.owner.passwordRef" $base)) -}}
{{- end -}}
{{- toJson $entries -}}
{{- end -}}
