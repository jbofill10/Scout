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
	logger  *slog.Logger
}

// NewExtendedHandler returns an ExtendedHandler backed by fetcher.
func NewExtendedHandler(fetcher ExtendedFetcher, logger *slog.Logger) *ExtendedHandler {
	return &ExtendedHandler{fetcher: fetcher, logger: logger}
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

	c.JSON(http.StatusOK, info)
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

	h.logger.InfoContext(ctx, "Batch extended request completed",
		"total_requests", len(requests), "responses", len(results))
	c.JSON(http.StatusOK, results)
}
