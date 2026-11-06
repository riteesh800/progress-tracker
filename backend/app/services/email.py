from __future__ import annotations

import asyncio
import smtplib
from email.message import EmailMessage
from email.utils import formataddr, parseaddr

from app.core.config import settings
from app.core.exceptions import AppError
from app.core.logging import get_logger

logger = get_logger(__name__)


def _send_smtp(to_email: str, code: str) -> None:
    """Synchronous SMTP work — meant to run in a thread pool."""
    sender_raw = settings.smtp_from or settings.smtp_user
    name, addr = parseaddr(sender_raw)
    display_name = name or "Skill Progress Tracker"
    clean_addr = addr or sender_raw

    message = EmailMessage()
    message["Subject"] = f"{code} is your Skill Progress Tracker verification code"
    message["From"] = formataddr((display_name, clean_addr))
    message["To"] = to_email

    plain_content = (
        "Your Skill Progress Tracker verification code is:\n\n"
        f"    {code}\n\n"
        "This code will expire in 15 minutes.\n"
        "If you did not request a password reset, you can safely ignore this email."
    )
    message.set_content(plain_content)

    html_content = f"""\
<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>Reset Password OTP</title>
</head>
<body style="margin: 0; padding: 24px; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background-color: #0b0f19; color: #f1f5f9;">
  <div style="max-width: 480px; margin: 0 auto; background-color: #111827; border: 1px solid #1f2937; border-radius: 12px; padding: 32px; box-shadow: 0 4px 20px rgba(0, 0, 0, 0.4);">
    <h2 style="margin: 0 0 8px 0; color: #38bdf8; font-size: 20px;">Skill Progress Tracker</h2>
    <p style="margin: 0 0 20px 0; color: #94a3b8; font-size: 14px; line-height: 1.5;">
      You requested to reset your password. Use the 6-digit one-time password (OTP) below to proceed:
    </p>
    <div style="background-color: #030712; border: 1px solid #1e293b; border-radius: 8px; padding: 18px; text-align: center; margin: 24px 0;">
      <span style="font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace; font-size: 32px; font-weight: 700; letter-spacing: 8px; color: #38bdf8;">{code}</span>
    </div>
    <p style="margin: 0 0 8px 0; color: #94a3b8; font-size: 13px;">
      This code is valid for <strong>15 minutes</strong> and can only be used once.
    </p>
    <p style="margin: 20px 0 0 0; color: #64748b; font-size: 12px; border-top: 1px solid #1f2937; padding-top: 16px;">
      If you did not request this code, no action is needed — your account remains secure.
    </p>
  </div>
</body>
</html>
"""
    message.add_alternative(html_content, subtype="html")

    if settings.smtp_port == 465:
        with smtplib.SMTP_SSL(settings.smtp_host, settings.smtp_port, timeout=20) as smtp:
            if settings.smtp_user and settings.smtp_password:
                smtp.login(settings.smtp_user, settings.smtp_password)
            smtp.send_message(message)
    else:
        with smtplib.SMTP(settings.smtp_host, settings.smtp_port, timeout=20) as smtp:
            smtp.ehlo()
            smtp.starttls()
            smtp.ehlo()
            if settings.smtp_user and settings.smtp_password:
                smtp.login(settings.smtp_user, settings.smtp_password)
            smtp.send_message(message)


async def send_password_reset_email(to_email: str, code: str) -> None:
    if settings.environment == "test":
        return
    if not settings.smtp_host:
        raise AppError(
            503,
            "email_unconfigured",
            "Email sending is not configured. Ask the administrator to set SMTP settings.",
        )
    try:
        await asyncio.to_thread(_send_smtp, to_email, code)
    except AppError:
        raise
    except Exception:
        logger.exception("Failed to send password reset email to a registered address")
        raise AppError(503, "email_failed", "Could not send the reset code. Please try again later.")
    logger.info("Password reset email dispatched")
