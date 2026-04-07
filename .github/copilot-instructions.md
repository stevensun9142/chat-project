# Chat Project — Copilot Instructions

> **IMPORTANT**: The user refers to you as **"Mr. I"** (first name A, last name I — short for A.I.). Respond to this name naturally.

## Project Overview

A Discord-style chat application built as a learning project for distributed systems. Currently on **Phase 6 of 10** (Phases 1–5 complete, Phase 6 next). Full architecture and build order are in `STEERING.md` (never push that file).

The user refers to me by **Mr. I**.

## Implementation Standards

- Always default to production-grade, industry-standard approaches. Never simplify to naive implementations.
- All `__init__.py` files are blank by convention. Never put code in `__init__.py` — use a named module.
- If an API route exceeds ~15 lines of logic or orchestrates multiple DAOs with conditional branching, extract a service layer.
- When proposing changes, explain the reasoning and get approval before proceeding to the next step.

## Tech Stack

- **Python 3.12**, venv at `.venv`, deps in `pyproject.toml`
- **FastAPI** — REST API with Pydantic validation
- **asyncpg** — async Postgres driver, connection pool in `app/dao/postgres/pool.py`
- **cassandra-driver** — sync Cassandra driver, session in `app/dao/cassandra/session.py`
- **bcrypt** — password hashing (slow-by-design, salted)
- **python-jose** — JWT encoding/decoding (HS256 symmetric)
- **Go** — Gateway Service (WebSocket + Kafka producer), Message Worker (Kafka consumer + Cassandra persistence), Presence & Router (Phase 6)
- **Go 1.22** — Gateway + Message Worker, installed at `/usr/local/go`
- **Kafka** — 3 brokers (KRaft mode, apache/kafka:3.7.1), topics: chat.messages, chat.delivery, presence.events
- **Kubernetes (Kind)** — all infrastructure runs in a Kind cluster, Helm chart at `k8s/chart/`
- **Docker Compose** — still in repo but NOT used; all infra on K8s
- **React 19 + Vite** — frontend SPA in `frontend/`, uses react-router-dom
- **Node.js 20** — for frontend tooling

## Project Structure

```
app/
  auth/utils.py            — bcrypt hashing, JWT creation, get_current_user dependency
  config/settings.py       — pydantic-settings Settings class (Postgres, Cassandra, JWT)
  dao/
    cassandra/
      session.py           — Cassandra cluster connection (lazy singleton)
      messages_dao.py       — insert/get messages (weekly bucket partitioning)
    postgres/
      pool.py              — asyncpg connection pool (lazy singleton)
      users_dao.py         — create/get users
      rooms_dao.py         — create/get/leave rooms, add members
      refresh_tokens_dao.py — store/lookup/delete refresh tokens (hashed)
  routes/
    auth.py                — POST /auth/register, /auth/login, /auth/refresh
    rooms.py               — CRUD rooms, members, message history
  main.py                  — FastAPI app + lifespan (startup/shutdown)
  models.py                — dataclasses: User, Room, RoomMember, Message
  schemas.py               — Pydantic request/response models
schema/
  postgres/init.sql         — users, rooms, room_members, refresh_tokens tables
  postgres/02_init_test.sql — test database (chat_db_test) with same schema
  cassandra/init.cql        — chat + chat_test keyspaces, messages table
frontend/
  src/
    api.js                 — fetch wrapper + all API client functions
    auth.jsx               — AuthContext provider (token storage, login/logout)
    jwtDecode.js           — client-side JWT payload decode (no verification)
    useWebSocket.js        — WebSocket hook: connect, reconnect with backoff, send/receive
    App.jsx                — react-router-dom routing with protected/guest routes
    pages/
      Login.jsx            — login form → POST /auth/login
      Register.jsx         — register form → POST /auth/register, auto-login
      Rooms.jsx            — room list sidebar, create/leave rooms, message history, add members, chat input
tests/
  conftest.py              — overrides settings to use test databases
  test_api.py              — Phase 2: 5 integration tests via httpx → FastAPI
  test_postgres.py         — Phase 1: 4 Postgres DAO tests
  test_cassandra.py        — Phase 1: 4 Cassandra DAO tests (note: sync, not async)
gateway/                     — (Phase 4, COMPLETE) Go WebSocket service
  Dockerfile                 — multi-stage: golang:1.22 → distroless
  main.go                    — entry point, reads GATEWAY_PORT, JWT_SECRET, KAFKA_BROKERS env vars
  gateway_test.go            — 5 Go integration tests (WS auth, presence, messages → Kafka)
  auth/
    jwt.go                   — JWTValidator: HS256 validation, keyFunc, extracts sub+username
  kafka/
    producer.go              — Kafka producer: publishes MessageEvent to chat.messages + chat.delivery, PresenceEvent to presence.events
  id/
    snowflake.go             — Snowflake ID generator: 41-bit timestamp + 10-bit machine_id + 12-bit sequence
  ws/
    client.go                — Client struct (conn + send channel + producer + idgen + identity), readPump, writePump, heartbeats
    handler.go               — HandleUpgrade: validates ?token=, pre-flight presence publish with retry, upgrades to WS, creates Client, starts pumps
    hub.go                   — Hub: thread-safe user_id → *Client registry, identity-aware Unregister (returns bool)
    messages.go              — ClientMessage (send_message) and ServerMessage (new_message, error) JSON types
message-worker/              — (Phase 5, in progress) Go Kafka consumer → Cassandra
  main.go                    — entry point, wires config + signal context
  message_worker_test.go     — 3 Go integration tests (persist, reject non-member, skip malformed)
  consumer/
    consumer.go              — Kafka consumer loop: fetch → membership check → persist → commit
  store/
    cassandra.go             — gocql session + PersistMessage() with weekly bucketing
    postgres.go              — lib/pq connection pool + IsRoomMember() check
k8s/
  kind-config.yaml           — Kind cluster config with host port mappings
  chart/
    Chart.yaml               — Helm umbrella chart
    values.yaml              — all configurable values (images, ports, storage, topics)
    templates/
      postgres.yaml          — ConfigMap (init SQL) + StatefulSet + NodePort Service
      cassandra.yaml         — StatefulSet + NodePort Service
      kafka.yaml             — 3 StatefulSets + headless Service + 3 NodePort Services + init Job
      gateway.yaml           — Gateway StatefulSet + NodePort 30801 Service
      secrets.yaml           — JWT Secret (shared by API + Gateway)
```

## Auth Architecture

- **Registration**: password → bcrypt hash → stored in Postgres
- **Login**: verify bcrypt → issue JWT access token (30 min) + opaque refresh token (7 days, stored as SHA-256 hash in Postgres)
- **Every request**: JWT verified via HMAC-SHA256 signature check (no DB call). User info (id, username) embedded in JWT claims.
- **Refresh**: old token deleted, new pair issued (token rotation). Old tokens are immediately invalid.
- **`get_current_user`**: FastAPI dependency — extracts Bearer token, decodes JWT, returns User from claims. No Postgres round-trip.

## Database Details

See `.github/instructions/database.instructions.md` for full schema details.

## API Endpoints

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| POST | `/auth/register` | No | Register user, returns UserResponse (no hash) |
| POST | `/auth/login` | No | Returns access_token + refresh_token |
| POST | `/auth/refresh` | No | Rotate refresh token, returns new pair |
| POST | `/rooms` | Yes | Create room with name + member_ids |
| GET | `/rooms` | Yes | List rooms for current user |
| GET | `/rooms/{id}` | Yes+member | Room detail (404 if not member) |
| GET | `/rooms/{id}/members` | Yes+member | List room members |
| POST | `/rooms/{id}/members` | Yes+member | Add members by username (`{"usernames": [...]}`) |
| DELETE | `/rooms/{id}/members` | Yes | Leave room |
| GET | `/rooms/{id}/messages` | Yes+member | Message history from Cassandra |

Non-members get 404 (not 403) to avoid leaking room existence.

## Frontend (Phase 3)

Minimal React SPA that wraps all API endpoints. No CSS framework — plain inline styles.

- **Auth flow**: tokens stored in `localStorage`. `AuthContext` provides `accessToken`, `user` (decoded from JWT), `saveTokens()`, `logout()`. JWT is decoded client-side (payload only, no verification — server handles that).
- **Routing**: `react-router-dom` with `ProtectedRoute` (redirects to `/login` if no token) and `GuestRoute` (redirects to `/` if already logged in).
- **API client** (`src/api.js`): single `request()` helper that handles JSON serialization, Bearer token injection, and error normalization. All endpoint functions are thin wrappers.
- **Add members**: uses usernames (not UUIDs). The route resolves usernames → UUIDs server-side via `get_user_by_username`.
- **No message sending**: the chat input is disabled — requires WebSocket (Phase 4).
- **CORS**: FastAPI allows `http://localhost:5173` and `:5174` origins with credentials.
- **Dev server**: Vite on port 5173. Backend on port 8000.

## Gateway Service (Phase 4 — COMPLETE)

Go WebSocket service at `gateway/`. All 8 steps done, 5 Go integration tests passing. Dockerized and deployed to K8s (`gateway-0` pod running).

- **Go module**: `github.com/stevensun/chat-project/gateway`
- **Dependencies**: gorilla/websocket v1.5.3, segmentio/kafka-go v0.4.47, golang-jwt/jwt/v5 v5.2.1
- **JWT auth**: validates `?token=` query param before HTTP→WS upgrade. Same HS256 shared secret as Python API.
- **Client struct**: wraps `*websocket.Conn` + `send chan []byte` (buffered 256) + `*kafka.Producer` + `*id.Generator` + identity (UserID, Username). One Client per WebSocket connection.
- **readPump**: runs on handler goroutine (blocks until disconnect). Infinite loop reading JSON from WebSocket, dispatches by message type. Pong handler resets 60s read deadline. Defer: publish disconnect presence event (only if `Unregister` returns true) + `conn.Close()`.
- **writePump**: spawned as background goroutine. `select` on send channel and 30s ping ticker. Sends close frame when channel is closed. 10s write deadline per write.
- **Hub**: `map[string]*Client` with `sync.RWMutex`. `Register(*Client)` closes old client's send channel on reconnect. `Unregister(userID, *Client)` checks pointer match before closing channel + deleting, returns `bool` (true if removed — prevents stale replaced clients from emitting false disconnects).
- **Kafka Producer**: one per Gateway pod, shared across all clients. Holds `GatewayID`. Three `kafka.Writer` instances — `chat.messages` (RequireAll acks), `chat.delivery` (RequireOne ack), `presence.events` (RequireOne ack). Messages/delivery keyed by room_id, presence keyed by user_id (hash partitioner). BatchTimeout 10ms.
- **Presence events**: connect event published in `HandleUpgrade` before WS upgrade (pre-flight, with exponential backoff retry up to 60s — returns HTTP 503 on failure). Disconnect event published in `readPump` defer only when `hub.Unregister()` returns true (best-effort, no retry — Phase 7 Redis TTL heartbeats are the safety net). Payload: `{user_id, username, gateway_id, event, timestamp}`.
- **Snowflake ID generator** (`id/snowflake.go`): 64-bit IDs — 41-bit timestamp (ms since 2026-01-01) + 10-bit machine_id (parsed from hostname trailing number, e.g. `gateway-2` → 2) + 12-bit sequence (4096/ms/machine). Mutex-protected. One instance per pod, shared by all clients. `handleSendMessage` calls `idgen.NextID()` before publishing.
- **handleSendMessage**: generates snowflake ID, validates room_id/content, publishes to Kafka with 5s context timeout. No echo — messages will come back via Router (Phase 6).
- **Env vars**: `JWT_SECRET` (required), `KAFKA_BROKERS` (required, comma-separated), `GATEWAY_PORT` (default 8001), `GATEWAY_ID` (optional, defaults to `os.Hostname()` — pod name in K8s). No defaults for secrets/infra — fails to start if missing.
- **Run locally**: `make gateway` (sets env vars) or `JWT_SECRET=change-me-in-prod KAFKA_BROKERS=localhost:9092,localhost:9093,localhost:9094 go run gateway/main.go`
- **Dockerfile**: multi-stage `golang:1.22` → `gcr.io/distroless/static-debian12`
- **K8s**: StatefulSet + NodePort 30801, JWT_SECRET from `jwt-secret` Secret, KAFKA_BROKERS pointing at in-cluster Kafka headless service
- **No room membership check**: Gateway is a thin relay. Membership validated downstream in Message Worker (Phase 5) and Router (Phase 6).

## Message Worker (Phase 5 — COMPLETE)

Go Kafka consumer at `message-worker/`. Consumes `chat.messages`, validates room membership, persists to Cassandra. All 4 steps done, 3 Go integration tests passing. Dockerized and deployed to K8s.

- **Go module**: `github.com/stevensun/chat-project/message-worker`
- **Dependencies**: gocql v1.7.0, lib/pq v1.12.3, segmentio/kafka-go v0.4.47
- **Consumer design**: `segmentio/kafka-go` Reader with consumer group `message-worker`, manual offset commit. At-least-once delivery.
- **Per-message flow**: deserialize `MessageEvent` → `IsRoomMember()` check in Postgres → `PersistMessage()` to Cassandra (weekly bucket) → commit offset
- **Retry**: transient errors (Postgres/Cassandra down) retry up to 3 times with 250ms backoff, then drop and commit
- **Skip permanently bad messages**: malformed JSON, missing fields, non-member → commit and move on
- **Cassandra consistency**: `ONE` (single-node dev). TODO: make configurable, switch to `LocalQuorum` for production.
- **Naming convention**: consumer dirs prefixed with topic name (`message-worker/`, future `delivery-worker/`)
- **Env vars**: `KAFKA_BROKERS` (required), `KAFKA_GROUP_ID` (default `message-worker`), `CASS_HOSTS`, `CASS_PORT`, `CASS_KEYSPACE`, `PG_DSN`
- **Run locally**: `make message-worker`
- **Dockerfile**: multi-stage `golang:1.22` → `gcr.io/distroless/static-debian12`
- **K8s**: Deployment (not StatefulSet — plain consumer, no stable identity needed), env vars point to in-cluster Postgres/Cassandra/Kafka
- **3 integration tests** in `message_worker_test.go`: persist valid message, reject non-member, skip malformed + persist next valid

### WebSocket Message Schema

Client → Server: `{"type": "send_message", "room_id": "<uuid>", "content": "<text>"}`

Server → Client: `{"type": "new_message", "message_id": 123456789, "room_id": "<uuid>", "sender_id": "<uuid>", "sender_name": "<username>", "content": "<text>", "created_at": "2026-04-05T12:00:00Z"}`

Error: `{"type": "error", "message": "<description>"}`

## Kubernetes Infrastructure

- **Kind cluster** named `chat`, config at `k8s/kind-config.yaml`
- **Helm chart** at `k8s/chart/` — single umbrella chart for all infra
- All pods in namespace `chat`
- **Host port mappings** via Kind extraPortMappings → NodePort services:
  - `localhost:5432` → Postgres (NodePort 30432)
  - `localhost:9042` → Cassandra (NodePort 30042)
  - `localhost:9092-9094` → Kafka brokers (NodePorts 30092-30094)
  - `localhost:8000` → API Service (NodePort 30800) — reserved, not deployed yet
  - `localhost:8001` → Gateway (NodePort 30801) — reserved, not deployed yet
- **kubectl/helm commands need `sudo`** (Kind cluster created with sudo)
- Kafka brokers use per-broker StatefulSets (unique node IDs, listeners, ports)
- Kafka headless service has `publishNotReadyAddresses: true` — required for broker DNS during startup (chicken-and-egg fix)
- Cassandra schema must be loaded manually after pod starts
- Kafka topics are created automatically by a Kubernetes Job (`kafka-init-topics`)

## Testing

- **No unit tests** — integration tests only, hitting real Docker databases.
- **Isolated test databases**: `chat_db_test` (Postgres), `chat_test` (Cassandra). Configured in `tests/conftest.py`.
- **21 total tests**: 13 Python (3 files) + 5 Go Gateway + 3 Go Message Worker. Cleanup deletes all rows after each test (dependency order).
- Run: `python -m pytest tests/ -v` (Python), `make test-gateway` (Gateway), `make test-message-worker` (Worker)
- Requires: Kind cluster running + Cassandra schema loaded

## Build & Run

```bash
# --- K8s Infrastructure ---
# Create Kind cluster (one-time)
sudo kind create cluster --name chat --config k8s/kind-config.yaml
# Deploy all infra via Helm
sudo helm --kube-context kind-chat install chat k8s/chart/ --namespace chat --create-namespace
# Load Cassandra schema (after cassandra-0 is Ready)
sudo kubectl --context kind-chat exec -i cassandra-0 -n chat -- cqlsh < schema/cassandra/init.cql
# Check pod status
sudo kubectl --context kind-chat get pods -n chat

# --- Application ---
# Run API server (connects to K8s databases via localhost ports)
source .venv/bin/activate && uvicorn app.main:app --port 8000
# Run Go gateway
cd gateway && JWT_SECRET=change-me-in-prod go run main.go
# Run Go message worker
cd message-worker && KAFKA_BROKERS=localhost:9092,localhost:9093,localhost:9094 go run main.go
# Run frontend dev server
cd frontend && npm run dev
# Run tests
python -m pytest tests/ -v

# --- Makefile shortcuts ---
make deploy            # First-time Helm install
make upgrade           # Helm upgrade after chart changes
make pods              # List pods
make api               # Run Python API on :8000
make gateway           # Run Go gateway on :8001
make message-worker    # Run Go message worker
make frontend          # Run Vite on :5173
make test              # Run pytest
make test-gateway      # Run Go gateway tests
make test-message-worker # Run Go message worker tests

# --- Helm upgrade after chart changes ---
sudo helm --kube-context kind-chat upgrade chat k8s/chart/ -n chat
```

## Key Patterns

- **DAOs are thin** — raw SQL/CQL, return dataclasses. No ORM.
- **Routes are thin** — validate auth, call DAO, return Pydantic schema.
- **Settings via pydantic-settings** — env vars override defaults (e.g., `JWT_SECRET=...`).
- **Lazy singletons** for DB connections — initialized on first use, also eagerly in lifespan for fail-fast.
- **Cassandra messages use weekly bucket partitioning** — partition key is `(room_id, bucket)` where bucket is `YYYY-Www`. `get_messages` walks back through buckets to fill the requested limit.
