package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"shared/telemetry"
	"torrenter/internal/models"

	"github.com/XSAM/otelsql"
	_ "github.com/lib/pq"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

var (
	repoTracer = otel.Tracer("torrenter/repository")
)

type Repository interface {
	UpsertLibraries(ctx context.Context, libs models.PlexLibrariesResponse)
	UpsertMovies(ctx context.Context, movies models.PlexMovieLibraryData)
	UpsertShows(ctx context.Context, shows *models.PlexShowLibraryData)
	SetPreferredLibrary(ctx context.Context, id int, libType string) error
	GetPreferredLibrary(ctx context.Context, libType string) (models.PlexLibrary, error)
	GetLibraryByType(ctx context.Context, libType string) (int, error)
	GetShowBaseDirectory(ctx context.Context, tvdbId string) (string, error)
	GetMovieBaseDirectory(ctx context.Context, tvdbId string) (string, error)
	EpisodeExistsByTvdbId(ctx context.Context, tvdbId string, season, episode int) (bool, error)
	MediaExists(ctx context.Context, id string) (bool, error)
	InsertDownloadHistory(ctx context.Context, mediaTitle string, season, episode, absoluteEpisode int, torrentHash, status, reason string) error
	UpdateDownloadHistoryStatus(ctx context.Context, torrentHash, status, reason string) error
	GetPreferredUploaders(ctx context.Context, mediaType string, isAnime bool) ([]string, error)
}

type Repo struct {
	db     *sql.DB
	logger *slog.Logger
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

	repo := &Repo{db: db, logger: logger}
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
	err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM Media WHERE id = $1);`, id).Scan(&exists)
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
		INSERT INTO DownloadHistory (mediaTitle, season, episode, absoluteEpisode, torrentHash, status, reason, trace_id, span_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, mediaTitle, season, episode, absoluteEpisode, torrentHash, status, reason, traceID, spanID)
	if err != nil {
		return fmt.Errorf("failed to insert download history: %w", err)
	}
	return nil
}

func (r *Repo) UpdateDownloadHistoryStatus(ctx context.Context, torrentHash, status, reason string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE DownloadHistory SET status = $1, reason = $2 WHERE torrentHash = $3
	`, status, reason, torrentHash)
	if err != nil {
		return fmt.Errorf("failed to update download history: %w", err)
	}
	return nil
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
