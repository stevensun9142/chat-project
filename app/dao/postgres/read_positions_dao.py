from datetime import datetime, timezone
from uuid import UUID

from app.dao.postgres.pool import get_pool


async def upsert_read_position(user_id: UUID, room_id: UUID) -> None:
    """Set the user's last-read pointer for a room to now."""
    pool = await get_pool()
    await pool.execute(
        """
        INSERT INTO read_positions (user_id, room_id, last_read_at)
        VALUES ($1, $2, now())
        ON CONFLICT (user_id, room_id)
        DO UPDATE SET last_read_at = now()
        """,
        user_id, room_id,
    )


async def get_read_positions(user_id: UUID) -> dict[UUID, datetime]:
    """Return {room_id: last_read_at} for all rooms the user has a position in."""
    pool = await get_pool()
    rows = await pool.fetch(
        "SELECT room_id, last_read_at FROM read_positions WHERE user_id = $1",
        user_id,
    )
    return {row["room_id"]: row["last_read_at"] for row in rows}
