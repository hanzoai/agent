{{- define "hanzo-agents.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "hanzo-agents.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := include "hanzo-agents.name" . -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "hanzo-agents.labels" -}}
app.kubernetes.io/name: {{ include "hanzo-agents.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | quote }}
{{- end -}}

{{- define "hanzo-agents.selectorLabels" -}}
app.kubernetes.io/name: {{ include "hanzo-agents.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "hanzo-agents.controlPlane.fullname" -}}
{{- printf "%s-control-plane" (include "hanzo-agents.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "hanzo-agents.postgres.fullname" -}}
{{- printf "%s-postgres" (include "hanzo-agents.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "hanzo-agents.demoAgent.fullname" -}}
{{- printf "%s-demo-agent" (include "hanzo-agents.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "hanzo-agents.controlPlane.grpcPort" -}}
{{- $grpcPort := int (default 0 .Values.controlPlane.service.grpcPort) -}}
{{- if eq $grpcPort 0 -}}
{{- add (int .Values.controlPlane.service.port) 100 -}}
{{- else -}}
{{- $grpcPort -}}
{{- end -}}
{{- end -}}

{{- define "hanzo-agents.controlPlane.postgresUrl" -}}
{{- $url := default "" .Values.controlPlane.storage.postgresUrl -}}
{{- if $url -}}
{{- $url -}}
{{- else if and .Values.postgres.enabled (not .Values.postgres.auth.existingSecret) -}}
{{- printf "postgres://%s:%s@%s:5432/%s?sslmode=disable" .Values.postgres.auth.username .Values.postgres.auth.password (include "hanzo-agents.postgres.fullname" .) .Values.postgres.auth.database -}}
{{- else -}}
{{- "" -}}
{{- end -}}
{{- end -}}

{{- define "hanzo-agents.apiAuth.secretName" -}}
{{- if .Values.apiAuth.existingSecret -}}
{{- .Values.apiAuth.existingSecret -}}
{{- else -}}
{{- printf "%s-api-auth" (include "hanzo-agents.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "hanzo-agents.postgres.secretName" -}}
{{- if .Values.postgres.auth.existingSecret -}}
{{- .Values.postgres.auth.existingSecret -}}
{{- else -}}
{{- printf "%s-postgres-auth" (include "hanzo-agents.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
