#!/bin/bash
# Patches the Kafka broker address for docker-compose (local dev) and loads
# the schema. Mounted as /docker-entrypoint-initdb.d/init.sh so ClickHouse
# runs it on first start.

INIT_SQL="/docker-entrypoint-initdb.d/init.sql"

sed -i "s|kafka-1-0.kafka-headless:9092|kafka-1:9092,kafka-2:9092,kafka-3:9092|g" "$INIT_SQL"

clickhouse-client --multiquery < "$INIT_SQL"
