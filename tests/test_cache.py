"""
Integration tests — Redis message cache layer.
Requires: Kind cluster running with redis-cache pod (localhost:6380).
"""

import uuid
from datetime import datetime, timezone
from unittest.mock import MagicMock

import pytest
import pytest_asyncio

from app.dao.redis import cache
from app.models import Message


def _make_message(room_id, message_id, sender_id=None, content="hello", seconds_ago=0):
    return Message(
        room_id=room_id,
        message_id=message_id,
        sender_id=sender_id or uuid.uuid4(),
        content=content,
        created_at=datetime(2026, 4, 9, 12, 0, 0, tzinfo=timezone.utc),
    )


@pytest_asyncio.fixture(autouse=True)
async def _setup_teardown():
    rdb = await cache.get_redis()
    yield
    await rdb.flushdb()
    await cache.close_redis()


@pytest.mark.asyncio
async def test_cache_miss_returns_none():
    room_id = uuid.uuid4()
    result = await cache.get_cached_messages(room_id)
    assert result is None


@pytest.mark.asyncio
async def test_populate_and_read_cache():
    room_id = uuid.uuid4()
    messages = [_make_message(room_id, i, content=f"msg-{i}") for i in range(5)]

    await cache.populate_cache(room_id, messages)
    cached = await cache.get_cached_messages(room_id)

    assert cached is not None
    assert len(cached) == 5
    assert cached[0].content == "msg-0"
    assert cached[4].content == "msg-4"


@pytest.mark.asyncio
async def test_populate_empty_list_is_noop():
    room_id = uuid.uuid4()
    await cache.populate_cache(room_id, [])

    result = await cache.get_cached_messages(room_id)
    assert result is None


@pytest.mark.asyncio
async def test_cache_trims_to_limit():
    room_id = uuid.uuid4()
    messages = [_make_message(room_id, i, content=f"msg-{i}") for i in range(80)]

    await cache.populate_cache(room_id, messages)
    cached = await cache.get_cached_messages(room_id)

    assert len(cached) == cache.CACHE_LIMIT
    # Should keep the last 50 (most recent)
    assert cached[0].content == "msg-30"
    assert cached[-1].content == "msg-79"


@pytest.mark.asyncio
async def test_read_respects_limit_param():
    room_id = uuid.uuid4()
    messages = [_make_message(room_id, i, content=f"msg-{i}") for i in range(20)]

    await cache.populate_cache(room_id, messages)
    cached = await cache.get_cached_messages(room_id, limit=5)

    assert len(cached) == 5
    # Last 5 messages
    assert cached[0].content == "msg-15"
    assert cached[-1].content == "msg-19"


@pytest.mark.asyncio
async def test_populate_overwrites_existing():
    room_id = uuid.uuid4()
    old = [_make_message(room_id, i, content=f"old-{i}") for i in range(3)]
    new = [_make_message(room_id, i, content=f"new-{i}") for i in range(2)]

    await cache.populate_cache(room_id, old)
    await cache.populate_cache(room_id, new)

    cached = await cache.get_cached_messages(room_id)
    assert len(cached) == 2
    assert cached[0].content == "new-0"
    assert cached[1].content == "new-1"


@pytest.mark.asyncio
async def test_rooms_are_isolated():
    room_a = uuid.uuid4()
    room_b = uuid.uuid4()

    await cache.populate_cache(room_a, [_make_message(room_a, 1, content="in A")])
    await cache.populate_cache(room_b, [_make_message(room_b, 2, content="in B")])

    cached_a = await cache.get_cached_messages(room_a)
    cached_b = await cache.get_cached_messages(room_b)

    assert len(cached_a) == 1
    assert cached_a[0].content == "in A"
    assert len(cached_b) == 1
    assert cached_b[0].content == "in B"


@pytest.mark.asyncio
async def test_acquire_and_release_lock():
    room_id = uuid.uuid4()

    token = await cache.acquire_lock(room_id)
    assert token is not None

    # Second acquire should fail (lock held)
    token2 = await cache.acquire_lock(room_id)
    assert token2 is None

    # Release and re-acquire
    await cache.release_lock(room_id, token)
    token3 = await cache.acquire_lock(room_id)
    assert token3 is not None
    await cache.release_lock(room_id, token3)


@pytest.mark.asyncio
async def test_release_wrong_token_is_noop():
    room_id = uuid.uuid4()

    token = await cache.acquire_lock(room_id)
    assert token is not None

    # Release with wrong token — lock should still be held
    await cache.release_lock(room_id, "wrong-token")
    token2 = await cache.acquire_lock(room_id)
    assert token2 is None

    await cache.release_lock(room_id, token)


@pytest.mark.asyncio
async def test_get_messages_cached_cache_hit():
    room_id = uuid.uuid4()
    messages = [_make_message(room_id, 1, content="cached")]

    await cache.populate_cache(room_id, messages)

    mock_fetch = MagicMock()
    result = await cache.get_messages_cached(room_id, limit=50, fetch_from_db=mock_fetch)

    assert len(result) == 1
    assert result[0].content == "cached"
    mock_fetch.assert_not_called()


@pytest.mark.asyncio
async def test_get_messages_cached_cache_miss_populates():
    room_id = uuid.uuid4()
    db_messages = [_make_message(room_id, 1, content="from db")]

    mock_fetch = MagicMock(return_value=db_messages)
    result = await cache.get_messages_cached(room_id, limit=50, fetch_from_db=mock_fetch)

    assert len(result) == 1
    assert result[0].content == "from db"
    mock_fetch.assert_called_once_with(room_id, limit=50)

    # Should now be cached
    mock_fetch.reset_mock()
    result2 = await cache.get_messages_cached(room_id, limit=50, fetch_from_db=mock_fetch)
    assert len(result2) == 1
    assert result2[0].content == "from db"
    mock_fetch.assert_not_called()


@pytest.mark.asyncio
async def test_message_fields_roundtrip():
    room_id = uuid.uuid4()
    sender_id = uuid.uuid4()
    ts = datetime(2026, 4, 9, 15, 30, 45, tzinfo=timezone.utc)

    original = Message(
        room_id=room_id,
        message_id=999888777,
        sender_id=sender_id,
        content="roundtrip test",
        created_at=ts,
    )
    await cache.populate_cache(room_id, [original])
    cached = await cache.get_cached_messages(room_id)

    assert len(cached) == 1
    m = cached[0]
    assert m.room_id == room_id
    assert m.message_id == 999888777
    assert m.sender_id == sender_id
    assert m.content == "roundtrip test"
    assert m.created_at == ts
