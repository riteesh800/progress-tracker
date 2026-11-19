package services

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"

	"skill-progress-tracker/backend-go/internal/config"
	"skill-progress-tracker/backend-go/internal/core"
)

var SendPasswordResetEmailFunc = defaultSendPasswordResetEmail

func SendPasswordResetEmail(cfg *config.Settings, toEmail, code string) error {
	return SendPasswordResetEmailFunc(cfg, toEmail, code)
}

func defaultSendPasswordResetEmail(cfg *config.Settings, toEmail, code string) error {
	if cfg.IsTest() {
		return nil
	}
	host := strings.TrimSpace(cfg.SMTPHost)
	if host == "" {
		return core.NewAppError(
			503,
			"email_unconfigured",
			"Email sending is not configured. Ask the administrator to set SMTP settings.",
		)
	}

	senderRaw := strings.TrimSpace(cfg.SMTPFrom)
	if senderRaw == "" {
		senderRaw = strings.TrimSpace(cfg.SMTPUser)
	}
	displayName := "Skill Progress Tracker"
	cleanAddr := senderRaw
	if parsed, err := mail.ParseAddress(senderRaw); err == nil {
		if parsed.Name != "" {
			displayName = parsed.Name
		}
		if parsed.Address != "" {
			cleanAddr = parsed.Address
		}
	}

	boundary := "===============SkillProgressTrackerOTPBoundary=="
	plainContent := fmt.Sprintf(
		"Your Skill Progress Tracker verification code is:\r\n\r\n"+
			"    %s\r\n\r\n"+
			"This code will expire in 15 minutes.\r\n"+
			"If you did not request a password reset, you can safely ignore this email.\r\n",
		code,
	)

	htmlContent := fmt.Sprintf(`<!DOCTYPE html>
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
      <span style="font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace; font-size: 32px; font-weight: 700; letter-spacing: 8px; color: #38bdf8;">%s</span>
    </div>
    <p style="margin: 0 0 8px 0; color: #94a3b8; font-size: 13px;">
      This code is valid for <strong>15 minutes</strong> and can only be used once.
    </p>
    <p style="margin: 20px 0 0 0; color: #64748b; font-size: 12px; border-top: 1px solid #1f2937; padding-top: 16px;">
      If you did not request this code, no action is needed — your account remains secure.
    </p>
  </div>
</body>
</html>`, code)

	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("From: %s <%s>\r\n", displayName, cleanAddr))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", toEmail))
	msg.WriteString(fmt.Sprintf("Subject: %s is your Skill Progress Tracker verification code\r\n", code))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=\"%s\"\r\n\r\n", boundary))
	msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	msg.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n\r\n")
	msg.WriteString(plainContent + "\r\n")
	msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	msg.WriteString("Content-Type: text/html; charset=\"utf-8\"\r\n\r\n")
	msg.WriteString(htmlContent + "\r\n")
	msg.WriteString(fmt.Sprintf("--%s--\r\n", boundary))

	addr := fmt.Sprintf("%s:%d", host, cfg.SMTPPort)
	dialer := &net.Dialer{Timeout: 20 * time.Second}

	var err error
	if cfg.SMTPPort == 465 {
		err = sendImplicitTLS(dialer, addr, host, cfg.SMTPUser, cfg.SMTPPassword, cleanAddr, toEmail, []byte(msg.String()))
	} else {
		err = sendStandardSMTP(dialer, addr, host, true, cfg.SMTPUser, cfg.SMTPPassword, cleanAddr, toEmail, []byte(msg.String()))
	}
	if err != nil {
		core.Logger.Error("Failed to send password reset email", "err", err.Error())
		return core.NewAppError(503, "email_failed", "Could not send the reset code. Please try again later.")
	}
	core.Logger.Info("Password reset email dispatched")
	return nil
}

func sendImplicitTLS(dialer *net.Dialer, addr, host, username, password, from, to string, msg []byte) error {
	conn, err := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
	if err != nil {
		return err
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Close()

	if username != "" && password != "" {
		auth := smtp.PlainAuth("", username, password, host)
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func sendStandardSMTP(dialer *net.Dialer, addr, host string, useTLS bool, username, password, from, to string, msg []byte) error {
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Close()

	if useTLS {
		if err := client.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
			return err
		}
	}
	if username != "" && password != "" {
		auth := smtp.PlainAuth("", username, password, host)
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}
