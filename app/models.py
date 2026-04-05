from dataclasses import dataclass
from datetime import datetime
from uuid import UUID


@dataclass(frozen=True)
class User:
    id: UUID
    username: str
    email: str
    created_at: datetime
    password_hash: str | None = None


@dataclass(frozen=True)
class Room:
    id: UUID
    name: str
    created_by: UUID
    created_at: datetime


@dataclass(frozen=True)
class RoomMember:
    id: UUID
    username: str
    joined_at: datetime


@dataclass(frozen=True)
class Message:
    room_id: UUID
    message_id: int
    sender_id: UUID
    content: str
    created_at: datetime
