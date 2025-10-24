package main

import (
	"database/sql"
	"fmt"
	"log"
	"torrenter/models"

	_ "github.com/lib/pq"
)

type Repository interface {
	UpsertLibraries(libs models.PlexLibrariesResponse)
	UpsertMovies(movies models.PlexMovieLibraryData)
	UpsertShows(shows *models.PlexShowLibraryData)
	SetPreferredLibrary(id int, libType string) error
	GetPreferredLibrary(libType string) (models.PlexLibrary, error)
	EpisodeExists(showTitle string, seasonNumber, episodeNumber int) (bool, error)
	EpisodeExistsByTvdbId(tvdbId string, seasonNumber, episodeNumber int) (bool, error)
	MediaExists(id string) (bool, error)
	GetLibraryByType(libType string) (int, error)
}

type Repo struct {
	db     *sql.DB
	logger *log.Logger
}

func NewRepo(logger *log.Logger, connStr string) (*Repo, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}
	repo := &Repo{db: db, logger: logger}
	return repo, nil
}

func (r *Repo) UpsertLibraries(libs models.PlexLibrariesResponse) {
	for _, dir := range libs.Directories {
		for _, loc := range dir.Locations {
			r.logger.Printf("Upserting %+v", loc)
			_, err := r.db.Exec(`INSERT INTO Libraries (id, type, path, section) VALUES ($1, $2, $3, $4) ON CONFLICT (id) DO UPDATE SET type=excluded.type, path=excluded.path, section=excluded.section`, loc.ID, dir.Type, loc.Path, dir.Key)
			if err != nil {
				r.logger.Printf("Failed to upsert library: %v", err)
			}
		}
	}
}

// Unsets the preferred library for a given type then sets the preferred library by ID
func (r *Repo) SetPreferredLibrary(id int, libType string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				r.logger.Printf("transaction rollback failed: %v", rbErr)
			}
		} else {
			if cmErr := tx.Commit(); cmErr != nil {
				r.logger.Printf("transaction commit failed: %v", cmErr)
			}
		}
	}()

	_, err = tx.Exec(`UPDATE Libraries SET preferred = 0 WHERE type = $1 AND preferred = 1;`, libType)
	if err != nil {
		return fmt.Errorf("failed to unset previous preferred: %w", err)
	}

	_, err = tx.Exec(`UPDATE Libraries SET preferred = 1 WHERE id = $1;`, id)
	if err != nil {
		return fmt.Errorf("failed to set preferred library: %w", err)
	}

	return nil
}

func (r *Repo) GetPreferredLibrary(libType string) (models.PlexLibrary, error) {
	var lib models.PlexLibrary
	err := r.db.QueryRow(`SELECT * FROM Libraries WHERE type = $1 AND preferred = 1;`, libType).Scan(&lib.Id, &lib.Type, &lib.Path, &lib.Preferred)
	if err != nil {
		if err == sql.ErrNoRows {
			return models.PlexLibrary{}, fmt.Errorf("no preferred library found for type %s", libType)
		}
		return models.PlexLibrary{}, fmt.Errorf("failed to query preferred library: %w", err)
	}
	return lib, nil
}

func (r *Repo) MediaExists(id string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM Media WHERE id = $1);`, id).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check media existence: %w", err)
	}
	return exists, nil
}

// EpisodeExistsByTvdbId checks if an episode already exists in Plex by TVDB ID, season, and episode number
// This is the preferred method as it's more reliable than title matching
func (r *Repo) EpisodeExistsByTvdbId(tvdbId string, seasonNumber, episodeNumber int) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1 FROM Episodes e
			JOIN Seasons s ON e.parentId = s.id
			JOIN Shows sh ON s.parentId = sh.id
			WHERE sh.tvdb_id = $1
			AND s.season_number = $2
			AND e.episode_number = $3
		)
	`
	var exists bool
	err := r.db.QueryRow(query, tvdbId, seasonNumber, episodeNumber).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check episode existence by TVDB ID: %w", err)
	}
	return exists, nil
}

// EpisodeExists checks if an episode already exists in Plex by show title, season, and episode number
// This is a fallback method when TVDB ID is not available
func (r *Repo) EpisodeExists(showTitle string, seasonNumber, episodeNumber int) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1 FROM Episodes e
			JOIN Seasons s ON e.parentId = s.id
			JOIN Shows sh ON s.parentId = sh.id
			WHERE sh.title = $1
			AND s.season_number = $2
			AND e.episode_number = $3
		)
	`
	var exists bool
	err := r.db.QueryRow(query, showTitle, seasonNumber, episodeNumber).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check episode existence: %w", err)
	}
	return exists, nil
}

func (r *Repo) GetAllLibrarySections() []models.PlexLibrary {
	var libs []models.PlexLibrary
	rows, err := r.db.Query("SELECT id, section, type, path, preferred FROM Libraries")
	if err != nil {
		r.logger.Printf("Failed to query library sections: %v", err)
		return libs
	}
	defer rows.Close()

	for rows.Next() {
		var lib models.PlexLibrary
		err := rows.Scan(&lib.Id, &lib.Section, &lib.Type, &lib.Path, &lib.Preferred)
		if err != nil {
			r.logger.Printf("Failed to scan library section: %v", err)
			continue
		}
		libs = append(libs, lib)
	}

	if err = rows.Err(); err != nil {
		r.logger.Printf("Error iterating over rows: %v", err)
	}

	return libs
}

func (r *Repo) GetLibraryByType(libType string) (int, error) {
	var section int
	err := r.db.QueryRow(`SELECT section FROM Libraries WHERE type = $1 LIMIT 1;`, libType).Scan(&section)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, fmt.Errorf("no library found for type %s", libType)
		}
		return 0, fmt.Errorf("failed to query library by type: %w", err)
	}
	return section, nil
}

func (r *Repo) UpsertMovies(movies models.PlexMovieLibraryData) {
	for _, movie := range movies.Movies {
		_, err := r.db.Exec(`
			INSERT INTO Movies (id, title, year, thumb, art)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (id) DO UPDATE SET
				title = EXCLUDED.title,
				year = EXCLUDED.year,
				thumb = EXCLUDED.thumb,
				art = EXCLUDED.art
		`, movie.Id, movie.Title, movie.Year, movie.Thumb, movie.Art)
		if err != nil {
			r.logger.Printf("Failed to upsert movie %s: %v", movie.Title, err)
			continue
		}

		for _, meta := range movie.MovieMeta {
			for _, part := range meta.Part {
				_, err = r.db.Exec(`
					INSERT INTO MovieMedia (parentId, video_resolution, file_path)
					VALUES ($1, $2, $3)
				`, movie.Id, meta.VideoResolution, part.File)
				if err != nil {
					r.logger.Printf("Failed to insert movie media for movie %s: %v", movie.Title, err)
				}
			}
		}
	}
}

func (r *Repo) UpsertShows(lib *models.PlexShowLibraryData) {
	for _, show := range lib.Shows {
		_, err := r.db.Exec(`
			INSERT INTO Shows (id, title, show_meta, thumb, tvdb_id)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (id) DO UPDATE SET
				title = EXCLUDED.title,
				show_meta = EXCLUDED.show_meta,
				thumb = EXCLUDED.thumb,
				tvdb_id = EXCLUDED.tvdb_id
		`, show.Id, show.Title, show.ShowMeta, show.Thumb, show.TvdbId)
		if err != nil {
			r.logger.Printf("Failed to upsert show %s: %v", show.Title, err)
			continue
		}

		for _, season := range show.Seasons {
			_, err := r.db.Exec(`
				INSERT INTO Seasons (id, parentId, season_meta, season_number)
				VALUES ($1, $2, $3, $4)
				ON CONFLICT (id) DO UPDATE SET
					parentId = EXCLUDED.parentId,
					season_meta = EXCLUDED.season_meta,
					season_number = EXCLUDED.season_number
			`, season.Id, show.Id, season.SeasonMeta, season.SeasonNumber)
			if err != nil {
				r.logger.Printf("Failed to upsert season %d: %v", season.SeasonNumber, err)
				continue
			}

			for _, episode := range season.Episodes {
				_, err = r.db.Exec(`
					INSERT INTO Episodes (id, parentId, episode_meta, episode_number)
					VALUES ($1, $2, $3, $4)
					ON CONFLICT (id) DO UPDATE SET
						parentId = EXCLUDED.parentId,
						episode_meta = EXCLUDED.episode_meta,
						episode_number = EXCLUDED.episode_number
				`, episode.Id, season.Id, episode.EpisodeMeta, episode.EpisodeNumber)
				if err != nil {
					r.logger.Printf("Failed to upsert episode %d: %v", episode.EpisodeNumber, err)
					continue
				}

				for _, media := range episode.Media {
					_, err = r.db.Exec(`
						INSERT INTO EpisodeMedia (id, parentId, video_resolution, file_path)
						VALUES ($1, $2, $3, $4)
						ON CONFLICT (id) DO UPDATE SET
							parentId = EXCLUDED.parentId,
							video_resolution = EXCLUDED.video_resolution,
							file_path = EXCLUDED.file_path
					`, media.Id, episode.Id, media.VideoResolution, media.File)
					if err != nil {
						r.logger.Printf("Failed to insert plex media for episode %s: %v\n%v", episode.Id, err, media)
					}
				}
			}
		}
	}
}
