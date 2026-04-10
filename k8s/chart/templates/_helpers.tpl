{{- define "chat.kafkaBrokers" -}}
{{- $replicas := int .Values.kafka.replicas -}}
{{- range $i := until $replicas -}}
{{- if $i }},{{ end -}}
kafka-{{ add $i 1 }}-0.kafka-headless:9092
{{- end -}}
{{- end -}}

{{- define "chat.imagePullSecrets" -}}
{{- if .Values.imagePullSecret }}
      imagePullSecrets:
        - name: {{ .Values.imagePullSecret }}
{{- end -}}
{{- end -}}
