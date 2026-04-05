from uuid import UUID

from app.dao.postgres.pool import get_pool
from app.models import Room, RoomMember


async def create_room(name: str, created_by: UUID) -> Room:
    pool = await get_pool()
    async with pool.acquire() as conn:
        async with conn.transaction():
            row = await conn.fetchrow(
                """
                INSERT INTO rooms (name, created_by)
                VALUES ($1, $2)
                RETURNING id, name, created_by, created_at
                """,
                name, created_by,
            )
            # Auto-join the creator
            await conn.execute(
                "INSERT INTO room_members (room_id, user_id) VALUES ($1, $2)",
                row["id"], created_by,
            )
    return Room(**dict(row))


async def get_room(room_id: UUID) -> Room | None:
    pool = await get_pool()
    row = await pool.fetchrow("SELECT id, name, created_by, created_at FROM rooms WHERE id = $1", room_id)
    return Room(**dict(row)) if row else None


async def get_rooms_for_user(user_id: UUID) -> list[Room]:
    pool = await get_pool()
    rows = await pool.fetch(
        """
        SELECT r.id, r.name, r.created_by, r.created_at
        FROM rooms r
        JOIN room_members rm ON r.id = rm.room_id
        WHERE rm.user_id = $1
        ORDER BY r.created_at
        """,
        user_id,
    )
    return [Room(**dict(r)) for r in rows]


async def join_room(room_id: UUID, user_id: UUID) -> None:
    pool = await get_pool()
    await pool.execute(
        """
        INSERT INTO room_members (room_id, user_id)
        VALUES ($1, $2)
        ON CONFLICT DO NOTHING
        """,
        room_id, user_id,
    )


async def leave_room(room_id: UUID, user_id: UUID) -> None:
    pool = await get_pool()
    await pool.execute(
        "DELETE FROM room_members WHERE room_id = $1 AND user_id = $2",
        room_id, user_id,
    )


async def get_room_members(room_id: UUID) -> list[RoomMember]:
    pool = await get_pool()
    rows = await pool.fetch(
        """
        SELECT u.id, u.username, rm.joined_at
        FROM users u
        JOIN room_members rm ON u.id = rm.user_id
        WHERE rm.room_id = $1
        ORDER BY rm.joined_at
        """,
        room_id,
    )
    return [RoomMember(**dict(r)) for r in rows]
