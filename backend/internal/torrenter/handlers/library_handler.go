package handlers

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jbofill10/scout/backend/internal/torrenter/interactors"
	"github.com/jbofill10/scout/backend/pkg/library"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

type LibraryHandler struct {
	interactor *interactors.LibraryInteractor
	logger     *slog.Logger
}

func NewLibraryHandler(interactor *interactors.LibraryInteractor, logger *slog.Logger) *LibraryHandler {
	return &LibraryHandler{
		interactor: interactor,
		logger:     logger,
	}
}

// GetShows retrieves all shows in the library
// GET /library/shows
func (h *LibraryHandler) GetShows(c *gin.Context) {
	ctx := c.Request.Context()

	h.logger.InfoContext(ctx, "Handling library shows request")

	shows, err := h.interactor.GetLibraryShows(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to get library shows", telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve library shows: " + err.Error()})
		return
	}

	h.logger.InfoContext(ctx, "Library shows request completed successfully", telemetry.WithTraceContext(ctx, "count", len(shows))...)
	c.JSON(http.StatusOK, shows)
}

// GetShowEpisodes retrieves all episodes (downloaded + missing) for a show
// GET /library/shows/:tvdbId/episodes
func (h *LibraryHandler) GetShowEpisodes(c *gin.Context) {
	ctx := c.Request.Context()
	tvdbId := c.Param("tvdbId")

	if tvdbId == "" {
		h.logger.WarnContext(ctx, "Missing tvdbId parameter")
		c.JSON(http.StatusBadRequest, gin.H{"error": "tvdbId parameter is required"})
		return
	}

	h.logger.InfoContext(ctx, "Handling show episodes request", telemetry.WithTraceContext(ctx, "tvdb_id", tvdbId)...)

	episodes, err := h.interactor.GetShowEpisodes(ctx, tvdbId)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to get show episodes", telemetry.WithTraceContext(ctx, "tvdb_id", tvdbId, "error", err)...)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve show episodes: " + err.Error()})
		return
	}

	h.logger.InfoContext(ctx, "Show episodes request completed successfully",
		telemetry.WithTraceContext(ctx, "tvdb_id", tvdbId, "count", len(episodes))...)
	c.JSON(http.StatusOK, episodes)
}

// GetShowMetadataStatus retrieves TVDB sync status for a show
// GET /library/shows/:tvdbId/metadata-status
func (h *LibraryHandler) GetShowMetadataStatus(c *gin.Context) {
	ctx := c.Request.Context()
	tvdbId := c.Param("tvdbId")

	if tvdbId == "" {
		h.logger.WarnContext(ctx, "Missing tvdbId parameter")
		c.JSON(http.StatusBadRequest, gin.H{"error": "tvdbId parameter is required"})
		return
	}

	h.logger.InfoContext(ctx, "Handling metadata status request",
		telemetry.WithTraceContext(ctx, "tvdb_id", tvdbId)...)

	status, err := h.interactor.GetShowMetadataStatus(ctx, tvdbId)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to get metadata status",
			telemetry.WithTraceContext(ctx, "tvdb_id", tvdbId, "error", err)...)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve metadata status: " + err.Error()})
		return
	}

	h.logger.InfoContext(ctx, "Metadata status request completed successfully",
		telemetry.WithTraceContext(ctx, "tvdb_id", tvdbId)...)
	c.JSON(http.StatusOK, status)
}

// GetMovies retrieves all movies in the library
// GET /library/movies
func (h *LibraryHandler) GetMovies(c *gin.Context) {
	ctx := c.Request.Context()

	h.logger.InfoContext(ctx, "Handling library movies request")

	movies, err := h.interactor.GetLibraryMovies(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to get library movies", telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve library movies: " + err.Error()})
		return
	}

	h.logger.InfoContext(ctx, "Library movies request completed successfully", telemetry.WithTraceContext(ctx, "count", len(movies))...)
	c.JSON(http.StatusOK, movies)
}

// SyncEpisodes stores TVDB episode metadata for a show
// POST /sync-episodes
// Request body: { "seriesTvdbId": "123", "episodes": [...] }
func (h *LibraryHandler) SyncEpisodes(c *gin.Context) {
	ctx := c.Request.Context()

	var request struct {
		SeriesTvdbId string                 `json:"seriesTvdbId" binding:"required"`
		Episodes     []library.TvdbEpisode `json:"episodes" binding:"required"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		h.logger.ErrorContext(ctx, "Failed to bind sync episodes request", telemetry.WithTraceContext(ctx, "error", err)...)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format: " + err.Error()})
		return
	}

	h.logger.InfoContext(ctx, "Handling sync episodes request",
		telemetry.WithTraceContext(ctx, "series_tvdb_id", request.SeriesTvdbId, "episode_count", len(request.Episodes))...)

	err := h.interactor.SyncShowEpisodes(ctx, request.SeriesTvdbId, request.Episodes)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to sync episodes",
			telemetry.WithTraceContext(ctx, "series_tvdb_id", request.SeriesTvdbId, "error", err)...)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to sync episodes: " + err.Error()})
		return
	}

	h.logger.InfoContext(ctx, "Sync episodes request completed successfully",
		telemetry.WithTraceContext(ctx, "series_tvdb_id", request.SeriesTvdbId, "episode_count", len(request.Episodes))...)
	c.JSON(http.StatusOK, gin.H{"message": "Episodes synced successfully", "count": len(request.Episodes)})
}
