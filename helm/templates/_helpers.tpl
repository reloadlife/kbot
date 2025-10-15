{{/*
Expand the name of the chart.
*/}}
{{- define "kubectl-bot.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "kubectl-bot.fullname" -}}
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
{{- define "kubectl-bot.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "kubectl-bot.labels" -}}
helm.sh/chart: {{ include "kubectl-bot.chart" . }}
{{ include "kubectl-bot.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "kubectl-bot.selectorLabels" -}}
app.kubernetes.io/name: {{ include "kubectl-bot.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "kubectl-bot.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "kubectl-bot.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Get the bot namespace
*/}}
{{- define "kubectl-bot.namespace" -}}
{{- if .Values.bot.namespace }}
{{- .Values.bot.namespace }}
{{- else }}
{{- .Release.Namespace }}
{{- end }}
{{- end }}

{{/*
Get the bot deployment name
*/}}
{{- define "kubectl-bot.deploymentName" -}}
{{- if .Values.bot.deploymentName }}
{{- .Values.bot.deploymentName }}
{{- else }}
{{- include "kubectl-bot.fullname" . }}
{{- end }}
{{- end }}

{{/*
Get the secret name for telegram token
*/}}
{{- define "kubectl-bot.secretName" -}}
{{- if .Values.telegram.existingSecret }}
{{- .Values.telegram.existingSecret }}
{{- else }}
{{- include "kubectl-bot.fullname" . }}-secrets
{{- end }}
{{- end }}

{{/*
Get the configmap name for admin IDs
*/}}
{{- define "kubectl-bot.configMapName" -}}
{{- if .Values.telegram.existingConfigMap }}
{{- .Values.telegram.existingConfigMap }}
{{- else }}
{{- include "kubectl-bot.fullname" . }}-config
{{- end }}
{{- end }}
