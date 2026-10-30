// Package handlers holds tvdb-proxy's gin handlers.
package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/search"
	"github.com/jbofill10/scout/backend/pkg/telemetry"

	"github.com/gin-gonic/gin"
)

// Searcher answers structured media searches.
type Searcher interface {
	Search(ctx context.Context, q search.Query, mediaType string) ([]tvdb.Media, error)
}

// SearchHandler serves GET /series.
type SearchHandler struct {
	searcher Searcher
	logger   *slog.Logger
}

// NewSearchHandler returns a SearchHandler backed by searcher.
func NewSearchHandler(searcher Searcher, logger *slog.Logger) *SearchHandler {
	return &SearchHandler{searcher: searcher, logger: logger}
}

// GetSeries handles GET /series: a structured media search. Besides mediaName
// and mediaType it accepts the filters search.Query carries (year, director,
// person, personRole, company, country, language, imdb, altName); see
// pkg/search for their meaning. At least one of mediaName, person, director
// or imdb must be present.
func (h *SearchHandler) GetSeries(c *gin.Context) {
	// Extract context for trace propagation
	ctx := c.Request.Context()

	query := search.FromProxyParams(c.Request.URL.Query())
	mediaType := c.Query(search.ParamMediaType)

	if query.IsEmpty() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Bad Request: 'mediaName' parameter is missing"})
		return
	}
	if mediaType != "series" && mediaType != "movie" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Bad Request: 'mediaType' must be 'series' or 'movie'"})
		return
	}

	response, err := h.searcher.Search(ctx, query, mediaType)
	if err != nil {
		h.logger.ErrorContext(ctx, "Search failed",
			telemetry.WithTraceContext(ctx, "title", query.Title, "error", err)...)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintln("Internal Server Error: Unable to query show", query.Title),
		})
		return
	}
	h.logger.InfoContext(ctx, "Search response", telemetry.WithTraceContext(ctx, "count", len(response))...)
	c.JSON(http.StatusOK, response)
}
