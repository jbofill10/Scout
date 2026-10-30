package dbmigrate

import (
	"bytes"
	"context"
	"log/slog"
	"sort"
	"testing"
	"testing/fstest"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestListOrdered(t *testing.T) {
	ms, err := List()
	require.NoError(t, err)
	require.NotEmpty(t, ms)
	names := make([]string, len(ms))
	for i, m := range ms {
		names[i] = m.Version
		require.NotEmpty(t, m.SQL)
	}
	require.True(t, sort.StringsAreSorted(names))
	idx := func(n string) int { return sort.SearchStrings(names, n) }
	require.Less(t, idx("001_add_notifications_table.sql"), idx("001_add_notifications_tvdb_id_index.sql"))
	require.Equal(t, "001_add_notifications_table.sql", names[0])
}

func TestListSortsAndFilters(t *testing.T) {
	fsys := fstest.MapFS{
		"m/002_b.sql": {Data: []byte("b")},
		"m/001_a.sql": {Data: []byte("a")},
		"m/notes.txt": {Data: []byte("x")},
	}
	ms, err := list(fsys, "m")
	require.NoError(t, err)
	require.Len(t, ms, 2)
	require.Equal(t, "001_a.sql", ms[0].Version)
}

func TestNeedsNoTransaction(t *testing.T) {
	require.True(t, NeedsNoTransaction("CREATE INDEX CONCURRENTLY i ON t(a);"))
	require.True(t, NeedsNoTransaction("create index concurrently i on t(a);"))
	require.False(t, NeedsNoTransaction("-- CONCURRENTLY is mentioned only here\nCREATE INDEX i ON t(a);"))
	require.False(t, NeedsNoTransaction("/* CONCURRENTLY */ SELECT 1;"))
	require.False(t, NeedsNoTransaction("CREATE INDEX i ON t(a);"))
}

func TestEmbeddedMigrationsHaveNoConcurrently(t *testing.T) {
	// Documents the current state; if one is added Run handles it outside a tx.
	ms, _ := List()
	for _, m := range ms {
		require.False(t, NeedsNoTransaction(m.SQL), m.Version)
	}
}

func TestPending(t *testing.T) {
	all := []Migration{{Version: "001_a.sql"}, {Version: "002_b.sql"}, {Version: "003_c.sql"}}
	got := Pending(all, map[string]bool{"001_a.sql": true, "003_c.sql": true})
	require.Len(t, got, 1)
	require.Equal(t, "002_b.sql", got[0].Version)
	require.Empty(t, Pending(all, map[string]bool{"001_a.sql": true, "002_b.sql": true, "003_c.sql": true}))
}

func TestAutoMigrateEnabled(t *testing.T) {
	t.Setenv("DB_AUTO_MIGRATE", "")
	require.True(t, AutoMigrateEnabled())
	t.Setenv("DB_AUTO_MIGRATE", "false")
	require.False(t, AutoMigrateEnabled())
	t.Setenv("DB_AUTO_MIGRATE", "true")
	require.True(t, AutoMigrateEnabled())
}

func TestRunDisabledTouchesNothing(t *testing.T) {
	t.Setenv("DB_AUTO_MIGRATE", "false")
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	var buf bytes.Buffer
	require.NoError(t, Run(context.Background(), db, slog.New(slog.NewTextHandler(&buf, nil))))
	require.Contains(t, buf.String(), "skipped")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRunSkipsApplied(t *testing.T) {
	t.Setenv("DB_AUTO_MIGRATE", "true")
	all, err := List()
	require.NoError(t, err)

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	mock.ExpectExec("pg_advisory_lock").WithArgs(lockKey).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS schema_migrations").WillReturnResult(sqlmock.NewResult(0, 0))
	rows := sqlmock.NewRows([]string{"version"})
	for _, m := range all[:len(all)-1] { // all but the last are applied
		rows.AddRow(m.Version)
	}
	mock.ExpectQuery("SELECT version FROM schema_migrations").WillReturnRows(rows)
	last := all[len(all)-1]
	mock.ExpectBegin()
	mock.ExpectExec(".").WillReturnResult(sqlmock.NewResult(0, 0)) // migration body
	mock.ExpectExec("INSERT INTO schema_migrations").WithArgs(last.Version).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectExec("pg_advisory_unlock").WithArgs(lockKey).WillReturnResult(sqlmock.NewResult(0, 0))

	var buf bytes.Buffer
	require.NoError(t, Run(context.Background(), db, slog.New(slog.NewTextHandler(&buf, nil))))
	require.NoError(t, mock.ExpectationsWereMet())
	require.Contains(t, buf.String(), "Applied migration")
	require.Contains(t, buf.String(), last.Version)
}

func TestRunAllAppliedIsNoop(t *testing.T) {
	t.Setenv("DB_AUTO_MIGRATE", "true")
	all, _ := List()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	mock.ExpectExec("pg_advisory_lock").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS schema_migrations").WillReturnResult(sqlmock.NewResult(0, 0))
	rows := sqlmock.NewRows([]string{"version"})
	for _, m := range all {
		rows.AddRow(m.Version)
	}
	mock.ExpectQuery("SELECT version").WillReturnRows(rows)
	mock.ExpectExec("pg_advisory_unlock").WillReturnResult(sqlmock.NewResult(0, 0))
	require.NoError(t, Run(context.Background(), db, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRunFailureNamesFileAndRollsBack(t *testing.T) {
	t.Setenv("DB_AUTO_MIGRATE", "true")
	all, _ := List()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	mock.ExpectExec("pg_advisory_lock").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS schema_migrations").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT version").WillReturnRows(sqlmock.NewRows([]string{"version"}))
	mock.ExpectBegin()
	mock.ExpectExec(".").WillReturnError(context.DeadlineExceeded)
	mock.ExpectRollback()
	mock.ExpectExec("pg_advisory_unlock").WillReturnResult(sqlmock.NewResult(0, 0))
	err = Run(context.Background(), db, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	require.Error(t, err)
	require.Contains(t, err.Error(), all[0].Version)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NoError(t, mock.ExpectationsWereMet())
}
