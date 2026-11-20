package config

import (
	"bufio"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	_ "time/tzdata"
)

var unsupportedAsyncpgParams = map[string]bool{
	"pgbouncer":       true,
	"supa":            true,
	"options":         true,
	"channel_binding": true,
}

var weakSecrets = map[string]bool{
	"":                              true,
	"dev-access-secret-change-me":   true,
	"dev-refresh-secret-change-me":  true,
	"change-me-access-secret":       true,
	"change-me-refresh-secret":      true,
	"change-me-internal-secret":     true,
	"dev-internal-secret-change-me": true,
	"ci-access":                     true,
	"ci-refresh":                    true,
	"test-access-secret":            true,
}

type Settings struct {
	DatabaseURL               string
	JWTSecret                 string
	JWTRefreshSecret          string
	AccessTokenExpireMinutes  int
	RefreshTokenExpireDays    int
	GoogleClientID            string
	GoogleClientSecret        string
	AnthropicAPIKey           string
	GeminiAPIKey              string
	GeminiModel               string
	CORSOrigins               string
	SMTPHost                  string
	SMTPPort                  int
	SMTPUser                  string
	SMTPPassword              string
	SMTPFrom                  string
	Environment               string
	MaxPDFBytes               int64
	MaxPDFPages               int
	AdminEmail                string
	AdminAccessCode           string
	AdminSessionExpireMinutes int
	CleanupSecret             string
	CronSecret                string
	PythonBin                 string
	PythonBridgeScript        string
	PythonParserURL           string
	InternalParserSecret      string
	SupabaseURL               string
	SupabaseServiceRoleKey    string
	SupabaseStorageBucket     string
	PDFStorageDir             string
}

func Load() *Settings {
	loadDotEnvFiles()

	env := getEnv("ENVIRONMENT", "development")
	defaultDB := "sqlite+aiosqlite:///./progress_tracker.db"
	if strings.ToLower(strings.TrimSpace(env)) == "production" {
		defaultDB = ""
	}

	s := &Settings{
		DatabaseURL:               getEnv("DATABASE_URL", defaultDB),
		JWTSecret:                 getEnv("JWT_SECRET", "dev-access-secret-change-me"),
		JWTRefreshSecret:          getEnv("JWT_REFRESH_SECRET", "dev-refresh-secret-change-me"),
		AccessTokenExpireMinutes:  getEnvInt("ACCESS_TOKEN_EXPIRE_MINUTES", 15),
		RefreshTokenExpireDays:    getEnvInt("REFRESH_TOKEN_EXPIRE_DAYS", 30),
		GoogleClientID:            getEnv("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret:        getEnv("GOOGLE_CLIENT_SECRET", ""),
		AnthropicAPIKey:           getEnv("ANTHROPIC_API_KEY", ""),
		GeminiAPIKey:              getEnv("GEMINI_API_KEY", ""),
		GeminiModel:               getEnv("GEMINI_MODEL", "gemini-3.8-flash"),
		CORSOrigins:               getEnv("CORS_ORIGINS", "http://localhost:5173"),
		SMTPHost:                  getEnv("SMTP_HOST", ""),
		SMTPPort:                  getEnvInt("SMTP_PORT", 587),
		SMTPUser:                  getEnv("SMTP_USER", ""),
		SMTPPassword:              getEnv("SMTP_PASSWORD", ""),
		SMTPFrom:                  getEnv("SMTP_FROM", "noreply@example.com"),
		Environment:               env,
		MaxPDFBytes:               int64(getEnvInt("MAX_PDF_BYTES", 4718592)), // 4.5 MB (4,718,592 bytes)
		MaxPDFPages:               getEnvInt("MAX_PDF_PAGES", 200),
		AdminEmail:                getEnv("ADMIN_EMAIL", "riteeritee251@gmail.com"),
		AdminAccessCode:           getEnv("ADMIN_ACCESS_CODE", ""),
		AdminSessionExpireMinutes: getEnvInt("ADMIN_SESSION_EXPIRE_MINUTES", 30),
		CleanupSecret:             getEnv("CLEANUP_SECRET", ""),
		CronSecret:                getEnv("CRON_SECRET", ""),
		PythonBin:                 getEnv("PYTHON_BIN", ""),
		PythonBridgeScript:        getEnv("PYTHON_BRIDGE_SCRIPT", ""),
		PythonParserURL:           getEnv("PYTHON_PARSER_URL", ""),
		InternalParserSecret:      getEnv("INTERNAL_PARSER_SECRET", ""),
		SupabaseURL:               getEnv("SUPABASE_URL", ""),
		SupabaseServiceRoleKey:    getEnv("SUPABASE_SERVICE_ROLE_KEY", ""),
		SupabaseStorageBucket:     getEnv("SUPABASE_STORAGE_BUCKET", "pdf-imports"),
		PDFStorageDir:             getEnv("PDF_STORAGE_DIR", filepath.Join(os.TempDir(), "pt-pdf-imports")),
	}
	if strings.TrimSpace(s.DatabaseURL) == "" && !s.IsProduction() {
		s.DatabaseURL = "sqlite+aiosqlite:///./progress_tracker.db"
	}
	if s.PythonBin == "" {
		s.PythonBin = detectPythonBin()
	}
	if s.PythonBridgeScript == "" {
		s.PythonBridgeScript = detectBridgeScript()
	}
	return s
}

func (s *Settings) EffectiveCleanupSecret() string {
	if strings.TrimSpace(s.CleanupSecret) != "" {
		return strings.TrimSpace(s.CleanupSecret)
	}
	return strings.TrimSpace(s.CronSecret)
}

// EffectiveInternalSecret returns ONLY the dedicated INTERNAL_PARSER_SECRET.
// It never falls back to JWT_SECRET, CLEANUP_SECRET, or CRON_SECRET.
func (s *Settings) EffectiveInternalSecret() string {
	return strings.TrimSpace(s.InternalParserSecret)
}

// EffectivePythonParserURL returns the configured PYTHON_PARSER_URL or derives
// the internal Vercel serverless Python endpoint when deployed on Vercel.
func (s *Settings) EffectivePythonParserURL() string {
	if u := strings.TrimSpace(s.PythonParserURL); u != "" {
		return u
	}
	for _, envKey := range []string{"VERCEL_PROJECT_PRODUCTION_URL", "VERCEL_BRANCH_URL", "VERCEL_URL"} {
		if host := strings.TrimRight(strings.TrimSpace(os.Getenv(envKey)), "/"); host != "" {
			if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
				host = "https://" + host
			}
			return host + "/api/internal_pdf_parser"
		}
	}
	return ""
}

func (s *Settings) HasSupabaseStorage() bool {
	return strings.TrimSpace(s.SupabaseURL) != "" &&
		strings.TrimSpace(s.SupabaseServiceRoleKey) != "" &&
		strings.TrimSpace(s.SupabaseStorageBucket) != ""
}

func (s *Settings) HasPartialSupabaseStorage() bool {
	return strings.TrimSpace(s.SupabaseURL) != "" ||
		strings.TrimSpace(s.SupabaseServiceRoleKey) != ""
}

// AllowsLocalSignedPDFStorage returns true ONLY in local development and automated test
// environments. Production and Vercel serverless environments NEVER fall back to local_signed.
func (s *Settings) AllowsLocalSignedPDFStorage() bool {
	if s.IsProduction() {
		return false
	}
	if s.IsServerless() && !s.IsTest() {
		return false
	}
	return true
}

func (s *Settings) ValidateStorageConfig() error {
	if !s.IsProduction() {
		return nil
	}
	rawURL := strings.TrimSpace(s.SupabaseURL)
	if rawURL == "" {
		return errors.New("SUPABASE_URL must be set in production for private PDF storage.")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
		return errors.New("SUPABASE_URL must be a valid HTTP/HTTPS URL in production.")
	}
	if strings.TrimSpace(s.SupabaseServiceRoleKey) == "" {
		return errors.New("SUPABASE_SERVICE_ROLE_KEY must be set in production for private PDF storage.")
	}
	if strings.TrimSpace(s.SupabaseStorageBucket) == "" {
		return errors.New("SUPABASE_STORAGE_BUCKET must be set in production for private PDF storage.")
	}
	return nil
}

func (s *Settings) CORSOriginList() []string {
	var origins []string
	seen := make(map[string]bool)
	for _, part := range strings.Split(s.CORSOrigins, ",") {
		o := strings.TrimRight(strings.TrimSpace(part), "/")
		if o != "" && !seen[o] {
			origins = append(origins, o)
			seen[o] = true
		}
	}
	for _, envKey := range []string{"VERCEL_URL", "VERCEL_BRANCH_URL", "VERCEL_PROJECT_PRODUCTION_URL"} {
		val := strings.TrimRight(strings.TrimSpace(os.Getenv(envKey)), "/")
		if val != "" {
			u := val
			if !strings.HasPrefix(u, "http") {
				u = "https://" + u
			}
			if !seen[u] {
				origins = append(origins, u)
				seen[u] = true
			}
		}
	}
	return origins
}

func (s *Settings) CleanDatabaseURL() string {
	u := strings.TrimSpace(s.DatabaseURL)
	u = strings.Trim(u, `"'`)
	if u == "" && !s.IsProduction() {
		return "sqlite+aiosqlite:///./progress_tracker.db"
	}
	return u
}

func (s *Settings) IsSQLite() bool {
	return strings.HasPrefix(strings.ToLower(s.CleanDatabaseURL()), "sqlite")
}

func (s *Settings) IsPostgresURL() bool {
	u := strings.ToLower(s.CleanDatabaseURL())
	return strings.HasPrefix(u, "postgres://") ||
		strings.HasPrefix(u, "postgresql://") ||
		strings.HasPrefix(u, "postgresql+asyncpg://")
}

func (s *Settings) IsProduction() bool {
	return strings.ToLower(strings.TrimSpace(s.Environment)) == "production"
}

func (s *Settings) IsTest() bool {
	return strings.ToLower(strings.TrimSpace(s.Environment)) == "test"
}

func (s *Settings) IsServerless() bool {
	return os.Getenv("VERCEL") != "" ||
		os.Getenv("VERCEL_ENV") != "" ||
		os.Getenv("VERCEL_URL") != "" ||
		os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != ""
}

// AsyncDatabaseURL reproduces Python's async_database_url normalization exactly.
func (s *Settings) AsyncDatabaseURL() string {
	raw := s.CleanDatabaseURL()
	if strings.HasPrefix(raw, "postgres://") {
		raw = "postgresql+asyncpg://" + strings.TrimPrefix(raw, "postgres://")
	} else if strings.HasPrefix(raw, "postgresql://") && !strings.HasPrefix(raw, "postgresql+") {
		raw = "postgresql+asyncpg://" + strings.TrimPrefix(raw, "postgresql://")
	}
	if strings.HasPrefix(raw, "postgresql+asyncpg://") && strings.Contains(raw, "?") {
		qIdx := strings.Index(raw, "?")
		base := raw[:qIdx]
		queryStr := raw[qIdx+1:]
		frag := ""
		if fIdx := strings.Index(queryStr, "#"); fIdx != -1 {
			frag = queryStr[fIdx:]
			queryStr = queryStr[:fIdx]
		}
		var cleaned []string
		for _, pair := range strings.Split(queryStr, "&") {
			if pair == "" {
				continue
			}
			kv := strings.SplitN(pair, "=", 2)
			k, _ := url.QueryUnescape(kv[0])
			v := ""
			if len(kv) > 1 {
				v, _ = url.QueryUnescape(kv[1])
			}
			kLower := strings.ToLower(k)
			if unsupportedAsyncpgParams[kLower] {
				continue
			}
			if kLower == "sslmode" {
				cleaned = append(cleaned, "ssl="+url.QueryEscape(v))
			} else {
				cleaned = append(cleaned, url.QueryEscape(k)+"="+url.QueryEscape(v))
			}
		}
		if len(cleaned) > 0 {
			raw = base + "?" + strings.Join(cleaned, "&") + frag
		} else {
			raw = base + frag
		}
	}
	return raw
}

// PostgresDSN returns a pgx-compatible PostgreSQL connection string.
func (s *Settings) PostgresDSN() string {
	raw := s.CleanDatabaseURL()
	if strings.HasPrefix(raw, "postgresql+asyncpg://") {
		raw = "postgres://" + strings.TrimPrefix(raw, "postgresql+asyncpg://")
	} else if strings.HasPrefix(raw, "postgresql://") {
		raw = "postgres://" + strings.TrimPrefix(raw, "postgresql://")
	}
	if strings.Contains(raw, "?") {
		qIdx := strings.Index(raw, "?")
		base := raw[:qIdx]
		queryStr := raw[qIdx+1:]
		var cleaned []string
		for _, pair := range strings.Split(queryStr, "&") {
			if pair == "" {
				continue
			}
			kv := strings.SplitN(pair, "=", 2)
			k, _ := url.QueryUnescape(kv[0])
			v := ""
			if len(kv) > 1 {
				v, _ = url.QueryUnescape(kv[1])
			}
			kLower := strings.ToLower(k)
			if unsupportedAsyncpgParams[kLower] {
				continue
			}
			if kLower == "ssl" {
				cleaned = append(cleaned, "sslmode="+url.QueryEscape(v))
			} else {
				cleaned = append(cleaned, url.QueryEscape(k)+"="+url.QueryEscape(v))
			}
		}
		if len(cleaned) > 0 {
			raw = base + "?" + strings.Join(cleaned, "&")
		} else {
			raw = base
		}
	}
	return raw
}

func (s *Settings) ValidateDatabaseConfig() error {
	raw := strings.TrimSpace(strings.Trim(s.DatabaseURL, `"'`))
	if s.IsProduction() {
		if raw == "" {
			return errors.New("DATABASE_URL must be set in production.")
		}
		if strings.HasPrefix(strings.ToLower(raw), "sqlite") {
			return errors.New("SQLite is not permitted in production; DATABASE_URL must be a PostgreSQL connection string.")
		}
		if !s.IsPostgresURL() {
			return errors.New("DATABASE_URL in production must be a valid PostgreSQL URL (postgres:// or postgresql://).")
		}
		parsed, err := url.Parse(s.PostgresDSN())
		if err != nil || parsed.Host == "" || strings.TrimPrefix(parsed.Path, "/") == "" {
			return errors.New("DATABASE_URL in production is missing a valid PostgreSQL host or database name.")
		}
		return nil
	}
	if !s.IsSQLite() && !s.IsPostgresURL() {
		return errors.New("DATABASE_URL must be a PostgreSQL or SQLite connection string.")
	}
	return nil
}

func (s *Settings) AssertProductionSecrets() error {
	if !s.IsProduction() {
		return nil
	}
	if err := s.ValidateDatabaseConfig(); err != nil {
		return err
	}
	if s.HasPartialSupabaseStorage() {
		if err := s.ValidateStorageConfig(); err != nil {
			return err
		}
	}
	if weakSecrets[s.JWTSecret] || len(s.JWTSecret) < 32 {
		return errors.New("JWT_SECRET must be set to a unique value of at least 32 characters in production.")
	}
	if weakSecrets[s.JWTRefreshSecret] || len(s.JWTRefreshSecret) < 32 {
		return errors.New("JWT_REFRESH_SECRET must be set to a unique value of at least 32 characters in production.")
	}
	internalSec := s.EffectiveInternalSecret()
	if weakSecrets[internalSec] || len(internalSec) < 32 {
		return errors.New("INTERNAL_PARSER_SECRET must be set to a unique value of at least 32 characters in production.")
	}
	if internalSec == s.JWTSecret || internalSec == s.JWTRefreshSecret ||
		(s.EffectiveCleanupSecret() != "" && internalSec == s.EffectiveCleanupSecret()) {
		return errors.New("INTERNAL_PARSER_SECRET must be independent from JWT_SECRET, JWT_REFRESH_SECRET, CLEANUP_SECRET, and CRON_SECRET.")
	}
	return nil
}

func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	val, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(val) == "" {
		return fallback
	}
	n, err := strconv.Atoi(strings.TrimSpace(val))
	if err != nil {
		return fallback
	}
	return n
}

func loadDotEnvFiles() {
	_, file, _, ok := runtime.Caller(0)
	var candidates []string
	if ok {
		baseDir := filepath.Dir(filepath.Dir(filepath.Dir(file))) // backend-go/
		rootDir := filepath.Dir(baseDir)                          // repo root
		candidates = append(candidates,
			filepath.Join(rootDir, ".env"),
			filepath.Join(rootDir, "backend", ".env"),
			filepath.Join(baseDir, ".env"),
		)
	}
	candidates = append(candidates, ".env", filepath.Join("..", ".env"), filepath.Join("..", "backend", ".env"))

	for _, path := range candidates {
		loadEnvFile(path)
	}
}

func loadEnvFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		val = strings.Trim(val, `"'`)
		if _, exists := os.LookupEnv(key); !exists && val != "" {
			_ = os.Setenv(key, val)
		}
	}
}

func detectPythonBin() string {
	_, file, _, ok := runtime.Caller(0)
	var roots []string
	if ok {
		baseDir := filepath.Dir(filepath.Dir(filepath.Dir(file)))
		rootDir := filepath.Dir(baseDir)
		roots = append(roots, rootDir, baseDir)
	}
	if wd, err := os.Getwd(); err == nil {
		roots = append(roots, wd, filepath.Dir(wd), filepath.Dir(filepath.Dir(wd)))
	}
	relPaths := []string{
		filepath.Join(".venv", "Scripts", "python.exe"),
		filepath.Join(".venv", "bin", "python"),
		filepath.Join("backend", ".venv", "Scripts", "python.exe"),
		filepath.Join("backend", ".venv", "bin", "python"),
	}
	for _, r := range roots {
		for _, rel := range relPaths {
			cand := filepath.Join(r, rel)
			if st, err := os.Stat(cand); err == nil && !st.IsDir() {
				return cand
			}
		}
	}
	return "python"
}

func detectBridgeScript() string {
	_, file, _, ok := runtime.Caller(0)
	if ok {
		baseDir := filepath.Dir(filepath.Dir(filepath.Dir(file)))
		cand := filepath.Join(baseDir, "pybridge", "pdf_bridge.py")
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			return cand
		}
	}
	if wd, err := os.Getwd(); err == nil {
		for _, cand := range []string{
			filepath.Join(wd, "pybridge", "pdf_bridge.py"),
			filepath.Join(wd, "backend-go", "pybridge", "pdf_bridge.py"),
			filepath.Join(filepath.Dir(wd), "pybridge", "pdf_bridge.py"),
		} {
			if st, err := os.Stat(cand); err == nil && !st.IsDir() {
				return cand
			}
		}
	}
	return "pybridge/pdf_bridge.py"
}
