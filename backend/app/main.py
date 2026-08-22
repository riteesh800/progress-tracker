from __future__ import annotations

from fastapi import FastAPI, HTTPException, Request
from fastapi.exceptions import RequestValidationError
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse
from starlette.middleware.base import BaseHTTPMiddleware
from slowapi.errors import RateLimitExceeded
from slowapi.middleware import SlowAPIMiddleware
from sqlalchemy import text

from app.api.routes import auth, dashboard, notes, pdf, personal_notes, search, skills, topics
from app.core.config import settings
from app.core.exceptions import AppError, ErrorHandlingMiddleware, error_payload
from app.core.logging import get_logger
from app.core.rate_limit import limiter, rate_limit_exceeded_handler
from app.db.session import engine
from app.db.base import Base
import app.models  # noqa: F401 — register metadata

logger = get_logger(__name__)


class SecurityHeadersMiddleware(BaseHTTPMiddleware):
    async def dispatch(self, request: Request, call_next):
        response = await call_next(request)
        response.headers.setdefault("X-Content-Type-Options", "nosniff")
        response.headers.setdefault("X-Frame-Options", "DENY")
        response.headers.setdefault("Referrer-Policy", "no-referrer")
        response.headers.setdefault("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
        if settings.is_production:
            response.headers.setdefault("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
        return response


def create_app() -> FastAPI:
    docs = None if settings.is_production else "/docs"
    redoc = None if settings.is_production else "/redoc"
    openapi = None if settings.is_production else "/openapi.json"
    application = FastAPI(
        title="Skill Progress Tracker",
        version="1.0.0",
        docs_url=docs,
        redoc_url=redoc,
        openapi_url=openapi,
    )
    application.state.limiter = limiter
    application.add_exception_handler(RateLimitExceeded, rate_limit_exceeded_handler)
    application.add_middleware(SlowAPIMiddleware)
    application.add_middleware(SecurityHeadersMiddleware)
    application.add_middleware(ErrorHandlingMiddleware)
    origins = settings.cors_origin_list
    if settings.is_production and ("*" in origins or not origins):
        logger.warning("Production CORS must be an explicit origin list")
        origins = [o for o in origins if o != "*"]
    application.add_middleware(
        CORSMiddleware,
        allow_origins=origins or ["http://localhost:5173"],
        allow_credentials=True,
        allow_methods=["*"],
        allow_headers=["*"],
    )

    @application.exception_handler(AppError)
    async def app_error_handler(_request: Request, exc: AppError):
        return JSONResponse(
            status_code=exc.status_code,
            content=error_payload(exc.status_code, exc.code, exc.message),
        )

    @application.exception_handler(HTTPException)
    async def http_handler(_request: Request, exc: HTTPException):
        detail = exc.detail
        if isinstance(detail, dict) and "code" in detail:
            return JSONResponse(
                status_code=exc.status_code,
                content=error_payload(exc.status_code, detail["code"], detail.get("message", "Error")),
            )
        message = detail if isinstance(detail, str) else "Request failed"
        return JSONResponse(
            status_code=exc.status_code,
            content=error_payload(exc.status_code, "http_error", message),
        )

    @application.exception_handler(RequestValidationError)
    async def validation_handler(_request: Request, exc: RequestValidationError):
        return JSONResponse(
            status_code=422,
            content=error_payload(422, "validation_error", "Invalid request payload."),
        )

    application.include_router(auth.router)
    application.include_router(skills.router)
    application.include_router(topics.router)
    application.include_router(notes.router)
    application.include_router(personal_notes.router)
    application.include_router(search.router)
    application.include_router(dashboard.router)
    application.include_router(pdf.router)

    @application.get("/health")
    async def health():
        async with engine.connect() as conn:
            await conn.execute(text("SELECT 1"))
        return {"status": "ok"}

    @application.on_event("startup")
    async def startup():
        settings.assert_production_secrets()
        if settings.database_url.startswith("sqlite"):
            async with engine.begin() as conn:
                await conn.run_sync(Base.metadata.create_all)

                def _sqlite_columns(sync_conn):
                    rows = sync_conn.exec_driver_sql("PRAGMA table_info(activity_log)").fetchall()
                    names = {row[1] for row in rows}
                    if "parent_name_snapshot" not in names:
                        sync_conn.exec_driver_sql("ALTER TABLE activity_log ADD COLUMN parent_name_snapshot TEXT")
                    if "summary" not in names:
                        sync_conn.exec_driver_sql("ALTER TABLE activity_log ADD COLUMN summary TEXT")

                await conn.run_sync(_sqlite_columns)
        logger.info("Skill Progress Tracker API started")

    @application.middleware("http")
    async def request_log(request: Request, call_next):
        response = await call_next(request)
        if response.status_code >= 500:
            logger.error(
                "Server error %s %s -> %s",
                request.method,
                request.url.path,
                response.status_code,
            )
        return response

    return application


app = create_app()
