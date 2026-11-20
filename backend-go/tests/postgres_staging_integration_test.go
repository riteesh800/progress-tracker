package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"skill-progress-tracker/backend-go/internal/config"
	"skill-progress-tracker/backend-go/internal/core"
	"skill-progress-tracker/backend-go/internal/database"
	"skill-progress-tracker/backend-go/internal/handlers"
	"skill-progress-tracker/backend-go/internal/services"
)

func openStagingPostgresTestEnv(t *testing.T) *TestEnv {
	t.Helper()
	pgURL := os.Getenv("STAGING_POSTGRES_URL")
	if pgURL == "" {
		pgURL = "postgresql+asyncpg://postgres@localhost:5432/progress_tracker_go_staging?pgbouncer=true&sslmode=disable"
	}

	root := projectRootDir()
	pythonBin := filepath.Join(root, ".venv", "Scripts", "python.exe")
	bridgeScript := filepath.Join(root, "backend-go", "pybridge", "pdf_bridge.py")

	cfg := &config.Settings{
		DatabaseURL:               pgURL,
		JWTSecret:                 "staging-pg-jwt-secret-key-with-32-bytes-minimum!!",
		JWTRefreshSecret:          "staging-pg-refresh-secret-key-32-bytes-minimum!!",
		AccessTokenExpireMinutes:  30,
		RefreshTokenExpireDays:    30,
		GoogleClientID:            "test-google-client-id",
		GeminiAPIKey:              "test-gemini-api-key",
		GeminiModel:               "gemini-3.8-flash",
		CORSOrigins:               "http://localhost:5173",
		Environment:               "test",
		MaxPDFBytes:               10 * 1024 * 1024, // 10 MB
		MaxPDFPages:               200,
		AdminEmail:                core.AdminEmail,
		AdminAccessCode:           "staging-admin-secret-code-12345",
		AdminSessionExpireMinutes: 30,
		CleanupSecret:             "staging-cleanup-secret-1234567890",
		CronSecret:                "staging-cron-secret-1234567890",
		PythonBin:                 pythonBin,
		PythonBridgeScript:        bridgeScript,
		InternalParserSecret:      "staging-internal-parser-secret-32-chars-minimum!!",
		PDFStorageDir:             filepath.Join(t.TempDir(), "pg-pdf-storage"),
	}

	db, err := database.Open(cfg)
	if err != nil {
		t.Skipf("staging postgres open failed: %v", err)
	}
	if !db.IsPostgres() {
		_ = db.SQL.Close()
		t.Fatalf("expected Dialect=postgres, got %s", db.Dialect)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var versionStr string
	if err := db.QueryRow(ctx, nil, "SELECT version()").Scan(&versionStr); err != nil {
		_ = db.SQL.Close()
		t.Skipf("staging postgres not reachable: %v", err)
	}
	t.Logf("Connected to live staging PostgreSQL: %s", versionStr)

	// Truncate staging tables cleanly before the test run (never runs on production; database name verified)
	var currentDB string
	_ = db.QueryRow(ctx, nil, "SELECT current_database()").Scan(&currentDB)
	if currentDB != "progress_tracker_go_staging" {
		_ = db.SQL.Close()
		t.Fatalf("refusing to truncate non-staging database %q", currentDB)
	}
	_, err = db.Exec(ctx, nil, `
		TRUNCATE TABLE
			goal_activities,
			goals,
			skill_timers,
			password_reset_tokens,
			refresh_tokens,
			import_jobs,
			personal_note_tasks,
			personal_notes,
			activity_log,
			notes,
			topics,
			skills,
			users
		RESTART IDENTITY CASCADE
	`)
	if err != nil {
		_ = db.SQL.Close()
		t.Fatalf("truncate staging postgres tables failed: %v", err)
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

func TestLivePostgreSQL18StagingFullIntegration(t *testing.T) {
	env := openStagingPostgresTestEnv(t)

	// 1. Health check on PostgreSQL
	code, health, _ := env.DoJSON(http.MethodGet, "/health", nil, nil)
	if code != http.StatusOK || health["status"] != "ok" {
		t.Fatalf("PostgreSQL /health failed: %d %v", code, health)
	}

	// 2. Register two isolated staging accounts + Admin account on PostgreSQL
	tokenA, refreshA := env.RegisterUser("staging_alice@example.com", "AlicePass123!", "StagingAlice")
	tokenB, _ := env.RegisterUser("staging_bob@example.com", "BobPass123!", "StagingBob")
	adminToken, _ := env.RegisterUser(core.AdminEmail, "AdminPass123!", "StagingAdmin")

	// 3. Login & Refresh token rotation on PostgreSQL
	code, loginA, _ := env.DoJSON(http.MethodPost, "/auth/login", map[string]any{
		"email":    "staging_alice@example.com",
		"password": "AlicePass123!",
	}, nil)
	if code != http.StatusOK || loginA["access_token"] == "" {
		t.Fatalf("PostgreSQL login failed: %d %v", code, loginA)
	}
	code, refOut, _ := env.DoJSON(http.MethodPost, "/auth/refresh", map[string]any{
		"refresh_token": refreshA,
	}, nil)
	if code != http.StatusOK || refOut["refresh_token"] == "" {
		t.Fatalf("PostgreSQL refresh failed: %d %v", code, refOut)
	}

	// 4. Forgot password & OTP verification on PostgreSQL
	code, fpOut, _ := env.DoJSON(http.MethodPost, "/auth/forgot-password", map[string]any{
		"email": "staging_bob@example.com",
	}, nil)
	if code != http.StatusOK {
		t.Fatalf("PostgreSQL forgot-password failed: %d %v", code, fpOut)
	}
	otp, _ := fpOut["dev_code"].(string)
	code, _, _ = env.DoJSON(http.MethodPost, "/auth/verify-reset-code", map[string]any{
		"email": "staging_bob@example.com",
		"code":  otp,
	}, nil)
	if code != http.StatusOK {
		t.Fatalf("PostgreSQL verify-reset-code failed: %d", code)
	}

	// 5. Skills, Topics Hierarchy, Leaf Completion, Progress Rollup on PostgreSQL
	code, skillA, rawSkill := env.DoJSON(http.MethodPost, "/skills", map[string]any{
		"name":        "PostgreSQL 18 Architecture",
		"description": "UUID, JSON, MVCC, and PgBouncer",
	}, AuthHeader(tokenA))
	if code != http.StatusCreated {
		t.Fatalf("PostgreSQL create skill failed: %d %s", code, string(rawSkill))
	}
	skillAID, _ := skillA["id"].(string)

	// Per-user unique constraint on PostgreSQL (23505): Alice cannot duplicate, Bob can create same name
	code, dupErr, _ := env.DoJSON(http.MethodPost, "/skills", map[string]any{
		"name": "PostgreSQL 18 Architecture",
	}, AuthHeader(tokenA))
	if code != http.StatusConflict || dupErr["code"] != "skill_exists" {
		t.Fatalf("PostgreSQL expected 409 skill_exists, got %d %v", code, dupErr)
	}
	code, skillB, _ := env.DoJSON(http.MethodPost, "/skills", map[string]any{
		"name": "PostgreSQL 18 Architecture",
	}, AuthHeader(tokenB))
	if code != http.StatusCreated || skillB["id"] == "" {
		t.Fatalf("PostgreSQL Bob create same skill name failed: %d %v", code, skillB)
	}

	// Create root topic & child topics on PostgreSQL (testing NULL and non-NULL UUID parent_id in SimpleProtocol)
	code, rootTopic, rawTopic := env.DoJSON(http.MethodPost, "/topics", map[string]any{
		"skill_id": skillAID,
		"name":     "Storage Engine",
	}, AuthHeader(tokenA))
	if code != http.StatusCreated {
		t.Fatalf("PostgreSQL create root topic failed: %d %s", code, string(rawTopic))
	}
	rootTopicID, _ := rootTopic["id"].(string)

	code, child1, _ := env.DoJSON(http.MethodPost, "/topics", map[string]any{
		"skill_id":  skillAID,
		"parent_id": rootTopicID,
		"name":      "Heap Tuples & TOAST",
	}, AuthHeader(tokenA))
	if code != http.StatusCreated {
		t.Fatalf("PostgreSQL create child1 failed: %d %v", code, child1)
	}
	child1ID, _ := child1["id"].(string)

	code, child2, _ := env.DoJSON(http.MethodPost, "/topics", map[string]any{
		"skill_id":  skillAID,
		"parent_id": rootTopicID,
		"name":      "WAL & Checkpoints",
	}, AuthHeader(tokenA))
	if code != http.StatusCreated {
		t.Fatalf("PostgreSQL create child2 failed: %d %v", code, child2)
	}
	child2ID, _ := child2["id"].(string)

	// Complete child1 -> 50% progress
	code, comp1, rawComp1 := env.DoJSON(http.MethodPost, "/topics/"+child1ID+"/complete", map[string]any{
		"completed": true,
	}, AuthHeader(tokenA))
	if code != http.StatusOK || comp1["is_completed"] != true {
		t.Fatalf("PostgreSQL complete child1 failed: %d %s", code, string(rawComp1))
	}
	code, skillMid, _ := env.DoJSON(http.MethodGet, "/skills/"+skillAID, nil, AuthHeader(tokenA))
	if code != http.StatusOK || skillMid["percent"] != float64(50) {
		t.Fatalf("PostgreSQL expected 50%% skill progress, got %d %v", code, skillMid)
	}

	// Complete child2 -> 100% progress
	code, _, _ = env.DoJSON(http.MethodPost, "/topics/"+child2ID+"/complete", map[string]any{
		"completed": true,
	}, AuthHeader(tokenA))
	if code != http.StatusOK {
		t.Fatalf("PostgreSQL complete child2 failed: %d", code)
	}

	// 6. Skill Timer POST, GET, Invalid Date Range (400 invalid_dates), and DELETE on PostgreSQL (DATE columns)
	code, timerErr, _ := env.DoJSON(http.MethodPost, "/skills/"+skillAID+"/timer", map[string]any{
		"from_date": "2026-09-10",
		"to_date":   "2026-09-01",
	}, AuthHeader(tokenA))
	if code != http.StatusBadRequest || timerErr["code"] != "invalid_dates" {
		t.Fatalf("PostgreSQL timer invalid_dates expected 400, got %d %v", code, timerErr)
	}
	code, timerOut, rawTimer := env.DoJSON(http.MethodPost, "/skills/"+skillAID+"/timer", map[string]any{
		"from_date": "2026-09-01",
		"to_date":   "2026-09-30",
	}, AuthHeader(tokenA))
	if code != http.StatusOK || timerOut["from_date"] != "2026-09-01" || timerOut["to_date"] != "2026-09-30" {
		t.Fatalf("PostgreSQL POST timer failed: %d %s", code, string(rawTimer))
	}
	code, timerGet, _ := env.DoJSON(http.MethodGet, "/skills/"+skillAID+"/timer", nil, AuthHeader(tokenA))
	if code != http.StatusOK || timerGet["from_date"] != "2026-09-01" {
		t.Fatalf("PostgreSQL GET timer failed: %d %v", code, timerGet)
	}

	// 7. Topic Notes, Personal Notes & Tasks, and PostgreSQL ILIKE Search
	code, noteOut, rawNote := env.DoJSON(http.MethodPost, "/notes", map[string]any{
		"topic_id": child1ID,
		"content":  "TOAST compresses values exceeding 2KB in PostgreSQL.",
	}, AuthHeader(tokenA))
	if code != http.StatusCreated {
		t.Fatalf("PostgreSQL create note failed: %d %s", code, string(rawNote))
	}
	noteID, _ := noteOut["id"].(string)

	code, pNote, _ := env.DoJSON(http.MethodPost, "/personal-notes", map[string]any{
		"title": "PostgreSQL Tuning Checklist",
	}, AuthHeader(tokenA))
	if code != http.StatusCreated {
		t.Fatalf("PostgreSQL create personal note failed: %d %v", code, pNote)
	}
	pNoteID, _ := pNote["id"].(string)
	code, pPatched, _ := env.DoJSON(http.MethodPatch, "/personal-notes/"+pNoteID, map[string]any{
		"title": "PostgreSQL 18 Tuning Checklist",
		"tasks": []map[string]any{
			{"content": "Configure shared_buffers", "is_completed": true},
			{"content": "Enable PgBouncer transaction pooling", "is_completed": false},
		},
	}, AuthHeader(tokenA))
	if code != http.StatusOK || pPatched["title"] != "PostgreSQL 18 Tuning Checklist" {
		t.Fatalf("PostgreSQL patch personal note failed: %d %v", code, pPatched)
	}

	// PostgreSQL ILIKE search across skills, topics, and notes
	code, _, rawSearch := env.DoJSON(http.MethodGet, "/search?q=toast", nil, AuthHeader(tokenA))
	if code != http.StatusOK {
		t.Fatalf("PostgreSQL search failed: %d %s", code, string(rawSearch))
	}
	var searchHits []map[string]any
	_ = json.Unmarshal(rawSearch, &searchHits)
	if len(searchHits) < 2 {
		t.Fatalf("PostgreSQL expected >=2 search hits for 'toast' (topic + note), got %d: %v", len(searchHits), searchHits)
	}

	// 8. Dashboard, Streak, Calendar, Activity on PostgreSQL (DATE & TIMESTAMPTZ scanning)
	code, dash, rawDash := env.DoJSON(http.MethodGet, "/dashboard", nil, AuthHeader(tokenA))
	if code != http.StatusOK || dash["total_skills"] != float64(1) {
		t.Fatalf("PostgreSQL /dashboard failed: %d %s", code, string(rawDash))
	}
	code, streak, rawStreak := env.DoJSON(http.MethodGet, "/streak", nil, AuthHeader(tokenA))
	if code != http.StatusOK || streak["current_streak"] != float64(1) {
		t.Fatalf("PostgreSQL /streak failed: %d %s", code, string(rawStreak))
	}
	code, cal, rawCal := env.DoJSON(http.MethodGet, "/streak/calendar", nil, AuthHeader(tokenA))
	if code != http.StatusOK || cal["days"] == nil {
		t.Fatalf("PostgreSQL /streak/calendar failed: %d %s", code, string(rawCal))
	}
	code, _, rawAct := env.DoJSON(http.MethodGet, "/activity", nil, AuthHeader(tokenA))
	if code != http.StatusOK {
		t.Fatalf("PostgreSQL /activity failed: %d %s", code, string(rawAct))
	}

	// 9. Cross-User Security & Isolation on PostgreSQL (Bob cannot access Alice's Skills, Topics, Notes, Tasks, Timers, Activity, PDF import jobs, or Uploaded PDF objects)
	for _, check := range []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodGet, "/skills/" + skillAID, nil},
		{http.MethodPatch, "/skills/" + skillAID, map[string]any{"name": "Hijacked"}},
		{http.MethodDelete, "/skills/" + skillAID, nil},
		{http.MethodGet, "/skills/" + skillAID + "/topics", nil},
		{http.MethodGet, "/skills/" + skillAID + "/timer", nil},
		{http.MethodPost, "/skills/" + skillAID + "/timer", map[string]any{"from_date": "2026-09-01", "to_date": "2026-09-10"}},
		{http.MethodDelete, "/skills/" + skillAID + "/timer", nil},
		{http.MethodGet, "/topics/" + child1ID, nil},
		{http.MethodPost, "/topics/" + child1ID + "/complete", map[string]any{"completed": false}},
		{http.MethodGet, "/notes/" + noteID, nil},
		{http.MethodPatch, "/notes/" + noteID, map[string]any{"content": "Hijacked"}},
		{http.MethodDelete, "/notes/" + noteID, nil},
		{http.MethodGet, "/personal-notes/" + pNoteID, nil},
		{http.MethodPatch, "/personal-notes/" + pNoteID, map[string]any{"title": "Hijacked", "tasks": []map[string]any{{"content": "Stolen", "is_completed": true}}}},
		{http.MethodDelete, "/personal-notes/" + pNoteID, nil},
	} {
		st, _, _ := env.DoJSON(check.method, check.path, check.body, AuthHeader(tokenB))
		if st != http.StatusNotFound {
			t.Fatalf("PostgreSQL cross-user check %s %s expected 404, got %d", check.method, check.path, st)
		}
	}

	// Verify Bob's /activity and /search do not leak Alice's activity or notes/topics
	code, _, rawBobAct := env.DoJSON(http.MethodGet, "/activity", nil, AuthHeader(tokenB))
	if code != http.StatusOK || strings.Contains(string(rawBobAct), "Heap Tuples") {
		t.Fatalf("PostgreSQL Bob /activity leaked Alice's activity: %s", string(rawBobAct))
	}
	code, _, rawBobSearch := env.DoJSON(http.MethodGet, "/search?q=toast", nil, AuthHeader(tokenB))
	if code != http.StatusOK || strings.Contains(string(rawBobSearch), "TOAST") {
		t.Fatalf("PostgreSQL Bob /search leaked Alice's note/topic: %s", string(rawBobSearch))
	}

	// 10. 10 MB PDF Upload via Supabase Storage + Python Parser + PostgreSQL JSON Columns (import_jobs)
	var mu sync.Mutex
	objects := make(map[string][]byte)
	deletedKeys := make(map[string]bool)

	var supaServer *httptest.Server
	supaServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/storage/v1/object/upload/sign/"):
			key := strings.TrimPrefix(r.URL.Path, "/storage/v1/object/upload/sign/pdf-imports/")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"url": "/storage/v1/object/upload/sign/pdf-imports/" + key + "?token=pg-upload-token",
			})
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/storage/v1/object/upload/sign/"):
			key := strings.TrimPrefix(r.URL.Path, "/storage/v1/object/upload/sign/pdf-imports/")
			data, _ := io.ReadAll(r.Body)
			mu.Lock()
			objects[key] = data
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"Key":"` + key + `"}`))
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/storage/v1/object/sign/"):
			key := strings.TrimPrefix(r.URL.Path, "/storage/v1/object/sign/pdf-imports/")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"signedURL": supaServer.URL + "/storage/v1/object/sign/pdf-imports/" + key + "?token=pg-download-token",
			})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/storage/v1/object/sign/"):
			key := strings.TrimPrefix(r.URL.Path, "/storage/v1/object/sign/pdf-imports/")
			mu.Lock()
			data, ok := objects[key]
			mu.Unlock()
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/pdf")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/storage/v1/object/"):
			key := strings.TrimPrefix(r.URL.Path, "/storage/v1/object/pdf-imports/")
			mu.Lock()
			delete(objects, key)
			deletedKeys[key] = true
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer supaServer.Close()

	root := projectRootDir()
	apiDir := filepath.Join(root, "backend-go", "api")
	pyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		pyRunner := `
import io, os, sys
sys.path.insert(0, sys.argv[1])
import internal_pdf_parser

class CaptureHandler(internal_pdf_parser.handler):
    def __init__(self, headers_dict, body_bytes):
        self.headers = headers_dict
        self.rfile = io.BytesIO(body_bytes)
        self.wfile = io.BytesIO()
        self.status_code = 200
    def send_response(self, code, message=None):
        self.status_code = code
    def send_header(self, k, v):
        pass
    def end_headers(self):
        pass

raw_in = sys.stdin.buffer.read()
h = CaptureHandler({
    "Content-Type": os.environ.get("TEST_CT", ""),
    "Content-Length": str(len(raw_in)),
    "X-Internal-Bridge-Timestamp": os.environ.get("TEST_TS", ""),
    "X-Internal-Bridge-Signature": os.environ.get("TEST_SIG", ""),
}, raw_in)
h.do_POST()
sys.stdout.buffer.write(f"{h.status_code}\n".encode("utf-8") + h.wfile.getvalue())
`
		cmd := exec.Command(env.Cfg.PythonBin, "-c", pyRunner, apiDir)
		cmd.Stdin = bytes.NewReader(bodyBytes)
		cmd.Env = append(os.Environ(),
			"INTERNAL_PARSER_SECRET="+env.Cfg.EffectiveInternalSecret(),
			"SUPABASE_URL="+supaServer.URL,
			"MAX_PDF_BYTES="+strconv.FormatInt(env.Cfg.MaxPDFBytes, 10),
			"TEST_CT="+r.Header.Get("Content-Type"),
			"TEST_TS="+r.Header.Get("X-Internal-Bridge-Timestamp"),
			"TEST_SIG="+r.Header.Get("X-Internal-Bridge-Signature"),
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write(out)
			return
		}
		parts := bytes.SplitN(out, []byte("\n"), 2)
		stCode, _ := strconv.Atoi(strings.TrimSpace(string(parts[0])))
		if stCode == 0 {
			stCode = http.StatusOK
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(stCode)
		if len(parts) > 1 {
			_, _ = w.Write(parts[1])
		}
	}))
	defer pyServer.Close()

	env.Cfg.SupabaseURL = supaServer.URL
	env.Cfg.SupabaseServiceRoleKey = "staging-service-role-key"
	env.Cfg.SupabaseStorageBucket = "pdf-imports"
	env.Cfg.PythonParserURL = pyServer.URL

	pdfPayload := generateValidSyllabusPDFBytesWithSize(t, env, int(env.Cfg.MaxPDFBytes))
	code, urlOut, rawURLOut := env.DoJSON(http.MethodPost, "/pdf/upload-url", map[string]any{
		"filename":        "pg_4_5mb_syllabus.pdf",
		"content_type":    "application/pdf",
		"file_size_bytes": len(pdfPayload),
	}, AuthHeader(tokenA))
	if code != http.StatusOK {
		t.Fatalf("PostgreSQL /pdf/upload-url failed: %d %s", code, string(rawURLOut))
	}
	uploadURL, _ := urlOut["upload_url"].(string)
	objectKey, _ := urlOut["object_key"].(string)
	uploadToken, _ := urlOut["upload_token"].(string)

	putReq, _ := http.NewRequest(http.MethodPut, uploadURL, bytes.NewReader(pdfPayload))
	putReq.Header.Set("Content-Type", "application/pdf")
	putRes, err := http.DefaultClient.Do(putReq)
	if err != nil || putRes.StatusCode != http.StatusOK {
		t.Fatalf("PostgreSQL PUT failed: %v", err)
	}
	_ = putRes.Body.Close()

	code, jobOut, rawJob := env.DoJSON(http.MethodPost, "/pdf/upload-complete", map[string]any{
		"object_key":        objectKey,
		"original_filename": "pg_4_5mb_syllabus.pdf",
		"file_size_bytes":   len(pdfPayload),
		"upload_token":      uploadToken,
	}, AuthHeader(tokenA))
	if code != http.StatusOK || jobOut["status"] != "ready_for_preview" {
		t.Fatalf("PostgreSQL /pdf/upload-complete failed: %d %s", code, string(rawJob))
	}
	jobID, _ := jobOut["id"].(string)

	// Bob cannot view, patch, or confirm Alice's import job or complete Alice's storage object on PostgreSQL
	code, _, _ = env.DoJSON(http.MethodGet, "/pdf/jobs/"+jobID, nil, AuthHeader(tokenB))
	if code != http.StatusNotFound {
		t.Fatalf("PostgreSQL Bob GET Alice job expected 404, got %d", code)
	}
	code, _, _ = env.DoJSON(http.MethodPost, "/pdf/upload-complete", map[string]any{
		"object_key":        objectKey,
		"original_filename": "pg_4_5mb_syllabus.pdf",
		"file_size_bytes":   len(pdfPayload),
		"upload_token":      uploadToken,
	}, AuthHeader(tokenB))
	if code != http.StatusForbidden {
		t.Fatalf("PostgreSQL Bob /pdf/upload-complete on Alice object_key expected 403, got %d", code)
	}

	// Confirm import job on PostgreSQL (persists Skill, Topics, JSON columns, and ActivityLog in transaction)
	code, confOut, rawConf := env.DoJSON(http.MethodPost, "/pdf/jobs/"+jobID+"/confirm", map[string]any{
		"skill_name":        "Imported Networks 10MB on Postgres",
		"skill_description": "Verified on PostgreSQL 18.3",
	}, AuthHeader(tokenA))
	if code != http.StatusOK || confOut["status"] != "confirmed" {
		t.Fatalf("PostgreSQL confirm job failed: %d %s", code, string(rawConf))
	}
	importedSkillID, _ := confOut["skill_id"].(string)

	// 11. Skill Soft-Delete (Trash), Restore, and Permanent Purge on PostgreSQL (testing SET NULL on import_jobs & activity_log)
	code, _, rawDel := env.DoJSON(http.MethodDelete, "/skills/"+importedSkillID+"?confirm=true", nil, AuthHeader(tokenA))
	if code != http.StatusOK {
		t.Fatalf("PostgreSQL soft delete skill failed: %d %s", code, string(rawDel))
	}
	code, _, rawRest := env.DoJSON(http.MethodPost, "/skills/"+importedSkillID+"/restore", nil, AuthHeader(tokenA))
	if code != http.StatusOK {
		t.Fatalf("PostgreSQL restore skill failed: %d %s", code, string(rawRest))
	}
	code, _, _ = env.DoJSON(http.MethodDelete, "/skills/"+importedSkillID+"?confirm=true", nil, AuthHeader(tokenA))
	if code != http.StatusOK {
		t.Fatalf("PostgreSQL second soft delete failed: %d", code)
	}
	code, _, rawPurge := env.DoJSON(http.MethodDelete, "/skills/"+importedSkillID+"/permanent", nil, AuthHeader(tokenA))
	if code != http.StatusOK {
		t.Fatalf("PostgreSQL permanent purge failed: %d %s", code, string(rawPurge))
	}

	// Verify import_jobs.skill_id was set to NULL on PostgreSQL while the import_job row itself remains intact
	code, jobAfterPurge, _ := env.DoJSON(http.MethodGet, "/pdf/jobs/"+jobID, nil, AuthHeader(tokenA))
	if code != http.StatusOK || jobAfterPurge["skill_id"] != nil {
		t.Fatalf("PostgreSQL expected import_job.skill_id to be nil after skill purge, got %d %v", code, jobAfterPurge)
	}

	// 12. Admin Portal & Cron Cleanup on PostgreSQL
	code, unlockOut, _ := env.DoJSON(http.MethodPost, "/admin/unlock", map[string]any{
		"access_code": env.Cfg.AdminAccessCode,
	}, AuthHeader(adminToken))
	if code != http.StatusOK {
		t.Fatalf("PostgreSQL admin unlock failed: %d %v", code, unlockOut)
	}
	xAdmin, _ := unlockOut["admin_token"].(string)
	code, _, rawUsers := env.DoJSON(http.MethodGet, "/admin/users", nil, map[string]string{
		"Authorization":   "Bearer " + adminToken,
		"X-Admin-Session": xAdmin,
	})
	if code != http.StatusOK {
		t.Fatalf("PostgreSQL /admin/users failed: %d %s", code, string(rawUsers))
	}

	// 13. AI Syllabus Generation, Refinement & Confirmation on PostgreSQL
	origGemini := services.CallGeminiFunc
	t.Cleanup(func() { services.CallGeminiFunc = origGemini })
	services.CallGeminiFunc = func(_ context.Context, _ *config.Settings, _ string, _ float64) (string, error) {
		return `{"skill_name":"Postgres AI Skill","topics":[{"name":"MVCC","children":[{"name":"Vacuum"},{"name":"Visibility Map"}]}]}`, nil
	}
	code, aiGen, _ := env.DoJSON(http.MethodPost, "/ai/generate-syllabus", map[string]any{
		"subject": "Postgres Internals",
		"depth":   "medium",
		"purpose": "interview",
	}, AuthHeader(tokenA))
	if code != http.StatusOK {
		t.Fatalf("PostgreSQL /ai/generate-syllabus failed: %d %v", code, aiGen)
	}
	code, aiConf, rawAIConf := env.DoJSON(http.MethodPost, "/ai/confirm-syllabus", map[string]any{
		"skill_name":        "Postgres AI Skill",
		"skill_description": "Confirmed on PostgreSQL 18.3",
		"topics":            aiGen["topics"],
	}, AuthHeader(tokenA))
	if code != http.StatusCreated || aiConf["total_leaves"] != float64(2) {
		t.Fatalf("PostgreSQL /ai/confirm-syllabus failed: %d %s", code, string(rawAIConf))
	}

	// 14. Goals Full Lifecycle & Isolation on PostgreSQL
	futureDeadline := time.Now().Add(72 * time.Hour).Format(time.RFC3339)
	code, goalA, rawGoalA := env.DoJSON(http.MethodPost, "/goals", map[string]any{
		"goal_name":       "Master Distributed Storage on PostgreSQL",
		"deadline_at":     futureDeadline,
		"timezone":        "UTC",
		"motivation_line": "Consistency beats intensity every day.",
	}, AuthHeader(tokenA))
	if code != http.StatusCreated {
		t.Fatalf("PostgreSQL create goal failed: %d %s", code, string(rawGoalA))
	}
	goalAID, _ := goalA["id"].(string)

	// Add Activity to Goal on PostgreSQL
	code, actA, rawActA := env.DoJSON(http.MethodPost, "/goals/"+goalAID+"/activities", map[string]any{
		"activity_text": "Read Chapter 4 on WAL and Checkpoints",
	}, AuthHeader(tokenA))
	if code != http.StatusCreated {
		t.Fatalf("PostgreSQL add goal activity failed: %d %s", code, string(rawActA))
	}
	_ = actA

	// Verify Goal Progress on PostgreSQL
	code, progA, rawProgA := env.DoJSON(http.MethodGet, "/goals/"+goalAID+"/progress", nil, AuthHeader(tokenA))
	if code != http.StatusOK {
		t.Fatalf("PostgreSQL get goal progress failed: %d %s", code, string(rawProgA))
	}
	if currentScore, _ := progA["current_score"].(float64); currentScore != 1 {
		t.Fatalf("expected current_score 1 on PostgreSQL, got %v", progA["current_score"])
	}

	// Move Goal to Trash on PostgreSQL
	code, _, _ = env.DoJSON(http.MethodDelete, "/goals/"+goalAID, nil, AuthHeader(tokenA))
	if code != http.StatusOK {
		t.Fatalf("PostgreSQL delete goal to trash failed: %d", code)
	}

	// Restore Goal on PostgreSQL
	code, _, _ = env.DoJSON(http.MethodPost, "/goals/"+goalAID+"/restore", nil, AuthHeader(tokenA))
	if code != http.StatusOK {
		t.Fatalf("PostgreSQL restore goal failed: %d", code)
	}

	// Permanent Delete on PostgreSQL
	_, _, _ = env.DoJSON(http.MethodDelete, "/goals/"+goalAID, nil, AuthHeader(tokenA))
	code, _, _ = env.DoJSON(http.MethodDelete, "/goals/"+goalAID+"/permanent", nil, AuthHeader(tokenA))
	if code != http.StatusOK {
		t.Fatalf("PostgreSQL permanent purge goal failed: %d", code)
	}
}
