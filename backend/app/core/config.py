from __future__ import annotations

from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_file=".env", extra="ignore")

    database_url: str = "sqlite+aiosqlite:///./progress_tracker.db"
    jwt_secret: str = "dev-access-secret-change-me"
    jwt_refresh_secret: str = "dev-refresh-secret-change-me"
    access_token_expire_minutes: int = 15
    refresh_token_expire_days: int = 30
    google_client_id: str = ""
    google_client_secret: str = ""
    anthropic_api_key: str = ""
    cors_origins: str = "http://localhost:5173"
    smtp_host: str = ""
    smtp_port: int = 587
    smtp_user: str = ""
    smtp_password: str = ""
    smtp_from: str = "noreply@example.com"
    environment: str = "development"
    max_pdf_bytes: int = 10 * 1024 * 1024
    max_pdf_pages: int = 200

    @property
    def cors_origin_list(self) -> list[str]:
        return [o.strip() for o in self.cors_origins.split(",") if o.strip()]

    @property
    def is_sqlite(self) -> bool:
        return self.database_url.startswith("sqlite")

    @property
    def is_production(self) -> bool:
        return self.environment.lower() == "production"

    def assert_production_secrets(self) -> None:
        if not self.is_production:
            return
        weak = {
            "",
            "dev-access-secret-change-me",
            "change-me-access-secret",
            "ci-access",
            "test-access-secret",
        }
        if self.jwt_secret in weak or len(self.jwt_secret) < 32:
            raise RuntimeError(
                "JWT_SECRET must be set to a unique value of at least 32 characters in production."
            )


settings = Settings()
