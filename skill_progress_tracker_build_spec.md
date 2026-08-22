# Skill Progress Tracker — Full Build Specification

This document is the single source of truth for building this application. It merges the original product brief with 17 rounds of clarification, plus a handful of technical defaults chosen where the brief was silent (each one is explicitly labeled as a default, not assumed).

## How to Use This Document (read first)

1. **Build phase by phase** — see Part 12. Don't skip ahead. Verify each phase against its acceptance criteria before starting the next.
2. **Never silently drop or simplify an edge case** because the happy path works. Every change to the topic hierarchy, progress engine, PDF import, streak engine, auth, or user data must be checked against every rule in Part 4 and Part 5 that touches the same entity.
3. **The backend is the source of truth.** Ownership, hierarchy validity, progress math, and deletion behavior are enforced server-side — never only in the frontend, and never trust a client-supplied user ID.
4. This is a real production app, not a CRUD demo. Loading/empty/error states, transactions, and authorization checks are not optional polish — they're part of the definition of done for every phase.
5. Where this doc states an explicit default, build to that default. Where it says "your judgment," use good judgment and leave a short comment explaining the choice.

**Table of contents:** 1. Vision · 2. Tech Stack · 3. Data Model · 4. Core Business Rules · 5. Streak Engine · 6. PDF Import Pipeline · 7. Notes & Search · 8. Dashboard · 9. Auth · 10. API Surface · 11. Frontend & UX States · 12. Security & Ops · 13. Free-Tier Hosting · 14. Build Phases · 15. Decision Log

---

## Part 1 — Vision & Core Workflow

A user takes something messy — a university syllabus PDF — and turns it into a structured, trackable topic tree with progress, notes, streaks, and a dashboard. The app must always be able to answer, at a glance:

> What am I learning? What topics are inside it? What have I completed? What's remaining? What did I study recently? What should I study next?

**Core workflow:** `Create Skill → Upload PDF → Parse → Preview/Edit → Confirm → Track Leaf Topics → Add Notes → Mark Progress → Search → Maintain Streak → View Dashboard`

Progress is always measured at the **leaf topic** level, never at parent categories directly. Parent progress is always *derived*, never independently set.

---

## Part 2 — Tech Stack (Final)

| Layer | Choice | Why |
|---|---|---|
| Frontend | React 19 + Vite + TypeScript | Matches the brief; modern, strongly typed, fast dev loop |
| Backend | Python + FastAPI + SQLAlchemy 2.0 (async) + Pydantic v2 | Matches the brief; async end-to-end fits Postgres + AI calls well |
| Auth | JWT access (15 min) + refresh (30 days, rotated, revocable) | Standard, secure default |
| Database | PostgreSQL via Supabase | Matches the brief |
| DB connection | **pgbouncer, transaction-pooling mode** (Supabase's pooled connection string, port 6543, not 5432) | *Default added:* Supabase free tier caps direct connections low; an async backend on Render can exhaust them fast without pooling |
| AI (PDF parsing fallback only) | Anthropic API (Claude), called server-side only | Only invoked for sections the rule-based parser can't confidently structure — see Part 6 |
| Search | PostgreSQL full-text search (`tsvector` + GIN index) | *Default added:* free, no extra infra, sufficient at individual-user scale |
| Frontend hosting | Vercel | Matches the brief |
| Backend hosting | Render (free web service) | Matches the brief — see Part 13 for how the app must handle its cold-start behavior |
| DB/Auth infra | Supabase | Matches the brief |
| CI/CD | GitHub Actions | Frontend: install → typecheck → build. Backend: install → lint → test |

Nothing here overrides the brief structurally — it fills in the specific tools for the things the brief deliberately left open (AI provider, search engine, connection pooling).

---

## Part 3 — Data Model

Ownership chain: every row below a `User` is reachable through a `user_id` (directly or via its parent), and **every query must filter by the authenticated user's ID** — never trust a client-supplied ID.

```
users
  id (pk)
  email (unique, not null)
  password_hash (nullable — null if the account was created via Google and never set a local password)
  google_id (nullable, unique)
  name
  timezone (IANA string, e.g. "Asia/Kolkata" — default added, see Part 5)
  is_active
  created_at, updated_at

skills
  id (pk)
  user_id (fk -> users, not null)
  name
  description (nullable)
  created_at, updated_at

topics
  id (pk)
  skill_id (fk -> skills, not null)
  parent_id (fk -> topics, nullable — null means root topic of the skill)
  name
  description (nullable)
  order_index (int — sibling ordering)
  is_completed (bool, default false)
  completed_at (timestamp, nullable — set on completion, left as-is on un-complete so history isn't lost mid-record; the authoritative history lives in activity_log, not here)
  created_at, updated_at
  -- NOTE: no stored `is_leaf` column. Leaf status is *derived* (a topic with
  -- zero children is a leaf). Compute it per-request via a single aggregated
  -- query (e.g. LEFT JOIN topics AS children GROUP BY parent_id having
  -- count(children)=0), not a per-node round trip. See Part 4.1.

notes
  id (pk)
  topic_id (fk -> topics, not null)
  user_id (fk -> users, not null — redundant with topic ownership but makes ownership checks and search a single-table filter)
  content (text, plain text — see Decision Log #12)
  created_at, updated_at

activity_log            -- append-only. Powers BOTH the streak engine and the "recent activity" feed.
  id (pk)
  user_id (fk -> users, not null)
  action_type (enum: 'topic_completed' | 'note_added' | 'note_updated' | 'topic_created' | 'skill_created' | 'pdf_imported')
  topic_id (fk -> topics, nullable, ON DELETE SET NULL)
  skill_id (fk -> skills, nullable, ON DELETE SET NULL)
  topic_name_snapshot (text, nullable — copied at write time so the feed and streak history stay meaningful even after the topic/skill is deleted)
  skill_name_snapshot (text, nullable)
  occurred_at (timestamptz, not null)
  activity_date (date, not null — occurred_at converted to the user's stored timezone; this is the column streak math groups on)

import_jobs
  id (pk)
  user_id (fk -> users, not null)
  skill_id (fk -> skills, nullable, ON DELETE SET NULL — set only after confirmation)
  original_filename
  file_size_bytes
  status (enum: 'uploaded' | 'extracting' | 'parsing' | 'ready_for_preview' | 'confirmed' | 'failed')
  confidence_notes (jsonb, nullable — which generated nodes were low-confidence, for the Preview UI to flag)
  error_message (nullable)
  generated_tree_json (jsonb — the proposed hierarchy, edited in place by the Preview Editor until confirmed)
  created_at, updated_at

refresh_tokens
  id (pk)
  user_id (fk -> users, not null)
  token_hash (not null)
  expires_at (not null)
  revoked (bool, default false)
  created_at
```

**Indexes:** `topics(skill_id, parent_id, order_index)`, `topics(parent_id)`, `activity_log(user_id, activity_date)`, `activity_log(user_id, action_type, activity_date)`, GIN index for full-text search across `skills.name/description`, `topics.name/description`, `notes.content`. Unique constraint on `users.email`. **No uniqueness constraint on topic names** — see Decision Log / Rule 4.9.

**Why `activity_log` survives topic/skill deletion:** streak credit, once earned, is permanent (Decision Log #2). If deleting a topic cascaded into deleting its `activity_log` rows, deleting a topic you completed today could silently un-earn today's streak day. So the foreign keys are `ON DELETE SET NULL` with a name snapshot taken at write time, not `ON DELETE CASCADE`.

---

## Part 4 — Core Business Rules

### 4.1 Leaf detection
A topic is a leaf **iff it has zero children right now**. Never store this as a boolean that can go stale — derive it from the `parent_id` relationships, computed in bulk per request (one aggregated query for a whole tree), not per-node.

**Leaf depth is not fixed and must never be hardcoded.** The same PDF can legitimately bottom out at different depths in different branches — e.g. one section's leaves are at `24.1`–`24.26` (depth 2, no further numbering under them) while another section's leaves are at `28.1.1`–`28.1.5` (depth 3). Whether a node is a leaf is decided purely by "does it currently have children," never by "is it at depth N." This also determines which control renders on that node in the UI — see 11.1.

### 4.2 Progress calculation
```
skill_progress = completed_leaf_topics / total_leaf_topics × 100
```
- Computed from **leaf topics only**, never by averaging parent nodes.
- Zero leaf topics → **0%**, never a divide-by-zero, never `NaN`. UI shows "No topics yet," not `0%` or an error.
- Parent-node progress is always *derived* by recursively aggregating its descendant leaves — never independently settable.
- Recalculate on every hierarchy change (leaf added, leaf deleted, leaf converted to parent, topic moved) — see 4.6.

### 4.3 Completion toggling (reversible)
- `topics.is_completed` can flip both ways. Un-completing a topic **decreases** progress % immediately (Decision Log #6).
- Toggling is **idempotent**: clicking "Complete" when it's already `true` is a no-op — no duplicate `activity_log` row, no `completed_at` change (this is what satisfies the brief's "clicking Complete twice must not create duplicate records").
- A **new** `activity_log` row (`action_type='topic_completed'`) is written every time a topic transitions `false → true` — including re-completions after an un-complete. This is deliberate: it lets the streak math work off "was there ≥1 completion event on date D," which naturally collapses multiple same-day completions into one streak day (see Part 5) and naturally handles re-completion without special-casing.
- Un-completing never deletes prior `activity_log` rows. That's what makes "streak credit stays" (Decision Log #2) simple: the streak engine only ever reads history forward, never depends on current `is_completed` state.

### 4.4 Circular hierarchy prevention
Before allowing a topic to be moved under a new parent, walk the **target parent's ancestor chain** up to the root. If the topic being moved appears in that chain, reject the move (409 or 422 — your judgment, be consistent) with a clear error. Enforce this **server-side only** — never rely on the frontend to prevent it.

### 4.5 Topic & Skill deletion — *default added*
The brief (Section 37) explicitly flags topic deletion as needing a rule but leaves it open, and never addresses Skill-level deletion at all. Default for both: **confirm, then cascade, in one transaction.**
- Before deleting a topic with descendants, the UI shows what's about to go (e.g. "12 descendants, 3 notes").
- On confirmation, delete the topic + its full subtree + their notes in a single DB transaction. `activity_log` rows referencing any deleted topic get `topic_id` set to `NULL` (name snapshot preserved) — never deleted, per Part 3.
- After deletion, recalculate the parent chain's progress immediately (see 4.6's worked example — deleting an incomplete leaf can *raise* the parent's %).
- This avoids both silent data loss and the friction of forcing manual leaf-by-leaf cleanup first.
- **Deleting a whole Skill** is the same pattern one level up: confirm (showing total topic/note counts), then delete the Skill + every Topic in it + their Notes, all in one transaction. `activity_log` rows get `skill_id`/`topic_id` set to `NULL` exactly as above. **`import_jobs` rows referencing the deleted skill also get `skill_id` set to `NULL`, never deleted** — the job stays as a historical/debugging trail of how the skill was originally imported (generated tree, confidence notes, any parse error), same reasoning as `activity_log`. Account deletion (Part 9) is the one place everything, `import_jobs` included, is actually hard-deleted.

### 4.6 Progress when the hierarchy changes
- **Adding a leaf:** total leaf count goes up, % drops accordingly (`4/4=100%` + 1 new leaf → `4/5=80%`). Never keep showing the old %.
- **Deleting an incomplete leaf:** total drops, % can rise (`4/5=80%` → delete the incomplete one → `4/4=100%`).
- **Leaf becomes a parent** (gains children): **auto-clear its old completed status** (Decision Log #8). Its progress now comes entirely from its new children, starting at 0% until they're completed.
- Example from the brief, preserved exactly: `A{B✓, C✓, D✗}` at 66.67%; delete D → `A{B✓, C✓}` → **100%**, not the stale 66.67%.

### 4.7 Duplicate topic names
Same name does not imply same entity (e.g. "Merge Sort" can legitimately exist under both `Arrays > Sorting` and `Arrays > Searching`). **Do not enforce global uniqueness on topic names.** Identity comes from `id` + position in the hierarchy, full stop.

### 4.8 Empty states, not errors
- Skill with 0 topics → "No topics yet," never `NaN%` or a crash.
- Empty note content (blank/whitespace-only) is rejected client- and server-side before it's ever saved.

### 4.9 Reordering / moving topics — *Decision Log #10*
No drag-and-drop. Simple controls: move up / move down among siblings, and a "move to parent" picker. Available both in the PDF Preview Editor (pre-import) and in the persistent Topic Tree (post-import, for corrections). Server-side, every move re-checks 4.4 (circular hierarchy) and re-derives 4.1/4.2 for both the old and new parent chains.

---

## Part 5 — Streak Engine

**Trigger:** only `activity_log` rows with `action_type='topic_completed'` count toward the streak (Decision Log #4). Notes, topic creation, etc. are logged too (for the activity feed, Part 8) but never move the streak.

**Definition of a "learning day":** at least one `topic_completed` row whose `activity_date` (already converted to the user's stored `timezone`) equals that calendar day. Ten completions in one day still count as **one** learning day.

**Timezone handling — *default added*.** The brief flags this as important but doesn't pick a rule. Default: capture the browser's IANA timezone (`Intl.DateTimeFormat().resolvedOptions().timeZone`) at signup into `users.timezone`, editable later in settings. Every `activity_date` is computed by converting `occurred_at` into that stored timezone — never the server's timezone, never a hardcoded UTC cutover. This is what makes an 11:59 PM vs 12:01 AM pair of actions resolve sanely for that specific user.

**Grace/freeze rule (Decision Log #5):** streak breaks (resets to 0) only when **two or more consecutive calendar days** have zero `topic_completed` activity. Missing exactly one day does not break it — that day simply contributes nothing, and the run continues from where it left off. This is an always-on rule, not a limited/consumable resource (no "streak freeze tokens").

**Current streak / longest streak:** compute on demand from `activity_log`, grouped by distinct `activity_date`, applying the grace rule above to find the current run length and the historical max. At individual-user scale (at most a few thousand rows even after years of daily use) this is fast enough to compute live — don't build a separately-cached counter that can drift out of sync; it's not needed here.

**Other streak edge cases, all satisfied by the design above:**
- First-ever activity → first `topic_completed` row starts a 1-day streak.
- Manually editing/deleting progress → does not retroactively remove streak days (4.3, 4.5).
- Returning after a long break → streak resets per the grace rule above; no special-case code needed.

---

## Part 6 — PDF Import Pipeline

`Upload → Validate → Extract Text → Structure Detection (rule-based, primary) → AI Fallback (low-confidence sections only) → Merge → Generate Proposed Tree → Preview/Edit → Confirm → Persist (transaction)`

### 6.1 Validation (before anything else runs)
- Max size **10MB**, max **~200 pages** (Decision Log). Reject over-limit uploads immediately with a clear message.
- Validate actual file content/MIME type, not just the filename extension.
- If the extractor finds **no extractable text** (scanned/image-only PDF) → **reject with a clear error**, no OCR attempt (Decision Log). Message should explicitly suggest exporting/printing to a text-based PDF instead of guessing.

### 6.2 Structure detection — hybrid (Decision Log #1)
The parser is tuned primarily around **decimal outline numbering** — `1`, `2`, `2.1`, `2.2`, `3`, `3.1`, `3.n`, going arbitrarily deep — since that's the dominant pattern in real uploads per your own sample. Rule-based pass, roughly:
1. Regex-match lines against a decimal-numbering pattern (`^\s*(\d+(\.\d+)*)\.?\s+(.+)`) to build the primary tree by number-of-segments = depth.
2. Secondary heuristics for patterns that break the pure decimal chain but are still structurally obvious: `UNIT N — Title` / `Chapter N` headers (treated as a level-0 grouping), and bullet-only blocks (`•` with no numbering) nested under the nearest heading, treated as a flat list of leaves at that heading's child level.
3. Score each detected node's confidence (e.g. "matched clean decimal pattern" = high; "inferred from indentation only, no numbering" = low).
4. **Any section the rule-based pass can't confidently structure** gets sent to the Anthropic API with the raw text for that section and a prompt asking for a strict JSON hierarchy back (name/children only, no extra prose) — this is the *only* place an AI call happens, and only for the ambiguous remainder, not the whole document every time.
5. Merge rule-based + AI-derived branches back into one proposed tree, preserving original document order throughout.

**Theory + Lab syllabus in one PDF (Decision Log #4):** if the parser detects two structurally distinct top-level regions (e.g. a numbered `UNIT`/chapter sequence and a separate `Lab N —` sequence), nest them as two named branches — `Theory` and `Labs` — under **one** Skill, not two skills.

### 6.3 Async job pattern
AI calls and large-PDF parsing can take real time — don't do this synchronously in the request/response cycle (Render's free tier and typical reverse-proxy timeouts won't tolerate a 30+ second hold). Run parsing as a background task, track status on `import_jobs.status`, and have the frontend poll `GET /pdf/jobs/{id}` with a "Parsing PDF…" / "Analyzing structure…" loading state until `ready_for_preview`.

### 6.4 Preview Editor (mandatory gate)
Never persist a generated hierarchy directly. The user can rename, delete, add, move, and reorder nodes (4.9) before confirming. Low-confidence nodes (from 6.2 step 3) are visually flagged so the user knows where to look closely — never silently guess and hope.

### 6.5 Confirm & persist
On confirmation, create the Skill + full Topic tree as a **single database transaction** (bulk insert). If anything fails mid-way, roll back entirely — never leave a half-imported tree, unless partial import is something you've explicitly decided to support (it isn't, by default).

### 6.6 Re-uploading a PDF (Decision Log #3)
Every PDF import — including re-uploading the same or a related file into what's conceptually "the same subject" — **always creates a brand-new Skill**. No merge/append/replace logic against an existing skill's tree. Simplest possible rule; user can rename/delete skills manually if they end up with duplicates.

### 6.7 File security
Treat every upload as untrusted: validate real content-type, enforce the size limit server-side (not just client-side), never execute uploaded content, clean up temp files after processing, and never leak internal filesystem paths in error messages.

---

## Part 7 — Notes & Search

**Notes (Decision Log #12):** plain text. CRUD, scoped to `topic_id` + `user_id`. Reject blank/whitespace-only content before save (4.8).

**Search:** PostgreSQL full-text search (`tsvector` + GIN, Part 2) across skill names/descriptions, topic names/descriptions, and note content in one query. **Every result is filtered by `user_id = current_user.id`** — a user must never see another user's results, full stop. Results link directly to the relevant tree location, not just a text snippet.

---

## Part 8 — Dashboard

**Global dashboard:** total skills, total topics, completed topics, overall progress, current streak, longest streak, recent activity. Computed via SQL aggregation (`COUNT`/`SUM`/`GROUP BY`), not by loading every row into Python — this matters once topic counts reach the hundreds/thousands the brief anticipates.

**Recent activity feed:** pulls from `activity_log` across **all** action types (not just `topic_completed`) — this is where "Completed Binary Search" and "Updated Graph Notes" both show up, exactly as in the original example. It's a display concern, separate from the streak calculation, which filters to `topic_completed` only (Part 5).

**Per-skill dashboard:** progress % shown as the circular/donut graph (11.1, #3), drill-down `Skill → Category → Subcategory → Leaf Topic`.

**"Study next" suggestion (Decision Log #17):** a simple heuristic widget, not a recommendation engine — e.g. surface the oldest not-yet-completed leaf topic (by tree position or creation order) or the skill with the lowest completion % that still has incomplete leaves. Keep it to one clear suggestion, no spaced-repetition/ML for v1.

---

## Part 9 — Auth

- **Register / login / logout / refresh** via email + password. Passwords hashed (bcrypt or argon2) — never stored plaintext.
- **JWT:** short-lived access token (15 min), longer refresh token (30 days), rotated on each use, revocable (via `refresh_tokens.revoked`, checked on logout and on refresh).
- **Google Sign-In:** verify the Google ID token server-side, look up by email.
  - **Account linking (Decision Log #14):** if an email+password account already exists with that email, auto-link — same account, both login methods now work. If not, create a new user with `google_id` set and `password_hash` null.
- **Forgot password (Decision Log #13):** standard flow — generate a time-limited (e.g. 15 min), single-use reset code, email it to the registered address. Not a Google-reverification flow.
- **Account deletion (Decision Log #15):** hard delete — immediately and permanently removes the user and everything owned by them (skills, topics, notes, activity log, import jobs), in a transaction. No soft-delete/grace period.
- **Authorization:** every user-owned endpoint verifies `resource.user_id == current_user.id` server-side. `GET /skills/123` must 403/404 (pick one, be consistent) if it belongs to someone else — being authenticated is not being authorized.

---

## Part 10 — API Surface (representative, not exhaustive)

```
POST   /auth/register
POST   /auth/login
POST   /auth/refresh
POST   /auth/logout
POST   /auth/google
POST   /auth/forgot-password
POST   /auth/reset-password

GET    /skills            POST /skills           GET/PATCH/DELETE /skills/{id}
GET    /skills/{id}/topics
POST   /topics            GET/PATCH/DELETE /topics/{id}
POST   /topics/{id}/complete     (idempotent toggle, see 4.3)
POST   /topics/{id}/move         (server re-checks 4.4 before applying)
GET    /topics/{id}/children

POST   /notes             GET/PATCH/DELETE /notes/{id}
GET    /search?q=...

GET    /streak
GET    /activity

POST   /pdf/upload
GET    /pdf/jobs/{id}            (poll for status — see 6.3)
PATCH  /pdf/jobs/{id}/tree       (edit the proposed tree in the Preview Editor)
POST   /pdf/jobs/{id}/confirm    (persist, see 6.5)

GET    /dashboard
GET    /dashboard/suggested-next
```

Every endpoint: validates input (Pydantic schemas), authenticates via the JWT dependency, authorizes ownership, and returns consistent error shapes — see Part 12 for status codes.

---

## Part 11 — Frontend & UX States

### 11.1 Progress visualization — three distinct indicators

Straight from the mockups in the original brief. Three different visual treatments in three different places — never substitute one for another:

1. **Leaf topics → tick box / checkbox.** Only ever on nodes with zero children. Binary complete/not-complete — this is the only control that fires 4.3 and the streak trigger in Part 5.
2. **Non-leaf topics, at any depth → line/bar progress graph with a fraction label** (e.g. "9/10", "13/13"). Every node that currently has children shows this — root topics, mid-tree categories, everything up the chain. It fills proportionally from completed/total leaf descendants and updates live — up the *entire* ancestor chain at once — as leaves get ticked.
3. **Skill level only → circular/donut graph with a percentage** (e.g. "85%, 401/474"). One per skill, shown wherever a skill's overall progress appears (dashboard, skill detail page). Individual topics never get this style — only the line graph in #2.

A node is either a leaf (tick box) or a parent (line graph) — never both, and never assumed by depth (4.1: the same tree can leaf out at depth 2 in one branch and depth 3 in another, e.g. `24.1`–`24.26` vs `28.1.1`–`28.1.5`).

- **API layer:** dedicated modules (`auth.api.ts`, `skills.api.ts`, `topics.api.ts`, `progress.api.ts`, `notes.api.ts`, `search.api.ts`, `streak.api.ts`, `pdf.api.ts`) — never raw fetches scattered through components.
- **State:** separate server/API state from UI state, auth state, and form state. Recommend a server-state library (e.g. TanStack Query) for the frontend so loading/success/error/empty states and post-mutation cache invalidation aren't hand-rolled per component — this directly satisfies the brief's loading-state and stale-UI requirements below.
- **Every async operation has an explicit state:** loading / success / error / empty. No blank screens while data loads (e.g. "Loading skills…", "Parsing PDF…", "Saving topic…").
- **Optimistic UI** is fine for low-risk actions (e.g. toggling a leaf complete), but must roll back cleanly to server state if the backend rejects it — never leave the UI showing a state the backend didn't accept.
- **Empty states** on every major page ("No skills yet — create your first one," "No topics yet — upload a syllabus or add manually," "No notes yet," "No recent activity yet").
- **Responsive, mobile-first**, especially the topic tree — it must stay usable on a small screen, not assume desktop width.

---

## Part 12 — Security & Non-Functional Requirements

- **Error handling:** distinguish 400 / 401 / 403 / 404 / 409 / 422 / 500 on the backend; frontend shows user-friendly messages, never a raw stack trace.
- **Input validation:** every write endpoint validates shape server-side (Pydantic) and re-verifies that referenced resources (`skill_id`, `parent_id`, etc.) belong to the authenticated user — never trust the frontend sent valid or owned IDs.
- **Rate limiting** *(default added, not previously specified)*: e.g. 5 login attempts / 15 min per IP+email, 10 PDF uploads / hour per user, a general per-user API throttle. In-memory limiter (e.g. `slowapi`) is fine given Render's free tier runs a single instance — no Redis needed for v1.
- **CORS:** explicitly allow only the deployed frontend origin — no wildcard in production.
- **Secrets:** `DATABASE_URL`, `JWT_SECRET`, `JWT_REFRESH_SECRET`, `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `ANTHROPIC_API_KEY`, `CORS_ORIGINS` — all via environment variables, never committed. Ship a `.env.example` with names only.
- **Logging:** log auth failures, server errors, PDF parsing failures, DB errors. Never log passwords, tokens, or secrets.
- **Transactions:** any multi-row dependent write (PDF import confirmation, cascading topic deletion, account deletion) runs inside a single DB transaction — begin, apply, validate, commit; roll back fully on any failure.

---

## Part 13 — Free-Tier Hosting *(default added — confirm or override before deploying)*

Render's free web service sleeps after ~15 minutes idle; the first request after a break can take 30–50 seconds to wake it. Supabase's free tier caps concurrent direct connections. Build defensively for this by default:
- Use the pooled (pgbouncer, port 6543) connection string, not the direct one (Part 2).
- Frontend shows an explicit "waking up the server…" loading state (not a blank screen or a spinner with no explanation) on requests that are taking unusually long, particularly right after a period of inactivity.
- Consider a lightweight periodic keep-alive ping if cold starts prove disruptive in practice — optional, not required for v1.

If you end up on paid/always-on hosting instead, this section can be dropped, but the pooled connection string is still good practice regardless.

---

## Part 14 — Build Phases

Work through these in order. Each phase's acceptance criteria must pass before moving to the next.

1. **DB schema & migrations** — all tables in Part 3, indexes, FK constraints (including the `ON DELETE SET NULL` on `activity_log`). *Done when:* migrations run clean on a fresh DB; constraints are verified with a quick manual insert/violate test.
2. **Backend foundation** — FastAPI app, config, async DB session, error-handling middleware, logging. *Done when:* a health-check endpoint returns 200 through the full stack (app → DB).
3. **Auth** — register, login, JWT issue/refresh/revoke, Google Sign-In + account linking, forgot/reset password. *Done when:* every user-owned endpoint added later rejects an unauthenticated request and a mismatched-owner request.
4. **Skills & Topics CRUD** — including move (4.4/4.9), delete (4.5), leaf detection (4.1). *Done when:* circular-move attempts are rejected server-side, deleting a topic with descendants requires confirmation and removes the whole subtree in one transaction.
5. **Progress engine** — leaf-based calculation (4.2), recalculation on every hierarchy change (4.6), completion toggle (4.3). *Done when:* the worked examples in Part 4 (delete-D-gets-100%, add-leaf-drops-to-80%, leaf-becomes-parent-clears-status) all pass as tests.
6. **Notes** — CRUD, empty-content validation. *Done when:* a blank note is rejected client- and server-side.
7. **Streak engine** — activity log writes, grace-rule streak calculation, timezone-aware `activity_date` (Part 5). *Done when:* a simulated missed-one-day case doesn't break the streak, a missed-two-day case does, and un-completing a topic doesn't reduce a previously-earned streak day.
8. **Search** — Postgres FTS across skills/topics/notes, user-scoped. *Done when:* a search from one account never returns another account's data.
9. **PDF import — rule-based layer** — validation, extraction, decimal-numbering detector, UNIT/Lab heuristics, confidence scoring. *Done when:* the sample DBMS-style PDF produces a correct tree with Theory and Labs as two branches under one skill.
10. **PDF import — AI fallback + async job** — Anthropic API call for low-confidence sections, `import_jobs` status polling. *Done when:* a deliberately messy/unstructured section still produces a reasonable tree instead of garbage, and the frontend shows a proper "Parsing…" state throughout.
11. **PDF Preview Editor** — edit before confirm, low-confidence flags, atomic persist on confirm (6.5). *Done when:* nothing is written to `skills`/`topics` until explicit confirmation, and a mid-import failure leaves zero partial rows.
12. **Dashboard** — global stats, per-skill drill-down, recent activity feed, "study next" suggestion. *Done when:* stats are computed via SQL aggregation, not full-table Python loops.
13. **Frontend foundation** — routing, auth state, API layer modules, loading/empty/error states everywhere. 
14. **Frontend: Skill & Tree UI** — tree with expand/collapse, progress indicators, move/reorder controls (no drag-and-drop), notes access, responsive down to mobile width.
15. **Integration pass** — every workflow end-to-end, real data, both happy paths and the edge cases in Part 4/5/6.
16. **Security & ops pass** — rate limiting, CORS lockdown, secrets audit, logging audit (no sensitive data logged).
17. **Deployment** — Vercel + Render + Supabase, pooled connection string, env vars configured, GitHub Actions CI green.
18. **Final audit** — re-walk every rule in Parts 4–9 against the deployed app; confirm nothing was simplified away during implementation.

---

## Part 15 — Decision Log (quick reference)

| # | Question | Decision |
|---|---|---|
| 1 | PDF parsing approach | Hybrid: decimal-numbering regex primary, AI fallback for low-confidence sections only |
| 2 | Un-completing a leaf | Reversible; progress updates down; streak credit already earned is never revoked |
| 3 | Re-uploading a PDF | Always creates a brand-new Skill; no merge/replace |
| 4 | Theory + Lab syllabus in one PDF | One skill, two top-level branches ("Theory", "Labs") |
| 5 | Max PDF size | 10MB / ~200 pages |
| 6 | Scanned/image-only PDFs | Reject with a clear error; no OCR |
| 7 | Streak trigger | Leaf completion only |
| 8 | Missing one day | Grace/freeze — streak survives exactly one missed day; breaks on 2+ consecutive |
| 9 | Nesting depth cap | Unlimited |
| 10 | Reordering topics | No drag-and-drop; simple up/down + "move to parent" controls |
| 11 | Leaf gains children (becomes parent) | Auto-clear its old completed status |
| 12 | Notes format | Plain text |
| 13 | "Forgot password via Gmail OTP" | Standard flow — reset code emailed to the registered address |
| 14 | Email+password account + later Google Sign-In, same email | Auto-link, same account |
| 15 | Account deletion | Hard delete everything immediately, no grace period |
| 16 | Free-tier hosting | Design defensively for Render cold-starts + Supabase connection limits (Part 13) |
| 17 | "Study next" suggestion | Build a simple heuristic widget for v1 |
| — | Topic deletion with descendants *(default added)* | Confirm, then cascade-delete in one transaction (Part 4.5) |
| — | Timezone for streak/activity dates *(default added)* | Store browser-detected IANA timezone per user at signup; editable later (Part 5) |
| — | Skill deletion, and `import_jobs` on skill delete *(default added)* | Same confirm-then-cascade pattern as topic deletion, one level up; `import_jobs.skill_id` set to `NULL` (not deleted) to preserve import history (Part 4.5) |

---

## Closing Directive

The database is the source of truth; the frontend renders and manipulates that state only through validated, authorized backend APIs. Every business rule above — ownership, hierarchy validity, progress math, deletion behavior, streak integrity — is enforced server-side. Do not remove or simplify any edge-case behavior in this document merely because the normal workflow already works. Every change to the topic hierarchy, progress system, PDF import, streak engine, authentication, or user data must be evaluated against every related rule before it's considered done.
