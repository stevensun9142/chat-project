import uuid
from datetime import date, datetime, timedelta, timezone

from cassandra.query import SimpleStatement

from app.dao.cassandra.session import get_session
from app.models import Message


def _week_bucket(dt: datetime) -> str:
    """Return ISO week bucket string like '2026-W14'."""
    iso = dt.isocalendar()
    return f"{iso.year}-W{iso.week:02d}"


def _prev_week_bucket(bucket: str) -> str:
    """Given '2026-W14', return '2026-W13' (handles year rollover)."""
    year, week = int(bucket[:4]), int(bucket[6:])
    monday = date.fromisocalendar(year, week, 1)
    prev_monday = monday - timedelta(days=7)
    iso = prev_monday.isocalendar()
    return f"{iso[0]}-W{iso[1]:02d}"


def insert_message(
    room_id: uuid.UUID,
    message_id: int,
    sender_id: uuid.UUID,
    content: str,
    created_at: datetime | None = None,
) -> None:
    if created_at is None:
        created_at = datetime.now(timezone.utc)
    bucket = _week_bucket(created_at)
    session = get_session()
    session.execute(
        """
        INSERT INTO messages (room_id, bucket, message_id, sender_id, content, created_at)
        VALUES (%s, %s, %s, %s, %s, %s)
        """,
        (room_id, bucket, message_id, sender_id, content, created_at),
    )


def get_messages(
    room_id: uuid.UUID,
    limit: int = 50,
    max_buckets: int = 4,
) -> list[Message]:
    """Fetch latest messages, walking back through weekly buckets as needed."""
    session = get_session()
    stmt = SimpleStatement(
        "SELECT room_id, bucket, message_id, sender_id, content, created_at "
        "FROM messages WHERE room_id = %s AND bucket = %s LIMIT %s",
    )
    results: list[Message] = []
    bucket = _week_bucket(datetime.now(timezone.utc))
    remaining = limit

    for _ in range(max_buckets):
        rows = session.execute(stmt, (room_id, bucket, remaining))
        for row in rows:
            results.append(
                Message(
                    room_id=row.room_id,
                    message_id=row.message_id,
                    sender_id=row.sender_id,
                    content=row.content,
                    created_at=row.created_at,
                )
            )
        remaining = limit - len(results)
        if remaining <= 0:
            break
        bucket = _prev_week_bucket(bucket)

    results.reverse()  # Cassandra returns newest-first; UI needs oldest-first
    return results


def count_messages_since(
    room_id: uuid.UUID,
    since: datetime,
    max_buckets: int = 4,
) -> int:
    """Count messages in a room created after `since`, walking weekly buckets."""
    session = get_session()
    stmt = SimpleStatement(
        "SELECT COUNT(*) FROM messages "
        "WHERE room_id = %s AND bucket = %s AND created_at > %s",
    )
    total = 0
    bucket = _week_bucket(datetime.now(timezone.utc))
    since_bucket = _week_bucket(since)

    for _ in range(max_buckets):
        rows = session.execute(stmt, (room_id, bucket, since))
        total += rows.one()[0]
        if bucket == since_bucket:
            break
        bucket = _prev_week_bucket(bucket)

    return total
