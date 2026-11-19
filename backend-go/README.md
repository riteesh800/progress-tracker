# Skill Progress Tracker — Go Backend (`backend-go/`)

High-performance Go 1.27 migration of the Skill Progress Tracker backend (`backend/`), preserving **100% of the existing API contracts, database schema, security controls, user isolation, Argon2id password compatibility, and Python PDF parsing logic**.

---

## Directory Structure

```text
backend-go/
├── api/
│   ├── index.go                  # Vercel Go Serverless Function entrypoint
│   └── internal_pdf_parser.py    # Internal HMAC-authenticated Python PDF parser function for Vercel
├── cmd/
│   └── server/
│       └── main.go               # Standalone HTTP server entrypoint (:8000)
├── internal/
│   ├── config/
│   │   └── config.go             # Environment & .env loader, production secret assertions, DSN normalization
│   ├── core/
│   │   ├── admin.go              # Admin email check & constant-time access-code verification
│   │   ├── errors.go             # AppError & {"error": {"status", "code", "message"}} responses
│   │   ├── logging.go            # Structured logger with sensitive-key redaction
│   │   ├── ratelimit.go          # Per-IP and per-key sliding-window rate limiter
│   │   └── security.go           # Argon2id/Argon2i PHC verifier/hasher, HS256 JWT, SHA-256 token hashing
│   ├── database/
│   │   ├── db.go                 # PostgreSQL (pgx SimpleProtocol for Supabase PgBouncer) & SQLite driver
│   │   └── types.go              # Cross-dialect UUID, timestamp, date, boolean, and JSON scanners
│   ├── handlers/
│   │   ├── admin.go              # /admin/* and /admin/cron/cleanup handlers
│   │   ├── ai.go                 # /ai/generate-syllabus, /ai/refine-syllabus, /ai/confirm-syllabus
│   │   ├── auth.go               # /auth/* (register, login, refresh, logout, google, OTP, change-password, me)
│   │   ├── dashboard.go          # /dashboard, /dashboard/suggested-next, /streak, /streak/calendar, /activity
│   │   ├── helpers.go            # Ownership guards (OwnedSkill, OwnedTopic) & batch topic loaders
│   │   ├── notes.go              # /notes CRUD
│   │   ├── pdf.go                # /pdf/upload, /pdf/jobs/{id}, /pdf/jobs/{id}/tree, /pdf/jobs/{id}/confirm
│   │   ├── personal_notes.go     # /personal-notes CRUD + task checklist replacement
│   │   ├── router.go             # Go 1.22+ method-aware ServeMux + middleware chain
│   │   ├── search.go             # /search (PostgreSQL FTS + ILIKE fallback / SQLite LIKE)
│   │   ├── skills.go             # /skills CRUD, 3-day trash, restore, permanent purge, delete-impact
│   │   ├── timer.go              # /skills/{id}/timer GET, PUT, DELETE
│   │   └── topics.go             # /topics create, get, children, leaf-only completion
│   ├── middleware/
│   │   └── middleware.go         # StripAPIPrefix, GZip, CORS, ErrorRecoveryAndLogging, SecurityHeaders, Auth
│   ├── models/
│   │   ├── models.go             # Domain structs & enums matching SQLAlchemy models
│   │   └── schemas.go            # Request/response DTOs & strict validators matching Pydantic schemas
│   └── services/
│       ├── activity.go           # Timezone-aware activity_date calculation & activity_log persistence
│       ├── email.go              # SMTP STARTTLS (587) & implicit SSL (465) OTP email sender
│       ├── gemini.go             # Gemini REST client, model fallback, discovery, and JSON repair
│       ├── google.go             # Google OAuth ID token RS256 JWKS verifier
│       ├── pdf_bridge.go         # HMAC-SHA256 authenticated bridge to Python PDF parser
│       └── progress.go           # O(N) memoized ProgressEngine & 1-day grace period streak calculator
├── pybridge/
│   └── pdf_bridge.py             # Subprocess bridge invoking backend/app/services/pdf_parser.py
├── tests/
│   ├── ai_pdf_compat_benchmark_test.go
│   ├── auth_security_test.go
│   ├── notes_search_dashboard_test.go
│   ├── skills_topics_progress_test.go
│   └── test_helpers_test.go
├── go.mod
├── go.sum
├── requirements.txt              # Minimal Python dependencies (pypdf, anthropic) for PDF bridge
└── vercel.json                   # Vercel routing & daily 03:00 UTC cleanup cron configuration
```

---

## Running Locally

```powershell
cd backend-go
go run ./cmd/server
```

The server reads `../backend/.env` (or `.env`) automatically and listens on `http://localhost:8000` (or `$PORT`).

---

## Running Automated Tests & Benchmarks

```powershell
cd backend-go

# Run all unit, integration, security/isolation, PDF bridge, and existing DB compatibility tests
go test -v ./tests/...

# Run performance benchmarks
go test -run=NONE -bench="." -benchmem ./tests/...
```

