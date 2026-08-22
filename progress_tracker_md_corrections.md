# Skill Progress Tracker — Corrections / Verification Notes

## Overall verdict

The uploaded `skill_progress_tracker_build_spec.md` is **more than 90% aligned** with the Skill Progress Tracker project we have established.

I therefore **did not modify the original `.md` file**.

This file records the points that should be treated as corrections, clarifications, or **not yet verified from the project definition I have available**.

---

## 1. What is clearly correct

The following parts match the Progress Tracker project very well:

- **Purpose:** turn a syllabus/learning document into a structured skill/topic hierarchy and track progress.
- **Core workflow:** PDF upload → parsing → topic tree → preview/edit → progress tracking.
- **Leaf-based progress:** progress is tracked at leaf topics, with parent progress derived from descendants.
- **Frontend:** React + Vite + TypeScript.
- **Backend:** FastAPI + Python.
- **Database:** PostgreSQL through Supabase.
- **ORM:** SQLAlchemy async.
- **Deployment:** Vercel frontend + Render backend + Supabase database.
- **CI/CD:** GitHub Actions.
- **Authentication:** email/password login plus Google login.
- **JWT-based authentication / refresh-token architecture:** consistent with the project design.
- **Skills / Topics / Notes / Search / Streak / Dashboard:** all are core parts of the project.
- **PDF Preview Editor:** the user should be able to inspect/edit the generated hierarchy before final persistence.
- **Responsive/mobile support:** consistent with the project goal.
- **Security and ownership checks:** backend must enforce user ownership and must not trust client-supplied ownership IDs.
- **Deployment environment variables and CORS configuration:** consistent with the deployment workflow we used.
- **The phased build approach:** matches the project's planned implementation sequence very closely.

The uploaded document itself describes the project as a system that converts a syllabus PDF into a structured, trackable topic tree with progress, notes, streaks, and a dashboard. fileciteturn0file0L17-L25

The technology choices also match the established project architecture, including React/Vite/TypeScript, FastAPI, SQLAlchemy async, PostgreSQL/Supabase, Vercel, Render, and GitHub Actions. fileciteturn0file0L29-L43

---

## 2. Important points that should NOT be treated as 100% confirmed project requirements

### 2.1 Anthropic / Claude as the PDF AI fallback

The file states:

> Anthropic API (Claude), called server-side only

and further defines AI fallback behavior around it. fileciteturn0file0L33-L39

This is **not something I can independently verify as a final, locked project requirement from the project history I currently have**.

### Correct interpretation

Treat the PDF parser architecture as confirmed, but treat **Anthropic/Claude specifically as an implementation choice unless the actual project code/config confirms it**.

---

### 2.2 PostgreSQL full-text search with `tsvector` + GIN

The file specifies PostgreSQL FTS with `tsvector` + GIN. fileciteturn0file0L39-L40

This is technically reasonable and fits the project, but it is **more specific than the core project description I have stored**.

### Correct interpretation

Search is definitely part of the application. The exact search implementation (`tsvector` + GIN) should be treated as an implementation detail unless it is already implemented in the current codebase.

---

### 2.3 Exact authentication timings

The file fixes:

- access token = 15 minutes
- refresh token = 30 days
- rotation
- revocation

These appear in the spec at the auth/data-model level. fileciteturn0file0L33-L36 fileciteturn0file0L257-L265

### Correct interpretation

The **JWT + refresh-token design is correct** for the project. The exact `15 min / 30 days` values should be treated as implementation parameters unless explicitly locked in the actual project configuration.

---

### 2.4 Topic/Skill deletion rules

The file introduces a fairly specific policy:

- confirm deletion
- cascade subtree deletion
- preserve activity history with `ON DELETE SET NULL`
- preserve `import_jobs`
- hard-delete an account

These rules are described in detail in the file. fileciteturn0file0L156-L163

### Correct interpretation

Deletion behavior is part of the project and should be handled safely/transactionally.

However, I **cannot confirm every exact cascade / `SET NULL` / historical-retention rule as part of the original project requirements** from the project information I currently have.

Treat these as design decisions that should be validated against the current implementation/product decision log.

---

### 2.5 Streak grace rule

The file defines a very specific streak rule:

- one missed day does not break the streak
- two consecutive missed days break it

This appears in the uploaded document. fileciteturn0file0L182-L196

### Correct interpretation

The **streak engine itself is definitely part of the Progress Tracker project**.

The exact **"one missed day is allowed / two missed days break"** rule is not something I can verify from the core project description I currently have.

So this should be considered **unverified until confirmed by the actual project rules/code**.

---

### 2.6 PDF size/page limits and no-OCR rule

The file fixes:

- 10 MB maximum
- approximately 200 pages maximum
- reject scanned/image-only PDFs
- no OCR

These are explicitly stated in the spec. fileciteturn0file0L203-L208

### Correct interpretation

PDF upload/parsing is definitely a core feature.

But these exact limits and the **"no OCR"** decision are specific implementation/product constraints that I cannot independently verify from the project summary I have.

---

### 2.7 Exact PDF parsing heuristics

The document heavily specifies:

- decimal outline numbering
- `UNIT N`
- `Chapter N`
- bullets
- confidence scoring
- AI fallback only for ambiguous sections
- Theory/Labs detection

This is described in detail in the PDF-import section. fileciteturn0file0L201-L218

### Correct interpretation

The important project requirement is:

**PDF → extract content → create a hierarchical topic tree → allow preview/edit → confirm.**

The exact parsing heuristics are implementation details and should not be treated as universally fixed unless the current parser/code confirms them.

---

### 2.8 "Every PDF re-upload creates a brand-new Skill"

The file makes this a hard rule. fileciteturn0file0L229-L230

### Correct interpretation

PDF import into a skill/topic structure is confirmed.

But the exact rule that **every re-upload must always create a new Skill** is not independently verified by the project summary I have. This should be confirmed against the actual product behavior.

---

### 2.9 "No drag-and-drop" topic reordering

The file explicitly prohibits drag-and-drop and requires:

- move up
- move down
- move to parent

This appears in the business rules and decision log. fileciteturn0file0L177-L179 fileciteturn0file0L387-L389

### Correct interpretation

Topic hierarchy editing/reordering is part of the project.

The **exact UI mechanism** should be treated as a design choice unless it is already implemented/explicitly finalized.

---

### 2.10 "Study next" algorithm

The file specifies a simple heuristic for suggesting the next topic. fileciteturn0file0L247-L253

### Correct interpretation

A dashboard and progress view are core project features.

The exact "oldest incomplete leaf / lowest-progress skill" algorithm should be treated as a v1 heuristic rather than a fundamental requirement unless it has been explicitly finalized.

---

## 3. One terminology correction worth keeping in mind

The project is best described as a:

**Full-stack Skill Progress Tracker**

rather than only a "PDF parser."

PDF parsing is an important **input feature**, but the actual application is the broader tracker:

**skills + topic tree + leaf progress + notes + search + streak + dashboard + authentication + PDF import/editor + deployment.**

The uploaded specification itself reflects this broader architecture. fileciteturn0file0L17-L25

---

## 4. Final recommendation

### Original file status

**Keep the original file unchanged.**

It is sufficiently close to the project that rewriting it would create more risk than value.

### What to treat as authoritative

Use the original file for the **overall architecture and feature set**, but validate the following before calling the specification "100% final":

1. Exact AI provider for PDF parsing.
2. Exact search implementation.
3. Exact JWT expiration values.
4. Exact topic/skill deletion semantics.
5. Exact streak grace-rule behavior.
6. Exact PDF size/page limits and OCR policy.
7. Exact PDF parsing heuristics.
8. Exact re-upload behavior.
9. Exact topic-reordering UI.
10. Exact "Study Next" heuristic.

### Bottom line

**Estimated match: ~90–95%.**

It is **not safe to call it 100% verified** from the project information currently available to me.

The core project architecture, workflow, stack, major modules, and deployment model are correctly represented; most uncertainty is in detailed business/implementation decisions rather than in the identity of the project.
