# Implementation decisions

The spec (`skill_progress_tracker_build_spec.md`) is the source of truth for architecture and business rules. `progress_tracker_md_corrections.md` flags some details as unverified / implementation-specific. Those are resolved here rather than silently dropped.

| Item | Spec default | Decision | Why |
|---|---|---|---|
| AI provider | Anthropic Claude | **Anthropic Claude**, optional at runtime | Spec Part 2/6; corrections §2.1. If `ANTHROPIC_API_KEY` is unset, low-confidence sections stay in the proposed tree with a `needs_review` flag instead of calling AI. |
| Search | Postgres `tsvector` + GIN | **Same in production Postgres**. Tests use SQL `LIKE` on SQLite. | Corrections §2.2. |
| JWT timings | 15 min access / 30 day refresh | **Adopted as specified** | Corrections §2.3; values are env-overridable. |
| Topic/skill deletion | Confirm + cascade; `activity_log`/`import_jobs` SET NULL | **Adopted as specified** (Part 4.5) | Corrections §2.4. Account delete remains hard-delete of everything. |
| Streak grace | Miss 1 day OK; miss 2+ consecutive days resets | **Adopted as specified** (Part 5) | Corrections §2.5. |
| PDF limits / OCR | 10MB, ~200 pages, no OCR | **Adopted as specified** | Corrections §2.6. |
| PDF heuristics | Decimal numbering, UNIT/Chapter, bullets, Theory/Labs | **Adopted as specified** (Part 6.2) | Corrections §2.7. |
| Re-upload PDF | Always a new Skill | **Adopted as specified** (6.6) | Corrections §2.8. |
| Reorder / rename / delete topics | Spec had up/down, move-to-parent, rename, delete | **Removed from product UI and topic APIs** (user request). Add-child remains as `+`. Branches stay collapsed by default so large trees do not freeze the page. |
| Study next | Oldest incomplete leaf (tree order) | **Oldest incomplete leaf by `(skill.created_at, topic.order path)`**; if none, skill with lowest % that still has incomplete leaves | Corrections §2.10. |
| Circular-move HTTP status | 409 or 422 | **422** | Consistent validation failure, not a conflict with an existing resource. |
| Other-user resource | 403 or 404 | **404** | Avoid leaking whether an ID exists. |
| Primary keys | Unspecified | **UUID** | Opaque IDs in URLs. |
| Password hashing | bcrypt or argon2 | **Argon2** | Spec allows either. |
| Password-reset storage | Not in Part 3 | Extra table `password_reset_tokens` | Needed for single-use time-limited codes (Part 9). |
| Background parsing | Unspecified queue | FastAPI `BackgroundTasks` | Single Render instance; no Redis for v1. |
| Local/CI database | Postgres required in prod | **SQLite + aiosqlite for automated tests** when Postgres is unavailable | No Docker in this environment. Alembic migrations target Postgres. |
| Reset-code delivery | Email | SMTP if configured; otherwise log a redacted notice in development only (never the raw code in production logs) | Needed to complete the flow without a mail vendor locked in. |

Unverified items were **not** removed; spec defaults were kept unless noted.
