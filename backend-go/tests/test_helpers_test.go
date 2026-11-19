package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"testing"

	"skill-progress-tracker/backend-go/internal/config"
	"skill-progress-tracker/backend-go/internal/core"
	"skill-progress-tracker/backend-go/internal/database"
	"skill-progress-tracker/backend-go/internal/handlers"
)

type TestEnv struct {
	T       *testing.T
	Cfg     *config.Settings
	DB      *database.DB
	Limiter *core.RateLimiter
	Router  http.Handler
	Server  *httptest.Server
}

func projectRootDir() string {
	_, filename, _, _ := runtime.Caller(0)
	// filename is .../backend-go/tests/test_helpers_test.go
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func NewTestEnv(t *testing.T) *TestEnv {
	t.Helper()
	root := projectRootDir()
	tmpDB := filepath.Join(t.TempDir(), "test.db")

	pythonBin := filepath.Join(root, ".venv", "Scripts", "python.exe")
	bridgeScript := filepath.Join(root, "backend-go", "pybridge", "pdf_bridge.py")

	cfg := &config.Settings{
		DatabaseURL:               "sqlite:///" + filepath.ToSlash(tmpDB),
		JWTSecret:                 "test-jwt-secret-key-with-32-bytes-minimum!!",
		JWTRefreshSecret:          "test-refresh-secret-key-with-32-bytes-min!!",
		AccessTokenExpireMinutes:  30,
		RefreshTokenExpireDays:    30,
		GoogleClientID:            "test-google-client-id",
		GeminiAPIKey:              "test-gemini-api-key",
		GeminiModel:               "gemini-3.8-flash",
		CORSOrigins:               "http://localhost:5173",
		Environment:               "test",
		MaxPDFBytes:               10 * 1024 * 1024, // 10 MB
		MaxPDFPages:               80,
		AdminEmail:                core.AdminEmail,
		AdminAccessCode:           "test-admin-secret-code-12345",
		AdminSessionExpireMinutes: 30,
		CleanupSecret:             "test-cleanup-secret-1234567890",
		CronSecret:                "test-cron-secret-1234567890",
		PythonBin:                 pythonBin,
		PythonBridgeScript:        bridgeScript,
		InternalParserSecret:      "test-internal-parser-secret-1234567890-strong",
		PDFStorageDir:             filepath.Join(t.TempDir(), "pdf-storage"),
	}

	db, err := database.Open(cfg)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := db.ResetForTesting(context.Background(), cfg); err != nil {
		t.Fatalf("failed to reset test db: %v", err)
	}

	limiter := core.NewRateLimiter(cfg)
	limiter.SetEnabled(false)
	router := handlers.NewRouter(cfg, db, limiter)
	srv := httptest.NewServer(router)

	t.Cleanup(func() {
		srv.Close()
		_ = db.SQL.Close()
	})

	return &TestEnv{
		T:       t,
		Cfg:     cfg,
		DB:      db,
		Limiter: limiter,
		Router:  router,
		Server:  srv,
	}
}

func (e *TestEnv) DoJSON(method, path string, body any, headers map[string]string) (int, map[string]any, []byte) {
	e.T.Helper()
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			e.T.Fatalf("marshal body: %v", err)
		}
		bodyReader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	e.Router.ServeHTTP(rec, req)
	raw := rec.Body.Bytes()
	var parsed map[string]any
	_ = json.Unmarshal(raw, &parsed)
	if parsed != nil {
		if errObj, ok := parsed["error"].(map[string]any); ok {
			if _, hasCode := parsed["code"]; !hasCode {
				parsed["code"] = errObj["code"]
				parsed["message"] = errObj["message"]
			}
		}
	}
	return rec.Code, parsed, raw
}

func (e *TestEnv) DoMultipartPDF(path, filename string, fileBytes []byte, headers map[string]string) (int, map[string]any, []byte) {
	e.T.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		e.T.Fatalf("create form file: %v", err)
	}
	if _, err := fw.Write(fileBytes); err != nil {
		e.T.Fatalf("write form file: %v", err)
	}
	_ = mw.Close()

	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	e.Router.ServeHTTP(rec, req)
	raw := rec.Body.Bytes()
	var parsed map[string]any
	_ = json.Unmarshal(raw, &parsed)
	if parsed != nil {
		if errObj, ok := parsed["error"].(map[string]any); ok {
			if _, hasCode := parsed["code"]; !hasCode {
				parsed["code"] = errObj["code"]
				parsed["message"] = errObj["message"]
			}
		}
	}
	return rec.Code, parsed, raw
}

func (e *TestEnv) DoRawPut(path, contentType string, fileBytes []byte, headers map[string]string) (int, map[string]any, []byte) {
	e.T.Helper()
	req := httptest.NewRequest(http.MethodPut, path, bytes.NewReader(fileBytes))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	e.Router.ServeHTTP(rec, req)
	raw := rec.Body.Bytes()
	var parsed map[string]any
	_ = json.Unmarshal(raw, &parsed)
	if parsed != nil {
		if errObj, ok := parsed["error"].(map[string]any); ok {
			if _, hasCode := parsed["code"]; !hasCode {
				parsed["code"] = errObj["code"]
				parsed["message"] = errObj["message"]
			}
		}
	}
	return rec.Code, parsed, raw
}

func (e *TestEnv) RegisterUser(email, password, name string) (string, string) {
	e.T.Helper()
	code, data, raw := e.DoJSON(http.MethodPost, "/auth/register", map[string]any{
		"email":    email,
		"password": password,
		"name":     name,
		"timezone": "UTC",
	}, nil)
	if code != http.StatusOK {
		e.T.Fatalf("RegisterUser(%s) status=%d body=%s", email, code, string(raw))
	}
	access, _ := data["access_token"].(string)
	refresh, _ := data["refresh_token"].(string)
	return access, refresh
}

func AuthHeader(token string) map[string]string {
	return map[string]string{
		"Authorization": "Bearer " + token,
	}
}
