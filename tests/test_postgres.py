"""
Phase 1 integration tests — Postgres data layer.
Requires: docker compose up -d (postgres healthy)
"""

import pytest
import pytest_asyncio

from app.dao.postgres import users_dao as user_db
from app.dao.postgres import rooms_dao as room_db
from app.dao.postgres.pool import get_pool, close_pool


@pytest_asyncio.fixture(autouse=True)
async def _setup_teardown():
    """Ensure pool is open and clean up test data after each test."""
    pool = await get_pool()
    yield
    # Clean up in dependency order
    await pool.execute("DELETE FROM refresh_tokens")
    await pool.execute("DELETE FROM friendships")
    await pool.execute("DELETE FROM read_positions")
    await pool.execute("DELETE FROM room_members")
    await pool.execute("DELETE FROM rooms")
    await pool.execute("DELETE FROM users")
    await close_pool()


@pytest.mark.asyncio
async def test_create_and_get_user():
    user = await user_db.create_user("alice", "alice@example.com", "hashed_pw")
    assert user.username == "alice"
    assert user.email == "alice@example.com"

    fetched = await user_db.get_user_by_id(user.id)
    assert fetched is not None
    assert fetched.username == "alice"

    by_name = await user_db.get_user_by_username("alice")
    assert by_name is not None
    assert by_name.password_hash == "hashed_pw"


@pytest.mark.asyncio
async def test_create_room_auto_joins_creator():
    user = await user_db.create_user("bob", "bob@example.com", "hashed_pw")
    room = await room_db.create_room("general", user.id, [])
    assert room.name == "general"

    members = await room_db.get_room_members(room.id)
    assert len(members) == 1
    assert members[0].id == user.id

    rooms = await room_db.get_rooms_for_user(user.id)
    assert len(rooms) == 1
    assert rooms[0].id == room.id


@pytest.mark.asyncio
async def test_join_and_leave_room():
    alice = await user_db.create_user("alice2", "alice2@example.com", "hashed_pw")
    bob = await user_db.create_user("bob2", "bob2@example.com", "hashed_pw")
    room = await room_db.create_room("dev", alice.id, [])

    await room_db.add_members(room.id, [bob.id])
    members = await room_db.get_room_members(room.id)
    assert len(members) == 2

    await room_db.leave_room(room.id, bob.id)
    members = await room_db.get_room_members(room.id)
    assert len(members) == 1
    assert members[0].id == alice.id


@pytest.mark.asyncio
async def test_duplicate_join_is_idempotent():
    user = await user_db.create_user("charlie", "charlie@example.com", "hashed_pw")
    room = await room_db.create_room("random", user.id, [])

    # Add again — should not raise
    await room_db.add_members(room.id, [user.id])
    members = await room_db.get_room_members(room.id)
    assert len(members) == 1
