-- ClickHouse analytics schema
-- Ingests from Kafka topic `chat.messages` and materializes tables
-- optimized for keyword frequency, user behavior, and ML feature queries.
--
-- Setup:
--   1. Deploy ClickHouse (single node, 4GB+ RAM, low-memory tuned)
--   2. Run this file against ClickHouse:
--        clickhouse-client --multiquery < schema/clickhouse/init.sql
--   3. ClickHouse will begin consuming from Kafka automatically.

-- ============================================================
-- Database
-- ============================================================

CREATE DATABASE IF NOT EXISTS analytics;

-- ============================================================
-- 1. Kafka engine table (virtual — reads from Kafka, never stores)
--    One row per message event on `chat.messages`.
-- ============================================================

CREATE TABLE IF NOT EXISTS analytics.messages_kafka
(
    message_id  Int64,
    room_id     String,
    sender_id   String,
    sender_name String,
    content     String,
    created_at  String              -- raw RFC3339 from Go producer
)
ENGINE = Kafka
SETTINGS
    kafka_broker_list       = 'kafka-1-0.kafka-headless:9092',
    kafka_topic_list        = 'chat.messages',
    kafka_group_id          = 'clickhouse-analytics',
    kafka_format            = 'JSONEachRow',
    kafka_num_consumers     = 1,
    kafka_max_block_size    = 65536;

-- ============================================================
-- 2. Core fact table — one row per message
--    Sorted for time-range + room scans; partitioned monthly.
-- ============================================================

CREATE TABLE IF NOT EXISTS analytics.messages
(
    message_id    Int64,
    room_id       String,
    sender_id     String,
    sender_name   LowCardinality(String),
    content       String,
    content_lower String              MATERIALIZED lower(content),
    created_at    DateTime64(3, 'UTC'),
    ingest_time   DateTime64(3, 'UTC') DEFAULT now64(3),

    -- Token bloom filter: lets hasToken() skip granules that
    -- definitely do not contain a keyword.
    INDEX idx_content_tokens content_lower
        TYPE tokenbf_v1(32768, 3, 0) GRANULARITY 4
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(created_at)
ORDER BY (toDate(created_at), room_id, created_at, message_id)
TTL toDateTime(created_at) + INTERVAL 12 MONTH
SETTINGS index_granularity = 8192;

-- ============================================================
-- 3. Kafka → messages materialized view (the ingest pipeline)
--    Parses RFC3339 `created_at` into DateTime64 on the fly.
-- ============================================================

CREATE MATERIALIZED VIEW IF NOT EXISTS analytics.messages_kafka_mv
TO analytics.messages AS
SELECT
    message_id,
    room_id,
    sender_id,
    sender_name,
    content,
    parseDateTimeBestEffort(created_at) AS created_at
FROM analytics.messages_kafka;

-- ============================================================
-- 4. Token-exploded table — one row per word per message
--    Powers keyword frequency, trending terms, and topic
--    extraction queries for ML / LLM summarization.
-- ============================================================

CREATE TABLE IF NOT EXISTS analytics.message_tokens
(
    sender_id   String,
    room_id     String,
    token       LowCardinality(String),
    created_at  DateTime64(3, 'UTC'),
    message_id  Int64
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(created_at)
ORDER BY (sender_id, toDate(created_at), token)
SETTINGS index_granularity = 8192;

-- Populate from messages on insert. splitByNonAlpha tokenizes
-- content; arrayJoin explodes into one row per token.
-- Tokens shorter than 3 chars are dropped (articles, etc.).
CREATE MATERIALIZED VIEW IF NOT EXISTS analytics.message_tokens_mv
TO analytics.message_tokens AS
SELECT
    sender_id,
    room_id,
    arrayJoin(splitByNonAlpha(lower(content))) AS token,
    created_at,
    message_id
FROM analytics.messages
WHERE length(token) >= 3;

-- ============================================================
-- 5. Stopword list (common English words to exclude from
--    topic extraction). Loaded once, queried with IN.
-- ============================================================

CREATE TABLE IF NOT EXISTS analytics.stopwords
(
    word String
)
ENGINE = MergeTree
ORDER BY word;

INSERT INTO analytics.stopwords (word) VALUES
    ('the'),('and'),('for'),('are'),('but'),('not'),('you'),
    ('all'),('can'),('had'),('her'),('was'),('one'),('our'),
    ('out'),('has'),('have'),('from'),('been'),('they'),('that'),
    ('this'),('will'),('with'),('what'),('your'),('which'),('when'),
    ('them'),('than'),('each'),('make'),('like'),('just'),('into'),
    ('over'),('also'),('some'),('could'),('would'),('other'),('were'),
    ('more'),('there'),('their'),('about'),('should'),('does'),('its');

-- ============================================================
-- 6. Example analytical queries (not executed — reference only)
-- ============================================================

-- 6a. Keyword frequency over time
--     "How often does 'bug' appear per hour in the last 2 weeks?"
--
-- SELECT
--     toStartOfHour(created_at) AS hour,
--     count()                   AS hits
-- FROM analytics.messages
-- WHERE created_at >= now() - INTERVAL 14 DAY
--   AND hasToken(content_lower, 'bug')
-- GROUP BY hour
-- ORDER BY hour;

-- 6b. Top keywords for a user (ML feature / LLM summarization input)
--     "What did user X talk about most in the last 30 days?"
--
-- SELECT
--     token,
--     count()                              AS freq,
--     round(count() / sum(count()) OVER (), 4) AS pct
-- FROM analytics.message_tokens
-- WHERE sender_id = '{user_id}'
--   AND created_at >= now() - INTERVAL 30 DAY
--   AND token NOT IN (SELECT word FROM analytics.stopwords)
-- GROUP BY token
-- ORDER BY freq DESC
-- LIMIT 20;

-- 6c. Trending terms (this week vs last week)
--     "What topics are spiking for a user?"
--
-- SELECT
--     token,
--     countIf(created_at >= now() - INTERVAL 7 DAY)  AS this_week,
--     countIf(created_at BETWEEN now() - INTERVAL 14 DAY
--                            AND now() - INTERVAL 7 DAY) AS last_week,
--     round(if(last_week > 0,
--              (this_week - last_week) / last_week,
--              this_week), 2)                           AS trend
-- FROM analytics.message_tokens
-- WHERE sender_id = '{user_id}'
--   AND created_at >= now() - INTERVAL 14 DAY
--   AND token NOT IN (SELECT word FROM analytics.stopwords)
-- GROUP BY token
-- HAVING this_week >= 5
-- ORDER BY trend DESC
-- LIMIT 10;

-- 6d. User engagement features (behavioral feature vector)
--     "Engagement profile for every active user in the last 90 days"
--
-- SELECT
--     sender_id,
--     count()                                   AS total_messages,
--     uniq(toDate(created_at))                  AS active_days,
--     uniq(room_id)                             AS rooms_used,
--     avg(length(content))                      AS avg_msg_length,
--     quantile(0.5)(toHour(created_at))         AS median_active_hour,
--     max(created_at)                           AS last_seen
-- FROM analytics.messages
-- WHERE created_at >= now() - INTERVAL 90 DAY
-- GROUP BY sender_id;

-- 6e. Room health / channel analytics
--     "Daily unique senders and message volume per room"
--
-- SELECT
--     room_id,
--     toStartOfDay(created_at)               AS day,
--     uniq(sender_id)                        AS unique_senders,
--     count()                                AS messages,
--     round(count() / uniq(sender_id), 1)    AS msgs_per_user
-- FROM analytics.messages
-- GROUP BY room_id, day
-- ORDER BY day DESC;
