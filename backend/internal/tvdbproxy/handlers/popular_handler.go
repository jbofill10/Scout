package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/jbofill10/scout/backend/internal/tvdbproxy/popular"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"

	"github.com/gin-gonic/gin"
)

// GenreLookup resolves a genre name to its TVDB id.
type GenreLookup interface {
	LookupID(name string) (int, bool)
}

// PopularRanker ranks the media behind one popular row; popular.Service implements it.
type PopularRanker interface {
	Rank(ctx context.Context, kind popular.Kind, genre string, limit int) ([]tvdb.Media, error)
}

// PopularHandler serves GET /series/popular and GET /movies/popular.
type PopularHandler struct {
	ranker PopularRanker
	genres GenreLookup
	rater  Rater
	logger *slog.Logger
}

// NewPopularHandler returns a PopularHandler that ranks through ranker and
// validates genre names through genres. rater may be nil, in which case rows
// carry no ratings.
func NewPopularHandler(ranker PopularRanker, genres GenreLookup, rater Rater, logger *slog.Logger) *PopularHandler {
	return &PopularHandler{ranker: ranker, genres: genres, rater: rater, logger: logger}
}

// GetPopularSeries handles GET /series/popular
func (h *PopularHandler) GetPopularSeries(c *gin.Context) {
	h.serve(c, popular.KindSeries)
}

// GetPopularMovies handles GET /movies/popular
func (h *PopularHandler) GetPopularMovies(c *gin.Context) {
	h.serve(c, popular.KindMovie)
}

// serve answers one row: `genre` (optional, a TVDB genre name) and `limit`
// (default 20, at most 50). No episode enrichment: the webserver fetches
// episodes separately for the rows that need them.
func (h *PopularHandler) serve(c *gin.Context, kind popular.Kind) {
	ctx := c.Request.Context()

	genreName := c.Query("genre")
	limit, err := strconv.Atoi(c.DefaultQuery("limit", strconv.Itoa(popular.DefaultLimit)))
	if err != nil || limit <= 0 {
		limit = popular.DefaultLimit
	}
	if limit > popular.MaxLimit {
		limit = popular.MaxLimit
	}

	if genreName != "" {
		if _, ok := h.genres.LookupID(genreName); !ok {
			h.logger.WarnContext(ctx, "Genre not found in cache", "genre", genreName)
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Genre '%s' not found", genreName)})
			return
		}
	}

	h.logger.InfoContext(ctx, "Fetching popular row", "kind", string(kind), "genre", genreName, "limit", limit)

	results, err := h.ranker.Rank(ctx, kind, genreName, limit)
	if err != nil {
		if errors.Is(err, popular.ErrUnknownGenre) {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Genre '%s' not found", genreName)})
			return
		}
		h.logger.ErrorContext(ctx, "Failed to rank popular row", "kind", string(kind), "genre", genreName, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch popular " + string(kind)})
		return
	}
	if results == nil {
		results = []tvdb.Media{}
	}
	// Rows are rated as they are built, so this is a cache lookup per item
	// that only matters for a row ranked before the rater was ready.
	if h.rater != nil {
		h.rater.Attach(ctx, results, interactiveWait)
	}

	h.logger.InfoContext(ctx, "Returning popular row", "kind", string(kind), "count", len(results))
	c.JSON(http.StatusOK, results)
}
