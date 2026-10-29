// Package dbmigrate applies the embedded SQL migrations at service startup.
package dbmigrate

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	_ "github.com/lib/pq" // registers the "postgres" driver
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// lockKey is the fixed pg_advisory_lock key shared by every service that migrates.
const lockKey int64 = 7_262_024_001

const createTable = `CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`

var (
	lineComment  = regexp.MustCompile(`--[^\n]*`)
	blockComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	concurrently = regexp.MustCompile(`(?i)\bCONCURRENTLY\b`)
)

// Migration is one embedded SQL file.
type Migration struct {
	Version string // file name, e.g. 001_add_notifications_table.sql
	SQL     string
}

// AutoMigrateEnabled reports whether DB_AUTO_MIGRATE allows running (default true).
func AutoMigrateEnabled() bool {
	v := strings.TrimSpace(os.Getenv("DB_AUTO_MIGRATE"))
	if v == "" {
		return true
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return true
	}
	return b
}

// List returns the embedded migrations in lexical file-name order.
func List() ([]Migration, error) {
	return list(migrationsFS, "migrations")
}

func list(fsys fs.FS, dir string) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations: %w", err)
	}
	var out []Migration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		b, err := fs.ReadFile(fsys, dir+"/"+e.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", e.Name(), err)
		}
		out = append(out, Migration{Version: e.Name(), SQL: string(b)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

// NeedsNoTransaction reports whether the SQL uses CONCURRENTLY (outside comments),
// which PostgreSQL refuses to run inside a transaction block.
func NeedsNoTransaction(sqlText string) bool {
	s := blockComment.ReplaceAllString(sqlText, "")
	s = lineComment.ReplaceAllString(s, "")
	return concurrently.MatchString(s)
}

// Pending returns the migrations whose version is not in applied, preserving order.
func Pending(all []Migration, applied map[string]bool) []Migration {
	var out []Migration
	for _, m := range all {
		if !applied[m.Version] {
			out = append(out, m)
		}
	}
	return out
}

// Run brings the schema up to date. It is safe to call concurrently from several
// services: an advisory lock serialises them and already-applied files are skipped.
func Run(ctx context.Context, db *sql.DB, logger *slog.Logger) error {
	if !AutoMigrateEnabled() {
		logger.Info("Database migrations skipped (DB_AUTO_MIGRATE=false)")
		return nil
	}
	all, err := List()
	if err != nil {
		return err
	}

	// The advisory lock is session-scoped, so hold one connection for the whole run.
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("dbmigrate: get connection: %w", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", lockKey); err != nil {
		return fmt.Errorf("dbmigrate: acquire advisory lock: %w", err)
	}
	defer func() {
		// Fresh context: ctx may already be cancelled.
		if _, err := conn.ExecContext(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", lockKey); err != nil {
			logger.Warn("Failed to release migration advisory lock", "error", err)
		}
	}()

	if _, err := conn.ExecContext(ctx, createTable); err != nil {
		return fmt.Errorf("dbmigrate: create schema_migrations: %w", err)
	}

	applied, err := loadApplied(ctx, conn)
	if err != nil {
		return err
	}
	pending := Pending(all, applied)

	for _, m := range pending {
		if err := apply(ctx, conn, m); err != nil {
			return fmt.Errorf("dbmigrate: apply %s: %w", m.Version, err)
		}
		logger.Info("Applied migration", "version", m.Version)
	}
	logger.Info("Database migrations complete",
		"total", len(all), "applied", len(pending), "skipped", len(all)-len(pending))
	return nil
}

func loadApplied(ctx context.Context, conn *sql.Conn) (map[string]bool, error) {
	rows, err := conn.QueryContext(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("dbmigrate: read schema_migrations: %w", err)
	}
	defer rows.Close()
	applied := map[string]bool{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("dbmigrate: scan schema_migrations: %w", err)
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("dbmigrate: read schema_migrations: %w", err)
	}
	return applied, nil
}

const insertVersion = "INSERT INTO schema_migrations (version) VALUES ($1) ON CONFLICT DO NOTHING"

func apply(ctx context.Context, conn *sql.Conn, m Migration) error {
	if NeedsNoTransaction(m.SQL) {
		if _, err := conn.ExecContext(ctx, m.SQL); err != nil {
			return err
		}
		_, err := conn.ExecContext(ctx, insertVersion, m.Version)
		return err
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, m.SQL); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.ExecContext(ctx, insertVersion, m.Version); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// RunWithConnStr opens a short-lived connection, runs the migrations and closes it.
func RunWithConnStr(ctx context.Context, connStr string, logger *slog.Logger) error {
	if !AutoMigrateEnabled() {
		logger.Info("Database migrations skipped (DB_AUTO_MIGRATE=false)")
		return nil
	}
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return fmt.Errorf("dbmigrate: open database: %w", err)
	}
	defer db.Close()
	return Run(ctx, db, logger)
}
