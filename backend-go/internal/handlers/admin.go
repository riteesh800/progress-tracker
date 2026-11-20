package handlers

import (
	"context"
	"crypto/hmac"
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"skill-progress-tracker/backend-go/internal/core"
	"skill-progress-tracker/backend-go/internal/database"
	"skill-progress-tracker/backend-go/internal/middleware"
	"skill-progress-tracker/backend-go/internal/models"
)

func userStatus(user *models.User, now time.Time) string {
	if user.DeletionRequestedAt != nil {
		return "Deleted"
	}
	activeCutoff := now.Add(-7 * 24 * time.Hour)
	if user.LastActiveAt != nil && !user.LastActiveAt.UTC().Before(activeCutoff) {
		return "Active"
	}
	if !user.CreatedAt.UTC().Before(activeCutoff) {
		return "Active"
	}
	return "Not active"
}

func (s *Server) runAccountCleanup(ctx context.Context) (int, error) {
	now := time.Now().UTC()
	rows, err := s.DB.Query(
		ctx, nil,
		`SELECT id, deletion_deadline FROM users WHERE deletion_requested_at IS NOT NULL AND deletion_deadline IS NOT NULL`,
	)
	if err != nil {
		return 0, err
	}
	var expiredIDs []uuid.UUID
	for rows.Next() {
		var rawID string
		var rawDeadline any
		if err := rows.Scan(&rawID, &rawDeadline); err != nil {
			continue
		}
		id, err := database.ParseUUID(rawID)
		if err != nil {
			continue
		}
		dl, err := database.ParseNullTime(rawDeadline)
		if err != nil || dl == nil {
			continue
		}
		if !now.Before(dl.UTC()) {
			expiredIDs = append(expiredIDs, id)
		}
	}
	rows.Close()

	deletedCount := 0
	for _, uid := range expiredIDs {
		res, err := s.DB.Exec(ctx, nil, `DELETE FROM users WHERE id = ?`, s.DB.FormatUUID(uid))
		if err == nil {
			if n, err := res.RowsAffected(); err == nil && n > 0 {
				deletedCount += int(n)
			}
		}
	}
	return deletedCount, nil
}

func (s *Server) runSkillCleanup(ctx context.Context) (int, error) {
	now := time.Now().UTC()
	rows, err := s.DB.Query(
		ctx, nil,
		`SELECT id, purge_at FROM skills WHERE deleted_at IS NOT NULL AND purge_at IS NOT NULL`,
	)
	if err != nil {
		return 0, err
	}
	var expiredIDs []uuid.UUID
	for rows.Next() {
		var rawID string
		var rawPurge any
		if err := rows.Scan(&rawID, &rawPurge); err != nil {
			continue
		}
		id, err := database.ParseUUID(rawID)
		if err != nil {
			continue
		}
		pa, err := database.ParseNullTime(rawPurge)
		if err != nil || pa == nil {
			continue
		}
		if !now.Before(pa.UTC()) {
			expiredIDs = append(expiredIDs, id)
		}
	}
	rows.Close()

	purgedCount := 0
	for _, sid := range expiredIDs {
		err := s.DB.WithTx(ctx, func(tx *sql.Tx) error {
			return s.purgeSkillRecord(ctx, tx, sid)
		})
		if err == nil {
			purgedCount++
		}
	}
	return purgedCount, nil
}

func (s *Server) runGoalCleanup(ctx context.Context) (int, error) {
	now := time.Now().UTC()
	rows, err := s.DB.Query(
		ctx, nil,
		`SELECT id, purge_at FROM goals WHERE deleted_at IS NOT NULL AND purge_at IS NOT NULL`,
	)
	if err != nil {
		return 0, err
	}
	var expiredIDs []uuid.UUID
	for rows.Next() {
		var rawID string
		var rawPurge any
		if err := rows.Scan(&rawID, &rawPurge); err != nil {
			continue
		}
		id, err := database.ParseUUID(rawID)
		if err != nil {
			continue
		}
		pa, err := database.ParseNullTime(rawPurge)
		if err != nil || pa == nil {
			continue
		}
		if !now.Before(pa.UTC()) {
			expiredIDs = append(expiredIDs, id)
		}
	}
	rows.Close()

	purgedCount := 0
	for _, gid := range expiredIDs {
		err := s.DB.WithTx(ctx, func(tx *sql.Tx) error {
			return s.PurgeGoalRecord(ctx, tx, gid)
		})
		if err == nil {
			purgedCount++
		}
	}
	return purgedCount, nil
}

func (s *Server) UnlockAdmin(w http.ResponseWriter, r *http.Request, user *models.User) {
	body, ok := DecodeBody[models.AdminUnlockIn](w, r)
	if !ok {
		return
	}
	if appErr := s.Limiter.EnforceLimitKey(s.Cfg, "admin_unlock:"+user.ID.String(), 5, 900, "Too many access code attempts. Try again in 15 minutes."); appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}
	if !core.AdminAccessConfigured(s.Cfg) {
		core.WriteError(w, http.StatusServiceUnavailable, "admin_unconfigured", "Database Access Code is not configured on the server.")
		return
	}
	if !core.VerifyAdminAccessCode(s.Cfg, body.AccessCode) {
		core.WriteError(w, http.StatusForbidden, "invalid_access_code", "Invalid Database Access Code.")
		return
	}
	expMin := s.Cfg.AdminSessionExpireMinutes
	token, err := core.CreateAccessToken(
		s.Cfg,
		user.ID.String(),
		map[string]any{"type": "admin"},
		&expMin,
	)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	core.WriteJSON(w, http.StatusOK, models.AdminUnlockOut{
		AdminToken: token,
		ExpiresIn:  s.Cfg.AdminSessionExpireMinutes * 60,
	})
}

func (s *Server) ListUsers(w http.ResponseWriter, r *http.Request, _ *models.User) {
	now := time.Now().UTC()
	rows, err := s.DB.Query(
		r.Context(), nil,
		`SELECT `+middleware.UserColumns+` FROM users ORDER BY created_at ASC, email ASC`,
	)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	defer rows.Close()

	out := make([]models.AdminUserOut, 0)
	for rows.Next() {
		u, err := middleware.ScanUser(rows)
		if err != nil {
			core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
			return
		}
		name := u.Name
		if name == "" {
			name = strings.Split(u.Email, "@")[0]
		}
		out = append(out, models.AdminUserOut{
			ID:     u.ID,
			Name:   name,
			Email:  u.Email,
			Status: userStatus(u, now),
		})
	}
	core.WriteJSON(w, http.StatusOK, out)
}

func (s *Server) AdminResetUserPassword(w http.ResponseWriter, r *http.Request, _ *models.User) {
	targetID, ok := ParsePathUUID(w, r.PathValue("user_id"))
	if !ok {
		return
	}
	body, ok := DecodeBody[models.AdminPasswordIn](w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	target, err := s.findUserByID(ctx, nil, targetID)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	if target == nil {
		core.WriteError(w, http.StatusNotFound, "not_found", "User not found.")
		return
	}
	pwdHash, err := core.HashPassword(body.NewPassword)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	now := time.Now().UTC()
	err = s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := s.DB.Exec(
			ctx, tx,
			`UPDATE users SET password_hash = ?, must_change_password = ?, updated_at = ? WHERE id = ?`,
			pwdHash, true, s.DB.FormatTime(now), s.DB.FormatUUID(target.ID),
		); err != nil {
			return err
		}
		_, err := s.DB.Exec(
			ctx, tx,
			`UPDATE refresh_tokens SET revoked = ? WHERE user_id = ?`,
			true, s.DB.FormatUUID(target.ID),
		)
		return err
	})
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	core.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) CleanupExpiredAccounts(w http.ResponseWriter, r *http.Request, _ *models.User) {
	count, err := s.runAccountCleanup(r.Context())
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	core.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted_count": count})
}

func (s *Server) CronCleanupExpiredAccounts(w http.ResponseWriter, r *http.Request) {
	expected := s.Cfg.EffectiveCleanupSecret()
	if len(expected) < 16 {
		core.WriteError(w, http.StatusServiceUnavailable, "cleanup_unconfigured", "Cleanup secret is not configured.")
		return
	}
	auth := r.Header.Get("Authorization")
	if auth == "" || !strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		core.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Missing cleanup authorization.")
		return
	}
	parts := strings.SplitN(auth, " ", 2)
	provided := ""
	if len(parts) == 2 {
		provided = strings.TrimSpace(parts[1])
	}
	if !hmac.Equal([]byte(provided), []byte(expected)) {
		core.WriteError(w, http.StatusForbidden, "forbidden", "Invalid cleanup secret.")
		return
	}
	accountsDeleted, err := s.runAccountCleanup(r.Context())
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	skillsPurged, err := s.runSkillCleanup(r.Context())
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	goalsPurged, err := s.runGoalCleanup(r.Context())
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	core.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":               true,
		"accounts_deleted": accountsDeleted,
		"skills_purged":    skillsPurged,
		"goals_purged":     goalsPurged,
	})
}
