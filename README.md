# Skill Progress Tracker

Full-stack tracker: syllabus PDF → topic tree → leaf progress, notes, search, streaks, dashboard.

Authoritative spec: `skill_progress_tracker_build_spec.md`. Implementation choices: `DECISIONS.md`.

## Local run

### Backend

```bash
cd backend
python -m pip install -r requirements.txt
copy ..\.env.example .env   # Windows; or cp on Unix
uvicorn app.main:app --reload --port 8000
```

SQLite is the default for local/dev. For production use the Supabase **pooled** URL (`postgresql+asyncpg://...@...:6543/postgres`).

Apply Postgres migrations:

```bash
cd backend
alembic upgrade head
```

### Frontend

```bash
cd frontend
npm install
npm run dev
```

Vite proxies API paths to `http://localhost:8000`. For production set `VITE_API_BASE` to the Render URL.

## Tests

```bash
cd backend
pytest -q
```

## Deploy

- Database: Supabase Postgres (pooled port 6543)
- API: Render web service from `backend/` (`uvicorn app.main:app`)
- Web: Vercel from `frontend/`
- Env vars: see `.env.example` (`DATABASE_URL`, `JWT_SECRET`, `JWT_REFRESH_SECRET`, `GOOGLE_CLIENT_ID`, `ANTHROPIC_API_KEY`, `CORS_ORIGINS`)

## Phase checklist

1–12 backend modules are covered by `backend/tests/test_phase*.py`.  
13–14 frontend: React 19 + Vite + TanStack Query, dedicated `*.api.ts` modules, loading/empty/error states, leaf checkboxes / parent bars / skill donuts, up/down + move-to-parent (no drag-and-drop).  
15–16 auth ownership, rate limits, CORS, logging.  
17 CI in `.github/workflows/ci.yml`.
