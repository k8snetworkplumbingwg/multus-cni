{{- define "multusCni.name" -}}
  {{- .Values.nameOverride | default .Chart.Name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "multusCni.fullname" -}}
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

{{- define "multusCni.labels" -}}
{{ include "multusCni.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "multusCni.selectorLabels" -}}
app.kubernetes.io/name: {{ include "multusCni.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app: multus-cni
{{- end }}

{{- define "multusCni.namespace" -}}
  {{- .Values.namespaceOverride | default .Release.Namespace }}
{{- end }}

{{- define "multusCni.imageTag" -}}
  {{- if .Values.image.tag }}
    {{- .Values.image.tag }}
  {{- else if eq .Values.pluginType "thick" }}
    {{- print .Chart.AppVersion "-thick" }}
  {{- else }}
    {{- .Chart.AppVersion }}
  {{- end }}
{{- end }}

{{- define "multusCni.image" -}}
  {{- printf "%s/%s:%s" .Values.image.registry .Values.image.repository (include "multusCni.imageTag" .) }}
{{- end }}
