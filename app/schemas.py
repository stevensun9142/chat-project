from datetime import datetime
from uuid import UUID

from pydantic import BaseModel, EmailStr


# --- Auth ---

class RegisterRequest(BaseModel):
    username: str
    email: EmailStr
    password: str


class LoginRequest(BaseModel):
    username: str
    password: str


class RefreshRequest(BaseModel):
    refresh_token: str


class TokenResponse(BaseModel):
    access_token: str
    refresh_token: str
    token_type: str = "bearer"


class UserResponse(BaseModel):
    id: UUID
    username: str
    email: str
    created_at: datetime


# --- Rooms ---

class RoomCreateRequest(BaseModel):
    name: str
    member_ids: list[UUID]


class RoomAddMembersRequest(BaseModel):
    usernames: list[str]


class RoomResponse(BaseModel):
    id: UUID
    name: str
    created_by: UUID
    created_at: datetime


class MemberResponse(BaseModel):
    id: UUID
    username: str
    joined_at: datetime


# --- Messages ---

class MessageResponse(BaseModel):
    room_id: UUID
    message_id: int
    sender_id: UUID
    content: str
    created_at: datetime


# --- Unread ---

class UnreadCountsResponse(BaseModel):
    counts: dict[str, int]
