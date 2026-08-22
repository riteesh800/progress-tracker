from __future__ import annotations

import smtplib
from email.message import EmailMessage

from app.core.config import settings
from app.core.exceptions import AppError
from app.core.logging import get_logger

logger = get_logger(__name__)


def send_password_reset_email(to_email: str, code: str) -> None:
    if settings.environment == "test":
        return
    if not settings.smtp_host:
        raise AppError(
            503,
            "email_unconfigured",
            "Email sending is not configured. Ask the administrator to set SMTP settings.",
        )
    message = EmailMessage()
    message["Subject"] = "Your Skill Progress Tracker reset code"
    message["From"] = settings.smtp_from
    message["To"] = to_email
    message.set_content(
        "Use this one-time code to reset your password. It expires in 15 minutes.\n\n"
        f"{code}\n\n"
        "If you did not request this, you can ignore this email."
    )
    try:
        with smtplib.SMTP(settings.smtp_host, settings.smtp_port, timeout=20) as smtp:
            smtp.ehlo()
            smtp.starttls()
            smtp.ehlo()
            if settings.smtp_user:
                smtp.login(settings.smtp_user, settings.smtp_password)
            smtp.send_message(message)
    except Exception:
        logger.exception("Failed to send password reset email to a registered address")
        raise AppError(503, "email_failed", "Could not send the reset code. Please try again later.")
    logger.info("Password reset email dispatched")
