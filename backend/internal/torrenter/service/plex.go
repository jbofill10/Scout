package service

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
)

var (
	mediaSectionBase = "/library/sections"
	libraryTypeMovie = "movie"
	libraryTypeShow  = "show"
	tracer           = otel.Tracer("torrenter/plex")
)

type PlexHandler struct {
	cfg            *models.PlexCfg
	repo           Repository
	logger         *slog.Logger
	httpClient     *http.Client
	mediaProcessor MediaProcessor
}

func NewPlexHandler(repo Repository, logger *slog.Logger, cfg *models.PlexCfg, mediaProcessor MediaProcessor) *PlexHandler {
	// Use plain HTTP client without otelhttp to avoid redundant auto-instrumented spans
	// We create manual spans with descriptive names in fetchAndUnmarshal() and getLibraries()
	return &PlexHandler{
		cfg:            cfg,
		repo:           repo,
		logger:         logger,
		mediaProcessor: mediaProcessor,
		httpClient: &http.Client{
			Transport: http.DefaultTransport,
		},
	}
}

// extractTvdbId extracts TVDB ID from a slice of PlexGuid objects
func extractTvdbId(guids []models.PlexGuid) string {
	for _, guid := range guids {
		if len(guid.ID) > 7 && guid.ID[:7] == "tvdb://" {
			return guid.ID[7:] // Strip "tvdb://" prefix
		}
	}
	return ""
}

func (p *PlexHandler) getLibraries(ctx context.Context) models.PlexLibrariesResponse {
	ctx, span := tracer.Start(ctx, "getLibraries")
	defer span.End()

	url := p.cfg.Host + mediaSectionBase
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to create request")
		p.logger.ErrorContext(ctx, "Failed to create request for Plex libraries", "error", err)
		os.Exit(1)
	}

	// Create manual span for HTTP request with descriptive name
	httpSpanName := fmt.Sprintf("Plex GET %s", req.URL.Path)
	httpCtx, httpSpan := tracer.Start(ctx, httpSpanName)
	defer httpSpan.End()

	req.Header.Set("X-Plex-Token", p.cfg.Key)

	resp, err := p.httpClient.Do(req.WithContext(httpCtx))
	if err != nil {
		httpSpan.RecordError(err)
		httpSpan.SetStatus(codes.Error, "HTTP request failed")
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to fetch libraries")
		p.logger.ErrorContext(ctx, "Failed to fetch Plex libraries", "error", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		httpSpan.RecordError(err)
		httpSpan.SetStatus(codes.Error, "Failed to read response body")
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to read response")
		p.logger.ErrorContext(ctx, "Failed to read Plex libraries response", "error", err)
		os.Exit(1)
	}

	httpSpan.SetStatus(codes.Ok, "HTTP request successful")

	var libraryRes models.PlexLibrariesResponse
	err = xml.Unmarshal(bodyBytes, &libraryRes)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to unmarshal response")
		p.logger.ErrorContext(ctx, "Failed to unmarshal Plex libraries response", "error", err)
		os.Exit(1)
	}

	span.SetAttributes(attribute.Int("library.count", len(libraryRes.Directories)))
	span.SetStatus(codes.Ok, "Libraries fetched successfully")
	p.logger.DebugContext(ctx, "Fetched Plex libraries", "count", len(libraryRes.Directories))

	return libraryRes
}

func (p *PlexHandler) SyncPlexLibrary(ctx context.Context) {
	ctx, span := tracer.Start(ctx, "SyncPlexLibrary",
		trace.WithAttributes(
			attribute.String("plex.host", p.cfg.Host),
		),
	)
	defer span.End()

	p.logger.InfoContext(ctx, "Starting Plex library sync")

	libraries := p.getLibraries(ctx)
	p.repo.UpsertLibraries(ctx, libraries)

	movies := p.getMovies(ctx)
	p.repo.UpsertMovies(ctx, movies)

	shows := p.getShows(ctx)
	if shows != nil {
		p.repo.UpsertShows(ctx, shows)
	}

	// Invalidate cache entries that now exist in the database
	if p.mediaProcessor != nil {
		p.mediaProcessor.InvalidateCache(ctx)
		p.logger.InfoContext(ctx, "Cache invalidation triggered after Plex sync")
	}

	span.SetStatus(codes.Ok, "Plex library sync completed successfully")
	p.logger.InfoContext(ctx, "Plex Library Sync Complete")
}

func (p *PlexHandler) getMovies(ctx context.Context) models.PlexMovieLibraryData {
	ctx, span := tracer.Start(ctx, "getMovies")
	defer span.End()

	p.logger.InfoContext(ctx, "Starting plex movie media sync")
	movies := models.PlexMovieLibraryData{}

	movieLibrary, err := p.repo.GetPreferredLibrary(ctx, libraryTypeMovie)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to get movie library")
		p.logger.ErrorContext(ctx, "Error getting movie library", "error", err)
		return movies
	}

	url := fmt.Sprintf("%s%s/%d/all?includeGuids=1", p.cfg.Host, mediaSectionBase, movieLibrary.Section)
	span.SetAttributes(attribute.String("plex.url", url))

	err = p.fetchAndUnmarshal(ctx, url, &movies)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to fetch movies")
		p.logger.ErrorContext(ctx, "Error fetching movies", "error", err)
		return movies
	}

	// Extract TVDB IDs and base directories from movie data
	_, extractSpan := tracer.Start(ctx, "extractMovieTvdbIds")
	for i := range movies.Movies {
		movies.Movies[i].TvdbId = extractTvdbId(movies.Movies[i].Guids)

		// Extract base directory from first available file path
		for _, media := range movies.Movies[i].MovieMeta {
			if len(media.Part) > 0 && media.Part[0].File != "" {
				movies.Movies[i].BaseDirectory = extractMovieBaseDirectory(media.Part[0].File, movieLibrary.Path)
				break
			}
		}
	}
	extractSpan.SetAttributes(attribute.Int("movies_processed", len(movies.Movies)))
	extractSpan.SetStatus(codes.Ok, "TVDB IDs and base directories extracted")
	extractSpan.End()

	span.SetAttributes(attribute.Int("movie.count", len(movies.Movies)))
	span.SetStatus(codes.Ok, "Movies fetched successfully")
	p.logger.InfoContext(ctx, "Fetched movies", "count", len(movies.Movies))

	return movies
}

func (p *PlexHandler) fetchAndUnmarshal(ctx context.Context, url string, v interface{}) error {
	// Create manual span with descriptive name based on URL path
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	// Extract path for span name
	spanName := fmt.Sprintf("Plex GET %s", req.URL.Path)
	ctx, span := tracer.Start(ctx, spanName)
	defer span.End()

	req.Header.Set("X-Plex-Token", p.cfg.Key)
	resp, err := p.httpClient.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "HTTP request failed")
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		err := fmt.Errorf("non-200 response: %d", resp.StatusCode)
		span.RecordError(err)
		span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", resp.StatusCode))
		return err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to read response body")
		return err
	}

	err = xml.Unmarshal(body, v)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to unmarshal XML")
		return err
	}

	span.SetStatus(codes.Ok, "Request successful")
	return nil
}

func (p *PlexHandler) getShows(ctx context.Context) *models.PlexShowLibraryData {
	ctx, span := tracer.Start(ctx, "getShows")
	defer span.End()

	showLibrary, err := p.repo.GetPreferredLibrary(ctx, libraryTypeShow)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to get show library")
		p.logger.ErrorContext(ctx, "Error getting show library", "error", err)
		return nil
	}

	url := fmt.Sprintf("%s/library/sections/%d/all?includeGuids=1", p.cfg.Host, showLibrary.Section)
	span.SetAttributes(attribute.String("plex.url", url))

	var showsResp models.PlexShowsResponse
	err = p.fetchAndUnmarshal(ctx, url, &showsResp)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to fetch shows")
		p.logger.ErrorContext(ctx, "Error fetching shows", "error", err)
		return nil
	}

	libraryData := &models.PlexShowLibraryData{
		Shows: make([]models.PlexShowData, 0, len(showsResp.Shows)),
	}

	_, processShowsSpan := tracer.Start(ctx, "processShows")
	processShowsSpan.SetAttributes(attribute.Int("show_count", len(showsResp.Shows)))
	defer processShowsSpan.End()

	for _, show := range showsResp.Shows {
		showCtx, showSpan := tracer.Start(ctx, "processShow")
		showSpan.SetAttributes(attribute.String("show_title", show.Title))
		// Extract TVDB ID from show Guids
		tvdbId := extractTvdbId(show.Guids)

		showData := models.PlexShowData{
			Id:       show.ShowKey,
			Title:    show.Title,
			ShowMeta: show.Key,
			Thumb:    show.Thumb,
			TvdbId:   tvdbId,
			Seasons:  make([]models.PlexSeasonData, 0),
		}

		// Get seasons for this show with includeGuids=1
		seasonsURL := fmt.Sprintf("%s%s?includeGuids=1", p.cfg.Host, show.Key)
		var seasonsResp models.PlexSeasonsResponse
		err = p.fetchAndUnmarshal(showCtx, seasonsURL, &seasonsResp)
		if err != nil {
			p.logger.ErrorContext(showCtx, "Error fetching seasons for show", "show", show.Title, "error", err)
			showSpan.RecordError(err)
			showSpan.SetStatus(codes.Error, "Failed to fetch seasons")
			showSpan.End()
			continue
		}

		seasonCount := 0
		episodeCount := 0
		for _, season := range seasonsResp.Seasons {
			seasonCtx, seasonSpan := tracer.Start(showCtx, "processSeason")
			seasonSpan.SetAttributes(attribute.Int("season_number", season.Index))

			seasonCount++
			// Extract TVDB ID for season
			seasonTvdbId := extractTvdbId(season.Guids)

			seasonData := models.PlexSeasonData{
				Id:           season.SeasonKey,
				SeasonMeta:   season.Key,
				SeasonNumber: season.Index,
				TvdbId:       seasonTvdbId,
				Episodes:     make([]models.PlexEpisodeData, 0),
			}

			// Get episodes for this season with includeGuids=1
			episodesURL := fmt.Sprintf("%s%s?includeGuids=1", p.cfg.Host, season.Key)
			var episodesResp models.PlexEpisodesResponse
			err = p.fetchAndUnmarshal(seasonCtx, episodesURL, &episodesResp)
			if err != nil {
				p.logger.ErrorContext(seasonCtx, "Error fetching episodes for season", "season", season.Title, "error", err)
				seasonSpan.RecordError(err)
				seasonSpan.SetStatus(codes.Error, "Failed to fetch episodes")
				seasonSpan.End()
				continue
			}

			for _, episode := range episodesResp.Videos {
				episodeCount++
				// Extract TVDB ID for episode
				episodeTvdbId := extractTvdbId(episode.Guids)

				episodeData := models.PlexEpisodeData{
					Id:            episode.EpisodeKey,
					EpisodeMeta:   episode.Key,
					EpisodeNumber: episode.Index,
					TvdbId:        episodeTvdbId,
					Media:         []models.PlexMediaData{},
				}
				for _, media := range episode.Media {
					mediaData := models.PlexMediaData{
						VideoResolution: media.VideoResolution,
					}
					if len(media.Part) > 0 {
						mediaData.File = media.Part[0].File
						mediaData.Id = media.Part[0].Id
					}
					episodeData.Media = append(episodeData.Media, mediaData)
				}
				seasonData.Episodes = append(seasonData.Episodes, episodeData)
			}

			seasonSpan.SetAttributes(attribute.Int("episode_count", len(seasonData.Episodes)))
			seasonSpan.SetStatus(codes.Ok, "Season processed successfully")
			seasonSpan.End()

			showData.Seasons = append(showData.Seasons, seasonData)
		}

		// Extract base directory from first available episode file path
		for _, season := range showData.Seasons {
			for _, episode := range season.Episodes {
				for _, media := range episode.Media {
					if media.File != "" {
						showData.BaseDirectory = extractShowBaseDirectory(media.File, showLibrary.Path)
						break
					}
				}
				if showData.BaseDirectory != "" {
					break
				}
			}
			if showData.BaseDirectory != "" {
				break
			}
		}

		showSpan.SetAttributes(
			attribute.Int("season_count", seasonCount),
			attribute.Int("episode_count", episodeCount))
		showSpan.SetStatus(codes.Ok, "Show processed successfully")
		showSpan.End()

		libraryData.Shows = append(libraryData.Shows, showData)
	}

	processShowsSpan.SetStatus(codes.Ok, "All shows processed")

	span.SetAttributes(attribute.Int("show.count", len(libraryData.Shows)))
	span.SetStatus(codes.Ok, "Shows fetched successfully")
	p.logger.InfoContext(ctx, "Fetched shows", "count", len(libraryData.Shows))

	return libraryData
}

// extractShowBaseDirectory extracts the show's base directory by combining the library path
// with the first subdirectory from the file path. This works regardless of file organization.
// Example: filePath="/data/shows/Breaking Bad/Season 01/ep.mkv", libraryPath="/data/shows" → "/data/shows/Breaking Bad"
func extractShowBaseDirectory(filePath, libraryPath string) string {
	if filePath == "" || libraryPath == "" {
		return ""
	}

	// Clean paths for consistent comparison
	filePath = filepath.Clean(filePath)
	libraryPath = filepath.Clean(libraryPath)

	// Remove library path prefix from file path
	if !strings.HasPrefix(filePath, libraryPath) {
		return ""
	}

	// Get relative path after library
	relativePath := strings.TrimPrefix(filePath, libraryPath)
	relativePath = strings.TrimPrefix(relativePath, string(filepath.Separator))

	// Split and take first directory component (the show directory)
	parts := strings.Split(relativePath, string(filepath.Separator))
	if len(parts) == 0 || parts[0] == "" {
		return ""
	}

	// Return library path + show directory
	return filepath.Join(libraryPath, parts[0])
}

// extractMovieBaseDirectory extracts the movie's base directory by combining the library path
// with the first subdirectory from the file path.
// Example: filePath="/data/movies/The Matrix (1999)/The Matrix.mkv", libraryPath="/data/movies" → "/data/movies/The Matrix (1999)"
func extractMovieBaseDirectory(filePath, libraryPath string) string {
	if filePath == "" || libraryPath == "" {
		return ""
	}

	// Clean paths for consistent comparison
	filePath = filepath.Clean(filePath)
	libraryPath = filepath.Clean(libraryPath)

	// Remove library path prefix from file path
	if !strings.HasPrefix(filePath, libraryPath) {
		return ""
	}

	// Get relative path after library
	relativePath := strings.TrimPrefix(filePath, libraryPath)
	relativePath = strings.TrimPrefix(relativePath, string(filepath.Separator))

	// Split and take first directory component (the movie directory)
	parts := strings.Split(relativePath, string(filepath.Separator))
	if len(parts) == 0 || parts[0] == "" {
		return ""
	}

	// Return library path + movie directory
	return filepath.Join(libraryPath, parts[0])
}
