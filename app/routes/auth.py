from asyncpg import UniqueViolationError
from fastapi import APIRouter, HTTPException, status

from app.auth.utils import (
    create_access_token,
    create_refresh_token,
    hash_password,
    rotate_refresh_token,
    verify_password,
)
from app.dao.postgres.users_dao import get_user_by_username, create_user
from app.schemas import (
    LoginRequest,
    RefreshRequest,
    RegisterRequest,
    TokenResponse,
    UserResponse,
)

router = APIRouter(prefix="/auth", tags=["auth"])


@router.post("/register", response_model=UserResponse, status_code=status.HTTP_201_CREATED)
async def register(body: RegisterRequest):
    password_hash = hash_password(body.password)
    try:
        user = await create_user(body.username, body.email, password_hash)
    except UniqueViolationError:
        raise HTTPException(status_code=status.HTTP_409_CONFLICT, detail="Username or email already taken")
    return UserResponse(id=user.id, username=user.username, email=user.email, created_at=user.created_at)


@router.post("/login", response_model=TokenResponse)
async def login(body: LoginRequest):
    user = await get_user_by_username(body.username)
    if user is None or not verify_password(body.password, user.password_hash):
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="Invalid credentials")

    access_token = create_access_token(user.id, user.username)
    refresh_token = await create_refresh_token(user.id)
    return TokenResponse(access_token=access_token, refresh_token=refresh_token)


@router.post("/refresh", response_model=TokenResponse)
async def refresh(body: RefreshRequest):
    access_token, new_refresh_token = await rotate_refresh_token(body.refresh_token)
    return TokenResponse(access_token=access_token, refresh_token=new_refresh_token)
