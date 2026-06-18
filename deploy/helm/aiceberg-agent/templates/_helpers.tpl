{{- define "aiceberg-agent.name" -}}
aiceberg-agent
{{- end -}}

{{- define "aiceberg-agent.labels" -}}
app.kubernetes.io/name: {{ include "aiceberg-agent.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}
