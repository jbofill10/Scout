package handlers

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jbofill10/scout/backend/internal/webserver/clients"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

// ratingsBatchMax mirrors tvdb-proxy's cap on one batch.
const ratingsBatchMax = 200

// RatingsClient fetches scores for titles by TVDB id; TVDBProxyClient implements it.
type RatingsClient interface {
	GetRatingsBatch(ctx context.Context, requests []clients.RatingsRequest) ([]clients.RatingsResponse, error)
}

// RatingsHandler relays POST /api/ratings/batch to tvdb-proxy, for cards
// that only have a TVDB id (the Plex library).
type RatingsHandler struct {
	client RatingsClient
	logger *slog.Logger
}

// NewRatingsHandler returns a RatingsHandler relaying through client.
func NewRatingsHandler(client RatingsClient, logger *slog.Logger) *RatingsHandler {
	return &RatingsHandler{client: client, logger: logger}
}

// PostBatch handles POST /api/ratings/batch. Input: array of {id, mediaType};
// output: array of {id, mediaType, imdbId, ratings} in the same order.
func (h *RatingsHandler) PostBatch(c *gin.Context) {
	ctx := c.Request.Context()

	var requests []clients.RatingsRequest
	if err := c.ShouldBindJSON(&requests); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body: expected array of {id, mediaType}"})
		return
	}
	if len(requests) == 0 || len(requests) > ratingsBatchMax {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Batch must hold between 1 and 200 titles"})
		return
	}

	results, err := h.client.GetRatingsBatch(ctx, requests)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to fetch ratings batch",
			telemetry.WithTraceContext(ctx, "count", len(requests), "error", err)...)
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to fetch ratings from tvdb-proxy"})
		return
	}
	c.JSON(http.StatusOK, results)
}
