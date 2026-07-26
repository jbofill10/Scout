package interactors

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jbofill10/scout/backend/internal/torrenter/service"
	"github.com/jbofill10/scout/backend/pkg/library"
)

type LibraryInteractor struct {
	repo   service.Repository
	logger *slog.Logger
}

func NewLibraryInteractor(repo service.Repository, logger *slog.Logger) *LibraryInteractor {
	return &LibraryInteractor{
		repo:   repo,
		logger: logger,
	}
}

// GetLibraryShows retrieves all shows in the user's library
func (i *LibraryInteractor) GetLibraryShows(ctx context.Context) ([]library.LibraryShow, error) {
	i.logger.InfoContext(ctx, "Fetching all library shows")

	shows, err := i.repo.GetAllShows(ctx)
	if err != nil {
		i.logger.ErrorContext(ctx, "Failed to fetch library shows", "error", err)
		return nil, fmt.Errorf("failed to fetch library shows: %w", err)
	}

	i.logger.InfoContext(ctx, "Successfully fetched library shows", "count", len(shows))
	return shows, nil
}

// GetShowEpisodes retrieves all episodes (downloaded + missing) for a show
func (i *LibraryInteractor) GetShowEpisodes(ctx context.Context, tvdbId string) ([]library.EpisodeWithStatus, error) {
	i.logger.InfoContext(ctx, "Fetching episodes for show", "tvdb_id", tvdbId)

	episodes, err := i.repo.GetShowEpisodesWithStatus(ctx, tvdbId)
	if err != nil {
		i.logger.ErrorContext(ctx, "Failed to fetch show episodes", "tvdb_id", tvdbId, "error", err)
		return nil, fmt.Errorf("failed to fetch show episodes: %w", err)
	}

	i.logger.InfoContext(ctx, "Successfully fetched show episodes", "tvdb_id", tvdbId, "count", len(episodes))
	return episodes, nil
}

// GetShowMetadataStatus retrieves TVDB sync status for a show
func (i *LibraryInteractor) GetShowMetadataStatus(ctx context.Context, tvdbId string) (library.MetadataStatus, error) {
	i.logger.InfoContext(ctx, "Fetching metadata status for show", "tvdb_id", tvdbId)

	tvdbCount, plexCount, err := i.repo.GetTvdbMetadataStatus(ctx, tvdbId)
	if err != nil {
		i.logger.ErrorContext(ctx, "Failed to fetch metadata status", "tvdb_id", tvdbId, "error", err)
		return library.MetadataStatus{}, fmt.Errorf("failed to fetch metadata status: %w", err)
	}

	status := library.MetadataStatus{
		HasTvdbData:      tvdbCount > 0,
		TvdbEpisodeCount: tvdbCount,
		PlexEpisodeCount: plexCount,
		MissingCount:     tvdbCount - plexCount,
	}

	i.logger.InfoContext(ctx, "Successfully fetched metadata status",
		"tvdb_id", tvdbId,
		"has_tvdb_data", status.HasTvdbData,
		"missing_count", status.MissingCount)

	return status, nil
}

// GetLibraryMovies retrieves all movies in the user's library
func (i *LibraryInteractor) GetLibraryMovies(ctx context.Context) ([]library.LibraryMovie, error) {
	i.logger.InfoContext(ctx, "Fetching all library movies")

	movies, err := i.repo.GetAllMovies(ctx)
	if err != nil {
		i.logger.ErrorContext(ctx, "Failed to fetch library movies", "error", err)
		return nil, fmt.Errorf("failed to fetch library movies: %w", err)
	}

	i.logger.InfoContext(ctx, "Successfully fetched library movies", "count", len(movies))
	return movies, nil
}

// SyncShowEpisodes stores TVDB episode metadata for a show
// This enables "missing episode" detection by comparing TVDB data with Plex library
func (i *LibraryInteractor) SyncShowEpisodes(ctx context.Context, seriesTvdbId string, episodes []library.TvdbEpisode) error {
	i.logger.InfoContext(ctx, "Syncing TVDB episodes", "series_tvdb_id", seriesTvdbId, "episode_count", len(episodes))

	if len(episodes) == 0 {
		i.logger.WarnContext(ctx, "No episodes to sync", "series_tvdb_id", seriesTvdbId)
		return nil
	}

	err := i.repo.UpsertTvdbEpisodes(ctx, seriesTvdbId, episodes)
	if err != nil {
		i.logger.ErrorContext(ctx, "Failed to sync TVDB episodes", "series_tvdb_id", seriesTvdbId, "error", err)
		return fmt.Errorf("failed to sync TVDB episodes: %w", err)
	}

	i.logger.InfoContext(ctx, "Successfully synced TVDB episodes", "series_tvdb_id", seriesTvdbId, "episode_count", len(episodes))
	return nil
}
