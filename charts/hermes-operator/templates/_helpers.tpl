{{- define "hermes-operator.fullname" -}}
{{- printf "%s" (default .Chart.Name .Values.nameOverride) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "hermes-operator.labels" -}}
app.kubernetes.io/name: {{ include "hermes-operator.fullname" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: hermes.agent
{{- end -}}

{{- define "hermes-operator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "hermes-operator.fullname" . }}
{{- end -}}

{{- define "hermes-operator.image" -}}
{{- /*
Fall back to the chart's appVersion, normalised to the v-prefixed form the
release workflow publishes. appVersion is written bare by release-please in
Chart.yaml but a packaged chart may carry it v-prefixed, so strip any existing
"v" before adding one - otherwise the tag renders as "vv0.1.19".
*/ -}}
{{- $tag := default (printf "v%s" (trimPrefix "v" .Chart.AppVersion)) .Values.image.tag -}}
{{- printf "%s:%s" .Values.image.repository $tag -}}
{{- end -}}
