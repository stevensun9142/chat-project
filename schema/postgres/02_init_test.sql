-- Create test database and apply the same schema
CREATE DATABASE chat_db_test;

\connect chat_db_test

-- Users
CREATE TABLE users (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username    VARCHAR(32)  NOT NULL UNIQUE,
    email       VARCHAR(255) NOT NULL UNIQUE,
    password_hash TEXT       NOT NULL,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- Rooms
CREATE TABLE rooms (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(100) NOT NULL,
    created_by  UUID         NOT NULL REFERENCES users(id),
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- Room membership (many-to-many)
CREATE TABLE room_members (
    room_id     UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    joined_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (room_id, user_id)
);

CREATE INDEX idx_room_members_user ON room_members(user_id);

-- Refresh tokens (hashed, revocable)
CREATE TABLE refresh_tokens (
    token_hash  TEXT        PRIMARY KEY,
    user_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_refresh_tokens_user ON refresh_tokens(user_id);
