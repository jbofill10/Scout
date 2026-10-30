// Package settings resolves Scout's runtime settings for the torrenter: the
// environment supplies the defaults, the Settings table supplies overrides a
// user made from the Settings page, and the Resolver hands the services the
// effective value through a short-lived cache.
package settings

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// Store is the Settings table: one JSON value per key.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Load returns every stored override.
func (s *Store) Load(ctx context.Context) (map[string]json.RawMessage, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM Settings`)
	if err != nil {
		return nil, fmt.Errorf("load settings: %w", err)
	}
	defer rows.Close()

	out := map[string]json.RawMessage{}
	for rows.Next() {
		var key string
		var value []byte
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("scan setting: %w", err)
		}
		out[key] = json.RawMessage(value)
	}
	return out, rows.Err()
}

// Set writes one override, replacing any previous value.
func (s *Store) Set(ctx context.Context, key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode setting %s: %w", key, err)
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO Settings (key, value, updated_at) VALUES ($1, $2, now())
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`, key, raw)
	if err != nil {
		return fmt.Errorf("save setting %s: %w", key, err)
	}
	return nil
}

// Delete removes an override, so the environment default applies again.
func (s *Store) Delete(ctx context.Context, key string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM Settings WHERE key = $1`, key); err != nil {
		return fmt.Errorf("reset setting %s: %w", key, err)
	}
	return nil
}
