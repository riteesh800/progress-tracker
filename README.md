# Skill Progress Tracker

A full-stack web application that transforms messy syllabus PDFs into structured, trackable topic trees — with leaf-level progress tracking, notes, full-text search, learning streaks, and a real-time dashboard. Built for students and self-learners who want to answer at a glance: *What am I learning? What have I completed? What should I study next?*

---

## What It Does

### Core Workflow

```
Create Skill → Upload PDF → Parse → Preview / Edit → Confirm → Track Leaf Topics → Add Notes → Mark Progress → Search → Maintain Streak → View Dashboard
```

1. **PDF Import & Parsing** — Upload a university syllabus PDF (up to 10 MB / ~200 pages). A hybrid parser extracts structure using decimal-numbering regex as the primary pass, with Anthropic Claude AI as a fallback for low-confidence sections only. The proposed topic tree is presented in a Preview Editor where you can rename, delete, add, and reorder nodes before confirming.

2. **Topic Tree & Progress Tracking** — Every skill is organized as an arbitrarily deep tree of topics. Progress is always measured at the **leaf level** (topics with zero children get a checkbox), and parent progress is derived upward through the entire ancestor chain — never independently set. Completing a leaf fires real-time progress recalculation up to the skill root.

3. **Notes** — Attach plain-text notes to any topic. Notes are searchable and scoped per user.

4. **Full-Text Search** — PostgreSQL `tsvector` + GIN index across skill names, topic names, topic descriptions, and note content — always filtered by the authenticated user.

5. **Streak Engine** — Tracks daily learning streaks based on leaf-topic completions (timezone-aware, using the user's stored IANA timezone). A built-in grace rule means missing exactly one day won't break your streak — only two or more consecutive missed days trigger a reset.

6. **Dashboard** — Global stats (total skills, topics, completed count, overall progress %, current streak, longest streak), a recent activity feed across all action types, per-skill circular/donut progress charts, and a "Study Next" suggestion widget that surfaces the oldest incomplete leaf or the lowest-progress skill.

7. **Authentication** — Email + password registration with Argon2 hashing, JWT access/refresh tokens (15 min / 30 days, rotated and revocable), Google Sign-In with automatic account linking, and a forgot-password flow with time-limited single-use reset codes.

---

## Tech Stack

### Frontend

| Technology | Version | Why |
|---|---|---|
| **React** | 19 | Modern component model with concurrent features; strongly typed with TypeScript |
| **Vite** | 6 | Lightning-fast HMR and build times; native ES module dev server eliminates bundling overhead during development |
| **TypeScript** | 5.8 | Compile-time type safety catches bugs before they reach the browser; better IDE support and refactoring |
| **React Router** | 7 | Client-side SPA routing with nested layouts and URL parameter support |
| **TanStack Query** | 5 | Manages server-state (caching, background refetches, loading/error/empty states, mutation invalidation) so these aren't hand-rolled per component |

> **Why this frontend stack?** React 19 + Vite gives the fastest development loop with production-grade output. TanStack Query is critical — it handles every async data state (loading, success, error, empty, stale) declaratively, which directly satisfies the requirement that every page has explicit loading and empty states with no blank screens. TypeScript ensures the API layer contracts between frontend and backend stay in sync.

### Backend

| Technology | Version | Why |
|---|---|---|
| **Python** | 3.11+ | Async-native, excellent library ecosystem for PDF processing and AI integration |
| **FastAPI** | 0.115 | Async end-to-end with automatic OpenAPI docs, Pydantic validation on every endpoint, and dependency injection for auth |
| **SQLAlchemy** | 2.0 (async) | Mature async ORM with explicit query construction; supports both PostgreSQL (production) and SQLite (development/testing) |
| **Pydantic** | 2.11 | Input validation and serialization on every API endpoint — never trusts client-supplied data |
| **Alembic** | 1.15 | Database migration management for PostgreSQL schema changes |
| **Argon2-cffi** | 23.1 | Memory-hard password hashing (preferred over bcrypt for resistance to GPU attacks) |
| **PyJWT** | 2.10 | JWT token creation and verification for stateless authentication |
| **pypdf** | 5.4 | PDF text extraction for the import pipeline |
| **Anthropic** | 0.49 | Claude API client for AI-assisted PDF section parsing (low-confidence fallback only) |
| **slowapi** | 0.1.9 | Rate limiting (5 login attempts/15 min, 10 PDF uploads/hour, general per-user throttle) — in-memory, no Redis needed for v1 |
| **Ruff** | 0.11 | Fast Python linter for CI |
| **pytest + pytest-asyncio** | 8.3 / 0.26 | Async-aware test suite covering all 12+ build phases |

> **Why this backend stack?** FastAPI's async-native design pairs naturally with async PostgreSQL and Anthropic API calls — no thread-pool bottlenecks. SQLAlchemy 2.0's async mode means the entire request lifecycle is non-blocking. Pydantic v2 validates every request server-side, enforcing ownership, hierarchy rules, and data integrity at the API boundary, not just the frontend.

### Database

| Technology | Why |
|---|---|
| **PostgreSQL** (via Supabase) | Production database with full-text search (`tsvector` + GIN), transactional integrity for atomic operations (PDF confirm, cascade deletes, account deletion), and JSON column support for import job metadata |
| **SQLite + aiosqlite** | Local development and CI — zero-config, no Docker required. Tests use SQL `LIKE` as a fallback for full-text search |

> **Why dual databases?** PostgreSQL is required in production for full-text search, proper concurrent connections, and Supabase hosting. SQLite locally means contributors can run the full backend and test suite with zero infrastructure setup — just `pip install` and go.

### CI/CD

| Technology | Why |
|---|---|
| **GitHub Actions** | Automated pipeline on every push: backend (install → ruff lint → pytest) and frontend (install → typecheck → build) |

---

## Architecture Overview

```
frontend/                           backend/
├── src/                            ├── app/
│   ├── api/                        │   ├── api/
│   │   ├── client.ts               │   │   ├── deps.py          (auth dependency injection)
│   │   ├── auth.api.ts             │   │   └── routes/
│   │   ├── skills.api.ts           │   │       ├── auth.py
│   │   ├── topics.api.ts           │   │       ├── skills.py
│   │   ├── notes.api.ts            │   │       ├── topics.py
│   │   ├── pdf.api.ts              │   │       ├── notes.py
│   │   ├── search.api.ts           │   │       ├── search.py
│   │   ├── streak.api.ts           │   │       ├── dashboard.py
│   │   └── progress.api.ts         │   │       ├── pdf.py
│   ├── components/                 │   │       └── personal_notes.py
│   │   ├── TopicTree.tsx           │   ├── core/
│   │   ├── ProgressVisuals.tsx     │   │   ├── config.py        (pydantic-settings)
│   │   ├── ConfirmDialog.tsx       │   │   ├── security.py      (JWT + Argon2)
│   │   ├── NotesPanel.tsx          │   │   ├── exceptions.py
│   │   ├── PasswordField.tsx       │   │   ├── rate_limit.py
│   │   └── TimezoneSelect.tsx      │   │   └── logging.py
│   ├── pages/                      │   ├── models/
│   │   ├── DashboardPage.tsx       │   │   ├── __init__.py      (all SQLAlchemy models)
│   │   ├── SkillsPage.tsx          │   │   └── enums.py
│   │   ├── SkillDetailPage.tsx     │   ├── services/
│   │   ├── ImportPage.tsx          │   │   ├── progress.py      (leaf-based calculation)
│   │   ├── SearchPage.tsx          │   │   ├── pdf_parser.py    (hybrid: regex + AI)
│   │   ├── StreakPage.tsx          │   │   ├── activity.py      (streak computation)
│   │   ├── ActivityPage.tsx        │   │   └── email.py
│   │   ├── MakeNotePage.tsx        │   ├── db/
│   │   ├── LoginPage.tsx           │   │   ├── session.py       (async engine)
│   │   └── SettingsPage.tsx        │   │   └── base.py
│   ├── lib/                        │   ├── schemas.py           (Pydantic request/response)
│   │   ├── topicTree.ts            │   └── main.py              (app factory + middleware)
│   │   ├── activityText.ts         ├── alembic/                 (Postgres migrations)
│   │   └── skillColors.ts          ├── tests/                   (phase-by-phase test suite)
│   ├── App.tsx                     └── requirements.txt
│   ├── main.tsx
│   └── styles.css
├── vite.config.ts
├── vercel.json
└── package.json
```

The frontend API layer is organized into dedicated `*.api.ts` modules — never raw `fetch` calls in components. The backend follows a layered architecture: **routes** (HTTP handling) → **services** (business logic) → **models** (database) with Pydantic schemas validating every request/response boundary.

---

## Deployment

This application is designed for free-tier hosting across three services, each handling a specific layer:

### Supabase — Database (PostgreSQL)

Supabase provides a managed PostgreSQL instance with built-in connection pooling.

- **What it hosts:** The PostgreSQL database (users, skills, topics, notes, activity log, import jobs, refresh tokens).
- **Why Supabase:** Free-tier PostgreSQL with pgbouncer connection pooling out of the box. The pooled connection string (port `6543`, transaction-pooling mode) is critical — Supabase's free tier caps direct connections very low, and an async FastAPI backend on Render can exhaust them without pooling.
- **Setup:**
  1. Create a new Supabase project.
  2. Go to **Settings → Database → Connection string** and copy the **pooled** URI (port `6543`, not `5432`).
  3. Format it as: `postgresql+asyncpg://USER:PASSWORD@HOST:6543/postgres`
  4. This becomes the `DATABASE_URL` environment variable for the backend.
  5. Run Alembic migrations against this database: `cd backend && alembic upgrade head`

### Render — Backend API

Render hosts the FastAPI backend as a web service.

- **What it hosts:** The Python/FastAPI API server, serving all `/auth`, `/skills`, `/topics`, `/notes`, `/search`, `/streak`, `/dashboard`, and `/pdf` endpoints.
- **Why Render:** Free-tier web service with auto-deploy from GitHub. The `render.yaml` blueprint defines the service configuration.
- **Behavior:** Render's free tier sleeps after ~15 minutes of inactivity. First request after a sleep can take 30–50 seconds (cold start). The frontend handles this with an explicit "Waking up the server…" banner — not a blank screen.
- **Setup:**
  1. Connect your GitHub repo to Render.
  2. Create a **Web Service** with root directory set to `backend/`.
  3. Build command: `pip install -r requirements.txt`
  4. Start command: `uvicorn app.main:app --host 0.0.0.0 --port $PORT`
  5. Set these environment variables:
     - `DATABASE_URL` — the Supabase pooled connection string
     - `JWT_SECRET` — a unique string of at least 32 characters
     - `JWT_REFRESH_SECRET` — a different unique string
     - `CORS_ORIGINS` — your Vercel frontend URL (e.g., `https://your-app.vercel.app`)
     - `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` — for Google Sign-In (optional)
     - `ANTHROPIC_API_KEY` — for AI-assisted PDF parsing (optional; without it, low-confidence sections use a `needs_review` flag instead)
     - `ENVIRONMENT` — set to `production`

### Vercel — Frontend

Vercel hosts the React SPA (static build output).

- **What it hosts:** The compiled React + Vite frontend (`frontend/dist/`).
- **Why Vercel:** Zero-config static site hosting with automatic deploys from GitHub, global CDN, and SPA rewrite rules built in.
- **Setup:**
  1. Connect your GitHub repo to Vercel.
  2. Set the **Root Directory** to `frontend/`.
  3. Framework Preset: **Vite**.
  4. Build command: `npm run build` (runs `tsc --noEmit && vite build`).
  5. Output directory: `dist/`.
  6. Set environment variable:
     - `VITE_API_BASE` — your Render backend URL (e.g., `https://progress-tracker-api.onrender.com`)
     - `VITE_GOOGLE_CLIENT_ID` — same as the backend's Google Client ID (if using Google Sign-In)
  7. The `vercel.json` rewrites all routes to `index.html` for SPA client-side routing.

### Environment Variables Reference

See `.env.example` for the complete list:

| Variable | Required | Description |
|---|---|---|
| `DATABASE_URL` | Yes | Supabase pooled Postgres URI (`postgresql+asyncpg://...@...:6543/postgres`) |
| `JWT_SECRET` | Yes | Access token signing secret (≥32 chars in production) |
| `JWT_REFRESH_SECRET` | Yes | Refresh token signing secret |
| `GOOGLE_CLIENT_ID` | No | Google OAuth client ID |
| `GOOGLE_CLIENT_SECRET` | No | Google OAuth client secret |
| `ANTHROPIC_API_KEY` | No | Anthropic Claude API key for PDF AI fallback |
| `CORS_ORIGINS` | Yes | Comma-separated allowed origins (no wildcard in production) |
| `SMTP_HOST` / `SMTP_PORT` / `SMTP_USER` / `SMTP_PASSWORD` | No | SMTP config for forgot-password emails |
| `ENVIRONMENT` | Yes | `development` or `production` |
| `VITE_API_BASE` | Prod only | Backend URL for the frontend in production |
| `VITE_GOOGLE_CLIENT_ID` | No | Google Client ID for the frontend |

---

## Testing

The backend has a phase-by-phase test suite covering schema, health check, auth, CRUD, progress engine, notes, streaks, search, PDF parsing, and dashboard:

```bash
cd backend
pytest -q
```

CI runs on every push via GitHub Actions (`.github/workflows/ci.yml`): backend lint + tests, frontend typecheck + build.

---

## Run Locally

**Backend:** `cd backend && pip install -r requirements.txt && copy ..\.env.example .env && uvicorn app.main:app --reload --port 8000`  
**Frontend:** `cd frontend && npm install && npm run dev` — opens at `http://localhost:5173`, Vite proxies all API calls to the backend at port 8000.
