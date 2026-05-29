{{- define "omni-cluster-operator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "omni-cluster-operator.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "omni-cluster-operator.labels" -}}
app.kubernetes.io/name: {{ include "omni-cluster-operator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | quote }}
{{- end -}}

{{- define "omni-cluster-operator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "omni-cluster-operator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
control-plane: controller-manager
{{- end -}}

{{- define "omni-cluster-operator.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (printf "%s-controller-manager" (include "omni-cluster-operator.fullname" .)) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{- define "omni-cluster-operator.webhookServiceName" -}}
{{- printf "%s-webhook-service" (include "omni-cluster-operator.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "omni-cluster-operator.metricsServiceName" -}}
{{- printf "%s-controller-manager-metrics-service" (include "omni-cluster-operator.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "omni-cluster-operator.issuerName" -}}
{{- printf "%s-selfsigned-issuer" (include "omni-cluster-operator.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "omni-cluster-operator.servingCertName" -}}
{{- printf "%s-serving-cert" (include "omni-cluster-operator.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "omni-cluster-operator.webhookSecretName" -}}
{{- printf "%s-webhook-server-cert" (include "omni-cluster-operator.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "omni-cluster-operator.image" -}}
{{- $tag := default .Chart.AppVersion .Values.image.tag -}}
{{- printf "%s:%s" .Values.image.repository $tag -}}
{{- end -}}
