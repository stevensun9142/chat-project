from contextlib import asynccontextmanager

from fastapi import FastAPI

from app.dao.cassandra.session import close_session, get_session
from app.dao.postgres.pool import close_pool, get_pool
from app.routes.auth import router as auth_router
from app.routes.rooms import router as rooms_router


@asynccontextmanager
async def lifespan(app: FastAPI):
    await get_pool()
    get_session()
    yield
    await close_pool()
    close_session()


app = FastAPI(title="Chat API", lifespan=lifespan)
app.include_router(auth_router)
app.include_router(rooms_router)
