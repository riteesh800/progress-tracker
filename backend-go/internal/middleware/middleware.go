package middleware

import (
	"compress/gzip"
	"context"
	"database/sql"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"skill-progress-tracker/backend-go/internal/config"
	"skill-progress-tracker/backend-go/internal/core"
	"skill-progress-tracker/backend-go/internal/database"
	"skill-progress-tracker/backend-go/internal/models"
)

type contextKey string

const UserContextKey contextKey = "current_user"

var vercelOriginRegex = regexp.MustCompile(`^https://.*\.vercel\.app$`)

// StripAPIPrefix allows routes to be called with or without a leading '/api' prefix on Vercel.
func StripAPIPrefix(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/api" {
			r2 := new(http.Request)
			*r2 = *r
			u2 := *r.URL
			u2.Path = "/"
			r2.URL = &u2
			next.ServeHTTP(w, r2)
			return
		}
		if strings.HasPrefix(path, "/api/") {
			r2 := new(http.Request)
			*r2 = *r
			u2 := *r.URL
			u2.Path = path[4:]
			r2.URL = &u2
			next.ServeHTTP(w, r2)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// SecurityHeaders adds security headers matching SecurityHeadersMiddleware in main.py.
func SecurityHeaders(cfg *config.Settings, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		if h.Get("X-Content-Type-Options") == "" {
			h.Set("X-Content-Type-Options", "nosniff")
		}
		if h.Get("X-Frame-Options") == "" {
			h.Set("X-Frame-Options", "DENY")
		}
		if h.Get("Referrer-Policy") == "" {
			h.Set("Referrer-Policy", "no-referrer")
		}
		if h.Get("Permissions-Policy") == "" {
			h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		}
		if cfg.IsProduction() && h.Get("Strict-Transport-Security") == "" {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

// CORS handles CORS headers and preflight OPTIONS requests matching FastAPI CORSMiddleware in main.py.
func CORS(cfg *config.Settings, next http.Handler) http.Handler {
	origins := cfg.CORSOriginList()
	if cfg.IsProduction() {
		filtered := make([]string, 0, len(origins))
		for _, o := range origins {
			if o != "*" {
				filtered = append(filtered, o)
			}
		}
		origins = filtered
	}
	if len(origins) == 0 {
		origins = []string{"http://localhost:5173"}
	}
	allowedSet := make(map[string]bool, len(origins))
	for _, o := range origins {
		allowedSet[o] = true
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			allowed := allowedSet[origin] || allowedSet["*"] || vercelOriginRegex.MatchString(origin)
			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Add("Vary", "Origin")
			}
		}
		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			reqHeaders := r.Header.Get("Access-Control-Request-Headers")
			w.Header().Set("Access-Control-Allow-Methods", "DELETE, GET, HEAD, OPTIONS, PATCH, POST, PUT")
			if reqHeaders != "" {
				w.Header().Set("Access-Control-Allow-Headers", reqHeaders)
			} else {
				w.Header().Set("Access-Control-Allow-Headers", "*")
			}
			w.Header().Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type gzipResponseWriter struct {
	http.ResponseWriter
	gw          *gzip.Writer
	buf         []byte
	statusCode  int
	wroteHeader bool
	decided     bool
	useGzip     bool
}

func (g *gzipResponseWriter) WriteHeader(code int) {
	if g.wroteHeader {
		return
	}
	g.statusCode = code
	g.wroteHeader = true
}

func (g *gzipResponseWriter) Write(b []byte) (int, error) {
	if !g.wroteHeader {
		g.WriteHeader(http.StatusOK)
	}
	if !g.decided {
		g.buf = append(g.buf, b...)
		if len(g.buf) >= 500 {
			g.decide(true)
			_, err := g.gw.Write(g.buf)
			g.buf = nil
			return len(b), err
		}
		return len(b), nil
	}
	if g.useGzip {
		return g.gw.Write(b)
	}
	return g.ResponseWriter.Write(b)
}

func (g *gzipResponseWriter) decide(compress bool) {
	if g.decided {
		return
	}
	g.decided = true
	if compress && g.ResponseWriter.Header().Get("Content-Encoding") == "" {
		g.useGzip = true
		g.ResponseWriter.Header().Del("Content-Length")
		g.ResponseWriter.Header().Set("Content-Encoding", "gzip")
		g.ResponseWriter.Header().Add("Vary", "Accept-Encoding")
		g.ResponseWriter.WriteHeader(g.statusCode)
		g.gw = gzip.NewWriter(g.ResponseWriter)
	} else {
		g.useGzip = false
		g.ResponseWriter.WriteHeader(g.statusCode)
	}
}

func (g *gzipResponseWriter) Flush() error {
	if !g.wroteHeader {
		return nil
	}
	if !g.decided {
		g.decide(len(g.buf) >= 500)
		if len(g.buf) > 0 {
			if g.useGzip {
				if _, err := g.gw.Write(g.buf); err != nil {
					return err
				}
			} else {
				if _, err := g.ResponseWriter.Write(g.buf); err != nil {
					return err
				}
			}
			g.buf = nil
		}
	}
	if g.useGzip && g.gw != nil {
		return g.gw.Close()
	}
	return nil
}

// GZip compresses responses >= 500 bytes when the client accepts gzip, matching GZipMiddleware(minimum_size=500).
func GZip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}
		grw := &gzipResponseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}
		defer func() {
			_ = grw.Flush()
		}()
		next.ServeHTTP(grw, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.statusCode = code
	s.ResponseWriter.WriteHeader(code)
}

// ErrorRecoveryAndLogging recovers from panics and logs >= 500 errors matching ErrorHandlingMiddleware & request_log in main.py.
func ErrorRecoveryAndLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		defer func() {
			if err := recover(); err != nil {
				core.Logger.Error("Unhandled error on request", "path", r.URL.Path, "error", err)
				core.WriteError(rec, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
			}
			if rec.statusCode >= 500 {
				core.Logger.Error("Server error", "method", r.Method, "path", r.URL.Path, "status", rec.statusCode)
			}
		}()
		next.ServeHTTP(rec, r)
	})
}

func isMeaningfulActivity(r *http.Request) bool {
	path := strings.TrimRight(r.URL.Path, "/")
	if path == "/health" || path == "/auth/refresh" || path == "/auth/logout" {
		return false
	}
	if r.Method == http.MethodDelete && strings.HasSuffix(path, "/auth/me") {
		return false
	}
	if r.Method == http.MethodGet {
		return false
	}
	return true
}

func passwordChangeAllowed(r *http.Request) bool {
	path := strings.TrimRight(r.URL.Path, "/")
	if r.Method == http.MethodGet && (path == "/auth/me" || strings.HasSuffix(path, "/auth/me")) {
		return true
	}
	if r.Method == http.MethodPost && (path == "/auth/change-password" || strings.HasSuffix(path, "/auth/change-password")) {
		return true
	}
	return false
}

// ScanUser scans a row into models.User using the standard column order:
// id, email, password_hash, google_id, name, timezone, is_active, must_change_password, deletion_requested_at, deletion_deadline, last_active_at, created_at, updated_at
func ScanUser(scanner interface{ Scan(dest ...any) error }) (*models.User, error) {
	var rawID, email, name, tz string
	var pwdHash, googleID, delReq, delDead, lastActive, created, updated any
	var isActive, mustChange any
	err := scanner.Scan(
		&rawID, &email, &pwdHash, &googleID, &name, &tz,
		&isActive, &mustChange, &delReq, &delDead, &lastActive,
		&created, &updated,
	)
	if err != nil {
		return nil, err
	}
	id, err := database.ParseUUID(rawID)
	if err != nil {
		return nil, err
	}
	delReqTime, _ := database.ParseNullTime(delReq)
	delDeadTime, _ := database.ParseNullTime(delDead)
	lastActiveTime, _ := database.ParseNullTime(lastActive)
	createdTime, _ := database.ParseTime(created)
	updatedTime, _ := database.ParseTime(updated)
	return &models.User{
		ID:                  id,
		Email:               email,
		PasswordHash:        database.ParseNullString(pwdHash),
		GoogleID:            database.ParseNullString(googleID),
		Name:                name,
		Timezone:            tz,
		IsActive:            database.ParseBool(isActive),
		MustChangePassword:  database.ParseBool(mustChange),
		DeletionRequestedAt: delReqTime,
		DeletionDeadline:    delDeadTime,
		LastActiveAt:        lastActiveTime,
		CreatedAt:           createdTime,
		UpdatedAt:           updatedTime,
	}, nil
}

const UserColumns = `id, email, password_hash, google_id, name, timezone, is_active, must_change_password, deletion_requested_at, deletion_deadline, last_active_at, created_at, updated_at`

// Authenticate validates the Bearer JWT access token, loads the user, enforces 72h deletion expiry,
// enforces must_change_password, and updates last_active_at / cancels pending deletion on meaningful activity.
func Authenticate(ctx context.Context, cfg *config.Settings, db *database.DB, r *http.Request) (*models.User, *core.AppError) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" || !strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		return nil, core.NewAppError(http.StatusUnauthorized, "unauthenticated", "Missing or invalid authorization header.")
	}
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
		return nil, core.NewAppError(http.StatusUnauthorized, "unauthenticated", "Missing or invalid authorization header.")
	}
	token := parts[1]

	payload, err := core.DecodeAccessToken(cfg, token)
	if err != nil {
		if err == core.ErrTokenExpired {
			return nil, core.NewAppError(http.StatusUnauthorized, "token_expired", "Access token expired.")
		}
		return nil, core.NewAppError(http.StatusUnauthorized, "invalid_token", "Invalid access token.")
	}
	tokenType, _ := payload["type"].(string)
	if tokenType != "access" {
		return nil, core.NewAppError(http.StatusUnauthorized, "invalid_token", "Invalid access token.")
	}
	subStr, _ := payload["sub"].(string)
	uid, err := uuid.Parse(subStr)
	if err != nil {
		return nil, core.NewAppError(http.StatusUnauthorized, "invalid_token", "Invalid access token.")
	}

	row := db.QueryRow(ctx, nil, `SELECT `+UserColumns+` FROM users WHERE id = ?`, db.FormatUUID(uid))
	user, err := ScanUser(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, core.NewAppError(http.StatusUnauthorized, "unauthenticated", "User not found or inactive.")
		}
		return nil, core.NewAppError(http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
	}
	if user == nil || !user.IsActive {
		return nil, core.NewAppError(http.StatusUnauthorized, "unauthenticated", "User not found or inactive.")
	}

	// Check if deletion deadline has expired
	if user.DeletionRequestedAt != nil && user.DeletionDeadline != nil {
		now := time.Now().UTC()
		deadline := user.DeletionDeadline.UTC()
		if !now.Before(deadline) {
			_, _ = db.Exec(ctx, nil, `DELETE FROM users WHERE id = ?`, db.FormatUUID(user.ID))
			return nil, core.NewAppError(http.StatusUnauthorized, "unauthenticated", "User not found or inactive.")
		}
	}

	if user.MustChangePassword && !passwordChangeAllowed(r) {
		return nil, core.NewAppError(http.StatusForbidden, "password_change_required", "You must set a new password before continuing.")
	}

	// Track meaningful user activity and cancel pending deletion if applicable
	if isMeaningfulActivity(r) {
		now := time.Now().UTC()
		wasPending := user.DeletionRequestedAt != nil
		if wasPending {
			user.DeletionRequestedAt = nil
			user.DeletionDeadline = nil
			user.LastActiveAt = &now
			user.UpdatedAt = now
			_, _ = db.Exec(
				ctx, nil,
				`UPDATE users SET deletion_requested_at = NULL, deletion_deadline = NULL, last_active_at = ?, updated_at = ? WHERE id = ?`,
				db.FormatTime(now), db.FormatTime(now), db.FormatUUID(user.ID),
			)
		} else {
			user.LastActiveAt = &now
			user.UpdatedAt = now
			_, _ = db.Exec(
				ctx, nil,
				`UPDATE users SET last_active_at = ?, updated_at = ? WHERE id = ?`,
				db.FormatTime(now), db.FormatTime(now), db.FormatUUID(user.ID),
			)
		}
	}

	return user, nil
}

// RequireAuth wraps a handler that needs an authenticated *models.User.
func RequireAuth(cfg *config.Settings, db *database.DB, next func(http.ResponseWriter, *http.Request, *models.User)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, appErr := Authenticate(r.Context(), cfg, db, r)
		if appErr != nil {
			core.WriteAppError(w, appErr)
			return
		}
		ctx := context.WithValue(r.Context(), UserContextKey, user)
		next(w, r.WithContext(ctx), user)
	}
}

// RequireAdminAccount ensures the authenticated user's email matches the admin email.
func RequireAdminAccount(cfg *config.Settings, db *database.DB, next func(http.ResponseWriter, *http.Request, *models.User)) http.HandlerFunc {
	return RequireAuth(cfg, db, func(w http.ResponseWriter, r *http.Request, user *models.User) {
		if !core.IsAdminEmail(user.Email) {
			core.WriteError(w, http.StatusForbidden, "forbidden", "Admin is not available for this account.")
			return
		}
		next(w, r, user)
	})
}

// RequireVerifiedAdmin ensures the user is the admin account AND provides a valid X-Admin-Session JWT.
func RequireVerifiedAdmin(cfg *config.Settings, db *database.DB, next func(http.ResponseWriter, *http.Request, *models.User)) http.HandlerFunc {
	return RequireAdminAccount(cfg, db, func(w http.ResponseWriter, r *http.Request, user *models.User) {
		adminToken := r.Header.Get("X-Admin-Session")
		if adminToken == "" {
			core.WriteError(w, http.StatusUnauthorized, "admin_locked", "Database Access Code verification is required.")
			return
		}
		payload, err := core.DecodeAccessToken(cfg, adminToken)
		if err != nil {
			if err == core.ErrTokenExpired {
				core.WriteError(w, http.StatusUnauthorized, "admin_locked", "Admin session expired. Enter the Database Access Code again.")
				return
			}
			core.WriteError(w, http.StatusUnauthorized, "admin_locked", "Admin session is invalid.")
			return
		}
		tType, _ := payload["type"].(string)
		sub, _ := payload["sub"].(string)
		if tType != "admin" || sub != user.ID.String() {
			core.WriteError(w, http.StatusUnauthorized, "admin_locked", "Admin session is invalid.")
			return
		}
		next(w, r, user)
	})
}

// SerializeUser converts *models.User to models.UserOut matching deps.serialize_user.
func SerializeUser(user *models.User) models.UserOut {
	return models.UserOut{
		ID:                 user.ID,
		Email:              user.Email,
		Name:               user.Name,
		Timezone:           user.Timezone,
		IsActive:           user.IsActive,
		CanOpenAdmin:       core.IsAdminEmail(user.Email),
		MustChangePassword: user.MustChangePassword,
	}
}

// ReadBodyLimited reads at most maxBytes from r.Body.
func ReadBodyLimited(r *http.Request, maxBytes int64) ([]byte, error) {
	defer r.Body.Close()
	return io.ReadAll(io.LimitReader(r.Body, maxBytes+1))
}
