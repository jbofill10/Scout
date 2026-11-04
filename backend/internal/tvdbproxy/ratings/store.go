package ratings

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/lib/pq"
)

// Store keeps answers across restarts; PostgresStore implements it. Get
// returns the entries held for the keys asked, expired ones included, so a
// stale answer can stand in while a fresh lookup runs. Put adds or replaces
// entries.
type Store interface {
	Get(ctx context.Context, keys []string) (map[string]Entry, error)
	Put(ctx context.Context, entries map[string]Entry) error
}

// Entry is one stored answer: its JSON and when it stops being current.
type Entry struct {
	Value   json.RawMessage
	Expires time.Time
}

// PostgresStore keeps answers in the RatingsCache table of the PostgreSQL
// the other services share, so the scores and ids looked up for the library
// and the popular rows build up across restarts instead of being asked for
// again; a free OMDb key allows 1,000 lookups a day. Keys are namespaced by
// the cache that owns them ("omdb:tt0111161", "imdb-id:movies:358595").
type PostgresStore struct {
	db *sql.DB
}

// NewPostgresStore returns a store on db, whose schema dbmigrate keeps.
func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

// Get returns the entries held for the keys, expired ones included.
func (s *PostgresStore) Get(ctx context.Context, keys []string) (map[string]Entry, error) {
	entries := make(map[string]Entry, len(keys))
	if len(keys) == 0 {
		return entries, nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT key, value::text, expires_at FROM RatingsCache WHERE key = ANY($1)`, pq.Array(keys))
	if err != nil {
		return nil, fmt.Errorf("ratings store: read: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			key, value string
			expires    time.Time
		)
		if err := rows.Scan(&key, &value, &expires); err != nil {
			return nil, fmt.Errorf("ratings store: scan: %w", err)
		}
		entries[key] = Entry{Value: json.RawMessage(value), Expires: expires}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ratings store: read: %w", err)
	}
	return entries, nil
}

// Put adds or replaces the entries in one transaction, in key order so two
// writers of the same keys cannot deadlock.
func (s *PostgresStore) Put(ctx context.Context, entries map[string]Entry) error {
	if len(entries) == 0 {
		return nil
	}
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("ratings store: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO RatingsCache (key, value, expires_at, updated_at)
		VALUES ($1, $2::jsonb, $3, now())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, expires_at = EXCLUDED.expires_at, updated_at = now()`)
	if err != nil {
		return fmt.Errorf("ratings store: prepare: %w", err)
	}
	defer stmt.Close()
	for _, key := range keys {
		entry := entries[key]
		if _, err := stmt.ExecContext(ctx, key, string(entry.Value), entry.Expires); err != nil {
			return fmt.Errorf("ratings store: write %s: %w", key, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("ratings store: commit: %w", err)
	}
	return nil
}

// Prune deletes the entries that expired before cutoff and reports how many.
func (s *PostgresStore) Prune(ctx context.Context, cutoff time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM RatingsCache WHERE expires_at < $1`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("ratings store: prune: %w", err)
	}
	pruned, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("ratings store: prune: %w", err)
	}
	return pruned, nil
}

// Count reports how many entries the store holds.
func (s *PostgresStore) Count(ctx context.Context) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM RatingsCache`).Scan(&count); err != nil {
		return 0, fmt.Errorf("ratings store: count: %w", err)
	}
	return count, nil
}

// Close releases the database connections.
func (s *PostgresStore) Close() error {
	return s.db.Close()
}
