package handlers

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"skill-progress-tracker/backend-go/internal/core"
	"skill-progress-tracker/backend-go/internal/database"
	"skill-progress-tracker/backend-go/internal/middleware"
	"skill-progress-tracker/backend-go/internal/models"
	"skill-progress-tracker/backend-go/internal/services"
)

func (s *Server) issueTokens(ctx context.Context, q database.Querier, userID uuid.UUID) (models.TokenPair, error) {
	access, err := core.CreateAccessToken(s.Cfg, userID.String(), nil, nil)
	if err != nil {
		return models.TokenPair{}, err
	}
	rawRefresh := core.CreateRefreshTokenValue()
	now := time.Now().UTC()
	exp := now.Add(time.Duration(s.Cfg.RefreshTokenExpireDays) * 24 * time.Hour)
	_, err = s.DB.Exec(
		ctx, q,
		`INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, revoked, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		s.DB.FormatUUID(uuid.New()),
		s.DB.FormatUUID(userID),
		core.HashToken(rawRefresh),
		s.DB.FormatTime(exp),
		false,
		s.DB.FormatTime(now),
	)
	if err != nil {
		return models.TokenPair{}, err
	}
	return models.NewTokenPair(access, rawRefresh), nil
}

func (s *Server) findUserByEmail(ctx context.Context, q database.Querier, email string) (*models.User, error) {
	row := s.DB.QueryRow(ctx, q, `SELECT `+middleware.UserColumns+` FROM users WHERE email = ?`, strings.ToLower(email))
	u, err := middleware.ScanUser(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return u, err
}

func (s *Server) findUserByID(ctx context.Context, q database.Querier, id uuid.UUID) (*models.User, error) {
	row := s.DB.QueryRow(ctx, q, `SELECT `+middleware.UserColumns+` FROM users WHERE id = ?`, s.DB.FormatUUID(id))
	u, err := middleware.ScanUser(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return u, err
}

func (s *Server) Register(w http.ResponseWriter, r *http.Request) {
	if appErr := s.Limiter.EnforceLimit(s.Cfg, r, "register", 8, 900); appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}
	body, ok := DecodeBody[models.RegisterIn](w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	emailLower := strings.ToLower(body.Email)
	now := time.Now().UTC()

	existing, err := s.findUserByEmail(ctx, nil, emailLower)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	if existing != nil {
		if existing.DeletionRequestedAt != nil && existing.DeletionDeadline != nil {
			if !now.Before(existing.DeletionDeadline.UTC()) {
				_, _ = s.DB.Exec(ctx, nil, `DELETE FROM users WHERE id = ?`, s.DB.FormatUUID(existing.ID))
			} else {
				core.WriteError(w, http.StatusConflict, "email_taken", "An account with this email already exists.")
				return
			}
		} else {
			core.WriteError(w, http.StatusConflict, "email_taken", "An account with this email already exists.")
			return
		}
	}

	pwdHash, err := core.HashPassword(body.Password)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	name := strings.Split(emailLower, "@")[0]
	if body.Name != nil && *body.Name != "" {
		name = *body.Name
	}
	tz := "UTC"
	if body.Timezone != nil && *body.Timezone != "" {
		tz = *body.Timezone
	}
	uid := uuid.New()

	var tokens models.TokenPair
	err = s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := s.DB.Exec(
			ctx, tx,
			`INSERT INTO users (id, email, password_hash, google_id, name, timezone, is_active, must_change_password, deletion_requested_at, deletion_deadline, last_active_at, created_at, updated_at)
			 VALUES (?, ?, ?, NULL, ?, ?, ?, ?, NULL, NULL, ?, ?, ?)`,
			s.DB.FormatUUID(uid),
			emailLower,
			pwdHash,
			name,
			tz,
			true,
			false,
			s.DB.FormatTime(now),
			s.DB.FormatTime(now),
			s.DB.FormatTime(now),
		)
		if err != nil {
			return err
		}
		tokens, err = s.issueTokens(ctx, tx, uid)
		return err
	})
	if err != nil {
		if database.IsUniqueViolation(err) {
			core.WriteError(w, http.StatusConflict, "email_taken", "An account with this email already exists.")
			return
		}
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}

	core.WriteJSON(w, http.StatusOK, tokens)
}

func (s *Server) Login(w http.ResponseWriter, r *http.Request) {
	if appErr := s.Limiter.EnforceLimit(s.Cfg, r, "login", 5, 900); appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}
	body, ok := DecodeBody[models.LoginIn](w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	user, err := s.findUserByEmail(ctx, nil, strings.ToLower(body.Email))
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	if user == nil || user.PasswordHash == nil || !core.VerifyPassword(*body.Password, *user.PasswordHash) {
		core.WriteError(w, http.StatusUnauthorized, "invalid_credentials", "Incorrect email or password.")
		return
	}
	if !user.IsActive {
		core.WriteError(w, http.StatusUnauthorized, "inactive", "Account is inactive.")
		return
	}

	now := time.Now().UTC()
	if user.DeletionRequestedAt != nil {
		if user.DeletionDeadline != nil && !now.Before(user.DeletionDeadline.UTC()) {
			_, _ = s.DB.Exec(ctx, nil, `DELETE FROM users WHERE id = ?`, s.DB.FormatUUID(user.ID))
			core.WriteError(w, http.StatusUnauthorized, "invalid_credentials", "Incorrect email or password.")
			return
		}
	}

	var tokens models.TokenPair
	err = s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := s.DB.Exec(
			ctx, tx,
			`UPDATE users SET deletion_requested_at = NULL, deletion_deadline = NULL, last_active_at = ?, updated_at = ? WHERE id = ?`,
			s.DB.FormatTime(now), s.DB.FormatTime(now), s.DB.FormatUUID(user.ID),
		)
		if err != nil {
			return err
		}
		tokens, err = s.issueTokens(ctx, tx, user.ID)
		return err
	})
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	core.WriteJSON(w, http.StatusOK, tokens)
}

func (s *Server) Refresh(w http.ResponseWriter, r *http.Request) {
	body, ok := DecodeBody[models.RefreshIn](w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	tokenHash := core.HashToken(*body.RefreshToken)

	var rawID, rawUID string
	var expVal, createdVal any
	var revokedVal any
	err := s.DB.QueryRow(
		ctx, nil,
		`SELECT id, user_id, expires_at, revoked, created_at FROM refresh_tokens WHERE token_hash = ?`,
		tokenHash,
	).Scan(&rawID, &rawUID, &expVal, &revokedVal, &createdVal)
	if err != nil {
		core.WriteError(w, http.StatusUnauthorized, "invalid_refresh", "Refresh session is invalid or expired.")
		return
	}
	rowID, err := database.ParseUUID(rawID)
	if err != nil {
		core.WriteError(w, http.StatusUnauthorized, "invalid_refresh", "Refresh session is invalid or expired.")
		return
	}
	userID, err := database.ParseUUID(rawUID)
	if err != nil {
		core.WriteError(w, http.StatusUnauthorized, "invalid_refresh", "Refresh session is invalid or expired.")
		return
	}
	expiresAt, _ := database.ParseTime(expVal)
	revoked := database.ParseBool(revokedVal)

	user, err := s.findUserByID(ctx, nil, userID)
	if err != nil || user == nil || !user.IsActive {
		core.WriteError(w, http.StatusUnauthorized, "unauthenticated", "User is not active.")
		return
	}
	now := time.Now().UTC()
	if user.DeletionRequestedAt != nil && user.DeletionDeadline != nil {
		if !now.Before(user.DeletionDeadline.UTC()) {
			_, _ = s.DB.Exec(ctx, nil, `DELETE FROM users WHERE id = ?`, s.DB.FormatUUID(user.ID))
			core.WriteError(w, http.StatusUnauthorized, "unauthenticated", "User is not active.")
			return
		}
	}
	if expiresAt.UTC().Before(now) {
		core.WriteError(w, http.StatusUnauthorized, "invalid_refresh", "Refresh session is invalid or expired.")
		return
	}

	if revoked {
		// Grace period for concurrent refresh requests (e.g. React StrictMode double-mount)
		var latestCreatedVal any
		err := s.DB.QueryRow(
			ctx, nil,
			`SELECT created_at FROM refresh_tokens WHERE user_id = ? AND revoked = ? ORDER BY created_at DESC LIMIT 1`,
			s.DB.FormatUUID(user.ID), false,
		).Scan(&latestCreatedVal)
		if err != nil {
			core.WriteError(w, http.StatusUnauthorized, "invalid_refresh", "Refresh session is invalid or expired.")
			return
		}
		latestCreated, _ := database.ParseTime(latestCreatedVal)
		if now.Sub(latestCreated.UTC()).Seconds() > 20 {
			core.WriteError(w, http.StatusUnauthorized, "invalid_refresh", "Refresh session is invalid or expired.")
			return
		}
		var tokens models.TokenPair
		err = s.DB.WithTx(ctx, func(tx *sql.Tx) error {
			if _, err := s.DB.Exec(ctx, tx, `UPDATE refresh_tokens SET revoked = ? WHERE user_id = ?`, true, s.DB.FormatUUID(user.ID)); err != nil {
				return err
			}
			var err error
			tokens, err = s.issueTokens(ctx, tx, user.ID)
			return err
		})
		if err != nil {
			core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
			return
		}
		core.WriteJSON(w, http.StatusOK, tokens)
		return
	}

	var tokens models.TokenPair
	err = s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := s.DB.Exec(ctx, tx, `UPDATE refresh_tokens SET revoked = ? WHERE id = ?`, true, s.DB.FormatUUID(rowID)); err != nil {
			return err
		}
		var err error
		tokens, err = s.issueTokens(ctx, tx, user.ID)
		return err
	})
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	core.WriteJSON(w, http.StatusOK, tokens)
}

func (s *Server) Logout(w http.ResponseWriter, r *http.Request) {
	body, ok := DecodeBody[models.RefreshIn](w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	tokenHash := core.HashToken(*body.RefreshToken)
	var rawUID string
	err := s.DB.QueryRow(ctx, nil, `SELECT user_id FROM refresh_tokens WHERE token_hash = ?`, tokenHash).Scan(&rawUID)
	if err == nil {
		if uid, err := database.ParseUUID(rawUID); err == nil {
			_, _ = s.DB.Exec(ctx, nil, `UPDATE refresh_tokens SET revoked = ? WHERE user_id = ?`, true, s.DB.FormatUUID(uid))
		}
	}
	core.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) GoogleAuth(w http.ResponseWriter, r *http.Request) {
	if appErr := s.Limiter.EnforceLimit(s.Cfg, r, "google", 8, 900); appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}
	body, ok := DecodeBody[models.GoogleIn](w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	info, gErr := services.VerifyGoogleToken(ctx, s.Cfg, *body.IDToken)
	if gErr != nil {
		if appErr, ok := gErr.(*core.AppError); ok {
			core.WriteAppError(w, appErr)
		} else {
			core.WriteError(w, http.StatusUnauthorized, "invalid_google_token", "Google token could not be verified.")
		}
		return
	}
	emailLower := strings.ToLower(info.Email)
	googleID := info.Sub
	name := info.Name
	if name == "" {
		name = strings.Split(emailLower, "@")[0]
	}
	tz := "UTC"
	if body.Timezone != nil && *body.Timezone != "" {
		tz = *body.Timezone
	}

	user, err := s.findUserByEmail(ctx, nil, emailLower)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	now := time.Now().UTC()

	if user != nil && user.DeletionRequestedAt != nil {
		if user.DeletionDeadline != nil && !now.Before(user.DeletionDeadline.UTC()) {
			_, _ = s.DB.Exec(ctx, nil, `DELETE FROM users WHERE id = ?`, s.DB.FormatUUID(user.ID))
			user = nil
		}
	}

	var targetUserID uuid.UUID
	if user == nil {
		targetUserID = uuid.New()
		_, err = s.DB.Exec(
			ctx, nil,
			`INSERT INTO users (id, email, password_hash, google_id, name, timezone, is_active, must_change_password, deletion_requested_at, deletion_deadline, last_active_at, created_at, updated_at)
			 VALUES (?, ?, NULL, ?, ?, ?, ?, ?, NULL, NULL, ?, ?, ?)`,
			s.DB.FormatUUID(targetUserID),
			emailLower,
			googleID,
			name,
			tz,
			true,
			false,
			s.DB.FormatTime(now),
			s.DB.FormatTime(now),
			s.DB.FormatTime(now),
		)
		if err != nil {
			core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
			return
		}
	} else {
		if !user.IsActive {
			core.WriteError(w, http.StatusUnauthorized, "inactive", "Account is inactive.")
			return
		}
		if user.GoogleID != nil && *user.GoogleID != googleID {
			core.WriteError(w, http.StatusConflict, "google_mismatch", "Account is linked to a different Google identity.")
			return
		}
		targetUserID = user.ID
		_, err = s.DB.Exec(
			ctx, nil,
			`UPDATE users SET google_id = ?, deletion_requested_at = NULL, deletion_deadline = NULL, last_active_at = ?, updated_at = ? WHERE id = ?`,
			googleID,
			s.DB.FormatTime(now),
			s.DB.FormatTime(now),
			s.DB.FormatUUID(user.ID),
		)
		if err != nil {
			core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
			return
		}
	}

	tokens, err := s.issueTokens(ctx, nil, targetUserID)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	core.WriteJSON(w, http.StatusOK, tokens)
}

func generateOTPCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func (s *Server) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	if appErr := s.Limiter.EnforceLimit(s.Cfg, r, "forgot", 5, 900); appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}
	body, ok := DecodeBody[models.ForgotPasswordIn](w, r)
	if !ok {
		return
	}
	emailLower := strings.ToLower(body.Email)
	if appErr := s.Limiter.EnforceLimitKey(s.Cfg, "forgot-email:"+emailLower, 5, 900, "Too many reset code requests for this email. Please wait 15 minutes."); appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}

	ctx := r.Context()
	user, err := s.findUserByEmail(ctx, nil, emailLower)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	now := time.Now().UTC()
	if user != nil && user.DeletionRequestedAt != nil && user.DeletionDeadline != nil {
		if !now.Before(user.DeletionDeadline.UTC()) {
			_, _ = s.DB.Exec(ctx, nil, `DELETE FROM users WHERE id = ?`, s.DB.FormatUUID(user.ID))
			user = nil
		}
	}
	if user == nil {
		core.WriteError(w, http.StatusNotFound, "email_not_registered", "No account found with this email address. Please register first.")
		return
	}
	if user.PasswordHash == nil {
		core.WriteError(w, http.StatusBadRequest, "password_unavailable", "This account uses Google Sign-In and does not have a password. Please sign in with Google.")
		return
	}

	code, err := generateOTPCode()
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}

	var sendAppErr *core.AppError
	err = s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := s.DB.Exec(
			ctx, tx,
			`UPDATE password_reset_tokens SET used = ? WHERE user_id = ? AND used = ?`,
			true, s.DB.FormatUUID(user.ID), false,
		)
		if err != nil {
			return err
		}
		_, err = s.DB.Exec(
			ctx, tx,
			`INSERT INTO password_reset_tokens (id, user_id, code_hash, expires_at, used, created_at)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			s.DB.FormatUUID(uuid.New()),
			s.DB.FormatUUID(user.ID),
			core.HashToken(code),
			s.DB.FormatTime(now.Add(15*time.Minute)),
			false,
			s.DB.FormatTime(now),
		)
		if err != nil {
			return err
		}
		if sendErr := services.SendPasswordResetEmail(s.Cfg, user.Email, code); sendErr != nil {
			if ae, ok := sendErr.(*core.AppError); ok {
				sendAppErr = ae
			} else {
				sendAppErr = core.NewAppError(http.StatusServiceUnavailable, "email_failed", "Could not send the reset code. Please try again later.")
			}
			return sendErr
		}
		return nil
	})
	if sendAppErr != nil {
		core.WriteAppError(w, sendAppErr)
		return
	}
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}

	resp := map[string]any{"ok": true, "message": "Reset code sent to your email."}
	if s.Cfg.IsTest() {
		resp["dev_code"] = code
	}
	core.WriteJSON(w, http.StatusOK, resp)
}

func (s *Server) VerifyResetCode(w http.ResponseWriter, r *http.Request) {
	if appErr := s.Limiter.EnforceLimit(s.Cfg, r, "verify-reset", 8, 900); appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}
	body, ok := DecodeBody[models.VerifyResetIn](w, r)
	if !ok {
		return
	}
	emailLower := strings.ToLower(body.Email)
	if appErr := s.Limiter.EnforceLimitKey(s.Cfg, "verify-reset-email:"+emailLower, 6, 900, "Too many code verification attempts. Please wait 15 minutes or request a new code."); appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}

	ctx := r.Context()
	user, err := s.findUserByEmail(ctx, nil, emailLower)
	if err != nil || user == nil {
		core.WriteError(w, http.StatusBadRequest, "invalid_reset", "Invalid or expired reset code.")
		return
	}
	now := time.Now().UTC()
	if user.DeletionRequestedAt != nil && user.DeletionDeadline != nil {
		if !now.Before(user.DeletionDeadline.UTC()) {
			_, _ = s.DB.Exec(ctx, nil, `DELETE FROM users WHERE id = ?`, s.DB.FormatUUID(user.ID))
			core.WriteError(w, http.StatusBadRequest, "invalid_reset", "Invalid or expired reset code.")
			return
		}
	}
	codeHash := core.HashToken(strings.TrimSpace(*body.Code))
	var expVal any
	err = s.DB.QueryRow(
		ctx, nil,
		`SELECT expires_at FROM password_reset_tokens WHERE user_id = ? AND code_hash = ? AND used = ? ORDER BY created_at DESC LIMIT 1`,
		s.DB.FormatUUID(user.ID), codeHash, false,
	).Scan(&expVal)
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, "invalid_reset", "Invalid or expired reset code.")
		return
	}
	expTime, _ := database.ParseTime(expVal)
	if expTime.UTC().Before(now) {
		core.WriteError(w, http.StatusBadRequest, "invalid_reset", "Invalid or expired reset code.")
		return
	}
	core.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) ResetPassword(w http.ResponseWriter, r *http.Request) {
	if appErr := s.Limiter.EnforceLimit(s.Cfg, r, "reset", 8, 900); appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}
	body, ok := DecodeBody[models.ResetPasswordIn](w, r)
	if !ok {
		return
	}
	emailLower := strings.ToLower(body.Email)
	if appErr := s.Limiter.EnforceLimitKey(s.Cfg, "reset-email:"+emailLower, 6, 900, "Too many password reset attempts. Please wait 15 minutes or request a new code."); appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}

	ctx := r.Context()
	user, err := s.findUserByEmail(ctx, nil, emailLower)
	if err != nil || user == nil {
		core.WriteError(w, http.StatusBadRequest, "invalid_reset", "Invalid or expired reset code.")
		return
	}
	now := time.Now().UTC()
	if user.DeletionRequestedAt != nil && user.DeletionDeadline != nil {
		if !now.Before(user.DeletionDeadline.UTC()) {
			_, _ = s.DB.Exec(ctx, nil, `DELETE FROM users WHERE id = ?`, s.DB.FormatUUID(user.ID))
			core.WriteError(w, http.StatusBadRequest, "invalid_reset", "Invalid or expired reset code.")
			return
		}
	}
	codeHash := core.HashToken(strings.TrimSpace(*body.Code))
	var expVal any
	err = s.DB.QueryRow(
		ctx, nil,
		`SELECT expires_at FROM password_reset_tokens WHERE user_id = ? AND code_hash = ? AND used = ? ORDER BY created_at DESC LIMIT 1`,
		s.DB.FormatUUID(user.ID), codeHash, false,
	).Scan(&expVal)
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, "invalid_reset", "Invalid or expired reset code.")
		return
	}
	expTime, _ := database.ParseTime(expVal)
	if expTime.UTC().Before(now) {
		core.WriteError(w, http.StatusBadRequest, "invalid_reset", "Invalid or expired reset code.")
		return
	}

	pwdHash, err := core.HashPassword(body.NewPassword)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}

	err = s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := s.DB.Exec(ctx, tx, `UPDATE password_reset_tokens SET used = ? WHERE user_id = ?`, true, s.DB.FormatUUID(user.ID)); err != nil {
			return err
		}
		if _, err := s.DB.Exec(
			ctx, tx,
			`UPDATE users SET password_hash = ?, must_change_password = ?, deletion_requested_at = NULL, deletion_deadline = NULL, updated_at = ? WHERE id = ?`,
			pwdHash, false, s.DB.FormatTime(now), s.DB.FormatUUID(user.ID),
		); err != nil {
			return err
		}
		if _, err := s.DB.Exec(ctx, tx, `UPDATE refresh_tokens SET revoked = ? WHERE user_id = ?`, true, s.DB.FormatUUID(user.ID)); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}

	core.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) ChangePassword(w http.ResponseWriter, r *http.Request, user *models.User) {
	body, ok := DecodeBody[models.ForcedPasswordIn](w, r)
	if !ok {
		return
	}
	if !user.MustChangePassword {
		core.WriteError(w, http.StatusForbidden, "not_required", "Password change is not required for this account.")
		return
	}
	if user.PasswordHash != nil && core.VerifyPassword(body.NewPassword, *user.PasswordHash) {
		core.WriteError(w, http.StatusBadRequest, "password_reused", "Choose a new password different from the temporary password.")
		return
	}
	pwdHash, err := core.HashPassword(body.NewPassword)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	now := time.Now().UTC()
	_, err = s.DB.Exec(
		r.Context(), nil,
		`UPDATE users SET password_hash = ?, must_change_password = ?, updated_at = ? WHERE id = ?`,
		pwdHash, false, s.DB.FormatTime(now), s.DB.FormatUUID(user.ID),
	)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	user.PasswordHash = &pwdHash
	user.MustChangePassword = false
	core.WriteJSON(w, http.StatusOK, middleware.SerializeUser(user))
}

func (s *Server) Me(w http.ResponseWriter, r *http.Request, user *models.User) {
	core.WriteJSON(w, http.StatusOK, middleware.SerializeUser(user))
}

func (s *Server) UpdateMe(w http.ResponseWriter, r *http.Request, user *models.User) {
	body, ok := DecodeBody[models.UserUpdateIn](w, r)
	if !ok {
		return
	}
	if body.Name != nil {
		user.Name = *body.Name
	}
	if body.Timezone != nil {
		user.Timezone = *body.Timezone
	}
	now := time.Now().UTC()
	_, err := s.DB.Exec(
		r.Context(), nil,
		`UPDATE users SET name = ?, timezone = ?, updated_at = ? WHERE id = ?`,
		user.Name, user.Timezone, s.DB.FormatTime(now), s.DB.FormatUUID(user.ID),
	)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	core.WriteJSON(w, http.StatusOK, middleware.SerializeUser(user))
}

func (s *Server) DeleteMe(w http.ResponseWriter, r *http.Request, user *models.User) {
	now := time.Now().UTC()
	deadline := now.Add(72 * time.Hour)
	_, err := s.DB.Exec(
		r.Context(), nil,
		`UPDATE users SET deletion_requested_at = ?, deletion_deadline = ?, updated_at = ? WHERE id = ?`,
		s.DB.FormatTime(now), s.DB.FormatTime(deadline), s.DB.FormatTime(now), s.DB.FormatUUID(user.ID),
	)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	core.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}
