package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	"github.com/jbofill10/scout/backend/pkg/library"
	"github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/notifications"
	"github.com/jbofill10/scout/backend/pkg/telemetry"

	"errors"
	"github.com/XSAM/otelsql"
	"github.com/jbofill10/scout/backend/pkg/settings"
	"github.com/lib/pq"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"strings"
)

var (
	repoTracer = otel.Tracer("torrenter/repository")
)

type Repository interface {
	// Embed shared notifications.Repository interface (Get, Create, Update)
	notifications.Repository

	// Plex library methods
	UpsertLibraries(ctx context.Context, libs models.PlexLibrariesResponse)
	UpsertMovies(ctx context.Context, movies models.PlexMovieLibraryData)
	UpsertShows(ctx context.Context, shows *models.PlexShowLibraryData)
	PruneMissingShows(ctx context.Context, shows *models.PlexShowLibraryData) error
	PruneMissingMovies(ctx context.Context, movies models.PlexMovieLibraryData) error
	SetPreferredLibrary(ctx context.Context, id int, libType string) error
	GetPreferredLibrary(ctx context.Context, libType string) (models.PlexLibrary, error)
	GetLibraryByType(ctx context.Context, libType string) (int, error)
	GetShowBaseDirectory(ctx context.Context, tvdbId string) (string, error)
	GetMovieBaseDirectory(ctx context.Context, tvdbId string) (string, error)
	EpisodeExistsByTvdbId(ctx context.Context, tvdbId string, season, episode int) (bool, error)
	MovieExistsByTvdbId(ctx context.Context, tvdbId string) (bool, error)
	MediaExists(ctx context.Context, id string) (bool, error)
	GetShowSeasonEpisodes(ctx context.Context, tvdbId string) (map[int][]media.EpisodeInfo, error)
	InsertDownloadHistory(ctx context.Context, mediaTitle string, season, episode, absoluteEpisode int, torrentHash, status, reason string) error
	UpdateDownloadHistoryStatus(ctx context.Context, torrentHash, status, reason string) error
	GetPreferredUploaders(ctx context.Context, mediaType string, isAnime bool) ([]string, error)

	// In-flight torrent tracking, so completion monitors survive a restart
	InsertActiveTorrent(ctx context.Context, at *models.ActiveTorrent) error
	DeleteActiveTorrent(ctx context.Context, infoHash string) error
	GetActiveTorrents(ctx context.Context) ([]*models.ActiveTorrent, error)

	// Library browsing methods
	GetAllShows(ctx context.Context) ([]library.LibraryShow, error)
	GetAllMovies(ctx context.Context) ([]library.LibraryMovie, error)
	GetShowEpisodesWithStatus(ctx context.Context, tvdbId string) ([]library.EpisodeWithStatus, error)
	UpsertTvdbEpisodes(ctx context.Context, seriesTvdbId string, episodes []library.TvdbEpisode) error
	GetTvdbMetadataStatus(ctx context.Context, tvdbId string) (tvdbCount int, plexCount int, err error)
}

type Repo struct {
	*notifications.PostgresRepository
	db     *sql.DB
	logger *slog.Logger
}

// Ping checks database connectivity with the given context.
func (r *Repo) Ping(ctx context.Context) error {
	return r.db.PingContext(ctx)
}

// DB exposes the connection pool for packages that keep their own small
// tables alongside the repository (runtime settings).
func (r *Repo) DB() *sql.DB {
	return r.db
}

func NewRepo(logger *slog.Logger, connStr string) (*Repo, error) {
	// Register wrapped driver with otelsql for automatic SQL tracing
	driverName, err := otelsql.Register("postgres",
		otelsql.WithAttributes(
			semconv.DBSystemPostgreSQL,
			attribute.String("peer.service", "postgres"),
		),
		otelsql.WithSpanOptions(otelsql.SpanOptions{
			// Omit connection housekeeping spans (reduces noise from connection pool)
			OmitConnResetSession: true,
			OmitConnectorConnect: true,

			// Omit prepared statement creation spans (keep execution spans)
			OmitConnPrepare: true,

			// Omit row iteration spans (keep the query span itself)
			OmitRows: true,
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to register otelsql driver: %w", err)
	}

	db, err := sql.Open(driverName, connStr)
	if err != nil {
		return nil, err
	}

	repo := &Repo{
		PostgresRepository: notifications.NewPostgresRepository(db, logger),
		db:                 db,
		logger:             logger,
	}
	return repo, nil
}

func (r *Repo) UpsertLibraries(ctx context.Context, libs models.PlexLibrariesResponse) {
	ctx, span := repoTracer.Start(ctx, "repository.UpsertLibraries")
	defer span.End()

	libraryCount := 0
	for _, dir := range libs.Directories {
		libraryCount += len(dir.Locations)
		for _, loc := range dir.Locations {
			r.logger.DebugContext(ctx, "Upserting library", "location", loc)
			_, err := r.db.ExecContext(ctx, `INSERT INTO Libraries (id, type, path, section) VALUES ($1, $2, $3, $4) ON CONFLICT (id) DO UPDATE SET type=excluded.type, path=excluded.path, section=excluded.section`, loc.ID, dir.Type, loc.Path, dir.Key)
			if err != nil {
				r.logger.ErrorContext(ctx, "Failed to upsert library", "error", err)
				span.RecordError(err)
			}
		}
	}

	span.SetAttributes(attribute.Int("library_count", libraryCount))
	span.SetStatus(codes.Ok, "Libraries upserted successfully")
}

// Unsets the preferred library for a given type then sets the preferred library by ID
func (r *Repo) SetPreferredLibrary(ctx context.Context, id int, libType string) error {
	ctx, span := repoTracer.Start(ctx, "repository.SetPreferredLibrary")
	defer span.End()
	span.SetAttributes(attribute.Int("library_id", id), attribute.String("library_type", libType))

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to begin transaction")
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				r.logger.ErrorContext(ctx, "Transaction rollback failed", "error", rbErr)
			}
		} else {
			if cmErr := tx.Commit(); cmErr != nil {
				r.logger.ErrorContext(ctx, "Transaction commit failed", "error", cmErr)
			}
		}
	}()

	_, err = tx.ExecContext(ctx, `UPDATE Libraries SET preferred = 0 WHERE type = $1 AND preferred = 1;`, libType)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to unset previous preferred")
		return fmt.Errorf("failed to unset previous preferred: %w", err)
	}

	_, err = tx.ExecContext(ctx, `UPDATE Libraries SET preferred = 1 WHERE id = $1;`, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to set preferred library")
		return fmt.Errorf("failed to set preferred library: %w", err)
	}

	span.SetStatus(codes.Ok, "Preferred library set successfully")
	return nil
}

func (r *Repo) GetPreferredLibrary(ctx context.Context, libType string) (models.PlexLibrary, error) {
	var lib models.PlexLibrary
	err := r.db.QueryRowContext(ctx, `SELECT id, type, path, preferred, section FROM Libraries WHERE type = $1 AND preferred = 1;`, libType).Scan(&lib.Id, &lib.Type, &lib.Path, &lib.Preferred, &lib.Section)
	if err != nil {
		if err == sql.ErrNoRows {
			return models.PlexLibrary{}, fmt.Errorf("no preferred library found for type %s", libType)
		}
		return models.PlexLibrary{}, fmt.Errorf("failed to query preferred library: %w", err)
	}
	return lib, nil
}

func (r *Repo) MediaExists(ctx context.Context, id string) (bool, error) {
	var exists bool
	// Check all library-backed entities by both Plex ID and TVDB ID.
	// There is no unified "Media" table in the current schema.
	query := `
		SELECT EXISTS(
			SELECT 1 FROM Shows WHERE id = $1 OR tvdb_id = $1
			UNION ALL
			SELECT 1 FROM Movies WHERE id = $1 OR tvdb_id = $1
			UNION ALL
			SELECT 1 FROM Episodes WHERE id = $1 OR tvdb_id = $1
		);
	`
	err := r.db.QueryRowContext(ctx, query, id).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check media existence: %w", err)
	}
	return exists, nil
}

func (r *Repo) EpisodeExistsByTvdbId(ctx context.Context, tvdbId string, season, episode int) (bool, error) {
	var exists bool
	query := `
		SELECT EXISTS(
			SELECT 1
			FROM Episodes e
			JOIN Seasons s ON e.parentId = s.id
			JOIN Shows sh ON s.parentId = sh.id
			WHERE sh.tvdb_id = $1
			  AND s.season_number = $2
			  AND e.episode_number = $3
		)
	`
	err := r.db.QueryRowContext(ctx, query, tvdbId, season, episode).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check episode existence by tvdb id: %w", err)
	}

	return exists, nil
}

// MovieExistsByTvdbId checks if a movie exists in the Plex library by its TVDB ID
func (r *Repo) MovieExistsByTvdbId(ctx context.Context, tvdbId string) (bool, error) {
	if tvdbId == "" {
		return false, nil
	}

	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM Movies WHERE tvdb_id = $1)`
	err := r.db.QueryRowContext(ctx, query, tvdbId).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check movie existence by tvdb id: %w", err)
	}

	return exists, nil
}

func (r *Repo) GetAllLibrarySections() []models.PlexLibrary {
	var libs []models.PlexLibrary
	rows, err := r.db.Query("SELECT id, section, type, path, preferred FROM Libraries")
	if err != nil {
		r.logger.Error("Failed to query library sections", "error", err)
		return libs
	}
	defer rows.Close()

	for rows.Next() {
		var lib models.PlexLibrary
		err := rows.Scan(&lib.Id, &lib.Section, &lib.Type, &lib.Path, &lib.Preferred)
		if err != nil {
			r.logger.Error("Failed to scan library section", "error", err)
			continue
		}
		libs = append(libs, lib)
	}

	if err = rows.Err(); err != nil {
		r.logger.Error("Error iterating over rows", "error", err)
	}

	return libs
}

func (r *Repo) GetLibraryByType(ctx context.Context, libType string) (int, error) {
	var section int
	err := r.db.QueryRowContext(ctx, `SELECT section FROM Libraries WHERE type = $1 LIMIT 1;`, libType).Scan(&section)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, fmt.Errorf("no library found for type %s", libType)
		}
		return 0, fmt.Errorf("failed to query library by type: %w", err)
	}

	return section, nil
}

func (r *Repo) GetShowBaseDirectory(ctx context.Context, tvdbId string) (string, error) {
	var baseDir sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT base_directory FROM Shows WHERE tvdb_id = $1 AND base_directory IS NOT NULL LIMIT 1;`, tvdbId).Scan(&baseDir)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("no show found for tvdb_id %s or base_directory is NULL", tvdbId)
		}
		return "", fmt.Errorf("failed to query show base directory: %w", err)
	}

	return baseDir.String, nil
}

func (r *Repo) GetMovieBaseDirectory(ctx context.Context, tvdbId string) (string, error) {
	var baseDir sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT base_directory FROM Movies WHERE tvdb_id = $1 AND base_directory IS NOT NULL LIMIT 1;`, tvdbId).Scan(&baseDir)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("no movie found for tvdb_id %s or base_directory is NULL", tvdbId)
		}
		return "", fmt.Errorf("failed to query movie base directory: %w", err)
	}

	return baseDir.String, nil
}

func (r *Repo) UpsertMovies(ctx context.Context, movies models.PlexMovieLibraryData) {
	ctx, span := repoTracer.Start(ctx, "repository.UpsertMovies")
	defer span.End()

	for _, movie := range movies.Movies {
		_, err := r.db.ExecContext(ctx, `
			INSERT INTO Movies (id, title, year, thumb, art, tvdb_id, base_directory)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (id) DO UPDATE SET
				title = EXCLUDED.title,
				year = EXCLUDED.year,
				thumb = EXCLUDED.thumb,
				art = EXCLUDED.art,
				tvdb_id = EXCLUDED.tvdb_id,
				base_directory = EXCLUDED.base_directory
		`, movie.Id, movie.Title, movie.Year, movie.Thumb, movie.Art, movie.TvdbId, movie.BaseDirectory)
		if err != nil {
			r.logger.ErrorContext(ctx, "Failed to upsert movie", "title", movie.Title, "error", err)
			span.RecordError(err)
			continue
		}

		// Replace this movie's media rows rather than appending. MovieMedia has no
		// natural key to conflict on, so a plain INSERT duplicated every row on every
		// sync and the table grew without bound.
		if _, err := r.db.ExecContext(ctx, `DELETE FROM MovieMedia WHERE parentId = $1`, movie.Id); err != nil {
			r.logger.ErrorContext(ctx, "Failed to clear movie media", "title", movie.Title, "error", err)
			span.RecordError(err)
			continue
		}

		for _, meta := range movie.MovieMeta {
			for _, part := range meta.Part {
				_, err = r.db.ExecContext(ctx, `
					INSERT INTO MovieMedia (parentId, video_resolution, file_path)
					VALUES ($1, $2, $3)
				`, movie.Id, meta.VideoResolution, part.File)
				if err != nil {
					r.logger.ErrorContext(ctx, "Failed to insert movie media", "title", movie.Title, "error", err)
					span.RecordError(err)
				}
			}
		}
	}

	span.SetAttributes(attribute.Int("movie_count", len(movies.Movies)))
	span.SetStatus(codes.Ok, "Movies upserted successfully")
}

func (r *Repo) UpsertShows(ctx context.Context, lib *models.PlexShowLibraryData) {
	ctx, span := repoTracer.Start(ctx, "repository.UpsertShows")
	defer span.End()

	showCount := 0
	seasonCount := 0
	episodeCount := 0

	for _, show := range lib.Shows {
		showCount++
		_, err := r.db.ExecContext(ctx, `
			INSERT INTO Shows (id, title, show_meta, thumb, tvdb_id, base_directory)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (id) DO UPDATE SET
				title = EXCLUDED.title,
				show_meta = EXCLUDED.show_meta,
				thumb = EXCLUDED.thumb,
				tvdb_id = EXCLUDED.tvdb_id,
				base_directory = EXCLUDED.base_directory
		`, show.Id, show.Title, show.ShowMeta, show.Thumb, show.TvdbId, show.BaseDirectory)
		if err != nil {
			r.logger.ErrorContext(ctx, "Failed to upsert show", "title", show.Title, "error", err)
			span.RecordError(err)
			continue
		}

		for _, season := range show.Seasons {
			seasonCount++
			_, err := r.db.ExecContext(ctx, `
				INSERT INTO Seasons (id, parentId, season_meta, season_number, tvdb_id)
				VALUES ($1, $2, $3, $4, $5)
				ON CONFLICT (id) DO UPDATE SET
					parentId = EXCLUDED.parentId,
					season_meta = EXCLUDED.season_meta,
					season_number = EXCLUDED.season_number,
					tvdb_id = EXCLUDED.tvdb_id
			`, season.Id, show.Id, season.SeasonMeta, season.SeasonNumber, season.TvdbId)
			if err != nil {
				r.logger.ErrorContext(ctx, "Failed to upsert season", "season", season.SeasonNumber, "error", err)
				span.RecordError(err)
				continue
			}

			for _, episode := range season.Episodes {
				episodeCount++
				_, err = r.db.ExecContext(ctx, `
					INSERT INTO Episodes (id, parentId, episode_meta, episode_number, tvdb_id)
					VALUES ($1, $2, $3, $4, $5)
					ON CONFLICT (id) DO UPDATE SET
						parentId = EXCLUDED.parentId,
						episode_meta = EXCLUDED.episode_meta,
						episode_number = EXCLUDED.episode_number,
						tvdb_id = EXCLUDED.tvdb_id
				`, episode.Id, season.Id, episode.EpisodeMeta, episode.EpisodeNumber, episode.TvdbId)
				if err != nil {
					r.logger.ErrorContext(ctx, "Failed to upsert episode", "episode", episode.EpisodeNumber, "error", err)
					span.RecordError(err)
					continue
				}

				// Replace this episode's media rows rather than appending. EpisodeMedia has
				// no natural key to conflict on, so a plain INSERT duplicated every row on
				// every sync and the table grew without bound.
				if _, err := r.db.ExecContext(ctx,
					`DELETE FROM EpisodeMedia WHERE parentId = $1`, episode.Id); err != nil {
					r.logger.ErrorContext(ctx, "Failed to clear plex media for episode", "episode_id", episode.Id, "error", err)
					span.RecordError(err)
					continue
				}

				for _, media := range episode.Media {
					_, err = r.db.ExecContext(ctx, `
						INSERT INTO EpisodeMedia (parentId, video_resolution, file_path)
						VALUES ($1, $2, $3)
					`, episode.Id, media.VideoResolution, media.File)
					if err != nil {
						r.logger.ErrorContext(ctx, "Failed to insert plex media for episode", "episode_id", episode.Id, "error", err, "media", media)
						span.RecordError(err)
					}
				}
			}
		}
	}

	span.SetAttributes(
		attribute.Int("show_count", showCount),
		attribute.Int("season_count", seasonCount),
		attribute.Int("episode_count", episodeCount),
	)
	span.SetStatus(codes.Ok, "Shows upserted successfully")
}

func (r *Repo) InsertDownloadHistory(ctx context.Context, mediaTitle string, season, episode, absoluteEpisode int, torrentHash, status, reason string) error {
	// Extract trace and span IDs from context
	traceID, spanID := telemetry.GetTraceSpanIDs(ctx)

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO ShowDownloadHistory (mediaTitle, season, episode, absoluteEpisode, torrentHash, status, reason, trace_id, span_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, mediaTitle, season, episode, absoluteEpisode, torrentHash, status, reason, traceID, spanID)
	if err != nil {
		return fmt.Errorf("failed to insert download history: %w", err)
	}
	return nil
}

func (r *Repo) UpdateDownloadHistoryStatus(ctx context.Context, torrentHash, status, reason string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE ShowDownloadHistory SET status = $1, reason = $2 WHERE torrentHash = $3
	`, status, reason, torrentHash)
	if err != nil {
		return fmt.Errorf("failed to update download history: %w", err)
	}
	return nil
}

// InsertActiveTorrent records a torrent as in-flight so its completion monitor
// can be resumed after a restart. Re-inserting the same info hash refreshes the
// row rather than failing, which keeps a resumed monitor idempotent.
func (r *Repo) InsertActiveTorrent(ctx context.Context, at *models.ActiveTorrent) error {
	strategy, err := json.Marshal(at.Strategy)
	if err != nil {
		return fmt.Errorf("failed to marshal search strategy: %w", err)
	}
	var request []byte
	if at.Request != nil {
		if request, err = json.Marshal(at.Request); err != nil {
			return fmt.Errorf("failed to marshal search request: %w", err)
		}
	}

	traceID, spanID := telemetry.GetTraceSpanIDs(ctx)

	_, err = r.db.ExecContext(ctx, `
		INSERT INTO ActiveTorrents (info_hash, tracking_uuid, torrent_title, strategy, started_at, trace_id, span_id, request)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (info_hash) DO UPDATE SET
			tracking_uuid = EXCLUDED.tracking_uuid,
			torrent_title = EXCLUDED.torrent_title,
			strategy = EXCLUDED.strategy,
			trace_id = EXCLUDED.trace_id,
			span_id = EXCLUDED.span_id,
			request = EXCLUDED.request
	`, at.InfoHash, at.TrackingUUID, at.TorrentTitle, strategy, at.StartedAt, traceID, spanID, nullableJSON(request))
	if err != nil {
		return fmt.Errorf("failed to insert active torrent: %w", err)
	}
	return nil
}

// DeleteActiveTorrent clears the in-flight record once a torrent has been
// accounted for, whether it was processed or the monitor gave up on it.
func (r *Repo) DeleteActiveTorrent(ctx context.Context, infoHash string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM ActiveTorrents WHERE info_hash = $1", infoHash)
	if err != nil {
		return fmt.Errorf("failed to delete active torrent: %w", err)
	}
	return nil
}

// GetActiveTorrents returns every torrent still in flight, for resuming
// monitors on startup.
func (r *Repo) GetActiveTorrents(ctx context.Context) ([]*models.ActiveTorrent, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT info_hash, tracking_uuid, torrent_title, strategy, started_at, request
		FROM ActiveTorrents ORDER BY started_at
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query active torrents: %w", err)
	}
	defer rows.Close()

	active := []*models.ActiveTorrent{}
	for rows.Next() {
		var at models.ActiveTorrent
		var strategy, request []byte
		if err := rows.Scan(&at.InfoHash, &at.TrackingUUID, &at.TorrentTitle, &strategy, &at.StartedAt, &request); err != nil {
			r.logger.ErrorContext(ctx, "Error scanning active torrent", "error", err)
			continue
		}
		if len(request) > 0 {
			// A request that cannot be read only costs the ability to replace
			// this torrent; the monitor itself still resumes.
			if err := json.Unmarshal(request, &at.Request); err != nil {
				r.logger.WarnContext(ctx, "Ignoring unreadable search request on active torrent",
					"error", err, "info_hash", at.InfoHash)
				at.Request = nil
			}
		}
		// A row we cannot unmarshal can never be resumed; drop it rather than
		// letting it wedge the startup sweep on every boot.
		if err := json.Unmarshal(strategy, &at.Strategy); err != nil {
			r.logger.ErrorContext(ctx, "Discarding active torrent with unreadable strategy",
				"error", err, "info_hash", at.InfoHash, "title", at.TorrentTitle)
			if delErr := r.DeleteActiveTorrent(ctx, at.InfoHash); delErr != nil {
				r.logger.ErrorContext(ctx, "Failed to delete unreadable active torrent", "error", delErr)
			}
			continue
		}
		active = append(active, &at)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating active torrent rows: %w", err)
	}

	return active, nil
}

// nullableJSON turns an empty payload into SQL NULL so a JSONB column is not
// handed an empty string.
func nullableJSON(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}

// InsertFailedTorrent remembers a torrent that was tried for an item and given
// up on, so the next search for that item skips it.
func (r *Repo) InsertFailedTorrent(ctx context.Context, ft *models.FailedTorrent) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO FailedTorrents (tvdb_id, season, episode, info_hash, torrent_title, reason)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, ft.TvdbID, ft.Season, ft.Episode, ft.InfoHash, ft.Title, ft.Reason)
	if err != nil {
		return fmt.Errorf("failed to insert failed torrent: %w", err)
	}
	return nil
}

// GetFailedTorrents lists every torrent previously given up on for an item.
func (r *Repo) GetFailedTorrents(ctx context.Context, tvdbID string, season, episode int) ([]*models.FailedTorrent, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT tvdb_id, season, episode, info_hash, torrent_title, COALESCE(reason, '')
		FROM FailedTorrents WHERE tvdb_id = $1 AND season = $2 AND episode = $3
	`, tvdbID, season, episode)
	if err != nil {
		return nil, fmt.Errorf("failed to query failed torrents: %w", err)
	}
	defer rows.Close()

	failed := []*models.FailedTorrent{}
	for rows.Next() {
		var ft models.FailedTorrent
		if err := rows.Scan(&ft.TvdbID, &ft.Season, &ft.Episode, &ft.InfoHash, &ft.Title, &ft.Reason); err != nil {
			return nil, fmt.Errorf("failed to scan failed torrent: %w", err)
		}
		failed = append(failed, &ft)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating failed torrent rows: %w", err)
	}
	return failed, nil
}

func (r *Repo) GetPreferredUploaders(ctx context.Context, mediaType string, isAnime bool) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT uploaderName FROM UploaderPreferences WHERE mediaType = $1 AND isAnime = $2", mediaType, isAnime)
	if err != nil {
		return nil, fmt.Errorf("failed to query uploader preferences: %w", err)
	}
	defer rows.Close()

	preferred := []string{}
	for rows.Next() {
		var uploader string
		if err := rows.Scan(&uploader); err != nil {
			r.logger.ErrorContext(ctx, "Error scanning uploader", "error", err)
			continue
		}
		preferred = append(preferred, uploader)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating uploader rows: %w", err)
	}

	return preferred, nil
}

// GetShowSeasonEpisodes returns all episodes in a show grouped by season number, including TVDB IDs
func (r *Repo) GetShowSeasonEpisodes(ctx context.Context, tvdbId string) (map[int][]media.EpisodeInfo, error) {
	if tvdbId == "" {
		return make(map[int][]media.EpisodeInfo), nil
	}

	query := `
		SELECT s.season_number, e.episode_number, COALESCE(e.tvdb_id, '')
		FROM Episodes e
		JOIN Seasons s ON e.parentId = s.id
		JOIN Shows sh ON s.parentId = sh.id
		WHERE sh.tvdb_id = $1
		ORDER BY s.season_number, e.episode_number
	`

	rows, err := r.db.QueryContext(ctx, query, tvdbId)
	if err != nil {
		return nil, fmt.Errorf("failed to query show season episodes: %w", err)
	}
	defer rows.Close()

	result := make(map[int][]media.EpisodeInfo)
	for rows.Next() {
		var seasonNum, episodeNum int
		var epTvdbId string
		if err := rows.Scan(&seasonNum, &episodeNum, &epTvdbId); err != nil {
			r.logger.ErrorContext(ctx, "Error scanning episode", "error", err, "tvdb_id", tvdbId)
			continue
		}

		result[seasonNum] = append(result[seasonNum], media.EpisodeInfo{
			EpisodeNum: episodeNum,
			TvdbId:     epTvdbId,
		})
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating episode rows: %w", err)
	}

	return result, nil
}

// Note: CreateNotification, GetNotification, and UpdateNotification are provided by the embedded PostgresRepository
// OpenTelemetry tracing was removed in favor of code reuse. If detailed tracing is needed,
// these methods can be overridden with torrenter-specific implementations that include spans.

// PruneMissingShows removes shows, seasons, episodes and episode media that are no longer
// present in Plex. It is the counterpart to UpsertShows, which only ever adds or updates —
// without this, media deleted from Plex lingered in the mirror forever and made
// EpisodeExistsByTvdbId report true for files that are gone.
//
// Callers must only invoke this after a COMPLETE Plex fetch. Pruning against a partial
// result would delete live rows, so an empty show set is treated as "no data" and is
// refused rather than interpreted as "Plex is empty".
func (r *Repo) PruneMissingShows(ctx context.Context, lib *models.PlexShowLibraryData) error {
	ctx, span := repoTracer.Start(ctx, "repository.PruneMissingShows")
	defer span.End()

	if lib == nil || len(lib.Shows) == 0 {
		span.SetStatus(codes.Ok, "nothing to prune, empty show set")
		return nil
	}

	showIDs := make([]string, 0, len(lib.Shows))
	seasonIDs := []string{}
	episodeIDs := []string{}
	for _, show := range lib.Shows {
		showIDs = append(showIDs, show.Id)
		for _, season := range show.Seasons {
			seasonIDs = append(seasonIDs, season.Id)
			for _, episode := range season.Episodes {
				episodeIDs = append(episodeIDs, episode.Id)
			}
		}
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("begin prune shows transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Children first — these tables are linked by foreign keys with no cascade.
	//
	// The media queries also match NULL parentId. `NULL <> ALL(...)` evaluates to NULL,
	// not true, so without the explicit IS NULL arm an orphaned media row could never be
	// pruned by any keep-set.
	deletes := []struct {
		label string
		query string
		ids   []string
	}{
		{"episode media", `DELETE FROM EpisodeMedia WHERE parentId IS NULL OR parentId <> ALL($1)`, episodeIDs},
		{"episodes", `DELETE FROM Episodes WHERE id <> ALL($1)`, episodeIDs},
		{"seasons", `DELETE FROM Seasons WHERE id <> ALL($1)`, seasonIDs},
		{"shows", `DELETE FROM Shows WHERE id <> ALL($1)`, showIDs},
	}

	removed := map[string]int64{}
	for _, d := range deletes {
		// An empty keep-set is never safe to delete against: `x <> ALL(ARRAY[]::text[])`
		// is vacuously TRUE for every row, so this DELETE would empty the table. Reaching
		// here with no IDs means a level returned nothing, which we treat as absent data
		// rather than as "Plex has none of these".
		if len(d.ids) == 0 {
			r.logger.WarnContext(ctx, "Skipping prune level with an empty keep-set", "level", d.label)
			continue
		}

		res, err := tx.ExecContext(ctx, d.query, pq.Array(d.ids))
		if err != nil {
			span.RecordError(err)
			return fmt.Errorf("prune %s: %w", d.label, err)
		}
		if n, err := res.RowsAffected(); err == nil {
			removed[d.label] = n
		}
	}

	if err := tx.Commit(); err != nil {
		span.RecordError(err)
		return fmt.Errorf("commit prune shows: %w", err)
	}

	span.SetAttributes(
		attribute.Int64("pruned.episode_media", removed["episode media"]),
		attribute.Int64("pruned.episodes", removed["episodes"]),
		attribute.Int64("pruned.seasons", removed["seasons"]),
		attribute.Int64("pruned.shows", removed["shows"]),
	)
	span.SetStatus(codes.Ok, "Stale show rows pruned")
	r.logger.InfoContext(ctx, "Pruned stale show rows",
		"episode_media", removed["episode media"], "episodes", removed["episodes"],
		"seasons", removed["seasons"], "shows", removed["shows"])
	return nil
}

// PruneMissingMovies removes movies and movie media no longer present in Plex. Same
// contract as PruneMissingShows: only call it after a complete fetch.
func (r *Repo) PruneMissingMovies(ctx context.Context, movies models.PlexMovieLibraryData) error {
	ctx, span := repoTracer.Start(ctx, "repository.PruneMissingMovies")
	defer span.End()

	if len(movies.Movies) == 0 {
		span.SetStatus(codes.Ok, "nothing to prune, empty movie set")
		return nil
	}

	movieIDs := make([]string, 0, len(movies.Movies))
	for _, movie := range movies.Movies {
		movieIDs = append(movieIDs, movie.Id)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("begin prune movies transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// The IS NULL arm matters here too: `NULL <> ALL(...)` is NULL, so an orphaned media
	// row would otherwise be permanently un-prunable.
	mediaRes, err := tx.ExecContext(ctx,
		`DELETE FROM MovieMedia WHERE parentId IS NULL OR parentId <> ALL($1)`, pq.Array(movieIDs))
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("prune movie media: %w", err)
	}

	movieRes, err := tx.ExecContext(ctx, `DELETE FROM Movies WHERE id <> ALL($1)`, pq.Array(movieIDs))
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("prune movies: %w", err)
	}

	if err := tx.Commit(); err != nil {
		span.RecordError(err)
		return fmt.Errorf("commit prune movies: %w", err)
	}

	prunedMedia, _ := mediaRes.RowsAffected()
	prunedMovies, _ := movieRes.RowsAffected()
	span.SetAttributes(
		attribute.Int64("pruned.movie_media", prunedMedia),
		attribute.Int64("pruned.movies", prunedMovies),
	)
	span.SetStatus(codes.Ok, "Stale movie rows pruned")
	r.logger.InfoContext(ctx, "Pruned stale movie rows", "movie_media", prunedMedia, "movies", prunedMovies)
	return nil
}

// UpsertTvdbEpisodes stores TVDB episode metadata for a show
// This data is used to detect "missing episodes" (aired but not downloaded)
func (r *Repo) UpsertTvdbEpisodes(ctx context.Context, seriesTvdbId string, episodes []library.TvdbEpisode) error {
	ctx, span := repoTracer.Start(ctx, "repository.UpsertTvdbEpisodes")
	defer span.End()
	span.SetAttributes(
		attribute.String("series_tvdb_id", seriesTvdbId),
		attribute.Int("episode_count", len(episodes)),
	)

	if len(episodes) == 0 {
		span.SetStatus(codes.Ok, "No episodes to upsert")
		return nil
	}

	for _, ep := range episodes {
		// Convert empty aired date to NULL for PostgreSQL
		var airedDate interface{}
		if ep.Aired == "" {
			airedDate = nil
		} else {
			airedDate = ep.Aired
		}

		_, err := r.db.ExecContext(ctx, `
			INSERT INTO TvdbEpisodes (tvdb_id, series_tvdb_id, season_number, episode_number, absolute_number, name, aired)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (tvdb_id) DO UPDATE SET
				series_tvdb_id = EXCLUDED.series_tvdb_id,
				season_number = EXCLUDED.season_number,
				episode_number = EXCLUDED.episode_number,
				absolute_number = EXCLUDED.absolute_number,
				name = EXCLUDED.name,
				aired = EXCLUDED.aired
		`, ep.TvdbId, ep.SeriesTvdbId, ep.SeasonNumber, ep.EpisodeNumber, ep.AbsoluteNumber, ep.Name, airedDate)
		if err != nil {
			r.logger.ErrorContext(ctx, "Failed to upsert TVDB episode",
				"tvdb_id", ep.TvdbId, "season", ep.SeasonNumber, "episode", ep.EpisodeNumber, "error", err)
			span.RecordError(err)
			return fmt.Errorf("failed to upsert TVDB episode %s S%dE%d: %w", seriesTvdbId, ep.SeasonNumber, ep.EpisodeNumber, err)
		}
	}

	span.SetStatus(codes.Ok, "TVDB episodes upserted successfully")
	return nil
}

// GetShowEpisodesWithStatus returns all episodes (aired) with download status
// Uses LEFT JOIN to detect missing episodes (in TvdbEpisodes but not in Episodes)
func (r *Repo) GetShowEpisodesWithStatus(ctx context.Context, tvdbId string) ([]library.EpisodeWithStatus, error) {
	ctx, span := repoTracer.Start(ctx, "repository.GetShowEpisodesWithStatus")
	defer span.End()
	span.SetAttributes(attribute.String("tvdb_id", tvdbId))

	if tvdbId == "" {
		return []library.EpisodeWithStatus{}, nil
	}

	query := `
		SELECT
			te.tvdb_id,
			te.season_number,
			te.episode_number,
			COALESCE(te.absolute_number, 0) as absolute_number,
			COALESCE(te.name, '') as name,
			COALESCE(te.aired::text, '') as aired,
			CASE WHEN e.id IS NOT NULL THEN true ELSE false END as downloaded
		FROM TvdbEpisodes te
		LEFT JOIN Episodes e ON te.tvdb_id = e.tvdb_id
		WHERE te.series_tvdb_id = $1
		  AND te.aired <= CURRENT_DATE
		ORDER BY te.season_number, te.episode_number
	`

	rows, err := r.db.QueryContext(ctx, query, tvdbId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to query show episodes with status")
		return nil, fmt.Errorf("failed to query show episodes with status: %w", err)
	}
	defer rows.Close()

	var episodes []library.EpisodeWithStatus
	for rows.Next() {
		var ep library.EpisodeWithStatus
		if err := rows.Scan(&ep.TvdbId, &ep.SeasonNumber, &ep.EpisodeNumber, &ep.AbsoluteNumber, &ep.Name, &ep.Aired, &ep.Downloaded); err != nil {
			r.logger.ErrorContext(ctx, "Error scanning episode with status", "error", err, "tvdb_id", tvdbId)
			span.RecordError(err)
			continue
		}
		episodes = append(episodes, ep)
	}

	if err = rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Error iterating episode rows")
		return nil, fmt.Errorf("error iterating episode rows: %w", err)
	}

	span.SetAttributes(attribute.Int("episode_count", len(episodes)))
	span.SetStatus(codes.Ok, "Episodes with status retrieved successfully")
	return episodes, nil
}

// GetTvdbMetadataStatus returns counts for TVDB episodes vs Plex episodes
func (r *Repo) GetTvdbMetadataStatus(ctx context.Context, tvdbId string) (tvdbCount int, plexCount int, err error) {
	ctx, span := repoTracer.Start(ctx, "repository.GetTvdbMetadataStatus")
	defer span.End()
	span.SetAttributes(attribute.String("tvdb_id", tvdbId))

	// Count TVDB episodes (aired only, exclude Season 0 specials)
	err = r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM TvdbEpisodes
		WHERE series_tvdb_id = $1 AND aired <= CURRENT_DATE AND season_number != 0
	`, tvdbId).Scan(&tvdbCount)
	if err != nil {
		span.RecordError(err)
		return 0, 0, fmt.Errorf("failed to count TVDB episodes: %w", err)
	}

	// Count Plex episodes (exclude Season 0 specials)
	err = r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM Episodes e
		JOIN Seasons s ON e.parentid = s.id
		JOIN Shows sh ON s.parentid = sh.id
		WHERE sh.tvdb_id = $1 AND s.season_number != 0
	`, tvdbId).Scan(&plexCount)
	if err != nil {
		span.RecordError(err)
		return 0, 0, fmt.Errorf("failed to count Plex episodes: %w", err)
	}

	span.SetAttributes(
		attribute.Int("tvdb_count", tvdbCount),
		attribute.Int("plex_count", plexCount),
	)
	span.SetStatus(codes.Ok, "Metadata status retrieved")
	return tvdbCount, plexCount, nil
}

// GetAllShows returns all shows in the library
func (r *Repo) GetAllShows(ctx context.Context) ([]library.LibraryShow, error) {
	ctx, span := repoTracer.Start(ctx, "repository.GetAllShows")
	defer span.End()

	query := `
		SELECT COALESCE(tvdb_id, ''), title, COALESCE(thumb, '')
		FROM Shows
		WHERE tvdb_id IS NOT NULL AND tvdb_id != ''
		ORDER BY title ASC
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to query shows")
		return nil, fmt.Errorf("failed to query shows: %w", err)
	}
	defer rows.Close()

	var shows []library.LibraryShow
	for rows.Next() {
		var show library.LibraryShow
		if err := rows.Scan(&show.TvdbId, &show.Title, &show.Thumb); err != nil {
			r.logger.ErrorContext(ctx, "Error scanning show", "error", err)
			span.RecordError(err)
			continue
		}
		shows = append(shows, show)
	}

	if err = rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Error iterating show rows")
		return nil, fmt.Errorf("error iterating show rows: %w", err)
	}

	span.SetAttributes(attribute.Int("show_count", len(shows)))
	span.SetStatus(codes.Ok, "Shows retrieved successfully")
	return shows, nil
}

// GetAllMovies returns all movies in the library
func (r *Repo) GetAllMovies(ctx context.Context) ([]library.LibraryMovie, error) {
	ctx, span := repoTracer.Start(ctx, "repository.GetAllMovies")
	defer span.End()

	query := `
		SELECT COALESCE(tvdb_id, ''), title, COALESCE(thumb, ''), COALESCE(year, 0)
		FROM Movies
		WHERE tvdb_id IS NOT NULL AND tvdb_id != ''
		ORDER BY title ASC
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to query movies")
		return nil, fmt.Errorf("failed to query movies: %w", err)
	}
	defer rows.Close()

	var movies []library.LibraryMovie
	for rows.Next() {
		var movie library.LibraryMovie
		if err := rows.Scan(&movie.TvdbId, &movie.Title, &movie.Thumb, &movie.Year); err != nil {
			r.logger.ErrorContext(ctx, "Error scanning movie", "error", err)
			span.RecordError(err)
			continue
		}
		movies = append(movies, movie)
	}

	if err = rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Error iterating movie rows")
		return nil, fmt.Errorf("error iterating movie rows: %w", err)
	}

	span.SetAttributes(attribute.Int("movie_count", len(movies)))
	span.SetStatus(codes.Ok, "Movies retrieved successfully")
	return movies, nil
}

// ErrDuplicateUploader is returned when an uploader is already preferred for
// the same media type and anime flag.
var ErrDuplicateUploader = errors.New("uploader already preferred for this media type")

// ListUploaderPreferences returns every preferred uploader, grouped by media
// type and anime flag.
func (r *Repo) ListUploaderPreferences(ctx context.Context) ([]settings.UploaderPreference, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, mediaType, isAnime, uploaderName FROM UploaderPreferences ORDER BY mediaType, isAnime, lower(uploaderName)`)
	if err != nil {
		return nil, fmt.Errorf("failed to list uploader preferences: %w", err)
	}
	defer rows.Close()

	out := []settings.UploaderPreference{}
	for rows.Next() {
		var p settings.UploaderPreference
		if err := rows.Scan(&p.ID, &p.MediaType, &p.IsAnime, &p.Uploader); err != nil {
			return nil, fmt.Errorf("failed to scan uploader preference: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// AddUploaderPreference stores a preferred uploader and returns it with its id.
func (r *Repo) AddUploaderPreference(ctx context.Context, p settings.NewUploaderPreference) (settings.UploaderPreference, error) {
	var id int
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO UploaderPreferences (mediaType, isAnime, uploaderName) VALUES ($1, $2, $3) RETURNING id`,
		p.MediaType, p.IsAnime, strings.TrimSpace(p.Uploader)).Scan(&id)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return settings.UploaderPreference{}, ErrDuplicateUploader
		}
		return settings.UploaderPreference{}, fmt.Errorf("failed to add uploader preference: %w", err)
	}
	return settings.UploaderPreference{ID: id, MediaType: p.MediaType, IsAnime: p.IsAnime, Uploader: strings.TrimSpace(p.Uploader)}, nil
}

// DeleteUploaderPreference removes a preferred uploader by id, reporting
// whether a row existed.
func (r *Repo) DeleteUploaderPreference(ctx context.Context, id int) (bool, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM UploaderPreferences WHERE id = $1`, id)
	if err != nil {
		return false, fmt.Errorf("failed to delete uploader preference: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// GetMissingEpisodes returns, per library show, the aired non-special TVDB
// episodes with no Plex episode, in one query. Shows with nothing missing are
// left out.
func (r *Repo) GetMissingEpisodes(ctx context.Context) ([]library.MissingShow, error) {
	ctx, span := repoTracer.Start(ctx, "repository.GetMissingEpisodes")
	defer span.End()

	query := `
		SELECT sh.tvdb_id, sh.title, COALESCE(sh.thumb, ''),
		       te.tvdb_id, te.season_number, te.episode_number,
		       COALESCE(te.absolute_number, 0), COALESCE(te.name, ''), COALESCE(te.aired::text, '')
		FROM Shows sh
		JOIN TvdbEpisodes te ON te.series_tvdb_id = sh.tvdb_id
		LEFT JOIN Episodes e ON e.tvdb_id = te.tvdb_id
		WHERE sh.tvdb_id IS NOT NULL AND sh.tvdb_id != ''
		  AND te.season_number > 0
		  AND te.aired <= CURRENT_DATE
		  AND e.id IS NULL
		ORDER BY lower(sh.title), sh.tvdb_id, te.season_number, te.episode_number
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to query missing episodes: %w", err)
	}
	defer rows.Close()

	shows := []library.MissingShow{}
	index := map[string]int{}
	for rows.Next() {
		var showID, title, thumb string
		var ep library.MissingEpisode
		if err := rows.Scan(&showID, &title, &thumb, &ep.TvdbId, &ep.SeasonNumber, &ep.EpisodeNumber,
			&ep.AbsoluteNumber, &ep.Name, &ep.Aired); err != nil {
			return nil, fmt.Errorf("failed to scan missing episode: %w", err)
		}
		i, ok := index[showID]
		if !ok {
			index[showID] = len(shows)
			i = len(shows)
			shows = append(shows, library.MissingShow{TvdbId: showID, Title: title, Thumb: thumb, Episodes: []library.MissingEpisode{}})
		}
		shows[i].Episodes = append(shows[i].Episodes, ep)
	}
	return shows, rows.Err()
}

// GetUpgradeMedia returns every library file with its Plex resolution, for
// the upgrade pass to judge which items sit below the cutoff.
func (r *Repo) GetUpgradeMedia(ctx context.Context) ([]models.UpgradeMediaRow, error) {
	ctx, span := repoTracer.Start(ctx, "repository.GetUpgradeMedia")
	defer span.End()

	rows, err := r.db.QueryContext(ctx, `
		SELECT sh.tvdb_id, sh.title, e.tvdb_id, s.season_number, e.episode_number,
		       COALESCE(em.video_resolution, ''), COALESCE(em.file_path, '')
		FROM Episodes e
		JOIN Seasons s ON e.parentid = s.id
		JOIN Shows sh ON s.parentid = sh.id
		JOIN EpisodeMedia em ON em.parentid = e.id
		WHERE sh.tvdb_id IS NOT NULL AND sh.tvdb_id <> ''
		  AND e.tvdb_id IS NOT NULL AND e.tvdb_id <> ''
		  AND s.season_number > 0`)
	if err != nil {
		return nil, fmt.Errorf("failed to query episode media: %w", err)
	}
	out := []models.UpgradeMediaRow{}
	for rows.Next() {
		row := models.UpgradeMediaRow{Kind: "episode"}
		if err := rows.Scan(&row.ShowTvdbID, &row.ShowTitle, &row.EpisodeTvdbID, &row.Season, &row.Episode, &row.Resolution, &row.FilePath); err != nil {
			rows.Close()
			return nil, fmt.Errorf("failed to scan episode media: %w", err)
		}
		out = append(out, row)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	mrows, err := r.db.QueryContext(ctx, `
		SELECT m.tvdb_id, m.title, COALESCE(m.year, 0), COALESCE(mm.video_resolution, ''), COALESCE(mm.file_path, '')
		FROM Movies m
		JOIN MovieMedia mm ON mm.parentid = m.id
		WHERE m.tvdb_id IS NOT NULL AND m.tvdb_id <> ''`)
	if err != nil {
		return nil, fmt.Errorf("failed to query movie media: %w", err)
	}
	defer mrows.Close()
	for mrows.Next() {
		row := models.UpgradeMediaRow{Kind: "movie"}
		if err := mrows.Scan(&row.MovieTvdbID, &row.MovieTitle, &row.Year, &row.Resolution, &row.FilePath); err != nil {
			return nil, fmt.Errorf("failed to scan movie media: %w", err)
		}
		out = append(out, row)
	}
	return out, mrows.Err()
}

const upgradeAttemptColumns = `item_key, kind, title, COALESCE(season, 0), COALESCE(episode, 0), COALESCE(from_quality, ''),
	COALESCE(to_quality, ''), status, COALESCE(reason, ''), attempts, created_at, updated_at`

func scanUpgradeAttempt(rs interface{ Scan(...any) error }) (models.UpgradeAttempt, error) {
	var a models.UpgradeAttempt
	err := rs.Scan(&a.ItemKey, &a.Kind, &a.Title, &a.Season, &a.Episode, &a.FromQuality, &a.ToQuality, &a.Status, &a.Reason,
		&a.Attempts, &a.CreatedAt, &a.UpdatedAt)
	return a, err
}

// GetUpgradeAttempts returns every attempt keyed by item.
func (r *Repo) GetUpgradeAttempts(ctx context.Context) (map[string]models.UpgradeAttempt, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+upgradeAttemptColumns+` FROM UpgradeAttempts`)
	if err != nil {
		return nil, fmt.Errorf("failed to query upgrade attempts: %w", err)
	}
	defer rows.Close()
	out := map[string]models.UpgradeAttempt{}
	for rows.Next() {
		a, err := scanUpgradeAttempt(rows)
		if err != nil {
			return nil, err
		}
		out[a.ItemKey] = a
	}
	return out, rows.Err()
}

// ListUpgradeAttempts returns the most recently touched attempts.
func (r *Repo) ListUpgradeAttempts(ctx context.Context, limit int) ([]models.UpgradeAttempt, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+upgradeAttemptColumns+` FROM UpgradeAttempts ORDER BY updated_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list upgrade attempts: %w", err)
	}
	defer rows.Close()
	out := []models.UpgradeAttempt{}
	for rows.Next() {
		a, err := scanUpgradeAttempt(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// RecordUpgradeAttempt starts (or restarts) an attempt for an item, bumping
// its attempt count.
func (r *Repo) RecordUpgradeAttempt(ctx context.Context, a models.UpgradeAttempt) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO UpgradeAttempts (item_key, kind, title, season, episode, from_quality, to_quality, status, reason, attempts)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, ''), 1)
		ON CONFLICT (item_key) DO UPDATE SET
		    kind = EXCLUDED.kind, title = EXCLUDED.title, season = EXCLUDED.season, episode = EXCLUDED.episode,
		    from_quality = EXCLUDED.from_quality, to_quality = EXCLUDED.to_quality, status = EXCLUDED.status,
		    reason = EXCLUDED.reason, attempts = UpgradeAttempts.attempts + 1, updated_at = now()`,
		a.ItemKey, a.Kind, a.Title, a.Season, a.Episode, a.FromQuality, a.ToQuality, a.Status, a.Reason)
	if err != nil {
		return fmt.Errorf("failed to record upgrade attempt: %w", err)
	}
	return nil
}

// SetUpgradeAttemptStatus moves an attempt to a new status.
func (r *Repo) SetUpgradeAttemptStatus(ctx context.Context, itemKey, status, reason string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE UpgradeAttempts SET status = $2, reason = NULLIF($3, ''), updated_at = now() WHERE item_key = $1`,
		itemKey, status, reason)
	if err != nil {
		return fmt.Errorf("failed to update upgrade attempt: %w", err)
	}
	return nil
}
