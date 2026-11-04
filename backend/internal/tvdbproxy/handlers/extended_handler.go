package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"

	"github.com/gin-gonic/gin"
)

// batchConcurrency caps parallel TVDB lookups per batch request.
const batchConcurrency = 20

// ExtendedFetcher fetches enriched extended records.
type ExtendedFetcher interface {
	Extended(ctx context.Context, mediaId string, mediaType string) (*tvdb.TVDBSeriesExtendedResponse, error)
}

// ExtendedHandler serves GET /series/:id/extended and POST /series/batch/extended.
type ExtendedHandler struct {
	fetcher ExtendedFetcher
	rater   Rater
	logger  *slog.Logger
}

// NewExtendedHandler returns an ExtendedHandler backed by fetcher. rater may
// be nil, in which case records carry no ratings.
func NewExtendedHandler(fetcher ExtendedFetcher, rater Rater, logger *slog.Logger) *ExtendedHandler {
	return &ExtendedHandler{fetcher: fetcher, rater: rater, logger: logger}
}

// BatchExtendedRequest represents a single request in the batch
type BatchExtendedRequest struct {
	Id        string `json:"id" binding:"required"`
	MediaType string `json:"mediaType" binding:"required"`
}

// BatchExtendedResponse wraps a single response with optional error
type BatchExtendedResponse struct {
	Request *BatchExtendedRequest            `json:"request"`
	Data    *tvdb.TVDBSeriesExtendedResponse `json:"data,omitempty"`
	Error   string                           `json:"error,omitempty"`
}

// GetExtended handles GET /series/:id/extended?mediaType=...
func (h *ExtendedHandler) GetExtended(c *gin.Context) {
	mediaId := c.Param("id")
	mediaType := c.Query("mediaType")
	ctx := c.Request.Context()
	h.logger.InfoContext(ctx, "Fetching extended information", "media_id", mediaId, "media_type", mediaType)

	info, err := h.fetcher.Extended(ctx, mediaId, mediaType)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to fetch extended information", "media_id", mediaId, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Unable to get extended information for media ID %s: %v", mediaId, err),
		})
		return
	}
	rateExtended(ctx, h.rater, info)

	c.JSON(http.StatusOK, info)
}

// rateBatch fills the ratings on every fetched record in one lookup, so a
// row's worth of titles costs one scores query rather than one each.
func (h *ExtendedHandler) rateBatch(ctx context.Context, results []BatchExtendedResponse) {
	if h.rater == nil {
		return
	}
	ids := make([]string, 0, len(results))
	for _, result := range results {
		if result.Data != nil {
			if id := result.Data.Data.ImdbID(); id != "" {
				ids = append(ids, id)
			}
		}
	}
	if len(ids) == 0 {
		return
	}
	scores := h.rater.LookupMany(ctx, ids)
	for i := range results {
		if results[i].Data == nil {
			continue
		}
		if id := results[i].Data.Data.ImdbID(); id != "" {
			results[i].Data.Data.Ratings = scores[id]
		}
	}
}

// GetBatchExtended handles batch requests for extended media information
// POST /series/batch/extended
// Input: Array of {id: string, mediaType: string}
// Output: Array of extended responses (with partial failure support)
func (h *ExtendedHandler) GetBatchExtended(c *gin.Context) {
	ctx := c.Request.Context()

	var requests []BatchExtendedRequest
	if err := c.ShouldBindJSON(&requests); err != nil {
		h.logger.ErrorContext(ctx, "Invalid batch request body", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body: expected array of {id, mediaType}"})
		return
	}

	if len(requests) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Empty batch request"})
		return
	}

	h.logger.InfoContext(ctx, "Processing batch extended request", "count", len(requests))

	// Use buffered channel to limit concurrent requests (similar to episode enrichment pattern)
	ch := make(chan struct{}, batchConcurrency)
	var wg sync.WaitGroup

	// Thread-safe result collection
	var mu sync.Mutex
	results := make([]BatchExtendedResponse, 0, len(requests))

	for _, req := range requests {
		wg.Add(1)
		ch <- struct{}{} // Acquire semaphore

		go func(ctx context.Context, request BatchExtendedRequest) {
			defer wg.Done()
			defer func() { <-ch }() // Release semaphore

			h.logger.InfoContext(ctx, "Fetching extended info in batch",
				"media_id", request.Id, "media_type", request.MediaType)

			// Fetch extended information
			info, err := h.fetcher.Extended(ctx, request.Id, request.MediaType)

			response := BatchExtendedResponse{
				Request: &request,
			}

			if err != nil {
				// Log error but don't fail the entire batch
				h.logger.WarnContext(ctx, "Failed to fetch extended info in batch",
					"media_id", request.Id,
					"media_type", request.MediaType,
					"error", err)
				response.Error = err.Error()
			} else {
				response.Data = info
			}

			// Thread-safe append
			mu.Lock()
			results = append(results, response)
			mu.Unlock()
		}(ctx, req)
	}

	wg.Wait()
	close(ch)

	h.rateBatch(ctx, results)

	h.logger.InfoContext(ctx, "Batch extended request completed",
		"total_requests", len(requests), "responses", len(results))
	c.JSON(http.StatusOK, results)
}
