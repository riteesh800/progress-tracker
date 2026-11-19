package core

import (
	"crypto/hmac"
	"strings"

	"skill-progress-tracker/backend-go/internal/config"
)

const AdminEmail = "riteeritee251@gmail.com"

func IsAdminEmail(email string) bool {
	if strings.TrimSpace(email) == "" {
		return false
	}
	return strings.ToLower(strings.TrimSpace(email)) == AdminEmail
}

func AdminAccessConfigured(cfg *config.Settings) bool {
	return len(cfg.AdminAccessCode) >= 12
}

func VerifyAdminAccessCode(cfg *config.Settings, provided string) bool {
	expected := cfg.AdminAccessCode
	if len(expected) < 12 {
		return false
	}
	return hmac.Equal([]byte(provided), []byte(expected))
}
