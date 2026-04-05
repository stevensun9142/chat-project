from uuid import UUID

from app.dao.postgres.pool import get_pool
from app.models import User


async def create_user(username: str, email: str, password_hash: str) -> User:
    pool = await get_pool()
    row = await pool.fetchrow(
        """
        INSERT INTO users (username, email, password_hash)
        VALUES ($1, $2, $3)
        RETURNING id, username, email, created_at
        """,
        username, email, password_hash,
    )
    return User(**dict(row))


async def get_user_by_id(user_id: UUID) -> User | None:
    pool = await get_pool()
    row = await pool.fetchrow("SELECT id, username, email, created_at FROM users WHERE id = $1", user_id)
    return User(**dict(row)) if row else None


async def get_user_by_username(username: str) -> User | None:
    pool = await get_pool()
    row = await pool.fetchrow(
        "SELECT id, username, email, password_hash, created_at FROM users WHERE username = $1",
        username,
    )
    return User(**dict(row)) if row else None
