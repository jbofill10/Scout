//go:build integration

package repository

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"testing"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// These tests exercise the prune queries against a real Postgres, because the two ways
// pruning can go wrong are both SQL semantics that sqlmock cannot evaluate:
//
//   - `x <> ALL(ARRAY[]::text[])` is vacuously TRUE, so an empty keep-set deletes everything.
//   - `NULL <> ALL(...)` is NULL, so rows with a NULL parentId match nothing and can never
//     be pruned.
//
// Run with: go test -tags=integration ./internal/torrenter/repository/
// Requires DB_HOST, DB_USER, DB_PASSWORD, DB_NAME. Everything runs inside a throwaway
// schema that is dropped afterwards, so live tables are never touched.
func newPruneTestRepo(t *testing.T) (*Repo, func()) {
	t.Helper()

	host, user := os.Getenv("DB_HOST"), os.Getenv("DB_USER")
	pass, name := os.Getenv("DB_PASSWORD"), os.Getenv("DB_NAME")
	if host == "" || user == "" || name == "" {
		t.Skip("DB_HOST/DB_USER/DB_NAME not set; skipping prune integration test")
	}

	schema := fmt.Sprintf("prune_test_%d", os.Getpid())
	dsn := fmt.Sprintf("host=%s port=5432 user=%s password=%s dbname=%s sslmode=disable search_path=%s",
		host, user, pass, name, schema)

	admin, err := sql.Open("postgres", fmt.Sprintf(
		"host=%s port=5432 user=%s password=%s dbname=%s sslmode=disable", host, user, pass, name))
	require.NoError(t, err)
	_, err = admin.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE; CREATE SCHEMA " + schema)
	require.NoError(t, err)

	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	require.NoError(t, db.Ping())

	_, err = db.Exec(`
		CREATE TABLE Shows (id TEXT PRIMARY KEY, title TEXT, show_meta TEXT, thumb TEXT,
			tvdb_id TEXT, base_directory TEXT);
		CREATE TABLE Seasons (id TEXT PRIMARY KEY, parentId TEXT REFERENCES Shows(id),
			season_meta TEXT, season_number INTEGER, tvdb_id TEXT);
		CREATE TABLE Episodes (id TEXT PRIMARY KEY, parentId TEXT REFERENCES Seasons(id),
			episode_meta TEXT, episode_number INTEGER, tvdb_id TEXT);
		CREATE TABLE EpisodeMedia (id SERIAL PRIMARY KEY, parentId TEXT REFERENCES Episodes(id),
			video_resolution TEXT, file_path TEXT);
		CREATE TABLE Movies (id TEXT PRIMARY KEY, title TEXT, year INTEGER, thumb TEXT,
			art TEXT, tvdb_id TEXT, base_directory TEXT);
		CREATE TABLE MovieMedia (id SERIAL PRIMARY KEY, parentId TEXT REFERENCES Movies(id),
			video_resolution TEXT, file_path TEXT);
	`)
	require.NoError(t, err)

	repo := &Repo{db: db, logger: slog.New(slog.NewTextHandler(new(bytes.Buffer), nil))}

	return repo, func() {
		db.Close()
		_, _ = admin.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE")
		admin.Close()
	}
}

func countRows(t *testing.T, repo *Repo, table string) int {
	t.Helper()
	var n int
	require.NoError(t, repo.db.QueryRow("SELECT count(*) FROM "+table).Scan(&n))
	return n
}

// TestIntegrationPrune_EmptyChildLevelDoesNotWipeTables is the data-loss case: shows come
// back but seasons do not. Without the per-level empty guard the Seasons/Episodes/
// EpisodeMedia deletes run with an empty keep-set and empty every one of those tables.
func TestIntegrationPrune_EmptyChildLevelDoesNotWipeTables(t *testing.T) {
	repo, cleanup := newPruneTestRepo(t)
	defer cleanup()

	_, err := repo.db.Exec(`
		INSERT INTO Shows (id, title) VALUES ('show1', 'Keep Me');
		INSERT INTO Seasons (id, parentId, season_number) VALUES ('s1', 'show1', 1);
		INSERT INTO Episodes (id, parentId, episode_number) VALUES ('e1', 's1', 1);
		INSERT INTO EpisodeMedia (parentId, file_path) VALUES ('e1', '/data/e1.mkv');
	`)
	require.NoError(t, err)

	// A show with no seasons at all — exactly the shape that triggers the footgun.
	err = repo.PruneMissingShows(context.Background(), &models.PlexShowLibraryData{
		Shows: []models.PlexShowData{{Id: "show1"}},
	})
	require.NoError(t, err)

	require.Equal(t, 1, countRows(t, repo, "Shows"), "show should survive")
	require.Equal(t, 1, countRows(t, repo, "Seasons"), "seasons must not be wiped by an empty keep-set")
	require.Equal(t, 1, countRows(t, repo, "Episodes"), "episodes must not be wiped by an empty keep-set")
	require.Equal(t, 1, countRows(t, repo, "EpisodeMedia"), "media must not be wiped by an empty keep-set")
}

// TestIntegrationPrune_RemovesNullParentMedia proves the IS NULL arm works: without it,
// `NULL <> ALL(...)` is NULL and the orphan row is un-prunable forever.
func TestIntegrationPrune_RemovesNullParentMedia(t *testing.T) {
	repo, cleanup := newPruneTestRepo(t)
	defer cleanup()

	_, err := repo.db.Exec(`
		INSERT INTO Shows (id, title) VALUES ('show1', 'Keep Me');
		INSERT INTO Seasons (id, parentId, season_number) VALUES ('s1', 'show1', 1);
		INSERT INTO Episodes (id, parentId, episode_number) VALUES ('e1', 's1', 1);
		INSERT INTO EpisodeMedia (parentId, file_path) VALUES ('e1', '/data/e1.mkv');
		INSERT INTO EpisodeMedia (parentId, file_path) VALUES (NULL, '/data/orphan.mkv');
	`)
	require.NoError(t, err)
	require.Equal(t, 2, countRows(t, repo, "EpisodeMedia"))

	err = repo.PruneMissingShows(context.Background(), &models.PlexShowLibraryData{
		Shows: []models.PlexShowData{{
			Id:      "show1",
			Seasons: []models.PlexSeasonData{{Id: "s1", Episodes: []models.PlexEpisodeData{{Id: "e1"}}}},
		}},
	})
	require.NoError(t, err)

	require.Equal(t, 1, countRows(t, repo, "EpisodeMedia"), "the NULL-parent orphan should be pruned")
	require.Equal(t, 1, countRows(t, repo, "Episodes"))
}

// TestIntegrationPrune_RemovesStaleRows is the ordinary path, end to end against real FKs:
// rows absent from the keep-set go, rows present stay, and no FK constraint is violated.
func TestIntegrationPrune_RemovesStaleRows(t *testing.T) {
	repo, cleanup := newPruneTestRepo(t)
	defer cleanup()

	_, err := repo.db.Exec(`
		INSERT INTO Shows (id, title) VALUES ('keep', 'Keep'), ('drop', 'Drop');
		INSERT INTO Seasons (id, parentId, season_number) VALUES ('keep_s', 'keep', 1), ('drop_s', 'drop', 1);
		INSERT INTO Episodes (id, parentId, episode_number) VALUES ('keep_e', 'keep_s', 1), ('drop_e', 'drop_s', 1);
		INSERT INTO EpisodeMedia (parentId, file_path) VALUES ('keep_e', '/keep.mkv'), ('drop_e', '/drop.mkv');
	`)
	require.NoError(t, err)

	err = repo.PruneMissingShows(context.Background(), &models.PlexShowLibraryData{
		Shows: []models.PlexShowData{{
			Id:      "keep",
			Seasons: []models.PlexSeasonData{{Id: "keep_s", Episodes: []models.PlexEpisodeData{{Id: "keep_e"}}}},
		}},
	})
	require.NoError(t, err)

	require.Equal(t, 1, countRows(t, repo, "Shows"))
	require.Equal(t, 1, countRows(t, repo, "Seasons"))
	require.Equal(t, 1, countRows(t, repo, "Episodes"))
	require.Equal(t, 1, countRows(t, repo, "EpisodeMedia"))

	var id string
	require.NoError(t, repo.db.QueryRow("SELECT id FROM Shows").Scan(&id))
	require.Equal(t, "keep", id)
}

// TestIntegrationPrune_Movies covers the movie side, including the NULL-parent orphan.
func TestIntegrationPrune_Movies(t *testing.T) {
	repo, cleanup := newPruneTestRepo(t)
	defer cleanup()

	_, err := repo.db.Exec(`
		INSERT INTO Movies (id, title) VALUES ('keep', 'Keep'), ('drop', 'Drop');
		INSERT INTO MovieMedia (parentId, file_path) VALUES ('keep', '/keep.mkv'), ('drop', '/drop.mkv'), (NULL, '/orphan.mkv');
	`)
	require.NoError(t, err)

	err = repo.PruneMissingMovies(context.Background(), models.PlexMovieLibraryData{
		Movies: []models.Movie{{Id: "keep"}},
	})
	require.NoError(t, err)

	require.Equal(t, 1, countRows(t, repo, "Movies"))
	require.Equal(t, 1, countRows(t, repo, "MovieMedia"), "stale and NULL-parent media should both go")
}
