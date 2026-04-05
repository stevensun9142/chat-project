"""
Phase 2 integration tests — API Service (REST).
Requires: docker compose up -d (postgres + cassandra healthy)
"""

import uuid
from datetime import datetime, timedelta, timezone

import httpx
import pytest
import pytest_asyncio

from app.dao.cassandra import messages_dao as msg_db
from app.dao.cassandra.session import get_session, close_session
from app.dao.postgres.pool import get_pool, close_pool
from app.main import app


@pytest_asyncio.fixture(autouse=True)
async def _setup_teardown():
    pool = await get_pool()
    get_session()
    yield
    session = get_session()
    session.execute("TRUNCATE messages")
    await pool.execute("DELETE FROM refresh_tokens")
    await pool.execute("DELETE FROM room_members")
    await pool.execute("DELETE FROM rooms")
    await pool.execute("DELETE FROM users")
    await close_pool()
    close_session()


@pytest_asyncio.fixture
async def client():
    async with httpx.AsyncClient(transport=httpx.ASGITransport(app=app), base_url="http://test") as c:
        yield c


async def _register_and_login(client: httpx.AsyncClient, username: str) -> dict:
    """Helper: register a user and log in, return the token response body."""
    await client.post("/auth/register", json={
        "username": username,
        "email": f"{username}@example.com",
        "password": "testpass123",
    })
    resp = await client.post("/auth/login", json={
        "username": username,
        "password": "testpass123",
    })
    return resp.json()


def _auth_header(token: str) -> dict:
    return {"Authorization": f"Bearer {token}"}


@pytest.mark.asyncio
async def test_register_and_login(client: httpx.AsyncClient):
    resp = await client.post("/auth/register", json={
        "username": "alice",
        "email": "alice@example.com",
        "password": "securepass",
    })
    assert resp.status_code == 201
    body = resp.json()
    assert body["username"] == "alice"
    assert body["email"] == "alice@example.com"
    assert "password" not in body
    assert "password_hash" not in body

    resp = await client.post("/auth/login", json={
        "username": "alice",
        "password": "securepass",
    })
    assert resp.status_code == 200
    tokens = resp.json()
    assert "access_token" in tokens
    assert "refresh_token" in tokens
    assert tokens["token_type"] == "bearer"


@pytest.mark.asyncio
async def test_refresh_token_rotation(client: httpx.AsyncClient):
    tokens = await _register_and_login(client, "bob")

    resp = await client.post("/auth/refresh", json={
        "refresh_token": tokens["refresh_token"],
    })
    assert resp.status_code == 200
    new_tokens = resp.json()
    assert "access_token" in new_tokens
    assert new_tokens["refresh_token"] != tokens["refresh_token"]

    # Old refresh token should no longer work
    resp = await client.post("/auth/refresh", json={
        "refresh_token": tokens["refresh_token"],
    })
    assert resp.status_code == 401


@pytest.mark.asyncio
async def test_room_lifecycle(client: httpx.AsyncClient):
    # Register two users
    r1 = await client.post("/auth/register", json={
        "username": "user1", "email": "user1@example.com", "password": "pass123",
    })
    user1_id = r1.json()["id"]

    r2 = await client.post("/auth/register", json={
        "username": "user2", "email": "user2@example.com", "password": "pass123",
    })
    user2_id = r2.json()["id"]

    # Login as user1
    login_resp = await client.post("/auth/login", json={"username": "user1", "password": "pass123"})
    token1 = login_resp.json()["access_token"]

    # Login as user2
    login_resp = await client.post("/auth/login", json={"username": "user2", "password": "pass123"})
    token2 = login_resp.json()["access_token"]

    # user1 creates a room with user2 as a member
    resp = await client.post("/rooms", json={
        "name": "test-room",
        "member_ids": [user2_id],
    }, headers=_auth_header(token1))
    assert resp.status_code == 201
    room = resp.json()
    room_id = room["id"]
    assert room["name"] == "test-room"

    # Both users can see the room
    resp = await client.get("/rooms", headers=_auth_header(token1))
    assert any(r["id"] == room_id for r in resp.json())

    resp = await client.get("/rooms", headers=_auth_header(token2))
    assert any(r["id"] == room_id for r in resp.json())

    # user2 can see members
    resp = await client.get(f"/rooms/{room_id}/members", headers=_auth_header(token2))
    assert resp.status_code == 200
    members = resp.json()
    assert len(members) == 2
    member_ids = {m["id"] for m in members}
    assert user1_id in member_ids
    assert user2_id in member_ids

    # user2 leaves
    resp = await client.delete(f"/rooms/{room_id}/members", headers=_auth_header(token2))
    assert resp.status_code == 204

    # user2 can no longer see the room
    resp = await client.get(f"/rooms/{room_id}", headers=_auth_header(token2))
    assert resp.status_code == 404


@pytest.mark.asyncio
async def test_non_member_gets_404(client: httpx.AsyncClient):
    r1 = await client.post("/auth/register", json={
        "username": "insider", "email": "insider@example.com", "password": "pass123",
    })
    user1_id = r1.json()["id"]

    await client.post("/auth/register", json={
        "username": "outsider", "email": "outsider@example.com", "password": "pass123",
    })

    login1 = await client.post("/auth/login", json={"username": "insider", "password": "pass123"})
    token1 = login1.json()["access_token"]

    login2 = await client.post("/auth/login", json={"username": "outsider", "password": "pass123"})
    token2 = login2.json()["access_token"]

    # insider creates a room (no other members)
    resp = await client.post("/rooms", json={"name": "private", "member_ids": []}, headers=_auth_header(token1))
    room_id = resp.json()["id"]

    # outsider can't access it
    resp = await client.get(f"/rooms/{room_id}", headers=_auth_header(token2))
    assert resp.status_code == 404

    resp = await client.get(f"/rooms/{room_id}/members", headers=_auth_header(token2))
    assert resp.status_code == 404

    resp = await client.get(f"/rooms/{room_id}/messages", headers=_auth_header(token2))
    assert resp.status_code == 404


@pytest.mark.asyncio
async def test_message_history(client: httpx.AsyncClient):
    r1 = await client.post("/auth/register", json={
        "username": "msguser", "email": "msguser@example.com", "password": "pass123",
    })
    user1_id = r1.json()["id"]

    login = await client.post("/auth/login", json={"username": "msguser", "password": "pass123"})
    token = login.json()["access_token"]

    resp = await client.post("/rooms", json={"name": "chat-room", "member_ids": []}, headers=_auth_header(token))
    room_id = uuid.UUID(resp.json()["id"])

    # Insert messages directly via DAO (simulating what Message Worker will do in Phase 5)
    now = datetime.now(timezone.utc)
    sender_id = uuid.UUID(user1_id)
    msg_db.insert_message(room_id, 1, sender_id, "first message", created_at=now - timedelta(seconds=2))
    msg_db.insert_message(room_id, 2, sender_id, "second message", created_at=now)

    resp = await client.get(f"/rooms/{room_id}/messages?limit=50", headers=_auth_header(token))
    assert resp.status_code == 200
    messages = resp.json()
    assert len(messages) == 2
    assert messages[0]["content"] == "second message"
    assert messages[1]["content"] == "first message"
