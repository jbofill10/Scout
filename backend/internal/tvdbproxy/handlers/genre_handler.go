package handlers

import (
	"context"
	"log/slog"
	"net/http"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"

	"github.com/gin-gonic/gin"
)

// GenreCache is the genre cache as the genre handler uses it.
type GenreCache interface {
	NeedsRefresh() bool
	Refresh(ctx context.Context) error
	Genres() []tvdb.Genres
}

// GenreHandler serves GET /genres.
type GenreHandler struct {
	cache  GenreCache
	logger *slog.Logger
}

// NewGenreHandler returns a GenreHandler backed by cache.
func NewGenreHandler(cache GenreCache, logger *slog.Logger) *GenreHandler {
	return &GenreHandler{cache: cache, logger: logger}
}

// GetGenres handles GET /genres, refreshing the cache when it is stale or empty.
func (h *GenreHandler) GetGenres(c *gin.Context) {
	ctx := c.Request.Context()

	// Refresh cache if needed
	if h.cache.NeedsRefresh() {
		h.logger.InfoContext(ctx, "Genre cache is stale, refreshing")
		if err := h.cache.Refresh(ctx); err != nil {
			h.logger.ErrorContext(ctx, "Failed to refresh genre cache, returning cached data", "error", err)
			// Continue with cached data if available
		}
	}

	genres := h.cache.Genres()
	if len(genres) == 0 {
		// Try to refresh if cache is empty
		if err := h.cache.Refresh(ctx); err != nil {
			h.logger.ErrorContext(ctx, "Genre cache is empty and refresh failed", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch genres"})
			return
		}
		genres = h.cache.Genres()
	}

	c.JSON(http.StatusOK, gin.H{"data": genres})
}
