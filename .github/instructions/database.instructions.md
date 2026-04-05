---
description: "Use when working on database schemas, DAOs, migrations, or any Postgres/Cassandra data layer changes. Covers table schemas, partition keys, indexes, and DAO conventions."
applyTo: ["app/dao/**", "schema/**"]
---

# Database Schema & DAO Guide

## Postgres (Relational Data)

Connection: `localhost:5432`, user=`chat`, password=`chat_secret`, db=`chat_db` (test: `chat_db_test`)

### Tables

**users**
```sql
id          UUID PRIMARY KEY DEFAULT gen_random_uuid()
username    VARCHAR(32)  NOT NULL UNIQUE
email       VARCHAR(255) NOT NULL UNIQUE
password_hash TEXT       NOT NULL
created_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
```

**rooms**
```sql
id          UUID PRIMARY KEY DEFAULT gen_random_uuid()
name        VARCHAR(100) NOT NULL
created_by  UUID         NOT NULL REFERENCES users(id)
created_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
```

**room_members** (many-to-many)
```sql
room_id     UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE
user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE
joined_at   TIMESTAMPTZ NOT NULL DEFAULT now()
PRIMARY KEY (room_id, user_id)
INDEX idx_room_members_user ON room_members(user_id)
```

**refresh_tokens** (hashed, revocable)
```sql
token_hash  TEXT        PRIMARY KEY          -- SHA-256 of the raw token
user_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE
expires_at  TIMESTAMPTZ NOT NULL
created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
INDEX idx_refresh_tokens_user ON refresh_tokens(user_id)
```

### DAO Conventions (Postgres)
- Use `asyncpg` with the connection pool from `app/dao/postgres/pool.py`
- Return `app/models.py` dataclasses, not raw rows
- Use transactions (`async with conn.transaction()`) when multiple writes must be atomic
- Use `ON CONFLICT DO NOTHING` for idempotent inserts (e.g., adding members)
- `UniqueViolationError` from asyncpg is the exception for duplicate unique key violations
- Cleanup order for tests: refresh_tokens → room_members → rooms → users (foreign key deps)

## Cassandra (Message Storage)

Connection: `localhost:9042`, keyspace=`chat` (test: `chat_test`)

### Messages Table
```cql
PRIMARY KEY ((room_id, bucket), created_at, message_id)
CLUSTERING ORDER BY (created_at DESC, message_id DESC)
```

| Column | Type | Purpose |
|--------|------|---------|
| room_id | UUID | Part of partition key |
| bucket | TEXT | Weekly bucket `YYYY-Www` (e.g., `2026-W14`). Part of partition key. |
| message_id | BIGINT | Snowflake-style ID (generated at Gateway in future phases) |
| sender_id | UUID | Who sent the message |
| content | TEXT | Message body |
| created_at | TIMESTAMP | When the message was sent |

### Partition Strategy
- Partition key: `(room_id, bucket)` — all messages for the same room in the same week land on one partition
- Weekly bucketing prevents unbounded partition growth
- `get_messages()` walks backwards through buckets until the requested limit is filled (max 4 buckets)

### DAO Conventions (Cassandra)
- Uses the **sync** cassandra-driver (not async). Functions are regular `def`, not `async def`.
- Session from `app/dao/cassandra/session.py` — lazy singleton
- Keyspace is set at connection time (`cluster.connect(keyspace)`)
- Replication: `NetworkTopologyStrategy`, dc1: 3 (warns on single node — expected in dev)
