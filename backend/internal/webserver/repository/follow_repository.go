package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"
)

// FollowedShow is a show whose new episodes Scout requests on its own.
type FollowedShow struct {
	TvdbID    string `json:"tvdb_id"`
	Title     string `json:"title"`
	PosterUrl string `json:"poster_url,omitempty"`
	// Media is the request payload the follow check builds on (no episodes).
	Media          tvdb.Media `json:"-"`
	FollowedAt     time.Time  `json:"followed_at"`
	IncludeMissing bool       `json:"include_missing"`
	LastCheckedAt  *time.Time `json:"last_checked_at,omitempty"`
	LastCheckError string     `json:"last_check_error,omitempty"`
	// EpisodesAdded counts every episode a check has requested or scheduled.
	EpisodesAdded int `json:"episodes_added"`
}

// FollowRepository stores followed shows.
type FollowRepository interface {
	Upsert(ctx context.Context, show FollowedShow) error
	Delete(ctx context.Context, tvdbID string) (bool, error)
	List(ctx context.Context) ([]FollowedShow, error)
	Get(ctx context.Context, tvdbID string) (*FollowedShow, error)
	// RecordCheck notes a check's outcome and adds to the running episode count.
	RecordCheck(ctx context.Context, tvdbID string, added int, checkErr string) error
}

type FollowRepo struct {
	db     *sql.DB
	logger *slog.Logger
}

func NewFollowRepo(logger *slog.Logger, connStr string) (*FollowRepo, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	return &FollowRepo{db: db, logger: logger}, nil
}

const followColumns = `tvdb_id, title, poster_url, media, followed_at, include_missing, last_checked_at, last_check_error, episodes_added`

func (r *FollowRepo) Upsert(ctx context.Context, show FollowedShow) error {
	mediaJSON, err := json.Marshal(show.Media)
	if err != nil {
		return fmt.Errorf("failed to marshal media: %w", err)
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO FollowedShows (tvdb_id, title, poster_url, media, include_missing)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (tvdb_id) DO UPDATE SET
		     title = EXCLUDED.title, poster_url = EXCLUDED.poster_url, media = EXCLUDED.media,
		     include_missing = EXCLUDED.include_missing`,
		show.TvdbID, show.Title, show.PosterUrl, mediaJSON, show.IncludeMissing)
	if err != nil {
		return fmt.Errorf("failed to save followed show: %w", err)
	}
	return nil
}

func (r *FollowRepo) Delete(ctx context.Context, tvdbID string) (bool, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM FollowedShows WHERE tvdb_id = $1`, tvdbID)
	if err != nil {
		return false, fmt.Errorf("failed to unfollow show: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (r *FollowRepo) List(ctx context.Context) ([]FollowedShow, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+followColumns+` FROM FollowedShows ORDER BY lower(title)`)
	if err != nil {
		return nil, fmt.Errorf("failed to list followed shows: %w", err)
	}
	defer rows.Close()

	out := []FollowedShow{}
	for rows.Next() {
		show, err := scanFollowedShow(rows)
		if err != nil {
			r.logger.ErrorContext(ctx, "Failed to scan followed show", "error", err)
			continue
		}
		out = append(out, show)
	}
	return out, rows.Err()
}

func (r *FollowRepo) Get(ctx context.Context, tvdbID string) (*FollowedShow, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+followColumns+` FROM FollowedShows WHERE tvdb_id = $1`, tvdbID)
	if err != nil {
		return nil, fmt.Errorf("failed to load followed show: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	show, err := scanFollowedShow(rows)
	if err != nil {
		return nil, err
	}
	return &show, nil
}

func (r *FollowRepo) RecordCheck(ctx context.Context, tvdbID string, added int, checkErr string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE FollowedShows
		 SET last_checked_at = now(), last_check_error = NULLIF($2, ''), episodes_added = episodes_added + $3
		 WHERE tvdb_id = $1`, tvdbID, checkErr, added)
	if err != nil {
		return fmt.Errorf("failed to record follow check: %w", err)
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanFollowedShow(rs rowScanner) (FollowedShow, error) {
	var show FollowedShow
	var poster, lastErr sql.NullString
	var mediaJSON []byte
	var checked sql.NullTime
	if err := rs.Scan(&show.TvdbID, &show.Title, &poster, &mediaJSON, &show.FollowedAt, &show.IncludeMissing,
		&checked, &lastErr, &show.EpisodesAdded); err != nil {
		return show, err
	}
	show.PosterUrl = poster.String
	show.LastCheckError = lastErr.String
	if checked.Valid {
		t := checked.Time
		show.LastCheckedAt = &t
	}
	if err := json.Unmarshal(mediaJSON, &show.Media); err != nil {
		return show, fmt.Errorf("failed to unmarshal followed show media: %w", err)
	}
	return show, nil
}
