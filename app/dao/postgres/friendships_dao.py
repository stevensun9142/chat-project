from uuid import UUID

from app.dao.postgres.pool import get_pool
from app.models import User


async def send_request(user_id: UUID, friend_id: UUID) -> None:
    """Insert a pending friendship request. No-op if already exists."""
    pool = await get_pool()
    await pool.execute(
        """
        INSERT INTO friendships (user_id, friend_id, status)
        VALUES ($1, $2, 'pending')
        ON CONFLICT (user_id, friend_id) DO NOTHING
        """,
        user_id, friend_id,
    )


async def accept_request(user_id: UUID, friend_id: UUID) -> bool:
    """Accept a pending request from friend_id → user_id. Creates the reverse row.
    Returns True if a pending request existed and was accepted."""
    pool = await get_pool()
    async with pool.acquire() as conn:
        async with conn.transaction():
            # Update the original pending row to accepted
            result = await conn.execute(
                """
                UPDATE friendships SET status = 'accepted'
                WHERE user_id = $1 AND friend_id = $2 AND status = 'pending'
                """,
                friend_id, user_id,
            )
            if result == "UPDATE 0":
                return False
            # Insert the reverse direction
            await conn.execute(
                """
                INSERT INTO friendships (user_id, friend_id, status)
                VALUES ($1, $2, 'accepted')
                ON CONFLICT (user_id, friend_id) DO UPDATE SET status = 'accepted'
                """,
                user_id, friend_id,
            )
            return True


async def remove_friend(user_id: UUID, friend_id: UUID) -> None:
    """Remove friendship in both directions (works for pending or accepted)."""
    pool = await get_pool()
    await pool.execute(
        """
        DELETE FROM friendships
        WHERE (user_id = $1 AND friend_id = $2)
           OR (user_id = $2 AND friend_id = $1)
        """,
        user_id, friend_id,
    )


async def get_friends(user_id: UUID) -> list[User]:
    """Return all accepted friends for a user."""
    pool = await get_pool()
    rows = await pool.fetch(
        """
        SELECT u.id, u.username, u.email, u.created_at
        FROM friendships f
        JOIN users u ON u.id = f.friend_id
        WHERE f.user_id = $1 AND f.status = 'accepted'
        ORDER BY u.username
        """,
        user_id,
    )
    return [User(**dict(r)) for r in rows]


async def get_incoming_requests(user_id: UUID) -> list[User]:
    """Return users who sent a pending request to user_id."""
    pool = await get_pool()
    rows = await pool.fetch(
        """
        SELECT u.id, u.username, u.email, u.created_at
        FROM friendships f
        JOIN users u ON u.id = f.user_id
        WHERE f.friend_id = $1 AND f.status = 'pending'
        ORDER BY f.created_at DESC
        """,
        user_id,
    )
    return [User(**dict(r)) for r in rows]


async def are_friends(user_id: UUID, friend_id: UUID) -> bool:
    """Check if two users are accepted friends."""
    pool = await get_pool()
    row = await pool.fetchrow(
        """
        SELECT 1 FROM friendships
        WHERE user_id = $1 AND friend_id = $2 AND status = 'accepted'
        """,
        user_id, friend_id,
    )
    return row is not None


async def search_users(query: str, current_user_id: UUID, limit: int = 20) -> list[User]:
    """Search users by username prefix, excluding the current user."""
    pool = await get_pool()
    rows = await pool.fetch(
        """
        SELECT id, username, email, created_at
        FROM users
        WHERE username ILIKE $1 AND id != $2
        ORDER BY username
        LIMIT $3
        """,
        query + "%", current_user_id, limit,
    )
    return [User(**dict(r)) for r in rows]
