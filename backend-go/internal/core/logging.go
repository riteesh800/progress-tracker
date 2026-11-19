package core

import (
	"log/slog"
	"os"
	"strings"
)

var sensitiveKeys = []string{
	"password",
	"token",
	"secret",
	"authorization",
	"api_key",
	"refresh_token",
	"access_code",
	"admin_access_code",
	"new_password",
}

var Logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
	Level: slog.LevelInfo,
}))

func Redact(data map[string]any) map[string]any {
	out := make(map[string]any, len(data))
	for k, v := range data {
		kLower := strings.ToLower(k)
		isSensitive := false
		for _, s := range sensitiveKeys {
			if strings.Contains(kLower, s) {
				isSensitive = true
				break
			}
		}
		if isSensitive {
			out[k] = "[redacted]"
		} else {
			out[k] = v
		}
	}
	return out
}
