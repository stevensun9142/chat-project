"""
Phase 1 integration tests — Cassandra data layer.
Requires: docker compose up -d (cassandra healthy + keyspace/table created)
"""

import uuid
from datetime import datetime, timedelta, timezone

import pytest

from app.dao.cassandra import messages_dao as msg_db
from app.dao.cassandra.session import get_session, close_session


@pytest.fixture(autouse=True)
def _setup_teardown():
    session = get_session()
    yield
    session.execute("TRUNCATE messages")
    close_session()


def test_insert_and_get_messages():
    room_id = uuid.uuid4()
    sender_id = uuid.uuid4()

    now = datetime.now(timezone.utc)
    msg_db.insert_message(room_id, 1, sender_id, "hello", created_at=now - timedelta(seconds=2))
    msg_db.insert_message(room_id, 2, sender_id, "world", created_at=now)

    msgs = msg_db.get_messages(room_id)
    assert len(msgs) == 2
    # Ordered by created_at DESC — most recent first
    assert msgs[0].content == "world"
    assert msgs[1].content == "hello"


def test_messages_scoped_to_room():
    room_a = uuid.uuid4()
    room_b = uuid.uuid4()
    sender = uuid.uuid4()

    msg_db.insert_message(room_a, 10, sender, "msg in A")
    msg_db.insert_message(room_b, 11, sender, "msg in B")

    assert len(msg_db.get_messages(room_a)) == 1
    assert len(msg_db.get_messages(room_b)) == 1
    assert msg_db.get_messages(room_a)[0].content == "msg in A"


def test_get_messages_respects_limit():
    room_id = uuid.uuid4()
    sender = uuid.uuid4()

    for i in range(10):
        msg_db.insert_message(room_id, i, sender, f"msg-{i}")

    msgs = msg_db.get_messages(room_id, limit=3)
    assert len(msgs) == 3


def test_cross_bucket_read():
    """Messages spanning two weekly buckets are returned together."""
    room_id = uuid.uuid4()
    sender = uuid.uuid4()

    now = datetime.now(timezone.utc)
    last_week = now - timedelta(days=7)

    # Insert into last week's bucket
    msg_db.insert_message(room_id, 100, sender, "old msg", created_at=last_week)
    # Insert into this week's bucket
    msg_db.insert_message(room_id, 101, sender, "new msg", created_at=now)

    msgs = msg_db.get_messages(room_id, limit=10)
    assert len(msgs) == 2
    assert msgs[0].content == "new msg"
    assert msgs[1].content == "old msg"
