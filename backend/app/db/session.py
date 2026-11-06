from __future__ import annotations
import uuid

from collections.abc import AsyncGenerator

from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker, create_async_engine
from sqlalchemy.pool import NullPool, StaticPool
from sqlalchemy import event, text

from app.core.config import settings

connect_args = {}
engine_kwargs: dict = {"echo": False}
if settings.is_sqlite:
    connect_args = {"check_same_thread": False}
    engine_kwargs["poolclass"] = StaticPool
elif settings.is_serverless:
    # Serverless (Vercel) environment: use NullPool to avoid exhausting connections across ephemeral instances
    connect_args = {
        "statement_cache_size": 0,
        "prepared_statement_cache_size": 0,
        "prepared_statement_name_func": lambda: f"__asyncpg_{uuid.uuid4()}__",
    }
    engine_kwargs.update(
        poolclass=NullPool,
        pool_pre_ping=True,
    )
else:
    connect_args = {
        "statement_cache_size": 0,
        "prepared_statement_cache_size": 0,
        "prepared_statement_name_func": lambda: f"__asyncpg_{uuid.uuid4()}__",
    }
    # Tune pool for local/persistent server + Supabase connection pooler.
    engine_kwargs.update(
        pool_size=5,
        max_overflow=5,
        pool_recycle=300,
        pool_pre_ping=True,
        pool_timeout=30,
    )

engine = create_async_engine(settings.async_database_url, connect_args=connect_args, **engine_kwargs)

if settings.is_sqlite:

    @event.listens_for(engine.sync_engine, "connect")
    def _enable_sqlite_fks(dbapi_connection, _connection_record):
        cursor = dbapi_connection.cursor()
        cursor.execute("PRAGMA foreign_keys=ON")
        cursor.close()
SessionLocal = async_sessionmaker(engine, class_=AsyncSession, expire_on_commit=False)


async def get_session() -> AsyncGenerator[AsyncSession, None]:
    async with SessionLocal() as session:
        if settings.is_sqlite:
            await session.execute(text("PRAGMA foreign_keys=ON"))
        yield session
