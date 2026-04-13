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

{{/*
Redis presence address(es). Single node returns "redis:6379".
Cluster returns comma-separated "redis-0.redis:6379,redis-1.redis:6379,...".
*/}}
{{- define "chat.redisPresenceAddrs" -}}
{{- $replicas := int (default 1 .Values.redis.replicas) -}}
{{- if gt $replicas 1 -}}
  {{- range $i := until $replicas -}}
    {{- if $i }},{{ end -}}
    redis-{{ $i }}.redis:6379
  {{- end -}}
{{- else -}}
redis:6379
{{- end -}}
{{- end -}}
