from __future__ import annotations

import os
from pathlib import Path
from urllib.parse import parse_qsl, urlencode, urlsplit, urlunsplit
from pydantic_settings import BaseSettings, SettingsConfigDict

# Query parameters added by Supabase / libpq that asyncpg.connect() does not accept as kwargs
_UNSUPPORTED_ASYNCPG_PARAMS = {"pgbouncer", "supa", "options", "channel_binding"}

_BASE_DIR = Path(__file__).resolve().parent.parent.parent
_ROOT_DIR = _BASE_DIR.parent


class Settings(BaseSettings):
    model_config = SettingsConfigDict(
        env_file=(
            str(_ROOT_DIR / ".env"),
            str(_BASE_DIR / ".env"),
            ".env",
        ),
        extra="ignore",
    )

    database_url: str = "sqlite+aiosqlite:///./progress_tracker.db"
    jwt_secret: str = "dev-access-secret-change-me"
    jwt_refresh_secret: str = "dev-refresh-secret-change-me"
    access_token_expire_minutes: int = 15
    refresh_token_expire_days: int = 30
    google_client_id: str = ""
    google_client_secret: str = ""
    anthropic_api_key: str = ""
    gemini_api_key: str = ""
    gemini_model: str = "gemini-3.8-flash"
    cors_origins: str = "http://localhost:5173"
    smtp_host: str = ""
    smtp_port: int = 587
    smtp_user: str = ""
    smtp_password: str = ""
    smtp_from: str = "noreply@example.com"
    environment: str = "development"
    max_pdf_bytes: int = 10 * 1024 * 1024
    max_pdf_pages: int = 200
    admin_email: str = "riteeritee251@gmail.com"
    # Server-only secret. Never expose to the frontend.
    admin_access_code: str = ""
    admin_session_expire_minutes: int = 30
    # Shared secret for cron-triggered cleanup endpoint (Vercel Cron).
    # Must be at least 32 characters in production. Accepts CLEANUP_SECRET or CRON_SECRET.
    cleanup_secret: str = ""
    cron_secret: str = ""

    @property
    def effective_cleanup_secret(self) -> str:
        return (self.cleanup_secret or self.cron_secret).strip()

    @property
    def cors_origin_list(self) -> list[str]:
        origins = [o.strip().rstrip("/") for o in self.cors_origins.split(",") if o.strip()]
        for env_key in ("VERCEL_URL", "VERCEL_BRANCH_URL", "VERCEL_PROJECT_PRODUCTION_URL"):
            val = (os.getenv(env_key) or "").strip().rstrip("/")
            if val:
                url = val if val.startswith("http") else f"https://{val}"
                if url not in origins:
                    origins.append(url)
        return origins

    @property
    def is_sqlite(self) -> bool:
        return self.database_url.strip().strip('"').strip("'").startswith("sqlite")

    @property
    def is_production(self) -> bool:
        return self.environment.strip().lower() == "production"

    @property
    def is_serverless(self) -> bool:
        return bool(
            os.getenv("VERCEL")
            or os.getenv("VERCEL_ENV")
            or os.getenv("VERCEL_URL")
            or os.getenv("AWS_LAMBDA_FUNCTION_NAME")
        )

    @property
    def async_database_url(self) -> str:
        url = self.database_url.strip().strip('"').strip("'")
        if url.startswith("postgres://"):
            url = url.replace("postgres://", "postgresql+asyncpg://", 1)
        elif url.startswith("postgresql://") and not url.startswith("postgresql+"):
            url = url.replace("postgresql://", "postgresql+asyncpg://", 1)
        if url.startswith("postgresql+asyncpg://") and "?" in url:
            parts = urlsplit(url)
            cleaned_params: list[tuple[str, str]] = []
            for k, v in parse_qsl(parts.query, keep_blank_values=True):
                k_lower = k.lower()
                if k_lower in _UNSUPPORTED_ASYNCPG_PARAMS:
                    continue
                if k_lower == "sslmode":
                    cleaned_params.append(("ssl", v))
                else:
                    cleaned_params.append((k, v))
            url = urlunsplit((parts.scheme, parts.netloc, parts.path, urlencode(cleaned_params), parts.fragment))
        return url

    def assert_production_secrets(self) -> None:
        if not self.is_production:
            return
        weak = {
            "",
            "dev-access-secret-change-me",
            "dev-refresh-secret-change-me",
            "change-me-access-secret",
            "change-me-refresh-secret",
            "ci-access",
            "ci-refresh",
            "test-access-secret",
        }
        if self.jwt_secret in weak or len(self.jwt_secret) < 32:
            raise RuntimeError(
                "JWT_SECRET must be set to a unique value of at least 32 characters in production."
            )
        if self.jwt_refresh_secret in weak or len(self.jwt_refresh_secret) < 32:
            raise RuntimeError(
                "JWT_REFRESH_SECRET must be set to a unique value of at least 32 characters in production."
            )


settings = Settings()
