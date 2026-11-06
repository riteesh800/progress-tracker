# Skill Progress Tracker

A production-grade, full-stack learning platform that converts syllabi, curriculum PDFs, and study topics into interactive, multi-level hierarchical progress trees. Featuring Google Gemini AI curriculum generation, leaf-based progress rollups, in-context topic notes, personal scratchpad checklists, target deadline countdown timers, GitHub-style consistency heatmaps, full-text instant search, and a superuser administration portal.

Both the **React 19 frontend** and **FastAPI async backend** are deployed together on **Vercel** as a unified serverless monorepo, backed by a **Supabase PostgreSQL** database with native **Vercel Cron** automation.

---

## Architecture & Deployment Model

```
                                  ┌───────────────────────────────────────────────┐
                                  │                 VERCEL EDGE                   │
                                  │                                               │
                                  │   ┌───────────────────────────────────────┐   │
                                  │   │     React 19 SPA (Vite / TS)          │   │
                                  │   │     Static Build: frontend/dist/      │   │
                                  │   └───────────────────────────────────────┘   │
                                  │                       │                       │
                                  │      Rewrite /api/*   │   SPA Route /*        │
                                  │                       ▼                       │
                                  │   ┌───────────────────────────────────────┐   │
                                  │   │  FastAPI Serverless (api/index.py)    │   │
                                  │   │  • Python 3.11 + Async Engine         │   │
                                  │   │  • Pydantic v2 + SlowAPI Throttling   │   │
                                  │   │  • Max Duration: 60s                  │   │
                                  │   └───────────────────────────────────────┘   │
                                  │                       │                       │
                                  │   ┌───────────────────────────────────────┐   │
                                  │   │  Vercel Cron (0 0 * * *)              │   │
                                  │   │  Hits /api/admin/cron/cleanup daily   │   │
                                  │   └───────────────────────────────────────┘   │
                                  └───────────────────────┬───────────────────────┘
                                                          │
                                     Pooled TLS (Port 6543)
                                                          │
                                                          ▼
                                  ┌───────────────────────────────────────────────┐
                                  │            SUPABASE POSTGRESQL                │
                                  │                                               │
                                  │  • Full-Text Search (tsvector + GIN indexes)  │
                                  │  • NullPool connection pooling for serverless │
                                  │  • ON DELETE CASCADE relational integrity     │
                                  │  • JSONB storage for syllabus import trees    │
                                  └───────────────────────────────────────────────┘
```

- **Single Unified Vercel Project**: The entire application (frontend + serverless backend) is deployed from a single Git repository. `vercel.json` configures the static Vite build (`frontend/dist`), routes API requests `/api/*` to `api/index.py`, and maps client-side routing to `/index.html`.
- **Zero External Cron Dependencies**: Scheduled tasks (purging expired trash skills and cleaning up expired account deletions) are handled natively by **Vercel Cron** invoking `/api/admin/cron/cleanup` every day at midnight UTC (`0 0 * * *`), authenticated via Bearer `CRON_SECRET`.
- **Serverless-Optimized Database Connections**: SQLAlchemy's async engine is configured with `NullPool` in serverless environments, disabling persistent pool connections and statement caches to prevent connection exhaustion against Supabase's transaction pooler (port `6543`).

---

## Key Features & Capabilities

### 1. Automated AI Syllabus Generator (Google Gemini)
- **Instant Curriculum Design**: Enter any topic (e.g., "Python & Backend Architecture", "Distributed Systems", "Data Structures & Algorithms") to generate a complete topic hierarchy in seconds.
- **4 Granular Depth Tiers**:
  - **Easy (Beginner)**: ~10–20 topics focusing on definitions, basic terminology, and core introductory concepts.
  - **Medium (Core)**: ~20–40 topics covering fundamentals, commonly used patterns, and key mechanisms.
  - **Hard / Intermediate (In-Depth)**: ~40–70 topics covering deep technical knowledge, practical implementations, and sub-subtopics.
  - **Comprehensive (A-to-Z)**: ~65–110 topics delivering complete A-to-Z coverage from fundamentals to advanced production patterns.
- **3 Purpose-Driven Modes**:
  - **Technical Interview Preparation**: Prioritizes frequently tested interview questions, system design problems, and edge cases.
  - **Quiz / Exam Revision**: Emphasizes definitions, classifications, conceptual properties, and examinable fundamentals.
  - **A-to-Z Mastery**: Structured logical learning sequence covering the entire domain from first principles.
- **Serverless Timeout Guard**: Direct REST calls to Google Gemini (`gemini-3.8-flash`) with strict 46-second generation budgets and JSON self-repair to prevent Vercel 60s gateway timeouts.

### 2. Interactive AI Studio & Conversational Refinement
- **Conversational Refinement Chat**: Dialogue directly with Gemini to adapt the syllabus before saving (e.g., *"Add a section on Kafka and gRPC"*, *"Expand topic 2.1"*, *"Remove advanced calculus"*).
- **Two-Stage Candidate Validation**: AI-generated modifications appear as pending proposals with distinct **Accept** and **Discard** controls.
- **Full Undo History**: Revert any AI refinement step to restore previous syllabus structures.
- **Interactive Tree Preview Editor**: Rename nodes inline, delete unwanted branches, or add custom child subtopics manually before saving.
- **Session Persistence**: Draft syllabi and chat logs are cached in `sessionStorage` (`pt_ai_draft`) to survive browser refreshes.

### 3. PDF Syllabus Ingestion & Parsing
- **Document Processing**: Upload university or training syllabus PDFs up to 10 MB and ~200 pages.
- **Dual-Engine Extraction**:
  - **Regex Engine**: Primary parser identifying numerical hierarchies (`1.0`, `1.1`, `1.1.1`), standard section headers (Unit, Chapter, Module), Roman numerals, and bullet structures.
  - **Anthropic Claude AI Fallback**: Low-confidence text chunks are selectively analyzed by Claude 3.5 Haiku to infer missing parent-child relationships.
- **Asynchronous Status Polling**: Job processing tracks states (`uploaded` → `extracting` → `parsing` → `ready_for_preview` → `confirmed` / `failed`).
- **External AI Prompt Templates**: Built-in copyable prompt templates allow users to format external ChatGPT or Claude outputs into compatible JSON outlines.

### 4. Hierarchical Topic Tree & Leaf Progress Engine
- **Arbitrary Depth**: Create skills with deeply nested modules, chapters, and sub-concepts without depth constraints.
- **Strict Leaf-Level Progress**: Checkboxes only appear on **leaf topics** (topics with zero children). Progress rolls up mathematically:
  $$\text{Progress \%} = \frac{\text{Completed Leaf Topics}}{\text{Total Leaf Topics}} \times 100$$
- **Automatic State Invalidation**: If a completed leaf topic receives a child subtopic, its completed status is automatically cleared and parent progress recalculates upward.
- **LineBar & Donut Visuals**: Real-time progress meters on branches and skill cards display leaf counts and percentages.

### 5. Target Timeline & Goal Countdown Timer
- **Goal Pacing**: Set target study schedules for any skill by specifying a start date (`from_date`) and target completion date (`to_date`).
- **Dynamic Timeline Gauge**: Displays elapsed days against total duration (e.g., `42 / 90 days`).
- **Human-Friendly Countdown Readouts**: Real-time calculated text such as `"1 month 14 days remaining"`, `"Goal completed"`, or `"Timer will start from YYYY-MM-DD"` for future target dates.

### 6. In-Context Topic Notes
- **Integrated Drawer**: A slide-out notes drawer accessible directly from the topic tree without navigating away from the curriculum.
- **Markdown & Revision Logs**: Attach notes, code snippets, and explanations to specific topics.
- **Full Audit Trail**: Notes log `note_added` and `note_updated` activities with contextual topic and skill snapshots.

### 7. Personal Scratchpad & Checklist ("Make Note")
- **Independent Task Notebook**: A standalone daily scratchpad (`/make-note`) completely isolated from long-term skills and streaks.
- **Interactive Checklists**: Create notebook cards with custom titles (up to 35 characters) and checklist tasks with one-click strike-through completion.
- **Debounced Auto-Save**: Changes persist automatically to the backend after a 450ms debounce with instant visual feedback.

### 8. Consistency Heatmap & Streak Calendar
- **GitHub-Style Contribution Grid**: 365-day rolling activity matrix visualizing daily learning completions across 4 color-intensity tiers.
- **Multi-Year Archive**: Toggle between the current 12-month rolling window and past calendar years.
- **Grace Rule Engine**: Streaks are timezone-aware. Missing exactly 1 day preserves your streak; missing 2 or more consecutive days resets the counter to 0.
- **Hover Tooltips**: Inspect precise calendar dates, day-of-week labels, and completion tallies.

### 9. Unified Full-Text Global Search
- **Multi-Entity Indexing**: PostgreSQL `tsvector` + GIN index (with SQLite fallback) searching across skill names, topic names, descriptions, topic notes, and personal scratchpad notes.
- **Result Badges & Deep Links**: Categorized search badges (`skill`, `topic`, `note`, `personal_note`) with highlighted text snippets and `"Open in tree"` direct navigation links.
- **Trash-Aware**: Automatically filters out skills currently in the trash bin.

### 10. Trash Bin & Safe Deletion Buffer
- **Soft Deletion Quarantine**: Deleting a skill places it into the Trash bin (`/trash`) with a 3-day recovery window.
- **Expiration Countdown**: Live countdown display (`"Permanently deleted in X days Y hours"`).
- **One-Click Restoration**: Restore skills, topics, and notes intact.
- **Permanent Hard Delete**: Immediate purge with impact warning dialogs showing exact counts of topics and notes being erased.

### 11. Profile Management & Visual Progress Bars
- **Display Name & Timezones**: Edit user display names and configure IANA timezones (50+ global timezones supported) for accurate streak calculation.
- **Per-Skill Progress Bars**: Color-coded progress bars with distinct pastel hues for every active skill.
- **Milestone Celebration**: Displays congratulatory banners upon reaching 100% completion on any skill.
- **Account Deletion with 72-Hour Grace Period**: Requesting account deletion initiates a 72-hour recovery window. Logging in within 72 hours automatically aborts the deletion; otherwise, all data is permanently purged via cascade.

### 12. Security & Authentication Architecture
- **Password Security**: Argon2id memory-hard password hashing (resistant to GPU/ASIC brute-force attacks).
- **JWT Token Pair**: 15-minute access tokens and 30-day rotated refresh tokens with a 20-second race-condition buffer for concurrent client calls.
- **Forgot Password Workflow**: 6-digit email verification codes (15-minute expiry, 60-second resend cooldown) delivered via SMTP.
- **Google OAuth 2.0**: One-click Google Sign-In with automatic account linking and profile creation.
- **Hardened HTTP Headers**: Strict security headers applied to every response (`X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`, `Permissions-Policy`, and HSTS in production).
- **Rate Limiting**: SlowAPI protection against credential brute-forcing (5 login attempts / 15 min, 8 register attempts / 15 min, 40 AI syllabus generations / hour).

### 13. Superuser Database Administration Console
- **Passkey-Protected Portal**: Access to `/admin` requires both an authorized administrator email and a secure 12+ character Database Access Code (`ADMIN_ACCESS_CODE`).
- **Auto-Lock Security**: Admin session automatically locks (clearing cached tokens) on page unload, route change, tab switch, or window blur.
- **User Directory**: Audit all registered accounts with computed activity statuses:
  - `Active`: Logged in or created within the last 7 days.
  - `Not active`: Inactive for over 7 days.
  - `Deleted`: Account currently in pending-deletion grace period.
- **Administrative Password Overrides**: Administrators can assign temporary passwords to users. This flags `must_change_password = True` on the user, immediately revokes active refresh tokens, and routes the user to `/change-password` upon their next login.

---

## Tech Stack

### Frontend
| Technology | Version | Purpose |
|---|---|---|
| **React** | 19.0 | Concurrent rendering, component-driven UI architecture |
| **Vite** | 6.2 | ES module dev server, optimized production bundling |
| **TypeScript** | 5.8 | Strict type safety across components and API data contracts |
| **React Router** | 7.3 | Client-side routing, route guards, and nested layouts |
| **TanStack Query** | 5.67 | Server-state caching, optimistic updates, and background refetching |

### Backend
| Technology | Version | Purpose |
|---|---|---|
| **Python** | 3.11+ | Async runtime environment |
| **FastAPI** | 0.115 | Asynchronous REST API framework with automatic OpenAPI documentation |
| **SQLAlchemy** | 2.0 (async) | Async ORM supporting PostgreSQL (production) and SQLite (testing) |
| **Pydantic** | 2.11 | Data validation and schema enforcement at API boundaries |
| **Argon2-cffi** | 23.1 | Password hashing algorithm |
| **PyJWT** | 2.10 | Stateless JWT access and refresh token creation and verification |
| **slowapi** | 0.1.9 | IP and user rate limiting middleware |
| **pypdf** | 5.4 | PDF binary inspection and text extraction |
| **httpx** | 0.28 | Async HTTP client for Google Gemini REST integration |
| **Anthropic** | 0.49 | Claude API client for low-confidence PDF fallback parsing |
| **Alembic** | 1.15 | Database schema migrations |
| **pytest + pytest-asyncio** | 8.3 / 0.26 | Comprehensive async test suite (107+ tests) |

### Cloud Infrastructure
| Service | Role | Configuration |
|---|---|---|
| **Vercel** | Hosting & Serverless Execution | Full-stack deployment (Vite SPA + FastAPI serverless function `api/index.py`) |
| **Vercel Cron** | Scheduled Job Automation | Daily invocation of `/api/admin/cron/cleanup` (`0 0 * * *`) |
| **Supabase** | Managed PostgreSQL | Transaction-pooled database connection on port `6543` with `tsvector` + GIN indexes |

---

## Project Structure

```
.
├── api/
│   └── index.py                      # Vercel serverless entry point bridging FastAPI app
├── backend/
│   ├── alembic/                      # Database schema migrations
│   ├── app/
│   │   ├── api/
│   │   │   ├── deps.py               # Dependency injection (auth, db sessions, ownership)
│   │   │   └── routes/
│   │   │       ├── admin.py          # Admin unlock, user auditing, password overrides, cron
│   │   │       ├── ai.py             # Gemini syllabus generation, refinement, confirmation
│   │   │       ├── auth.py           # Register, login, refresh, Google OAuth, OTP reset, me
│   │   │       ├── dashboard.py      # Summary metrics, streak calendar, study-next suggestions
│   │   │       ├── notes.py          # Topic-attached revision notes CRUD
│   │   │       ├── pdf.py            # PDF upload, text extraction, status polling, confirmation
│   │   │       ├── personal_notes.py # Standalone scratchpad & checklist tasks CRUD
│   │   │       ├── search.py         # Full-text search engine (Postgres tsvector / SQLite LIKE)
│   │   │       ├── skills.py         # Skills CRUD, tree builders, trash management & impact
│   │   │       ├── timer.py          # Skill target schedule date range (from_date / to_date)
│   │   │       └── topics.py         # Topic creation, tree fetch, leaf completion toggles
│   │   ├── core/
│   │   │   ├── admin.py              # Superuser access code verification
│   │   │   ├── config.py             # Pydantic environment configuration
│   │   │   ├── exceptions.py         # Custom application exceptions and error handlers
│   │   │   ├── logging.py            # Structured logging utilities
│   │   │   ├── rate_limit.py         # SlowAPI rate limiting configuration
│   │   │   └── security.py           # Argon2 password hashing and JWT encoding/decoding
│   │   ├── db/
│   │   │   ├── base.py               # Declarative Base metadata
│   │   │   └── session.py            # Async engine and session maker (NullPool for serverless)
│   │   ├── models/
│   │   │   ├── enums.py              # ActionType and ImportJobStatus enumerations
│   │   │   └── __init__.py           # SQLAlchemy database models
│   │   ├── services/
│   │   │   ├── activity.py           # Activity log formatting and timezone resolution
│   │   │   ├── email.py              # SMTP email dispatch for password reset codes
│   │   │   ├── gemini.py             # Google Gemini API integration and syllabus refinement
│   │   │   ├── pdf_parser.py         # Syllabus outline regex parser and AI fallback
│   │   │   └── progress.py           # Leaf-based progress rollup engine and streak calculator
│   │   ├── schemas.py                # Pydantic request and response schemas
│   │   └── main.py                   # FastAPI application factory, middleware, and routers
│   ├── tests/                        # 107+ automated async test cases
│   └── requirements.txt              # Backend Python dependencies
├── frontend/
│   ├── src/
│   │   ├── api/                      # Modular API clients (auth, ai, skills, topics, etc.)
│   │   │   └── client.ts             # Base HTTP client, token refresh rotation, and retry logic
│   │   ├── components/
│   │   │   ├── ConfirmDialog.tsx     # Reusable action confirmation modal
│   │   │   ├── ConsistencyGraph.tsx  # GitHub-style 365-day activity heatmap
│   │   │   ├── NotesPanel.tsx        # Slide-out drawer for topic revision notes
│   │   │   ├── PasswordField.tsx     # Password input with visibility toggle
│   │   │   ├── PreviewEditor.tsx     # Editable topic tree preview editor
│   │   │   ├── ProgressVisuals.tsx   # SVG circular progress rings and donut charts
│   │   │   ├── SkillTimerCard.tsx    # Goal timeline countdown and progress gauge
│   │   │   ├── TimezoneSelect.tsx    # IANA timezone selector dropdown
│   │   │   └── TopicTree.tsx         # Multi-level accordion topic tree with leaf checkboxes
│   │   ├── lib/
│   │   │   ├── activityText.ts       # Human-readable event description formatters
│   │   │   ├── skillColors.ts        # Deterministic pastel color palette generator
│   │   │   └── topicTree.ts          # Tree hierarchy utility functions
│   │   ├── pages/
│   │   │   ├── ActivityPage.tsx      # Reverse-chronological activity audit trail
│   │   │   ├── AdminPage.tsx         # Passkey-protected admin console
│   │   │   ├── AIPreviewPage.tsx     # AI syllabus preview studio with refinement chat
│   │   │   ├── DashboardPage.tsx     # Main dashboard with stats, streaks, and heatmap
│   │   │   ├── ForceChangePasswordPage.tsx # Mandatory password reset screen
│   │   │   ├── GenerateAIPage.tsx    # AI syllabus generation input screen
│   │   │   ├── HomePage.tsx          # Landing page with interactive feature walkthroughs
│   │   │   ├── ImportPage.tsx        # PDF drag-and-drop syllabus importer
│   │   │   ├── LoginPage.tsx         # Login, registration, and OTP password recovery
│   │   │   ├── MakeNotePage.tsx      # Standalone scratchpad checklists
│   │   │   ├── SearchPage.tsx        # Global search interface with filtered hit cards
│   │   │   ├── SettingsPage.tsx      # User profile, timezones, and skill progress bars
│   │   │   ├── SkillDetailPage.tsx   # Topic tree detail view with timer and notes
│   │   │   ├── SkillsPage.tsx        # Grid view of all active skills
│   │   │   ├── StreakPage.tsx        # Detailed streak statistics and yearly calendar
│   │   │   └── TrashPage.tsx         # Trash bin with countdown timers and restore actions
│   │   ├── App.tsx                   # Main router configuration and authenticated shell layout
│   │   ├── main.tsx                  # React DOM entry point with TanStack Query provider
│   │   └── styles.css                # Global dark-theme responsive design system
│   ├── package.json                  # Frontend scripts and dependencies
│   └── vite.config.ts                # Vite build and proxy configuration
├── .env.example                      # Environment variables template
├── requirements.txt                  # Root requirements for Vercel Python runtime
└── vercel.json                       # Vercel deployment, serverless function, rewrites & cron
```

---

## Database Design & Entity Relationships

```mermaid
erDiagram
    User ||--o{ Skill : owns
    User ||--o{ PersonalNote : creates
    User ||--o{ ActivityLog : logs
    User ||--o{ ImportJob : uploads
    User ||--o{ RefreshToken : authenticates
    User ||--o{ PasswordResetToken : requests
    User ||--o{ SkillTimer : schedules

    Skill ||--o{ Topic : contains
    Skill ||--o| SkillTimer : targets
    
    Topic ||--o{ Topic : "parent of (recursive)"
    Topic ||--o{ Note : attaches
    
    PersonalNote ||--o{ PersonalNoteTask : contains
```

### Models Summary
- **User**: Core account identity with Argon2 password hash, optional `google_id`, IANA `timezone`, `must_change_password` flag, and `deletion_deadline` for 72-hour grace recovery.
- **Skill**: Top-level learning domain owned by a user, containing soft-delete fields (`deleted_at`, `purge_at`).
- **Topic**: Recursive hierarchical node (`parent_id` foreign key referencing `topics.id`). Only leaf topics store `is_completed` and `completed_at`.
- **Note**: Timestamped text revision note attached directly to a specific topic.
- **PersonalNote & PersonalNoteTask**: Separate notebook cards with ordered, checkable daily tasks (`order_index`, `is_completed`).
- **ActivityLog**: Immutable audit trail recording event types (`topic_completed`, `skill_created`, `ai_generated`, etc.) with captured topic/skill snapshots and localized calendar dates.
- **SkillTimer**: Start and target completion date range (`from_date`, `to_date`) attached to a specific skill for goal timeline tracking.
- **ImportJob**: Tracks PDF extraction status, confidence metadata, and generated tree structure JSON.
- **RefreshToken**: Cryptographically hashed tokens (`token_hash`) for secure 30-day session rotation.
- **PasswordResetToken**: Single-use 6-digit verification code hashes with 15-minute expirations.

---

## API Reference

### Authentication (`/auth`)
| Method | Endpoint | Description | Rate Limit |
|---|---|---|---|
| `POST` | `/auth/register` | Register new account with email & password | 8 / 15m |
| `POST` | `/auth/login` | Authenticate user & receive access/refresh token pair | 5 / 15m |
| `POST` | `/auth/refresh` | Rotate expired access token using refresh token | Standard |
| `POST` | `/auth/logout` | Revoke active refresh token | Standard |
| `POST` | `/auth/google` | Sign in / register via verified Google ID token | 8 / 15m |
| `POST` | `/auth/forgot-password` | Generate & email 6-digit OTP reset code | 5 / 15m |
| `POST` | `/auth/verify-reset-code` | Verify OTP reset code without consuming it | 8 / 15m |
| `POST` | `/auth/reset-password` | Reset password using verified reset code | 8 / 15m |
| `POST` | `/auth/change-password` | Force-change password after administrative override | Authenticated |
| `GET` | `/auth/me` | Fetch authenticated user profile & permissions | Authenticated |
| `PATCH` | `/auth/me` | Update user display name or IANA timezone | Authenticated |
| `DELETE` | `/auth/me` | Schedule account deletion (72-hour grace period) | Authenticated |

### AI Curriculum (`/ai`)
| Method | Endpoint | Description | Rate Limit |
|---|---|---|---|
| `POST` | `/ai/generate-syllabus` | Generate hierarchical curriculum via Gemini | 40 / hr |
| `POST` | `/ai/refine-syllabus` | Conversationally adjust curriculum tree via prompt | 60 / hr |
| `POST` | `/ai/confirm-syllabus` | Persist finalized curriculum as a new Skill with Topics | 30 / hr |

### Skills & Trash (`/skills`)
| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/skills` | List all active skills with calculated progress metrics |
| `POST` | `/skills` | Create a new empty skill |
| `GET` | `/skills/{skill_id}` | Retrieve skill details with leaf completion stats |
| `PATCH` | `/skills/{skill_id}` | Update skill name or description |
| `GET` | `/skills/{skill_id}/delete-impact` | Audit count of topics and notes that will be removed |
| `DELETE` | `/skills/{skill_id}` | Move skill to Trash (soft delete with 3-day recovery) |
| `GET` | `/skills/trash` | List all skills currently in the Trash bin |
| `POST` | `/skills/{skill_id}/restore` | Restore a trashed skill back to active status |
| `DELETE` | `/skills/{skill_id}/permanent` | Permanently purge a skill and its topics from the database |
| `GET` | `/skills/{skill_id}/topics` | Fetch entire hierarchical topic tree for a skill |

### Skill Timer (`/skills/{skill_id}/timer`)
| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/skills/{skill_id}/timer` | Fetch target schedule dates (`from_date`, `to_date`) |
| `POST` | `/skills/{skill_id}/timer` | Set or update target schedule dates |
| `DELETE` | `/skills/{skill_id}/timer` | Remove target schedule from a skill |

### Topics (`/topics`)
| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/topics` | Add a new root or child topic to a skill |
| `GET` | `/topics/{topic_id}` | Fetch topic metadata and derived progress |
| `GET` | `/topics/{topic_id}/children` | Fetch immediate children of a topic node |
| `POST` | `/topics/{topic_id}/complete` | Check or uncheck a leaf topic (recalculates progress) |

### Topic Notes (`/notes`)
| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/notes` | Create a revision note attached to a topic |
| `GET` | `/notes?topic_id={id}` | List all notes for a specific topic |
| `GET` | `/notes/{note_id}` | Retrieve a specific note |
| `PATCH` | `/notes/{note_id}` | Update content of an existing note |
| `DELETE` | `/notes/{note_id}` | Delete a note |

### Personal Scratchpad (`/personal-notes`)
| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/personal-notes` | List all standalone scratchpad notes with tasks |
| `POST` | `/personal-notes` | Create a new scratchpad note |
| `GET` | `/personal-notes/{id}` | Retrieve a specific scratchpad note |
| `PATCH` | `/personal-notes/{id}` | Update title or checklist tasks (auto-saved) |
| `DELETE` | `/personal-notes/{id}` | Delete a scratchpad note |

### PDF Syllabus Ingestion (`/pdf`)
| Method | Endpoint | Description | Rate Limit |
|---|---|---|---|
| `POST` | `/pdf/upload` | Upload syllabus PDF file (max 10 MB, ~200 pages) | 10 / hr |
| `GET` | `/pdf/jobs/{job_id}` | Poll asynchronous extraction and parsing status | Standard |
| `PATCH` | `/pdf/jobs/{job_id}/tree` | Modify proposed topic tree in Preview Editor | Standard |
| `POST` | `/pdf/jobs/{job_id}/confirm` | Atomically persist proposed tree into database | Standard |

### Dashboard & Analytics (`/dashboard`, `/streak`, `/activity`, `/search`)
| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/dashboard` | Fetch global summary statistics (skills, leaf topics, streak) |
| `GET` | `/dashboard/suggested-next`| Retrieve the next recommended topic or skill to study |
| `GET` | `/streak` | Get current and longest streak counters |
| `GET` | `/streak/calendar` | 365-day consistency calendar data for heatmap rendering |
| `GET` | `/activity` | Retrieve recent activity audit feed (limit 20) |
| `GET` | `/search?q={query}` | Execute full-text multi-entity search across all content |

### Administration & Cron (`/admin`)
| Method | Endpoint | Description | Security |
|---|---|---|---|
| `POST` | `/admin/unlock` | Validate Database Access Code and obtain admin token | Admin email + Access code |
| `GET` | `/admin/users` | List all users with computed activity statuses | Admin Bearer Token |
| `POST` | `/admin/users/{user_id}/password` | Override user password (forces change on login) | Admin Bearer Token |
| `POST` | `/admin/cleanup/expired-accounts` | Manually run account purge sweep | Admin Bearer Token |
| `GET / POST`| `/admin/cron/cleanup` | Automated daily cleanup of expired trash and accounts | Bearer `CRON_SECRET` |

---

## Deployment Guide (Vercel + Supabase)

### 1. Database Provisioning (Supabase)
1. Create a project at [supabase.com](https://supabase.com).
2. Navigate to **Project Settings → Database → Connection string**.
3. Select the **Transaction Pooler** mode (port `6543`, not direct port 5432).
4. Construct the asynchronous connection URI:
   ```env
   DATABASE_URL=postgresql+asyncpg://postgres.[PROJECT-REF]:[PASSWORD]@aws-0-[REGION].pooler.supabase.com:6543/postgres
   ```
5. Apply database migrations using Alembic:
   ```bash
   cd backend
   alembic upgrade head
   ```

### 2. Vercel Project Setup
1. Push your repository to GitHub.
2. In the [Vercel Dashboard](https://vercel.com), select **Add New Project** and import your repository.
3. Configure the project settings:
   - **Framework Preset**: Vite
   - **Root Directory**: `./` (leave as root; `vercel.json` coordinates both frontend and backend)
   - **Build Command**: `cd frontend && npm install && npm run build`
   - **Output Directory**: `frontend/dist`
4. Add the following **Environment Variables** in Vercel:

| Variable | Required | Description |
|---|---|---|
| `DATABASE_URL` | **Yes** | Supabase pooled Postgres URI (`postgresql+asyncpg://...:6543/postgres`) |
| `JWT_SECRET` | **Yes** | Access token secret (minimum 32 random characters in production) |
| `JWT_REFRESH_SECRET` | **Yes** | Refresh token secret (different 32+ character string) |
| `ENVIRONMENT` | **Yes** | Set to `production` |
| `CRON_SECRET` | **Yes** | Shared secret used to secure Vercel Cron requests (min 16 chars) |
| `GEMINI_API_KEY` | **Yes** | Google Gemini API key for AI syllabus generation |
| `GEMINI_MODEL` | No | Defaults to `gemini-3.8-flash` |
| `ADMIN_ACCESS_CODE` | Optional | Minimum 12-character passkey to unlock the superuser console |
| `ADMIN_EMAIL` | Optional | Administrator email address (default: `riteeritee251@gmail.com`) |
| `GOOGLE_CLIENT_ID` | Optional | Google OAuth 2.0 client ID |
| `GOOGLE_CLIENT_SECRET` | Optional | Google OAuth 2.0 client secret |
| `ANTHROPIC_API_KEY` | Optional | Anthropic API key for low-confidence PDF parsing fallback |
| `SMTP_HOST` | Optional | SMTP mail server hostname for sending OTP reset emails |
| `SMTP_PORT` | Optional | SMTP port (e.g., `587`) |
| `SMTP_USER` | Optional | SMTP username |
| `SMTP_PASSWORD` | Optional | SMTP app password |
| `SMTP_FROM` | Optional | Sender email address (e.g., `noreply@yourdomain.com`) |

5. Click **Deploy**. Vercel will build the React SPA, assemble the serverless Python API function (`api/index.py`), register the `/api/*` rewrites, and configure the native Vercel Cron schedule.

### 3. Vercel Cron Automation
The scheduled task in `vercel.json` triggers `/api/admin/cron/cleanup` every day at midnight UTC:
```json
{
  "crons": [
    {
      "path": "/api/admin/cron/cleanup",
      "schedule": "0 0 * * *"
    }
  ]
}
```
When invoked, Vercel Cron automatically supplies the `Authorization: Bearer <CRON_SECRET>` header. The endpoint runs an idempotent cleanup:
1. Permanently removes user accounts that reached their 72-hour deletion deadline.
2. Permanently purges skills from the Trash bin whose 3-day recovery deadline has expired.

---

## Local Development Setup

### Prerequisites
- Python 3.11 or higher
- Node.js 18 or higher & npm
- Git

### 1. Clone & Setup Environment
```bash
git clone https://github.com/your-username/skill-progress-tracker.git
cd "skill-progress-tracker"
copy .env.example .env
```
*(On macOS/Linux, use `cp .env.example .env`)*

### 2. Backend Setup
```bash
cd backend
python -m venv .venv

# Activate virtual environment
# Windows (PowerShell):
.venv\Scripts\Activate.ps1
# macOS/Linux:
source .venv/bin/activate

pip install -r requirements.txt
```

Run database migrations (or allow SQLite auto-creation in development):
```bash
alembic upgrade head
```

Start the FastAPI development server:
```bash
uvicorn app.main:app --reload --port 8000
```
API docs available at: `http://localhost:8000/docs`

### 3. Frontend Setup
In a separate terminal:
```bash
cd frontend
npm install
npm run dev
```
Open your browser at `http://localhost:5173`. Vite automatically proxies API requests (`/api` or relative paths) to the backend at port 8000.

### 4. Running Tests
The backend test suite covers 107+ tests across authentication, AI generation, PDF parsing, leaf progress calculations, and security boundaries:
```bash
cd backend
pytest -q
```

To run linter checks:
```bash
ruff check backend
```

---

## License

This project is licensed under the MIT License.