# Chat Project — Copilot Instructions

> **IMPORTANT**: The user refers to you as **"Mr. I"** (first name A, last name I — short for A.I.). Respond to this name naturally.

## Project Overview

A Discord-style chat application built as a learning project for distributed systems. Currently on **Phase 3 of 10** (Phases 1–3 complete). Full architecture and build order are in `STEERING.md` (never push that file).

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
- **Docker Compose** — Postgres 16 + Cassandra 4.1 (commands need `sudo`)
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
    App.jsx                — react-router-dom routing with protected/guest routes
    pages/
      Login.jsx            — login form → POST /auth/login
      Register.jsx         — register form → POST /auth/register, auto-login
      Rooms.jsx            — room list sidebar, create/leave rooms, message history, add members
tests/
  conftest.py              — overrides settings to use test databases
  test_api.py              — Phase 2: 5 integration tests via httpx → FastAPI
  test_postgres.py         — Phase 1: 4 Postgres DAO tests
  test_cassandra.py        — Phase 1: 4 Cassandra DAO tests (note: sync, not async)
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

## Testing

- **No unit tests** — integration tests only, hitting real Docker databases.
- **Isolated test databases**: `chat_db_test` (Postgres), `chat_test` (Cassandra). Configured in `tests/conftest.py`.
- **13 total tests** across 3 files. Cleanup deletes all rows after each test (dependency order).
- Run: `python -m pytest tests/ -v`
- Requires: `sudo docker compose up -d` + Cassandra schema loaded manually

## Build & Run

```bash
# Start databases
sudo docker compose up -d
# Wait for Cassandra, then load schema
sudo docker exec -i chat-cassandra cqlsh < schema/cassandra/init.cql
# Run API server
source .venv/bin/activate && uvicorn app.main:app --port 8000
# Run frontend dev server
cd frontend && npm run dev
# Run tests
python -m pytest tests/ -v
```

## Key Patterns

- **DAOs are thin** — raw SQL/CQL, return dataclasses. No ORM.
- **Routes are thin** — validate auth, call DAO, return Pydantic schema.
- **Settings via pydantic-settings** — env vars override defaults (e.g., `JWT_SECRET=...`).
- **Lazy singletons** for DB connections — initialized on first use, also eagerly in lifespan for fail-fast.
- **Cassandra messages use weekly bucket partitioning** — partition key is `(room_id, bucket)` where bucket is `YYYY-Www`. `get_messages` walks back through buckets to fill the requested limit.
