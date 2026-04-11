from contextlib import asynccontextmanager

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from app.config.settings import settings
from app.dao.cassandra.session import close_session, get_session
from app.dao.postgres.pool import close_pool, get_pool
from app.dao.redis.cache import close_redis, get_redis
from app.routes.auth import router as auth_router
from app.routes.friends import router as friends_router
from app.routes.rooms import router as rooms_router


@asynccontextmanager
async def lifespan(app: FastAPI):
    await get_pool()
    get_session()
    await get_redis()
    yield
    await close_redis()
    await close_pool()
    close_session()


app = FastAPI(title="Chat API", lifespan=lifespan)
app.add_middleware(
    CORSMiddleware,
    allow_origins=settings.cors_origins,
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)
app.include_router(auth_router)
app.include_router(friends_router)
app.include_router(rooms_router)
