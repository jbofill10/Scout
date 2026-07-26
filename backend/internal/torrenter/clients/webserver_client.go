package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"
)

// WebserverClient handles HTTP communication with the webserver service
type WebserverClient struct {
	BaseURL    string
	HTTPClient *http.Client
	Logger     *slog.Logger
}

// NewWebserverClient creates a new webserver HTTP client
func NewWebserverClient(baseURL string, logger *slog.Logger) *WebserverClient {
	return &WebserverClient{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		Logger: logger,
	}
}

// EpisodesRequest represents a request for episodes of a single series
type EpisodesRequest struct {
	SeriesId string `json:"seriesId"`
}

// EpisodesResponse represents the response for a single series' episodes
type EpisodesResponse struct {
	Request *EpisodesRequest         `json:"request"`
	Data    *tvdb.TVDBSeriesMetadata `json:"data,omitempty"`
	Error   string                   `json:"error,omitempty"`
}

// GetEpisodesBatch fetches episode metadata for multiple shows from webserver's TVDB batch endpoint
func (c *WebserverClient) GetEpisodesBatch(
	ctx context.Context,
	requests []EpisodesRequest,
) ([]EpisodesResponse, error) {
	url := fmt.Sprintf("%s/tvdb/batch/episodes", c.BaseURL)

	// Marshal request body
	body, err := json.Marshal(requests)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal batch episodes request: %w", err)
	}

	// Create HTTP request with context for tracing
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Execute request
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute batch episodes request: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			c.Logger.WarnContext(ctx, "Failed to close response body", "error", cerr)
		}
	}()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("webserver returned status %d", resp.StatusCode)
	}

	// Decode response
	var results []EpisodesResponse
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&results); err != nil {
		return nil, fmt.Errorf("failed to decode batch episodes response: %w", err)
	}

	return results, nil
}
