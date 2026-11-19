package database

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"

	"skill-progress-tracker/backend-go/internal/config"
)

type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type DB struct {
	SQL     *sql.DB
	Dialect string // "postgres" or "sqlite"
}

func Open(cfg *config.Settings) (*DB, error) {
	if err := cfg.ValidateDatabaseConfig(); err != nil {
		return nil, err
	}
	if cfg.IsSQLite() {
		dsn := sqliteDSN(cfg.CleanDatabaseURL())
		sqlDB, err := sql.Open("sqlite", dsn)
		if err != nil {
			return nil, err
		}
		if strings.Contains(dsn, ":memory:") {
			sqlDB.SetMaxOpenConns(1)
			sqlDB.SetMaxIdleConns(1)
		} else {
			sqlDB.SetMaxOpenConns(1)
			sqlDB.SetMaxIdleConns(1)
		}
		db := &DB{SQL: sqlDB, Dialect: "sqlite"}
		if err := db.InitSQLiteSchema(context.Background()); err != nil {
			_ = sqlDB.Close()
			return nil, err
		}
		return db, nil
	}

	pgDSN := cfg.PostgresDSN()
	connCfg, err := pgx.ParseConfig(pgDSN)
	if err != nil {
		return nil, fmt.Errorf("invalid postgres dsn: %w", err)
	}
	// Use SimpleProtocol so Supabase Transaction Pooler (PgBouncer port 6543)
	// never encounters prepared statement collisions across ephemeral connections.
	connCfg.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	sqlDB := stdlib.OpenDB(*connCfg)

	if cfg.IsServerless() {
		// Serverless (Vercel): release idle connections immediately to avoid exhausting Supabase pooler
		sqlDB.SetMaxOpenConns(2)
		sqlDB.SetMaxIdleConns(0)
		sqlDB.SetConnMaxLifetime(60 * time.Second)
	} else {
		// Persistent server: matches Python pool_size=5, max_overflow=5, pool_recycle=300
		sqlDB.SetMaxOpenConns(10)
		sqlDB.SetMaxIdleConns(5)
		sqlDB.SetConnMaxLifetime(300 * time.Second)
	}

	return &DB{SQL: sqlDB, Dialect: "postgres"}, nil
}

func sqliteDSN(raw string) string {
	u := raw
	for _, prefix := range []string{"sqlite+aiosqlite:///", "sqlite:///", "sqlite+aiosqlite://", "sqlite://"} {
		if strings.HasPrefix(u, prefix) {
			u = strings.TrimPrefix(u, prefix)
			break
		}
	}
	if u == "" || u == ":memory:" {
		return "file::memory:?cache=shared&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	}
	sep := "?"
	if strings.Contains(u, "?") {
		sep = "&"
	}
	return u + sep + "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
}

func (db *DB) IsPostgres() bool {
	return db.Dialect == "postgres"
}

func (db *DB) IsSQLite() bool {
	return db.Dialect == "sqlite"
}

// Rebind converts '?' placeholders to '$1, $2, ...' when Dialect is "postgres".
func (db *DB) Rebind(query string) string {
	if db.Dialect != "postgres" {
		return query
	}
	var sb strings.Builder
	sb.Grow(len(query) + 16)
	paramIdx := 1
	inQuote := false
	for i := 0; i < len(query); i++ {
		ch := query[i]
		if ch == '\'' {
			inQuote = !inQuote
			sb.WriteByte(ch)
			continue
		}
		if ch == '?' && !inQuote {
			sb.WriteByte('$')
			sb.WriteString(strconv.Itoa(paramIdx))
			paramIdx++
			continue
		}
		sb.WriteByte(ch)
	}
	return sb.String()
}

func (db *DB) FormatTime(t time.Time) any {
	utc := t.UTC()
	if db.Dialect == "sqlite" {
		return utc.Format("2006-01-02 15:04:05.000000")
	}
	return utc
}

func (db *DB) FormatNullTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return db.FormatTime(*t)
}

func (db *DB) FormatUUID(u uuid.UUID) string {
	return u.String()
}

func (db *DB) FormatNullUUID(u *uuid.UUID) any {
	if u == nil {
		return nil
	}
	return u.String()
}

func (db *DB) Exec(ctx context.Context, q Querier, query string, args ...any) (sql.Result, error) {
	if q == nil {
		q = db.SQL
	}
	return q.ExecContext(ctx, db.Rebind(query), args...)
}

func (db *DB) Query(ctx context.Context, q Querier, query string, args ...any) (*sql.Rows, error) {
	if q == nil {
		q = db.SQL
	}
	return q.QueryContext(ctx, db.Rebind(query), args...)
}

func (db *DB) QueryRow(ctx context.Context, q Querier, query string, args ...any) *sql.Row {
	if q == nil {
		q = db.SQL
	}
	return q.QueryRowContext(ctx, db.Rebind(query), args...)
}

func (db *DB) WithTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := db.SQL.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if db.IsSQLite() {
		_, _ = tx.ExecContext(ctx, "PRAGMA foreign_keys = ON")
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func IsUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "duplicate key") ||
		strings.Contains(msg, "23505") ||
		strings.Contains(msg, "sqlite_constraint_unique")
}

// InitSQLiteSchema ensures all tables and columns exist when running on SQLite (local/test).
func (db *DB) InitSQLiteSchema(ctx context.Context) error {
	if !db.IsSQLite() {
		return nil
	}
	if _, err := db.SQL.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		return err
	}
	ddl := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id CHAR(36) PRIMARY KEY,
			email VARCHAR(320) NOT NULL,
			password_hash VARCHAR(255),
			google_id VARCHAR(255) UNIQUE,
			name VARCHAR(200) NOT NULL DEFAULT '',
			timezone VARCHAR(64) NOT NULL DEFAULT 'UTC',
			is_active BOOLEAN NOT NULL DEFAULT 1,
			must_change_password BOOLEAN NOT NULL DEFAULT 0,
			deletion_requested_at DATETIME,
			deletion_deadline DATETIME,
			last_active_at DATETIME,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			CONSTRAINT uq_users_email UNIQUE (email)
		)`,
		`CREATE TABLE IF NOT EXISTS skills (
			id CHAR(36) PRIMARY KEY,
			user_id CHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			name VARCHAR(200) NOT NULL,
			description TEXT,
			deleted_at DATETIME,
			purge_at DATETIME,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			CONSTRAINT uq_skills_user_name UNIQUE (user_id, name)
		)`,
		`CREATE INDEX IF NOT EXISTS ix_skills_user_id ON skills(user_id)`,
		`CREATE TABLE IF NOT EXISTS topics (
			id CHAR(36) PRIMARY KEY,
			skill_id CHAR(36) NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
			parent_id CHAR(36) REFERENCES topics(id) ON DELETE CASCADE,
			name VARCHAR(500) NOT NULL,
			description TEXT,
			order_index INTEGER NOT NULL DEFAULT 0,
			is_completed BOOLEAN NOT NULL DEFAULT 0,
			completed_at DATETIME,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS ix_topics_skill_parent_order ON topics(skill_id, parent_id, order_index)`,
		`CREATE INDEX IF NOT EXISTS ix_topics_parent_id ON topics(parent_id)`,
		`CREATE INDEX IF NOT EXISTS ix_topics_skill_id ON topics(skill_id)`,
		`CREATE TABLE IF NOT EXISTS notes (
			id CHAR(36) PRIMARY KEY,
			topic_id CHAR(36) NOT NULL REFERENCES topics(id) ON DELETE CASCADE,
			user_id CHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			content TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS ix_notes_topic_id ON notes(topic_id)`,
		`CREATE INDEX IF NOT EXISTS ix_notes_user_id ON notes(user_id)`,
		`CREATE TABLE IF NOT EXISTS activity_log (
			id CHAR(36) PRIMARY KEY,
			user_id CHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			action_type VARCHAR(32) NOT NULL,
			topic_id CHAR(36) REFERENCES topics(id) ON DELETE SET NULL,
			skill_id CHAR(36) REFERENCES skills(id) ON DELETE SET NULL,
			topic_name_snapshot TEXT,
			skill_name_snapshot TEXT,
			parent_name_snapshot TEXT,
			summary TEXT,
			occurred_at DATETIME NOT NULL,
			activity_date DATE NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS ix_activity_user_date ON activity_log(user_id, activity_date)`,
		`CREATE INDEX IF NOT EXISTS ix_activity_user_type_date ON activity_log(user_id, action_type, activity_date)`,
		`CREATE INDEX IF NOT EXISTS ix_activity_user_occurred ON activity_log(user_id, occurred_at)`,
		`CREATE TABLE IF NOT EXISTS personal_notes (
			id CHAR(36) PRIMARY KEY,
			user_id CHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			title VARCHAR(300) NOT NULL DEFAULT 'Untitled note',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS ix_personal_notes_user_id ON personal_notes(user_id)`,
		`CREATE TABLE IF NOT EXISTS personal_note_tasks (
			id CHAR(36) PRIMARY KEY,
			note_id CHAR(36) NOT NULL REFERENCES personal_notes(id) ON DELETE CASCADE,
			content TEXT NOT NULL DEFAULT '',
			is_completed BOOLEAN NOT NULL DEFAULT 0,
			order_index INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS ix_personal_note_tasks_note_id ON personal_note_tasks(note_id)`,
		`CREATE TABLE IF NOT EXISTS import_jobs (
			id CHAR(36) PRIMARY KEY,
			user_id CHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			skill_id CHAR(36) REFERENCES skills(id) ON DELETE SET NULL,
			original_filename VARCHAR(500) NOT NULL,
			file_size_bytes INTEGER NOT NULL,
			status VARCHAR(32) NOT NULL DEFAULT 'uploaded',
			confidence_notes JSON,
			error_message TEXT,
			generated_tree_json JSON,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS ix_import_jobs_user_id ON import_jobs(user_id)`,
		`CREATE TABLE IF NOT EXISTS refresh_tokens (
			id CHAR(36) PRIMARY KEY,
			user_id CHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			token_hash VARCHAR(255) NOT NULL,
			expires_at DATETIME NOT NULL,
			revoked BOOLEAN NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS ix_refresh_tokens_user_id ON refresh_tokens(user_id)`,
		`CREATE INDEX IF NOT EXISTS ix_refresh_tokens_token_hash ON refresh_tokens(token_hash)`,
		`CREATE TABLE IF NOT EXISTS password_reset_tokens (
			id CHAR(36) PRIMARY KEY,
			user_id CHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			code_hash VARCHAR(255) NOT NULL,
			expires_at DATETIME NOT NULL,
			used BOOLEAN NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS ix_password_reset_tokens_user_id ON password_reset_tokens(user_id)`,
		`CREATE TABLE IF NOT EXISTS skill_timers (
			id CHAR(36) PRIMARY KEY,
			user_id CHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			skill_id CHAR(36) NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
			from_date DATE NOT NULL,
			to_date DATE NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			CONSTRAINT uq_skill_timers_user_skill UNIQUE (user_id, skill_id)
		)`,
		`CREATE INDEX IF NOT EXISTS ix_skill_timers_user_id ON skill_timers(user_id)`,
		`CREATE INDEX IF NOT EXISTS ix_skill_timers_skill_id ON skill_timers(skill_id)`,
	}
	for _, stmt := range ddl {
		if _, err := db.SQL.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}

	// Ensure incremental columns exist on older SQLite files (matching backend/app/main.py)
	alterChecks := []struct {
		table  string
		column string
		ddl    string
	}{
		{"activity_log", "parent_name_snapshot", "ALTER TABLE activity_log ADD COLUMN parent_name_snapshot TEXT"},
		{"activity_log", "summary", "ALTER TABLE activity_log ADD COLUMN summary TEXT"},
		{"users", "must_change_password", "ALTER TABLE users ADD COLUMN must_change_password BOOLEAN NOT NULL DEFAULT 0"},
		{"users", "deletion_requested_at", "ALTER TABLE users ADD COLUMN deletion_requested_at DATETIME"},
		{"users", "deletion_deadline", "ALTER TABLE users ADD COLUMN deletion_deadline DATETIME"},
		{"users", "last_active_at", "ALTER TABLE users ADD COLUMN last_active_at DATETIME"},
		{"skills", "deleted_at", "ALTER TABLE skills ADD COLUMN deleted_at DATETIME"},
		{"skills", "purge_at", "ALTER TABLE skills ADD COLUMN purge_at DATETIME"},
	}
	for _, chk := range alterChecks {
		has, err := db.sqliteHasColumn(ctx, chk.table, chk.column)
		if err == nil && !has {
			_, _ = db.SQL.ExecContext(ctx, chk.ddl)
		}
	}
	return nil
}

func (db *DB) sqliteHasColumn(ctx context.Context, table, column string) (bool, error) {
	rows, err := db.SQL.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull int
		var dflt any
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false, err
		}
		if strings.EqualFold(name, column) {
			return true, nil
		}
	}
	return false, nil
}

// ResetForTesting drops and recreates all tables in an isolated SQLite test database.
// Refuses to run on PostgreSQL or outside test environment.
func (db *DB) ResetForTesting(ctx context.Context, cfg *config.Settings) error {
	if !cfg.IsTest() || !db.IsSQLite() {
		return fmt.Errorf("ResetForTesting is only allowed on SQLite in test environment")
	}
	_, _ = db.SQL.ExecContext(ctx, "PRAGMA foreign_keys = OFF")
	tables := []string{
		"skill_timers",
		"password_reset_tokens",
		"refresh_tokens",
		"import_jobs",
		"personal_note_tasks",
		"personal_notes",
		"activity_log",
		"notes",
		"topics",
		"skills",
		"users",
	}
	for _, t := range tables {
		if _, err := db.SQL.ExecContext(ctx, "DROP TABLE IF EXISTS "+t); err != nil {
			return err
		}
	}
	_, _ = db.SQL.ExecContext(ctx, "PRAGMA foreign_keys = ON")
	return db.InitSQLiteSchema(ctx)
}
