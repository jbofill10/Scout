package handlers

import (
	"log/slog"
	"net/http"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/telemetry"

	"github.com/gin-gonic/gin"
)

// ratingsBatchMax caps one batch; the Library page asks in chunks.
const ratingsBatchMax = 200

// RatingsRequest names one title by TVDB id. Title and year are optional
// and let a film Wikidata has no item for be found by its Wikipedia article.
type RatingsRequest struct {
	Id        string `json:"id" binding:"required"`
	MediaType string `json:"mediaType" binding:"required"` // "series" or "movie"
	Title     string `json:"title,omitempty"`
	Year      string `json:"year,omitempty"`
}

// RatingsResponse is one title's scores, with the IMDb id they were found under.
type RatingsResponse struct {
	Id        string        `json:"id"`
	MediaType string        `json:"mediaType"`
	ImdbId    string        `json:"imdbId,omitempty"`
	Ratings   []tvdb.Rating `json:"ratings,omitempty"`
}

// RatingsHandler serves POST /ratings/batch: scores for titles that only
// have a TVDB id, such as the Plex library.
type RatingsHandler struct {
	rater  Rater
	logger *slog.Logger
}

// NewRatingsHandler returns a RatingsHandler rating through rater, which
// may be nil (every title then comes back without scores).
func NewRatingsHandler(rater Rater, logger *slog.Logger) *RatingsHandler {
	return &RatingsHandler{rater: rater, logger: logger}
}

// GetBatch handles POST /ratings/batch. Input: array of {id, mediaType};
// output: array of {id, mediaType, imdbId, ratings} in the same order.
func (h *RatingsHandler) GetBatch(c *gin.Context) {
	ctx := c.Request.Context()

	var requests []RatingsRequest
	if err := c.ShouldBindJSON(&requests); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body: expected array of {id, mediaType}"})
		return
	}
	if len(requests) == 0 || len(requests) > ratingsBatchMax {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Batch must hold between 1 and 200 titles"})
		return
	}

	media := make([]tvdb.Media, len(requests))
	for i, req := range requests {
		media[i] = tvdb.Media{Id: req.Id, Category: req.MediaType, Name: req.Title, Year: req.Year}
	}
	if h.rater != nil {
		h.rater.Attach(ctx, media, backgroundWait)
	}

	rated := 0
	results := make([]RatingsResponse, len(media))
	for i, item := range media {
		results[i] = RatingsResponse{Id: item.Id, MediaType: item.Category, ImdbId: item.ImdbID, Ratings: item.Ratings}
		if len(item.Ratings) > 0 {
			rated++
		}
	}
	h.logger.InfoContext(ctx, "Ratings batch", telemetry.WithTraceContext(ctx, "count", len(results), "rated", rated)...)
	c.JSON(http.StatusOK, results)
}
