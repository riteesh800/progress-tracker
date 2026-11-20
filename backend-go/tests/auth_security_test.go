package tests

import (
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"skill-progress-tracker/backend-go/internal/config"
	"skill-progress-tracker/backend-go/internal/core"
	"skill-progress-tracker/backend-go/internal/database"
)

func TestHealthAndMiddleware(t *testing.T) {
	env := NewTestEnv(t)

	// 1. Direct /health and /api/health (StripAPIPrefix)
	for _, p := range []string{"/health", "/api/health"} {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		req.Header.Set("Origin", "https://my-app-preview.vercel.app")
		rec := httptest.NewRecorder()
		env.Router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("%s expected 200, got %d", p, rec.Code)
		}
		if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("missing X-Content-Type-Options")
		}
		if rec.Header().Get("X-Frame-Options") != "DENY" {
			t.Fatalf("missing X-Frame-Options")
		}
		if rec.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Fatalf("missing Referrer-Policy")
		}
		if rec.Header().Get("Permissions-Policy") == "" {
			t.Fatalf("missing Permissions-Policy")
		}
		if rec.Header().Get("Access-Control-Allow-Origin") != "https://my-app-preview.vercel.app" {
			t.Fatalf("CORS vercel regex failed: got %q", rec.Header().Get("Access-Control-Allow-Origin"))
		}
	}

	// 2. Disallowed CORS origin should not receive Access-Control-Allow-Origin
	reqBadOrigin := httptest.NewRequest(http.MethodGet, "/health", nil)
	reqBadOrigin.Header.Set("Origin", "https://evil.example.com")
	recBad := httptest.NewRecorder()
	env.Router.ServeHTTP(recBad, reqBadOrigin)
	if recBad.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("evil origin should not be allowed, got %q", recBad.Header().Get("Access-Control-Allow-Origin"))
	}

	// 3. GZip compression on responses >= 500 bytes (e.g. /streak/calendar returns 365 days > 5KB)
	access, _ := env.RegisterUser("gzip@example.com", "Password123!", "GzipUser")
	reqGzip := httptest.NewRequest(http.MethodGet, "/streak/calendar", nil)
	reqGzip.Header.Set("Authorization", "Bearer "+access)
	reqGzip.Header.Set("Accept-Encoding", "gzip")
	recGzip := httptest.NewRecorder()
	env.Router.ServeHTTP(recGzip, reqGzip)
	if recGzip.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recGzip.Code)
	}
	if recGzip.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("expected Content-Encoding: gzip, got %q", recGzip.Header().Get("Content-Encoding"))
	}
	gr, err := gzip.NewReader(recGzip.Body)
	if err != nil {
		t.Fatalf("gzip reader failed: %v", err)
	}
	unzipped, err := io.ReadAll(gr)
	_ = gr.Close()
	if err != nil || !strings.Contains(string(unzipped), `"days"`) {
		t.Fatalf("unexpected unzipped body: %v", err)
	}
}

func TestAuthRegisterLoginRefreshRotationAndGrace(t *testing.T) {
	env := NewTestEnv(t)

	// Validation failure: short password
	code, data, _ := env.DoJSON(http.MethodPost, "/auth/register", map[string]any{
		"email":    "alice@example.com",
		"password": "short",
	}, nil)
	if code != http.StatusUnprocessableEntity || data["code"] != "validation_error" {
		t.Fatalf("expected 422 validation_error, got %d %v", code, data)
	}

	// Validation failure: invalid timezone
	code, _, _ = env.DoJSON(http.MethodPost, "/auth/register", map[string]any{
		"email":    "alice@example.com",
		"password": "ValidPassword123!",
		"timezone": "Invalid/Timezone",
	}, nil)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for invalid timezone, got %d", code)
	}

	// Valid registration
	access, refresh1 := env.RegisterUser("Alice@Example.com", "ValidPassword123!", "Alice")
	if access == "" || refresh1 == "" {
		t.Fatalf("expected access and refresh tokens")
	}

	// Duplicate email (case-insensitive)
	code, data, _ = env.DoJSON(http.MethodPost, "/auth/register", map[string]any{
		"email":    "alice@example.com",
		"password": "ValidPassword123!",
	}, nil)
	if code != http.StatusConflict || data["code"] != "email_taken" {
		t.Fatalf("expected 409 email_taken, got %d %v", code, data)
	}

	// Wrong password
	code, data, _ = env.DoJSON(http.MethodPost, "/auth/login", map[string]any{
		"email":    "alice@example.com",
		"password": "WrongPassword123!",
	}, nil)
	if code != http.StatusUnauthorized || data["code"] != "invalid_credentials" {
		t.Fatalf("expected 401 invalid_credentials, got %d %v", code, data)
	}

	// Refresh rotation
	code, data, _ = env.DoJSON(http.MethodPost, "/auth/refresh", map[string]any{
		"refresh_token": refresh1,
	}, nil)
	if code != http.StatusOK {
		t.Fatalf("refresh 1 failed: %d %v", code, data)
	}
	refresh2, _ := data["refresh_token"].(string)
	if refresh2 == "" || refresh2 == refresh1 {
		t.Fatalf("expected rotated refresh token")
	}

	// Concurrent refresh within 20s grace period using refresh1 should succeed!
	code, data, _ = env.DoJSON(http.MethodPost, "/auth/refresh", map[string]any{
		"refresh_token": refresh1,
	}, nil)
	if code != http.StatusOK {
		t.Fatalf("expected grace period refresh to succeed within 20s, got %d %v", code, data)
	}
	refresh3, _ := data["refresh_token"].(string)

	// Simulate latest token created > 20s ago -> reusing refresh1 must now fail with 401 invalid_refresh
	past := time.Now().UTC().Add(-30 * time.Second)
	_, err := env.DB.Exec(context.Background(), nil, `UPDATE refresh_tokens SET created_at = ?`, env.DB.FormatTime(past))
	if err != nil {
		t.Fatalf("update refresh_tokens created_at: %v", err)
	}
	code, data, _ = env.DoJSON(http.MethodPost, "/auth/refresh", map[string]any{
		"refresh_token": refresh1,
	}, nil)
	if code != http.StatusUnauthorized || data["code"] != "invalid_refresh" {
		t.Fatalf("expected 401 invalid_refresh after 20s grace window, got %d %v", code, data)
	}

	// Logout revokes all refresh tokens for user
	code, _, _ = env.DoJSON(http.MethodPost, "/auth/logout", map[string]any{
		"refresh_token": refresh3,
	}, nil)
	if code != http.StatusOK {
		t.Fatalf("logout failed: %d", code)
	}
	code, data, _ = env.DoJSON(http.MethodPost, "/auth/refresh", map[string]any{
		"refresh_token": refresh3,
	}, nil)
	if code != http.StatusUnauthorized || data["code"] != "invalid_refresh" {
		t.Fatalf("expected 401 after logout, got %d %v", code, data)
	}
}

func TestGoogleOAuthAndForgotPasswordOTP(t *testing.T) {
	env := NewTestEnv(t)

	// 1. Google sign-in creates a Google-only account in test mode ("test.guser@example.com")
	code, data, _ := env.DoJSON(http.MethodPost, "/auth/google", map[string]any{
		"id_token": "test.guser@example.com",
		"timezone": "Asia/Kolkata",
	}, nil)
	if code != http.StatusOK || data["access_token"] == "" {
		t.Fatalf("google auth failed: %d %v", code, data)
	}

	// 2. Forgot password on Google-only account should return 400 password_unavailable
	code, data, _ = env.DoJSON(http.MethodPost, "/auth/forgot-password", map[string]any{
		"email": "guser@example.com",
	}, nil)
	if code != http.StatusBadRequest || data["code"] != "password_unavailable" {
		t.Fatalf("expected 400 password_unavailable, got %d %v", code, data)
	}

	// 3. Forgot password on unregistered email -> 404 email_not_registered
	code, data, _ = env.DoJSON(http.MethodPost, "/auth/forgot-password", map[string]any{
		"email": "nobody@example.com",
	}, nil)
	if code != http.StatusNotFound || data["code"] != "email_not_registered" {
		t.Fatalf("expected 404 email_not_registered, got %d %v", code, data)
	}

	// 4. Register password account, request OTP, verify OTP, reset password
	_, oldRefresh := env.RegisterUser("resetme@example.com", "OldPassword123!", "ResetMe")
	code, data, _ = env.DoJSON(http.MethodPost, "/auth/forgot-password", map[string]any{
		"email": "resetme@example.com",
	}, nil)
	if code != http.StatusOK {
		t.Fatalf("forgot-password failed: %d %v", code, data)
	}
	otp, _ := data["dev_code"].(string)
	if len(otp) != 6 {
		t.Fatalf("expected 6-digit dev_code in test mode, got %q", otp)
	}

	// Wrong OTP -> 400 invalid_reset
	code, data, _ = env.DoJSON(http.MethodPost, "/auth/verify-reset-code", map[string]any{
		"email": "resetme@example.com",
		"code":  "000000",
	}, nil)
	if code != http.StatusBadRequest || data["code"] != "invalid_reset" {
		t.Fatalf("expected 400 invalid_reset, got %d %v", code, data)
	}

	// Valid OTP -> 200 OK
	code, _, _ = env.DoJSON(http.MethodPost, "/auth/verify-reset-code", map[string]any{
		"email": "resetme@example.com",
		"code":  otp,
	}, nil)
	if code != http.StatusOK {
		t.Fatalf("verify-reset-code failed: %d", code)
	}

	// Reset password with OTP
	code, _, _ = env.DoJSON(http.MethodPost, "/auth/reset-password", map[string]any{
		"email":        "resetme@example.com",
		"code":         otp,
		"new_password": "NewPassword456!",
	}, nil)
	if code != http.StatusOK {
		t.Fatalf("reset-password failed: %d", code)
	}

	// Reusing same OTP again must fail -> 400 invalid_reset
	code, data, _ = env.DoJSON(http.MethodPost, "/auth/reset-password", map[string]any{
		"email":        "resetme@example.com",
		"code":         otp,
		"new_password": "AnotherPassword789!",
	}, nil)
	if code != http.StatusBadRequest || data["code"] != "invalid_reset" {
		t.Fatalf("expected 400 invalid_reset on reused OTP, got %d %v", code, data)
	}

	// Old refresh token must be revoked after password reset
	code, _, _ = env.DoJSON(http.MethodPost, "/auth/refresh", map[string]any{
		"refresh_token": oldRefresh,
	}, nil)
	if code != http.StatusUnauthorized {
		t.Fatalf("expected old refresh token to be revoked after reset, got %d", code)
	}

	// Login with new password succeeds
	code, _, _ = env.DoJSON(http.MethodPost, "/auth/login", map[string]any{
		"email":    "resetme@example.com",
		"password": "NewPassword456!",
	}, nil)
	if code != http.StatusOK {
		t.Fatalf("login with new password failed: %d", code)
	}
}

func TestAccountDeletion72HourRecoveryAndExpiry(t *testing.T) {
	env := NewTestEnv(t)

	access, _ := env.RegisterUser("delete72@example.com", "Password123!", "DelUser")

	// Request account deletion (72h window)
	code, _, _ := env.DoJSON(http.MethodDelete, "/auth/me", nil, AuthHeader(access))
	if code != http.StatusOK {
		t.Fatalf("DELETE /auth/me failed: %d", code)
	}

	// Re-registering with the same email while within the 72h recovery window must fail with 409 email_taken
	code, data, _ := env.DoJSON(http.MethodPost, "/auth/register", map[string]any{
		"email":    "delete72@example.com",
		"password": "Password123!",
	}, nil)
	if code != http.StatusConflict || data["code"] != "email_taken" {
		t.Fatalf("expected 409 email_taken during 72h window, got %d %v", code, data)
	}

	// Logging in within 72h cancels pending deletion automatically!
	code, data, _ = env.DoJSON(http.MethodPost, "/auth/login", map[string]any{
		"email":    "delete72@example.com",
		"password": "Password123!",
	}, nil)
	if code != http.StatusOK {
		t.Fatalf("expected login within 72h to recover account, got %d %v", code, data)
	}
	access2, _ := data["access_token"].(string)

	// Request deletion again, then expire the deadline
	code, _, _ = env.DoJSON(http.MethodDelete, "/auth/me", nil, AuthHeader(access2))
	if code != http.StatusOK {
		t.Fatalf("second DELETE /auth/me failed: %d", code)
	}
	past := time.Now().UTC().Add(-1 * time.Hour)
	_, err := env.DB.Exec(context.Background(), nil, `UPDATE users SET deletion_deadline = ? WHERE email = ?`, env.DB.FormatTime(past), "delete72@example.com")
	if err != nil {
		t.Fatalf("expire deadline: %v", err)
	}

	// Using access token after deadline purges user and returns 401 unauthenticated
	code, data, _ = env.DoJSON(http.MethodGet, "/auth/me", nil, AuthHeader(access2))
	if code != http.StatusUnauthorized || data["code"] != "unauthenticated" {
		t.Fatalf("expected 401 unauthenticated after 72h deadline, got %d %v", code, data)
	}

	// Now re-registering with the same email succeeds!
	_, _ = env.RegisterUser("delete72@example.com", "NewAccountPass123!", "DelUserReborn")
}

func TestAccountDeletionExpiryInResetPasswordAndVerifyCode(t *testing.T) {
	env := NewTestEnv(t)

	// Case 1: VerifyResetCode when deletion deadline is expired permanently deletes account and returns 400 invalid_reset
	access1, _ := env.RegisterUser("reset_del_exp1@example.com", "Password123!", "DelUser1")
	_, forgotResp, _ := env.DoJSON(http.MethodPost, "/auth/forgot-password", map[string]any{"email": "reset_del_exp1@example.com"}, nil)
	otp1, _ := forgotResp["dev_code"].(string)
	if otp1 == "" {
		t.Fatalf("dev_code not returned in test environment")
	}

	// Request deletion
	code, _, _ := env.DoJSON(http.MethodDelete, "/auth/me", nil, AuthHeader(access1))
	if code != http.StatusOK {
		t.Fatalf("DELETE /auth/me failed: %d", code)
	}

	// Active within grace period -> verify succeeds
	code, _, _ = env.DoJSON(http.MethodPost, "/auth/verify-reset-code", map[string]any{
		"email": "reset_del_exp1@example.com",
		"code":  otp1,
	}, nil)
	if code != http.StatusOK {
		t.Fatalf("verify-reset-code within grace period expected 200, got %d", code)
	}

	// Now expire the deletion deadline
	past := time.Now().UTC().Add(-2 * time.Hour)
	_, err := env.DB.Exec(context.Background(), nil, `UPDATE users SET deletion_deadline = ? WHERE email = ?`, env.DB.FormatTime(past), "reset_del_exp1@example.com")
	if err != nil {
		t.Fatalf("update deletion_deadline: %v", err)
	}

	// Verify reset code after expiry -> must return 400 invalid_reset and delete the account
	code, data, _ := env.DoJSON(http.MethodPost, "/auth/verify-reset-code", map[string]any{
		"email": "reset_del_exp1@example.com",
		"code":  otp1,
	}, nil)
	if code != http.StatusBadRequest || data["code"] != "invalid_reset" {
		t.Fatalf("expected 400 invalid_reset after deletion expired, got %d %v", code, data)
	}

	// User should now be permanently gone from DB
	var count int
	_ = env.DB.QueryRow(context.Background(), nil, `SELECT COUNT(*) FROM users WHERE email = ?`, "reset_del_exp1@example.com").Scan(&count)
	if count != 0 {
		t.Fatalf("expected user to be permanently deleted from DB, count was %d", count)
	}

	// Case 2: ResetPassword when deletion deadline is expired permanently deletes account and returns 400 invalid_reset
	access2, _ := env.RegisterUser("reset_del_exp2@example.com", "Password123!", "DelUser2")
	_, forgotResp2, _ := env.DoJSON(http.MethodPost, "/auth/forgot-password", map[string]any{"email": "reset_del_exp2@example.com"}, nil)
	otp2, _ := forgotResp2["dev_code"].(string)

	code, _, _ = env.DoJSON(http.MethodDelete, "/auth/me", nil, AuthHeader(access2))
	if code != http.StatusOK {
		t.Fatalf("DELETE /auth/me failed: %d", code)
	}

	// Expire deletion deadline
	_, err = env.DB.Exec(context.Background(), nil, `UPDATE users SET deletion_deadline = ? WHERE email = ?`, env.DB.FormatTime(past), "reset_del_exp2@example.com")
	if err != nil {
		t.Fatalf("update deletion_deadline: %v", err)
	}

	code, data, _ = env.DoJSON(http.MethodPost, "/auth/reset-password", map[string]any{
		"email":        "reset_del_exp2@example.com",
		"code":         otp2,
		"new_password": "NewSecret123!",
	}, nil)
	if code != http.StatusBadRequest || data["code"] != "invalid_reset" {
		t.Fatalf("expected 400 invalid_reset after deletion expired, got %d %v", code, data)
	}

	_ = env.DB.QueryRow(context.Background(), nil, `SELECT COUNT(*) FROM users WHERE email = ?`, "reset_del_exp2@example.com").Scan(&count)
	if count != 0 {
		t.Fatalf("expected user to be permanently deleted from DB, count was %d", count)
	}

	// Case 3: ResetPassword within grace period recovers account (cancels pending deletion)
	access3, _ := env.RegisterUser("reset_del_rec3@example.com", "Password123!", "DelUser3")
	_, forgotResp3, _ := env.DoJSON(http.MethodPost, "/auth/forgot-password", map[string]any{"email": "reset_del_rec3@example.com"}, nil)
	otp3, _ := forgotResp3["dev_code"].(string)

	code, _, _ = env.DoJSON(http.MethodDelete, "/auth/me", nil, AuthHeader(access3))
	if code != http.StatusOK {
		t.Fatalf("DELETE /auth/me failed: %d", code)
	}

	code, _, _ = env.DoJSON(http.MethodPost, "/auth/reset-password", map[string]any{
		"email":        "reset_del_rec3@example.com",
		"code":         otp3,
		"new_password": "RecoveredPassword123!",
	}, nil)
	if code != http.StatusOK {
		t.Fatalf("reset-password within grace period expected 200, got %d", code)
	}

	// Login with new password works and pending deletion is cancelled
	code, loginData, _ := env.DoJSON(http.MethodPost, "/auth/login", map[string]any{
		"email":    "reset_del_rec3@example.com",
		"password": "RecoveredPassword123!",
	}, nil)
	if code != http.StatusOK {
		t.Fatalf("login after reset password recovery failed: %d %v", code, loginData)
	}
}

func TestAdminPortalForcedPasswordChangeAndCronCleanup(t *testing.T) {
	env := NewTestEnv(t)

	// Regular user and Admin user
	userToken, _ := env.RegisterUser("normal@example.com", "UserPass123!", "NormalUser")
	adminToken, _ := env.RegisterUser(core.AdminEmail, "AdminPass123!", "RiteeAdmin")

	// Normal user /auth/me has can_open_admin = false; admin has can_open_admin = true
	_, uMe, _ := env.DoJSON(http.MethodGet, "/auth/me", nil, AuthHeader(userToken))
	if uMe["can_open_admin"] != false {
		t.Fatalf("normal user must have can_open_admin=false")
	}
	_, aMe, _ := env.DoJSON(http.MethodGet, "/auth/me", nil, AuthHeader(adminToken))
	if aMe["can_open_admin"] != true {
		t.Fatalf("admin user must have can_open_admin=true")
	}

	// Normal user cannot unlock admin -> 403 forbidden
	code, data, _ := env.DoJSON(http.MethodPost, "/admin/unlock", map[string]any{
		"access_code": env.Cfg.AdminAccessCode,
	}, AuthHeader(userToken))
	if code != http.StatusForbidden || data["code"] != "forbidden" {
		t.Fatalf("expected 403 forbidden for normal user on /admin/unlock, got %d %v", code, data)
	}

	// Admin cannot call /admin/users without X-Admin-Session -> 401 admin_locked
	code, data, _ = env.DoJSON(http.MethodGet, "/admin/users", nil, AuthHeader(adminToken))
	if code != http.StatusUnauthorized || data["code"] != "admin_locked" {
		t.Fatalf("expected 401 admin_locked without X-Admin-Session, got %d %v", code, data)
	}

	// Admin unlock with wrong access code -> 403 invalid_access_code
	code, data, _ = env.DoJSON(http.MethodPost, "/admin/unlock", map[string]any{
		"access_code": "wrong-access-code",
	}, AuthHeader(adminToken))
	if code != http.StatusForbidden || data["code"] != "invalid_access_code" {
		t.Fatalf("expected 403 invalid_access_code, got %d %v", code, data)
	}

	// Admin unlock with valid access code -> 200 OK
	code, unlockData, _ := env.DoJSON(http.MethodPost, "/admin/unlock", map[string]any{
		"access_code": env.Cfg.AdminAccessCode,
	}, AuthHeader(adminToken))
	if code != http.StatusOK {
		t.Fatalf("admin unlock failed: %d %v", code, unlockData)
	}
	xAdminSession, _ := unlockData["admin_token"].(string)
	adminHeaders := map[string]string{
		"Authorization":   "Bearer " + adminToken,
		"X-Admin-Session": xAdminSession,
	}

	// Admin resets normal user's password -> sets must_change_password = true
	normalID, _ := uMe["id"].(string)
	code, _, _ = env.DoJSON(http.MethodPost, "/admin/users/"+normalID+"/password", map[string]any{
		"new_password": "TempPassword999!",
	}, adminHeaders)
	if code != http.StatusOK {
		t.Fatalf("admin reset user password failed: %d", code)
	}

	// Normal user logs in with temporary password
	code, loginData, _ := env.DoJSON(http.MethodPost, "/auth/login", map[string]any{
		"email":    "normal@example.com",
		"password": "TempPassword999!",
	}, nil)
	if code != http.StatusOK {
		t.Fatalf("login with temp password failed: %d", code)
	}
	tempToken, _ := loginData["access_token"].(string)

	// Normal user is blocked from regular endpoints (/skills, /dashboard) with 403 password_change_required
	code, data, _ = env.DoJSON(http.MethodGet, "/skills", nil, AuthHeader(tempToken))
	if code != http.StatusForbidden || data["code"] != "password_change_required" {
		t.Fatalf("expected 403 password_change_required on /skills, got %d %v", code, data)
	}

	// Normal user CAN access GET /auth/me and POST /auth/change-password
	code, meData, _ := env.DoJSON(http.MethodGet, "/auth/me", nil, AuthHeader(tempToken))
	if code != http.StatusOK || meData["must_change_password"] != true {
		t.Fatalf("expected GET /auth/me to succeed with must_change_password=true, got %d %v", code, meData)
	}

	// Reusing temporary password must fail with 400 password_reused
	code, data, _ = env.DoJSON(http.MethodPost, "/auth/change-password", map[string]any{
		"new_password": "TempPassword999!",
	}, AuthHeader(tempToken))
	if code != http.StatusBadRequest || data["code"] != "password_reused" {
		t.Fatalf("expected 400 password_reused, got %d %v", code, data)
	}

	// Setting a new password clears must_change_password
	code, meAfter, _ := env.DoJSON(http.MethodPost, "/auth/change-password", map[string]any{
		"new_password": "PermanentPassword123!",
	}, AuthHeader(tempToken))
	if code != http.StatusOK || meAfter["must_change_password"] != false {
		t.Fatalf("change-password failed: %d %v", code, meAfter)
	}

	// Cron cleanup endpoint requires Bearer CleanupSecret
	code, data, _ = env.DoJSON(http.MethodPost, "/admin/cron/cleanup", nil, nil)
	if code != http.StatusUnauthorized || data["code"] != "unauthenticated" {
		t.Fatalf("expected 401 on cron cleanup without header, got %d %v", code, data)
	}
	code, data, _ = env.DoJSON(http.MethodPost, "/admin/cron/cleanup", nil, AuthHeader("wrong-secret"))
	if code != http.StatusForbidden || data["code"] != "forbidden" {
		t.Fatalf("expected 403 on cron cleanup with wrong secret, got %d %v", code, data)
	}
	code, cronData, _ := env.DoJSON(http.MethodGet, "/admin/cron/cleanup", nil, AuthHeader(env.Cfg.EffectiveCleanupSecret()))
	if code != http.StatusOK || cronData["ok"] != true {
		t.Fatalf("cron cleanup failed: %d %v", code, cronData)
	}
}

func TestRateLimitingAndTokenSecurity(t *testing.T) {
	env := NewTestEnv(t)
	env.Limiter.SetEnabled(true)
	env.Limiter.Reset()

	// 5 login attempts allowed per 15m; 6th must return 429 rate_limited
	for i := 1; i <= 5; i++ {
		code, _, _ := env.DoJSON(http.MethodPost, "/auth/login", map[string]any{
			"email":    "ratelimit@example.com",
			"password": "Password123!",
		}, nil)
		if code != http.StatusUnauthorized {
			t.Fatalf("attempt %d expected 401, got %d", i, code)
		}
	}
	code, data, _ := env.DoJSON(http.MethodPost, "/auth/login", map[string]any{
		"email":    "ratelimit@example.com",
		"password": "Password123!",
	}, nil)
	if code != http.StatusTooManyRequests || data["code"] != "rate_limited" {
		t.Fatalf("6th login attempt expected 429 rate_limited, got %d %v", code, data)
	}

	env.Limiter.SetEnabled(false)

	// Expired JWT token returns 401 token_expired
	negMin := -5
	expiredToken, err := core.CreateAccessToken(env.Cfg, "00000000-0000-0000-0000-000000000001", nil, &negMin)
	if err != nil {
		t.Fatalf("create expired token: %v", err)
	}
	code, data, _ = env.DoJSON(http.MethodGet, "/auth/me", nil, AuthHeader(expiredToken))
	if code != http.StatusUnauthorized || data["code"] != "token_expired" {
		t.Fatalf("expected 401 token_expired, got %d %v", code, data)
	}

	// Tampered JWT token returns 401 invalid_token
	code, data, _ = env.DoJSON(http.MethodGet, "/auth/me", nil, AuthHeader(expiredToken+"tampered"))
	if code != http.StatusUnauthorized || data["code"] != "invalid_token" {
		t.Fatalf("expected 401 invalid_token, got %d %v", code, data)
	}
}

func TestProductionDatabaseAndSecretValidation(t *testing.T) {
	baseProdCfg := func() *config.Settings {
		return &config.Settings{
			Environment:            "production",
			DatabaseURL:            "postgresql+asyncpg://postgres.user:pass123@aws-0-ap-south-1.pooler.supabase.com:6543/postgres?pgbouncer=true&sslmode=require",
			JWTSecret:              "prod-jwt-secret-key-with-at-least-32-chars!!",
			JWTRefreshSecret:       "prod-refresh-secret-key-with-32-chars-min!!",
			InternalParserSecret:   "prod-independent-parser-secret-32-chars-min!!",
			CleanupSecret:          "prod-cleanup-secret-key-32-chars-minimum!!",
			CronSecret:             "prod-cron-secret-key-32-chars-minimum-123!!",
			SupabaseURL:            "https://exampleproject.supabase.co",
			SupabaseServiceRoleKey: "prod-supabase-service-role-key-secret",
			SupabaseStorageBucket:  "pdf-imports",
		}
	}

	// 1. Valid production configuration passes AssertProductionSecrets and normalizes Supabase PgBouncer DSN
	validCfg := baseProdCfg()
	if err := validCfg.AssertProductionSecrets(); err != nil {
		t.Fatalf("expected valid production config to pass, got: %v", err)
	}
	pgDSN := validCfg.PostgresDSN()
	if !strings.HasPrefix(pgDSN, "postgres://") || strings.Contains(pgDSN, "pgbouncer") {
		t.Fatalf("unexpected PostgresDSN normalization: %s", pgDSN)
	}

	// 2. Production MUST fail when DATABASE_URL is missing
	missingDB := baseProdCfg()
	missingDB.DatabaseURL = ""
	if err := missingDB.AssertProductionSecrets(); err == nil {
		t.Fatalf("expected production with empty DATABASE_URL to fail AssertProductionSecrets")
	}
	if _, err := database.Open(missingDB); err == nil {
		t.Fatalf("expected database.Open to fail when DATABASE_URL is empty in production")
	}

	// 3. Production MUST reject SQLite
	sqliteProd := baseProdCfg()
	sqliteProd.DatabaseURL = "sqlite+aiosqlite:///./progress_tracker.db"
	if err := sqliteProd.AssertProductionSecrets(); err == nil {
		t.Fatalf("expected production with SQLite to fail AssertProductionSecrets")
	}
	if _, err := database.Open(sqliteProd); err == nil {
		t.Fatalf("expected database.Open to reject SQLite in production")
	}

	// 4. Production MUST reject invalid PostgreSQL URL (missing host/db)
	badPG := baseProdCfg()
	badPG.DatabaseURL = "postgres:///onlypath"
	if err := badPG.AssertProductionSecrets(); err == nil {
		t.Fatalf("expected production with invalid Postgres URL to fail")
	}

	// 5. EffectiveInternalSecret MUST NOT fall back to JWT_SECRET, CLEANUP_SECRET, or CRON_SECRET
	noInternal := baseProdCfg()
	noInternal.InternalParserSecret = ""
	if got := noInternal.EffectiveInternalSecret(); got != "" {
		t.Fatalf("expected EffectiveInternalSecret() to be empty when unset, got %q", got)
	}
	if err := noInternal.AssertProductionSecrets(); err == nil {
		t.Fatalf("expected production with missing INTERNAL_PARSER_SECRET to fail")
	}

	// 6. INTERNAL_PARSER_SECRET must be at least 32 characters and independent of JWT/CLEANUP/CRON secrets
	shortInternal := baseProdCfg()
	shortInternal.InternalParserSecret = "too-short-secret"
	if err := shortInternal.AssertProductionSecrets(); err == nil {
		t.Fatalf("expected short INTERNAL_PARSER_SECRET to fail in production")
	}

	reusedJWT := baseProdCfg()
	reusedJWT.InternalParserSecret = reusedJWT.JWTSecret
	if err := reusedJWT.AssertProductionSecrets(); err == nil {
		t.Fatalf("expected INTERNAL_PARSER_SECRET equal to JWT_SECRET to fail in production")
	}

	reusedCleanup := baseProdCfg()
	reusedCleanup.InternalParserSecret = reusedCleanup.CleanupSecret
	if err := reusedCleanup.AssertProductionSecrets(); err == nil {
		t.Fatalf("expected INTERNAL_PARSER_SECRET equal to CLEANUP_SECRET to fail in production")
	}

	// 7. Supabase Storage is optional in production for direct <=4.5 MB uploads,
	// but if partially configured it must be complete (URL, service role key, and bucket).
	zeroStorage := baseProdCfg()
	zeroStorage.SupabaseURL = ""
	zeroStorage.SupabaseServiceRoleKey = ""
	if err := zeroStorage.AssertProductionSecrets(); err != nil {
		t.Fatalf("expected production without Supabase Storage to pass for direct <=4.5MB uploads, got: %v", err)
	}

	partialURL := baseProdCfg()
	partialURL.SupabaseServiceRoleKey = ""
	if err := partialURL.AssertProductionSecrets(); err == nil {
		t.Fatalf("expected production with partial Supabase config (missing service role key) to fail")
	}

	partialKey := baseProdCfg()
	partialKey.SupabaseURL = ""
	if err := partialKey.AssertProductionSecrets(); err == nil {
		t.Fatalf("expected production with partial Supabase config (missing URL) to fail")
	}

	partialBucket := baseProdCfg()
	partialBucket.SupabaseStorageBucket = ""
	if err := partialBucket.AssertProductionSecrets(); err == nil {
		t.Fatalf("expected production with partial Supabase config (missing bucket) to fail")
	}

	if zeroStorage.AllowsLocalSignedPDFStorage() {
		t.Fatalf("production must never allow local_signed PDF storage fallback")
	}
}

func TestCronSkillCleanupNullifiesForeignKeys(t *testing.T) {
	env := NewTestEnv(t)
	token, _ := env.RegisterUser("cronpurge@example.com", "Password123!", "CronPurgeUser")

	// Create skill and topic
	_, skill, _ := env.DoJSON(http.MethodPost, "/skills", map[string]any{
		"name": "Skill To Expire",
	}, AuthHeader(token))
	skillID, _ := skill["id"].(string)

	_, topic, _ := env.DoJSON(http.MethodPost, "/topics", map[string]any{
		"skill_id": skillID,
		"name":     "Leaf Topic",
	}, AuthHeader(token))
	topicID, _ := topic["id"].(string)

	// Complete topic so activity_log has both skill_id and topic_id references
	code, _, _ := env.DoJSON(http.MethodPost, "/topics/"+topicID+"/complete", map[string]any{
		"completed": true,
	}, AuthHeader(token))
	if code != http.StatusOK {
		t.Fatalf("complete topic failed: %d", code)
	}

	// Move skill to trash and expire its purge_at deadline
	code, _, _ = env.DoJSON(http.MethodDelete, "/skills/"+skillID+"?confirm=true", nil, AuthHeader(token))
	if code != http.StatusOK {
		t.Fatalf("trash skill failed: %d", code)
	}
	past := time.Now().UTC().Add(-2 * time.Hour)
	_, err := env.DB.Exec(context.Background(), nil, `UPDATE skills SET purge_at = ? WHERE id = ?`, env.DB.FormatTime(past), skillID)
	if err != nil {
		t.Fatalf("expire skill purge_at: %v", err)
	}

	// Trigger cron cleanup
	code, cronData, _ := env.DoJSON(http.MethodPost, "/admin/cron/cleanup", nil, AuthHeader(env.Cfg.EffectiveCleanupSecret()))
	if code != http.StatusOK || cronData["skills_purged"] != float64(1) {
		t.Fatalf("expected skills_purged=1, got %d %v", code, cronData)
	}

	// Verify activity_log entries were preserved with NULL skill_id and NULL topic_id
	var nonNullCount int
	err = env.DB.QueryRow(
		context.Background(), nil,
		`SELECT COUNT(*) FROM activity_log WHERE skill_id IS NOT NULL OR topic_id IS NOT NULL`,
	).Scan(&nonNullCount)
	if err != nil || nonNullCount != 0 {
		t.Fatalf("expected 0 non-null skill_id/topic_id in activity_log after purge, got %d (err=%v)", nonNullCount, err)
	}
	var totalLogs int
	_ = env.DB.QueryRow(context.Background(), nil, `SELECT COUNT(*) FROM activity_log`).Scan(&totalLogs)
	if totalLogs < 2 {
		t.Fatalf("expected activity_log rows to be preserved after skill purge, got %d", totalLogs)
	}
}
