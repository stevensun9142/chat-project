import secrets
from datetime import datetime, timedelta, timezone
from uuid import UUID

import bcrypt
from fastapi import Depends, HTTPException, status
from fastapi.security import HTTPAuthorizationCredentials, HTTPBearer
from jose import JWTError, jwt

from app.config.settings import settings
from app.dao.postgres.refresh_tokens_dao import (
    delete_refresh_token,
    lookup_refresh_token,
    store_refresh_token,
)
from app.dao.postgres.users_dao import get_user_by_id
from app.models import User

_bearer = HTTPBearer()


def hash_password(password: str) -> str:
    return bcrypt.hashpw(password.encode(), bcrypt.gensalt()).decode()


def verify_password(password: str, hashed: str) -> bool:
    return bcrypt.checkpw(password.encode(), hashed.encode())


def create_access_token(user_id: UUID, username: str) -> str:
    payload = {
        "sub": str(user_id),
        "username": username,
        "exp": datetime.now(timezone.utc) + timedelta(minutes=settings.jwt_expiry_minutes),
    }
    return jwt.encode(payload, settings.jwt_secret, algorithm="HS256")


async def create_refresh_token(user_id: UUID) -> str:
    token = secrets.token_urlsafe(32)
    expires_at = datetime.now(timezone.utc) + timedelta(days=settings.refresh_token_expiry_days)
    await store_refresh_token(token, user_id, expires_at)
    return token


async def rotate_refresh_token(old_token: str) -> tuple[str, str]:
    """Validate old refresh token, delete it, issue new access + refresh tokens.

    Returns (access_token, new_refresh_token).
    Raises 401 if the refresh token is invalid or expired.
    """
    row = await lookup_refresh_token(old_token)
    if row is None:
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="Invalid refresh token")

    await delete_refresh_token(old_token)

    user = await get_user_by_id(row["user_id"])
    if user is None:
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="User not found")

    access_token = create_access_token(user.id, user.username)
    new_refresh_token = await create_refresh_token(user.id)
    return access_token, new_refresh_token


async def get_current_user(
    credentials: HTTPAuthorizationCredentials = Depends(_bearer),
) -> User:
    try:
        payload = jwt.decode(credentials.credentials, settings.jwt_secret, algorithms=["HS256"])
        user_id = UUID(payload["sub"])
        username = payload["username"]
    except (JWTError, KeyError, ValueError):
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="Invalid token")

    return User(id=user_id, username=username, email="", created_at=datetime.min)
