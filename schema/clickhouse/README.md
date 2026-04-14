# ClickHouse Analytics Data Warehouse

Adds a ClickHouse instance that consumes messages from Kafka and stores them
in an analytics-optimized schema for keyword frequency, user behavior, and
ML/LLM feature extraction queries.

## Architecture

ClickHouse joins as a second, independent Kafka consumer of `chat.messages`
(consumer group `clickhouse-analytics`). No existing services are modified.

```
Gateway --> Kafka (chat.messages) --> ClickHouse Kafka engine
                |                         |
                v                         v
         message-worker            analytics.messages
         (existing, unchanged)           |
                                         v
                                  analytics.message_tokens
                                         |
                                         v
                                  Analytics API / LLM
```

## Prerequisites

- Kafka cluster running with the `chat.messages` topic already created
- ClickHouse server (see deployment options below)

## Schema overview

Defined in `init.sql` in this directory. Creates the `analytics` database with:

| Object | Type | Purpose |
|--------|------|---------|
| `messages_kafka` | Kafka engine table | Virtual consumer — reads JSON from Kafka, never stores |
| `messages` | MergeTree table | Core fact table, one row per message, monthly partitions |
| `messages_kafka_mv` | Materialized view | Pipes Kafka -> messages, parses RFC3339 timestamps |
| `message_tokens` | MergeTree table | One row per word per message, sorted by (sender, date, token) |
| `message_tokens_mv` | Materialized view | Auto-tokenizes on insert, filters short words |
| `stopwords` | MergeTree table | Common English words to exclude from topic queries |

## Setup tasks

### 1. Deploy ClickHouse

#### Local dev (docker-compose)

ClickHouse is included in `docker-compose.yml`. Run:

```bash
docker compose up -d
```

The schema loads automatically on first start via
`/docker-entrypoint-initdb.d/`. The local config uses
`kafka-1:9092,kafka-2:9092,kafka-3:9092` as the broker list (docker network).

For manual reload:

```bash
docker exec -i chat-clickhouse clickhouse-client --multiquery < schema/clickhouse/init.sql
```

#### Kubernetes (Kind / Oracle Cloud k3s)

The Helm chart includes a ClickHouse StatefulSet. Deploy with:

```bash
make deploy        # Kind (first time)
make upgrade       # Kind (update)
make cloud-deploy  # Oracle Cloud (first time)
make cloud-upgrade # Oracle Cloud (update)
```

Then load the schema:

```bash
make clickhouse-schema
```

The k8s init.sql uses `kafka-1-0.kafka-headless:9092` (cluster DNS).

### 2. Broker address handling

`init.sql` contains the Kafka broker address for the k8s environment. For
docker-compose, `docker-compose.yml` mounts a startup script that patches
the broker address to the docker network names before loading the schema.

### 3. Low-memory configuration (cloud / small clusters)

For the Oracle Cloud free tier (4GB RAM allocated to ClickHouse), a config
override is mounted at `/etc/clickhouse-server/config.d/low-memory.xml`:

- `max_server_memory_usage_to_ram_ratio`: 0.75
- `max_concurrent_queries`: 8
- `max_connections`: 64
- `mark_cache_size`: 256MB
- `uncompressed_cache_size`: 16MB
- MySQL/PostgreSQL compatibility ports disabled

This config is bundled in both docker-compose and the Helm chart ConfigMap.

### 4. Verify

After deployment and schema load:

```sql
-- Check Kafka consumer is working
SELECT count() FROM analytics.messages;

-- Check tokenization
SELECT count() FROM analytics.message_tokens;

-- Test a keyword query
SELECT toStartOfHour(created_at) AS hour, count() AS hits
FROM analytics.messages
WHERE hasToken(content_lower, 'hello')
GROUP BY hour
ORDER BY hour;
```

## Files

| File | Purpose |
|------|---------|
| `schema/clickhouse/init.sql` | Analytics database schema (Kafka engine, MergeTree tables, materialized views) |
| `schema/clickhouse/low-memory.xml` | ClickHouse config override for 4GB RAM environments |
| `schema/clickhouse/README.md` | This file |
| `docker-compose.yml` | `clickhouse` service + volume |
| `k8s/chart/templates/clickhouse.yaml` | Helm template — Service + StatefulSet + ConfigMaps |
| `k8s/chart/values.yaml` | Default ClickHouse values (image, storage, port) |
| `k8s/values-cloud.yaml` | Cloud-specific overrides |
| `k8s/kind-config.yaml` | Port mapping for local Kind access |
| `Makefile` | `clickhouse-schema` target |
