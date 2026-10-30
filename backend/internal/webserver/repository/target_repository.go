package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/lib/pq"
)

// Target kinds.
const (
	TargetWebhook = "webhook"
	TargetNtfy    = "ntfy"
	TargetDiscord = "discord"
)

// TargetEvents are the notification statuses a target can subscribe to.
var TargetEvents = []string{"completed", "failed", "cancelled"}

// NotificationTarget is somewhere download outcomes are sent.
type NotificationTarget struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
	URL  string `json:"url"`
	// Token is never returned by the API; HasToken says whether one is set.
	Token      string     `json:"-"`
	HasToken   bool       `json:"has_token"`
	Events     []string   `json:"events"`
	Enabled    bool       `json:"enabled"`
	LastSentAt *time.Time `json:"last_sent_at,omitempty"`
	LastError  string     `json:"last_error,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// Wants reports whether the target subscribes to an event.
func (t NotificationTarget) Wants(event string) bool {
	for _, e := range t.Events {
		if e == event {
			return true
		}
	}
	return false
}

// TargetRepository stores notification targets.
type TargetRepository interface {
	List(ctx context.Context) ([]NotificationTarget, error)
	Get(ctx context.Context, id int) (*NotificationTarget, error)
	Create(ctx context.Context, t NotificationTarget) (NotificationTarget, error)
	// Update replaces name, kind, url, events and enabled; the token only
	// when keepToken is false.
	Update(ctx context.Context, t NotificationTarget, keepToken bool) (bool, error)
	Delete(ctx context.Context, id int) (bool, error)
	RecordResult(ctx context.Context, id int, sentAt time.Time, errMsg string) error
}

type TargetRepo struct {
	db     *sql.DB
	logger *slog.Logger
}

func NewTargetRepo(logger *slog.Logger, connStr string) (*TargetRepo, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	return &TargetRepo{db: db, logger: logger}, nil
}

const targetColumns = `id, name, kind, url, COALESCE(token, ''), events, enabled, last_sent_at, COALESCE(last_error, ''), created_at`

func scanTarget(rs rowScanner) (NotificationTarget, error) {
	var t NotificationTarget
	var sent sql.NullTime
	if err := rs.Scan(&t.ID, &t.Name, &t.Kind, &t.URL, &t.Token, pq.Array(&t.Events), &t.Enabled, &sent, &t.LastError, &t.CreatedAt); err != nil {
		return t, err
	}
	if sent.Valid {
		at := sent.Time
		t.LastSentAt = &at
	}
	t.HasToken = t.Token != ""
	if t.Events == nil {
		t.Events = []string{}
	}
	return t, nil
}

func (r *TargetRepo) List(ctx context.Context) ([]NotificationTarget, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+targetColumns+` FROM NotificationTargets ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("failed to list notification targets: %w", err)
	}
	defer rows.Close()
	out := []NotificationTarget{}
	for rows.Next() {
		t, err := scanTarget(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *TargetRepo) Get(ctx context.Context, id int) (*NotificationTarget, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+targetColumns+` FROM NotificationTargets WHERE id = $1`, id)
	if err != nil {
		return nil, fmt.Errorf("failed to load notification target: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	t, err := scanTarget(rows)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *TargetRepo) Create(ctx context.Context, t NotificationTarget) (NotificationTarget, error) {
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO NotificationTargets (name, kind, url, token, events, enabled)
		 VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6) RETURNING id, created_at`,
		t.Name, t.Kind, t.URL, t.Token, pq.Array(t.Events), t.Enabled).Scan(&t.ID, &t.CreatedAt)
	if err != nil {
		return t, fmt.Errorf("failed to create notification target: %w", err)
	}
	t.HasToken = t.Token != ""
	return t, nil
}

func (r *TargetRepo) Update(ctx context.Context, t NotificationTarget, keepToken bool) (bool, error) {
	var res sql.Result
	var err error
	if keepToken {
		res, err = r.db.ExecContext(ctx,
			`UPDATE NotificationTargets SET name = $1, kind = $2, url = $3, events = $4, enabled = $5 WHERE id = $6`,
			t.Name, t.Kind, t.URL, pq.Array(t.Events), t.Enabled, t.ID)
	} else {
		res, err = r.db.ExecContext(ctx,
			`UPDATE NotificationTargets SET name = $1, kind = $2, url = $3, events = $4, enabled = $5, token = NULLIF($6, '') WHERE id = $7`,
			t.Name, t.Kind, t.URL, pq.Array(t.Events), t.Enabled, t.Token, t.ID)
	}
	if err != nil {
		return false, fmt.Errorf("failed to update notification target: %w", err)
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (r *TargetRepo) Delete(ctx context.Context, id int) (bool, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM NotificationTargets WHERE id = $1`, id)
	if err != nil {
		return false, fmt.Errorf("failed to delete notification target: %w", err)
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (r *TargetRepo) RecordResult(ctx context.Context, id int, sentAt time.Time, errMsg string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE NotificationTargets SET last_sent_at = CASE WHEN $2 = '' THEN $3 ELSE last_sent_at END, last_error = NULLIF($2, '') WHERE id = $1`,
		id, errMsg, sentAt)
	if err != nil {
		return fmt.Errorf("failed to record delivery result: %w", err)
	}
	return nil
}
