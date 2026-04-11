from fastapi import APIRouter, Depends, HTTPException, Query, status

from app.auth.utils import get_current_user
from app.dao.postgres.friendships_dao import (
    accept_request,
    get_friends,
    get_incoming_requests,
    remove_friend,
    search_users,
    send_request,
)
from app.dao.postgres.users_dao import get_user_by_username
from app.models import User
from app.schemas import FriendRequest, FriendRequestResponse, FriendResponse

router = APIRouter(prefix="/friends", tags=["friends"])


@router.get("", response_model=list[FriendResponse])
async def list_friends(user: User = Depends(get_current_user)):
    friends = await get_friends(user.id)
    return [FriendResponse(id=f.id, username=f.username, created_at=f.created_at) for f in friends]


@router.get("/requests", response_model=list[FriendRequestResponse])
async def list_requests(user: User = Depends(get_current_user)):
    requests = await get_incoming_requests(user.id)
    return [FriendRequestResponse(id=r.id, username=r.username, created_at=r.created_at) for r in requests]


@router.get("/search", response_model=list[FriendResponse])
async def search(q: str = Query(min_length=1, max_length=32), user: User = Depends(get_current_user)):
    users = await search_users(q, user.id)
    return [FriendResponse(id=u.id, username=u.username, created_at=u.created_at) for u in users]


@router.post("/request", status_code=status.HTTP_204_NO_CONTENT)
async def request_friend(body: FriendRequest, user: User = Depends(get_current_user)):
    target = await get_user_by_username(body.username)
    if target is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="User not found")
    if target.id == user.id:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail="Cannot friend yourself")
    await send_request(user.id, target.id)


@router.post("/accept", status_code=status.HTTP_204_NO_CONTENT)
async def accept_friend(body: FriendRequest, user: User = Depends(get_current_user)):
    target = await get_user_by_username(body.username)
    if target is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="User not found")
    accepted = await accept_request(user.id, target.id)
    if not accepted:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="No pending request from this user")


@router.delete("/{username}", status_code=status.HTTP_204_NO_CONTENT)
async def delete_friend(username: str, user: User = Depends(get_current_user)):
    target = await get_user_by_username(username)
    if target is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="User not found")
    await remove_friend(user.id, target.id)
