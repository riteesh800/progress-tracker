from __future__ import annotations

from fastapi import HTTPException, status
from fastapi.responses import JSONResponse
from starlette.middleware.base import BaseHTTPMiddleware
from starlette.requests import Request

from app.core.logging import get_logger

logger = get_logger(__name__)


class AppError(HTTPException):
    def __init__(self, status_code: int, code: str, message: str) -> None:
        super().__init__(status_code=status_code, detail={"code": code, "message": message})
        self.code = code
        self.message = message


def error_payload(status_code: int, code: str, message: str) -> dict:
    return {"error": {"status": status_code, "code": code, "message": message}}


class ErrorHandlingMiddleware(BaseHTTPMiddleware):
    async def dispatch(self, request: Request, call_next):
        try:
            return await call_next(request)
        except AppError as exc:
            return JSONResponse(
                status_code=exc.status_code,
                content=error_payload(exc.status_code, exc.code, exc.message),
            )
        except HTTPException as exc:
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
        except Exception:
            logger.exception("Unhandled server error")
            return JSONResponse(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                content=error_payload(500, "internal_error", "An unexpected error occurred."),
            )
