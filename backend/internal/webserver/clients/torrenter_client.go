package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/jbofill10/scout/backend/pkg/library"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type TorrenterClient struct {
	Host   string
	Client *http.Client
}

func NewTorrenterClient(host string) *TorrenterClient {
	return &TorrenterClient{
		Host: host,
		Client: &http.Client{
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		},
	}
}

func (c *TorrenterClient) Download(ctx context.Context, req tvdb.Media) error {
	url := fmt.Sprintf("http://%s/download", c.Host)
	slog.InfoContext(ctx, "Sending download request", "url", url, "media", req.Name)
	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(req); err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, buf)
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			slog.WarnContext(ctx, "Failed to close response body", "error", cerr)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("torrenter returned status %d", resp.StatusCode)
	}
	return nil
}

func (c *TorrenterClient) MediaExists(ctx context.Context, hash string) (bool, error) {
	url := fmt.Sprintf("http://%s/media/%s", c.Host, hash)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, err
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return false, err
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			slog.WarnContext(ctx, "Failed to close response body", "error", cerr)
		}
	}()
	if resp.StatusCode == http.StatusOK {
		return true, nil
	}
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	return false, fmt.Errorf("unexpected status: %d", resp.StatusCode)
}

// MediaExistsBatch checks multiple media items at once. It POSTs an array of
// tvdb.Media to the torrenter's /media/exists endpoint and expects a JSON
// response with `exists` and optional `in_progress` maps keyed by a unique
// identifier (we use the Media.Hash field if present).
func (c *TorrenterClient) MediaExistsBatch(ctx context.Context, items []tvdb.Media) (map[string]bool, map[string]bool, error) {
	url := fmt.Sprintf("http://%s/media/exists", c.Host)
	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(items); err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", url, buf)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			slog.WarnContext(ctx, "Failed to close response body", "error", cerr)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("torrenter returned status %d", resp.StatusCode)
	}

	var body struct {
		Exists     map[string]bool `json:"exists"`
		InProgress map[string]bool `json:"in_progress"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, nil, err
	}

	// Normalize nil maps to empty maps for caller convenience
	if body.Exists == nil {
		body.Exists = make(map[string]bool)
	}
	if body.InProgress == nil {
		body.InProgress = make(map[string]bool)
	}

	return body.Exists, body.InProgress, nil
}

// GetStatusBatch retrieves the download status for multiple media items at once.
// It POSTs an array of StatusRequest to the torrenter's /status/batch endpoint
// and returns a StatusBatchResponse with show and movie status information.
func (c *TorrenterClient) GetStatusBatch(ctx context.Context, requests []tvdb.StatusRequest) (tvdb.StatusBatchResponse, error) {
	url := fmt.Sprintf("http://%s/status/batch", c.Host)
	slog.InfoContext(ctx, "calling torrenter status batch", "request_count", len(requests))

	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(requests); err != nil {
		return tvdb.StatusBatchResponse{}, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, buf)
	if err != nil {
		return tvdb.StatusBatchResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(req)
	if err != nil {
		return tvdb.StatusBatchResponse{}, err
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			slog.WarnContext(ctx, "Failed to close response body", "error", cerr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return tvdb.StatusBatchResponse{}, fmt.Errorf("torrenter returned status %d", resp.StatusCode)
	}

	var response tvdb.StatusBatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return tvdb.StatusBatchResponse{}, err
	}

	return response, nil
}

// GetLibraryShows retrieves all shows in the user's library from torrenter
func (c *TorrenterClient) GetLibraryShows(ctx context.Context) ([]library.LibraryShow, error) {
	url := fmt.Sprintf("http://%s/library/shows", c.Host)
	slog.InfoContext(ctx, "Fetching library shows from torrenter", "url", url)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			slog.WarnContext(ctx, "Failed to close response body", "error", cerr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("torrenter returned status %d", resp.StatusCode)
	}

	var shows []library.LibraryShow
	if err := json.NewDecoder(resp.Body).Decode(&shows); err != nil {
		return nil, err
	}

	return shows, nil
}

// GetLibraryMovies retrieves all movies in the user's library from torrenter
func (c *TorrenterClient) GetLibraryMovies(ctx context.Context) ([]library.LibraryMovie, error) {
	url := fmt.Sprintf("http://%s/library/movies", c.Host)
	slog.InfoContext(ctx, "Fetching library movies from torrenter", "url", url)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			slog.WarnContext(ctx, "Failed to close response body", "error", cerr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("torrenter returned status %d", resp.StatusCode)
	}

	var movies []library.LibraryMovie
	if err := json.NewDecoder(resp.Body).Decode(&movies); err != nil {
		return nil, err
	}

	return movies, nil
}

// GetShowEpisodes retrieves all episodes (downloaded + missing) for a show from torrenter
func (c *TorrenterClient) GetShowEpisodes(ctx context.Context, tvdbId string) ([]library.EpisodeWithStatus, error) {
	url := fmt.Sprintf("http://%s/library/shows/%s/episodes", c.Host, tvdbId)
	slog.InfoContext(ctx, "Fetching show episodes from torrenter", "url", url, "tvdb_id", tvdbId)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			slog.WarnContext(ctx, "Failed to close response body", "error", cerr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("torrenter returned status %d", resp.StatusCode)
	}

	var episodes []library.EpisodeWithStatus
	if err := json.NewDecoder(resp.Body).Decode(&episodes); err != nil {
		return nil, err
	}

	return episodes, nil
}

// GetShowMetadataStatus retrieves TVDB sync status for a show from torrenter
func (c *TorrenterClient) GetShowMetadataStatus(ctx context.Context, tvdbId string) (library.MetadataStatus, error) {
	url := fmt.Sprintf("http://%s/library/shows/%s/metadata-status", c.Host, tvdbId)
	slog.InfoContext(ctx, "Fetching show metadata status from torrenter", "url", url, "tvdb_id", tvdbId)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return library.MetadataStatus{}, err
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return library.MetadataStatus{}, err
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			slog.WarnContext(ctx, "Failed to close response body", "error", cerr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return library.MetadataStatus{}, fmt.Errorf("torrenter returned status %d", resp.StatusCode)
	}

	var status library.MetadataStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return library.MetadataStatus{}, err
	}

	return status, nil
}

// SyncEpisodes sends TVDB episode metadata to torrenter for storage
// This enables "missing episode" detection by comparing TVDB data with Plex library
func (c *TorrenterClient) SyncEpisodes(ctx context.Context, seriesTvdbId string, episodes []library.TvdbEpisode) error {
	url := fmt.Sprintf("http://%s/sync-episodes", c.Host)
	slog.InfoContext(ctx, "Syncing episodes to torrenter", "url", url, "series_tvdb_id", seriesTvdbId, "episode_count", len(episodes))

	requestBody := struct {
		SeriesTvdbId string                 `json:"seriesTvdbId"`
		Episodes     []library.TvdbEpisode `json:"episodes"`
	}{
		SeriesTvdbId: seriesTvdbId,
		Episodes:     episodes,
	}

	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(requestBody); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			slog.WarnContext(ctx, "Failed to close response body", "error", cerr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("torrenter returned status %d", resp.StatusCode)
	}

	return nil
}
