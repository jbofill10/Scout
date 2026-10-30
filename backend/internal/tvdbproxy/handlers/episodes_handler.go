package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"sync"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"

	"github.com/gin-gonic/gin"
)

// EpisodesFetcher fetches a series' episode metadata.
type EpisodesFetcher interface {
	SeriesEpisodes(ctx context.Context, seriesId string) (tvdb.TVDBSeriesResponse, error)
}

// EpisodesHandler serves POST /series/batch/episodes.
type EpisodesHandler struct {
	fetcher EpisodesFetcher
	logger  *slog.Logger
}

// NewEpisodesHandler returns an EpisodesHandler backed by fetcher.
func NewEpisodesHandler(fetcher EpisodesFetcher, logger *slog.Logger) *EpisodesHandler {
	return &EpisodesHandler{fetcher: fetcher, logger: logger}
}

// BatchEpisodesRequest represents a single request for episode metadata
type BatchEpisodesRequest struct {
	SeriesId string `json:"seriesId" binding:"required"`
}

// BatchEpisodesResponse wraps episode metadata with optional error
type BatchEpisodesResponse struct {
	Request *BatchEpisodesRequest    `json:"request"`
	Data    *tvdb.TVDBSeriesMetadata `json:"data,omitempty"`
	Error   string                   `json:"error,omitempty"`
}

// GetBatchEpisodes handles batch requests for series episode metadata
// POST /series/batch/episodes
// Input: Array of {seriesId: string}
// Output: Array of episode metadata responses (with partial failure support)
func (h *EpisodesHandler) GetBatchEpisodes(c *gin.Context) {
	ctx := c.Request.Context()

	var requests []BatchEpisodesRequest
	if err := c.ShouldBindJSON(&requests); err != nil {
		h.logger.ErrorContext(ctx, "Invalid batch episodes request body", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body: expected array of {seriesId}"})
		return
	}

	if len(requests) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Empty batch request"})
		return
	}

	h.logger.InfoContext(ctx, "Processing batch episodes request", "count", len(requests))

	// Use buffered channel to limit concurrent requests
	ch := make(chan struct{}, batchConcurrency)
	var wg sync.WaitGroup

	// Thread-safe result collection
	var mu sync.Mutex
	results := make([]BatchEpisodesResponse, 0, len(requests))

	for _, req := range requests {
		wg.Add(1)
		ch <- struct{}{} // Acquire semaphore

		go func(ctx context.Context, request BatchEpisodesRequest) {
			defer wg.Done()
			defer func() { <-ch }() // Release semaphore

			h.logger.InfoContext(ctx, "Fetching episodes in batch", "series_id", request.SeriesId)

			// Fetch episode metadata
			seriesResponse, err := h.fetcher.SeriesEpisodes(ctx, request.SeriesId)

			response := BatchEpisodesResponse{
				Request: &request,
			}

			if err != nil {
				// Log error but don't fail the entire batch
				h.logger.WarnContext(ctx, "Failed to fetch episodes in batch",
					"series_id", request.SeriesId,
					"error", err)
				response.Error = err.Error()
			} else {
				response.Data = &seriesResponse.Data
			}

			// Thread-safe append
			mu.Lock()
			results = append(results, response)
			mu.Unlock()
		}(ctx, req)
	}

	wg.Wait()
	close(ch)

	h.logger.InfoContext(ctx, "Batch episodes request completed",
		"total_requests", len(requests), "responses", len(results))
	c.JSON(http.StatusOK, results)
}
