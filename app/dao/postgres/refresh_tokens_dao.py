import hashlib
from datetime import datetime, timedelta, timezone
from uuid import UUID

from app.dao.postgres.pool import get_pool


def _hash_token(token: str) -> str:
    return hashlib.sha256(token.encode()).hexdigest()


async def store_refresh_token(token: str, user_id: UUID, expires_at: datetime) -> None:
    pool = await get_pool()
    await pool.execute(
        """
        INSERT INTO refresh_tokens (token_hash, user_id, expires_at)
        VALUES ($1, $2, $3)
        """,
        _hash_token(token), user_id, expires_at,
    )


async def lookup_refresh_token(token: str) -> dict | None:
    pool = await get_pool()
    row = await pool.fetchrow(
        """
        SELECT token_hash, user_id, expires_at
        FROM refresh_tokens
        WHERE token_hash = $1
        """,
        _hash_token(token),
    )
    if row is None:
        return None
    if row["expires_at"] < datetime.now(timezone.utc):
        await delete_refresh_token(token)
        return None
    return dict(row)


async def delete_refresh_token(token: str) -> None:
    pool = await get_pool()
    await pool.execute(
        "DELETE FROM refresh_tokens WHERE token_hash = $1",
        _hash_token(token),
    )


async def delete_all_user_refresh_tokens(user_id: UUID) -> None:
    pool = await get_pool()
    await pool.execute(
        "DELETE FROM refresh_tokens WHERE user_id = $1",
        user_id,
    )
