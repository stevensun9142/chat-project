import asyncio
import json
import uuid
from datetime import datetime
from uuid import UUID

import redis.asyncio as aioredis

from app.config.settings import settings
from app.models import Message

_pool: aioredis.Redis | None = None

CACHE_PREFIX = "msg:"
LOCK_PREFIX = "lock:msg:"
CACHE_TTL = 600  # 10 minutes
CACHE_LIMIT = 50
LOCK_TTL_MS = 5000  # 5 seconds
LOCK_RETRY_DELAY = 0.05  # 50ms


async def get_redis() -> aioredis.Redis:
    global _pool
    if _pool is None:
        _pool = aioredis.from_url(settings.redis_cache_url, decode_responses=True)
    return _pool


async def close_redis() -> None:
    global _pool
    if _pool is not None:
        await _pool.aclose()
        _pool = None


async def get_cached_messages(room_id: UUID, limit: int = CACHE_LIMIT) -> list[Message] | None:
    """Return cached messages for a room, or None on cache miss."""
    rdb = await get_redis()
    key = CACHE_PREFIX + str(room_id)
    data = await rdb.lrange(key, -limit, -1)
    if not data:
        return None
    messages = []
    for raw in data:
        m = json.loads(raw)
        messages.append(
            Message(
                room_id=UUID(m["room_id"]),
                message_id=m["message_id"],
                sender_id=UUID(m["sender_id"]),
                content=m["content"],
                created_at=datetime.fromisoformat(m["created_at"]),
            )
        )
    return messages


async def populate_cache(room_id: UUID, messages: list[Message]) -> None:
    """Populate the cache list from a Cassandra fetch result."""
    if not messages:
        return
    rdb = await get_redis()
    key = CACHE_PREFIX + str(room_id)
    pipe = rdb.pipeline()
    pipe.delete(key)
    pipe.rpush(
        key,
        *[
            json.dumps(
                {
                    "message_id": m.message_id,
                    "room_id": str(m.room_id),
                    "sender_id": str(m.sender_id),
                    "content": m.content,
                    "created_at": m.created_at.isoformat(),
                }
            )
            for m in messages
        ],
    )
    pipe.ltrim(key, -CACHE_LIMIT, -1)
    pipe.expire(key, CACHE_TTL)
    await pipe.execute()


async def acquire_lock(room_id: UUID) -> str | None:
    """Try to acquire the stampede lock. Returns token on success, None on failure."""
    rdb = await get_redis()
    token = uuid.uuid4().hex
    ok = await rdb.set(LOCK_PREFIX + str(room_id), token, nx=True, px=LOCK_TTL_MS)
    return token if ok else None


_release_script = """
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("DEL", KEYS[1])
end
return 0
"""


async def release_lock(room_id: UUID, token: str) -> None:
    """Release the stampede lock if we still own it."""
    rdb = await get_redis()
    await rdb.eval(_release_script, 1, LOCK_PREFIX + str(room_id), token)


# --- Unread counts ---

UNREAD_PREFIX = "unread:"


async def get_unread_counts(user_id: UUID) -> dict[str, int]:
    """Return {room_id: count} for all rooms with unread messages."""
    rdb = await get_redis()
    data = await rdb.hgetall(UNREAD_PREFIX + str(user_id))
    return {room_id: int(count) for room_id, count in data.items() if int(count) > 0}


async def ack_unread(user_id: UUID, room_id: UUID) -> None:
    """Reset unread count for a room (user opened/viewed it)."""
    rdb = await get_redis()
    await rdb.hdel(UNREAD_PREFIX + str(user_id), str(room_id))


# --- Room members cache ---

MEMBERS_PREFIX = "members:"


async def evict_room_members(room_id: UUID) -> None:
    """Invalidate the cached member set for a room."""
    rdb = await get_redis()
    await rdb.delete(MEMBERS_PREFIX + str(room_id))


async def get_messages_cached(
    room_id: UUID,
    limit: int,
    fetch_from_db,
) -> list[Message]:
    """Cache-aside read with stampede protection.

    1. Check cache → return on hit.
    2. Try to acquire lock → winner fetches from DB, populates cache, releases lock.
    3. Losers wait briefly, retry cache, fall through to DB if still a miss.
    """
    cached = await get_cached_messages(room_id, limit)
    if cached is not None:
        return cached

    token = await acquire_lock(room_id)
    if token is not None:
        try:
            messages = fetch_from_db(room_id, limit=limit)
            await populate_cache(room_id, messages)
            return messages
        finally:
            await release_lock(room_id, token)

    # Lost the lock — wait for the winner to populate.
    await asyncio.sleep(LOCK_RETRY_DELAY)
    cached = await get_cached_messages(room_id, limit)
    if cached is not None:
        return cached

    # Fallback: fetch from DB directly (lock holder was slow or crashed).
    return fetch_from_db(room_id, limit=limit)
