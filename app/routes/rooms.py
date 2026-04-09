from uuid import UUID

from fastapi import APIRouter, Depends, HTTPException, Query, status

from app.auth.utils import get_current_user
from app.dao.cassandra.messages_dao import get_messages
from app.dao.redis.cache import get_messages_cached
from app.dao.postgres.rooms_dao import (
    add_members,
    create_room,
    get_room,
    get_room_members,
    get_rooms_for_user,
    leave_room,
)
from app.dao.postgres.users_dao import get_user_by_username
from app.models import User
from app.schemas import (
    MemberResponse,
    MessageResponse,
    RoomAddMembersRequest,
    RoomCreateRequest,
    RoomResponse,
)

router = APIRouter(prefix="/rooms", tags=["rooms"])


async def _verify_membership(room_id: UUID, user_id: UUID) -> None:
    members = await get_room_members(room_id)
    if not any(m.id == user_id for m in members):
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Room not found")


@router.post("", response_model=RoomResponse, status_code=status.HTTP_201_CREATED)
async def create(body: RoomCreateRequest, user: User = Depends(get_current_user)):
    room = await create_room(body.name, user.id, body.member_ids)
    return RoomResponse(id=room.id, name=room.name, created_by=room.created_by, created_at=room.created_at)


@router.get("", response_model=list[RoomResponse])
async def list_rooms(user: User = Depends(get_current_user)):
    rooms = await get_rooms_for_user(user.id)
    return [RoomResponse(id=r.id, name=r.name, created_by=r.created_by, created_at=r.created_at) for r in rooms]


@router.get("/{room_id}", response_model=RoomResponse)
async def get_room_detail(room_id: UUID, user: User = Depends(get_current_user)):
    await _verify_membership(room_id, user.id)
    room = await get_room(room_id)
    if room is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Room not found")
    return RoomResponse(id=room.id, name=room.name, created_by=room.created_by, created_at=room.created_at)


@router.get("/{room_id}/members", response_model=list[MemberResponse])
async def list_members(room_id: UUID, user: User = Depends(get_current_user)):
    await _verify_membership(room_id, user.id)
    members = await get_room_members(room_id)
    return [MemberResponse(id=m.id, username=m.username, joined_at=m.joined_at) for m in members]


@router.post("/{room_id}/members", status_code=status.HTTP_204_NO_CONTENT)
async def add_room_members(room_id: UUID, body: RoomAddMembersRequest, user: User = Depends(get_current_user)):
    await _verify_membership(room_id, user.id)
    user_ids = []
    for username in body.usernames:
        found = await get_user_by_username(username)
        if found is None:
            raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail=f"User '{username}' not found")
        user_ids.append(found.id)
    await add_members(room_id, user_ids)


@router.delete("/{room_id}/members", status_code=status.HTTP_204_NO_CONTENT)
async def leave(room_id: UUID, user: User = Depends(get_current_user)):
    await leave_room(room_id, user.id)


@router.get("/{room_id}/messages", response_model=list[MessageResponse])
async def message_history(
    room_id: UUID,
    user: User = Depends(get_current_user),
    limit: int = Query(default=50, ge=1, le=200),
):
    await _verify_membership(room_id, user.id)
    messages = await get_messages_cached(room_id, limit, get_messages)
    return [
        MessageResponse(
            room_id=m.room_id,
            message_id=m.message_id,
            sender_id=m.sender_id,
            content=m.content,
            created_at=m.created_at,
        )
        for m in messages
    ]
