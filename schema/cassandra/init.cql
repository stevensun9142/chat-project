-- Cassandra schema for chat messages
-- Run manually after Cassandra is healthy:
--   docker exec -i chat-cassandra cqlsh < db/cassandra/init.cql

CREATE KEYSPACE IF NOT EXISTS chat
WITH replication = {
    'class': 'NetworkTopologyStrategy',
    'dc1': 3
};

-- Messages table
-- Partition key: (room_id, bucket)  — weekly bucketing to bound partition size
--   bucket is a TEXT like '2026-W14' (ISO year-week)
-- Clustering key: created_at DESC + message_id for ordering within the bucket
CREATE TABLE IF NOT EXISTS chat.messages (
    room_id    UUID,
    bucket     TEXT,           -- ISO week: YYYY-Www (e.g. '2026-W14')
    message_id BIGINT,        -- snowflake-style ID generated at the Gateway
    sender_id  UUID,
    content    TEXT,
    created_at TIMESTAMP,
    PRIMARY KEY ((room_id, bucket), created_at, message_id)
) WITH CLUSTERING ORDER BY (created_at DESC, message_id DESC);

-- Test keyspace (same schema, isolated from real data)
CREATE KEYSPACE IF NOT EXISTS chat_test
WITH replication = {
    'class': 'NetworkTopologyStrategy',
    'dc1': 3
};

CREATE TABLE IF NOT EXISTS chat_test.messages (
    room_id    UUID,
    bucket     TEXT,
    message_id BIGINT,
    sender_id  UUID,
    content    TEXT,
    created_at TIMESTAMP,
    PRIMARY KEY ((room_id, bucket), created_at, message_id)
) WITH CLUSTERING ORDER BY (created_at DESC, message_id DESC);
