package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"log/slog"
	"net/http"
	"net/url"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type TVDBProxyClient struct {
	Host   string
	Client *http.Client
}

func NewTVDBProxyClient(host string) *TVDBProxyClient {
	return &TVDBProxyClient{
		Host: host,
		Client: &http.Client{
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		},
	}
}

func (c *TVDBProxyClient) Search(ctx context.Context, mediaType, mediaName string) ([]tvdb.Media, error) {
	searchURL := fmt.Sprintf("http://%s/series?mediaType=%s&mediaName=%s", c.Host, mediaType, url.QueryEscape(mediaName))
	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
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
	var results []tvdb.Media
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&results); err != nil {
		return nil, err
	}
	return results, nil
}

func (c *TVDBProxyClient) GetExtendedInfo(
	ctx context.Context, mediaId, mediaType string,
) (tvdb.TVDBSeriesExtendedResponse, error) {
	url := fmt.Sprintf("http://%s/series/%s/extended?mediaType=%s", c.Host, mediaId, mediaType)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return tvdb.TVDBSeriesExtendedResponse{}, err
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return tvdb.TVDBSeriesExtendedResponse{}, err
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			slog.WarnContext(ctx, "Failed to close response body", "error", cerr)
		}
	}()
	var info tvdb.TVDBSeriesExtendedResponse
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&info); err != nil {
		return tvdb.TVDBSeriesExtendedResponse{}, err
	}
	return info, nil
}

func (c *TVDBProxyClient) GetPopularShows(ctx context.Context, genre string, limit int) ([]tvdb.Media, error) {
	url := fmt.Sprintf("http://%s/series/popular?limit=%d", c.Host, limit)
	if genre != "" {
		url = fmt.Sprintf("%s&genre=%s", url, genre)
	}
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
	var shows []tvdb.Media
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&shows); err != nil {
		return nil, err
	}
	return shows, nil
}

func (c *TVDBProxyClient) GetPopularMovies(ctx context.Context, genre string, limit int) ([]tvdb.Media, error) {
	url := fmt.Sprintf("http://%s/movies/popular?limit=%d", c.Host, limit)
	if genre != "" {
		url = fmt.Sprintf("%s&genre=%s", url, genre)
	}
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
	var movies []tvdb.Media
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&movies); err != nil {
		return nil, err
	}
	return movies, nil
}

type ExtendedRequest struct {
	Id        string `json:"id"`
	MediaType string `json:"mediaType"` // "series" or "movie"
}

type ExtendedBatchResponse struct {
	Request *ExtendedRequest                 `json:"request"`
	Data    *tvdb.TVDBSeriesExtendedResponse `json:"data,omitempty"`
	Error   string                           `json:"error,omitempty"`
}

func (c *TVDBProxyClient) GetExtendedBatch(
	ctx context.Context, requests []ExtendedRequest,
) ([]tvdb.Media, error) {
	url := fmt.Sprintf("http://%s/series/batch/extended", c.Host)

	// Marshal request body
	body, err := json.Marshal(requests)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal batch request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

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
		return nil, fmt.Errorf("tvdb-proxy returned status %d", resp.StatusCode)
	}

	// Decode batch response format
	var batchResults []ExtendedBatchResponse
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&batchResults); err != nil {
		return nil, fmt.Errorf("failed to decode batch response: %w", err)
	}

	// Convert batch responses to Media objects
	results := make([]tvdb.Media, 0, len(batchResults))
	for _, batchResp := range batchResults {
		if batchResp.Error != "" || batchResp.Data == nil {
			// Skip failed requests
			slog.WarnContext(ctx, "Skipping failed batch extended request",
				"id", batchResp.Request.Id,
				"error", batchResp.Error)
			continue
		}

		// Convert aliases from []Alias to []string
		aliases := make([]string, 0, len(batchResp.Data.Data.Aliases))
		for _, alias := range batchResp.Data.Data.Aliases {
			aliases = append(aliases, alias.Name)
		}

		// Convert extended response to Media
		media := tvdb.Media{
			Id:           fmt.Sprintf("%d", batchResp.Data.Data.Id),
			Name:         batchResp.Data.Data.Name,
			OriginalName: batchResp.Data.Data.Name,
			Category:     batchResp.Request.MediaType,
			ImageUrl:     batchResp.Data.Data.Image,
			Slug:         batchResp.Data.Data.Slug,
			Status:       batchResp.Data.Data.Status,
			Overview:     batchResp.Data.Data.Overview,
			Year:         batchResp.Data.Data.Year,
			Aliases:      aliases,
			Score:        batchResp.Data.Data.Score,
			Metadata: tvdb.TVDBSeriesMetadata{
				Episodes: batchResp.Data.Data.Episodes,
			},
		}

		// Detect anime from genres
		for _, genre := range batchResp.Data.Data.Genres {
			if genre.Name == "Anime" || (batchResp.Request.MediaType == "movie" && genre.Name == "Animation") {
				media.Anime = true
				break
			}
		}

		results = append(results, media)
	}

	return results, nil
}

type EpisodesRequest struct {
	SeriesId string `json:"seriesId"`
}

type EpisodesResponse struct {
	Request *EpisodesRequest         `json:"request"`
	Data    *tvdb.TVDBSeriesMetadata `json:"data,omitempty"`
	Error   string                   `json:"error,omitempty"`
}

func (c *TVDBProxyClient) GetEpisodesBatch(
	ctx context.Context, requests []EpisodesRequest,
) ([]EpisodesResponse, error) {
	url := fmt.Sprintf("http://%s/series/batch/episodes", c.Host)

	// Marshal request body
	body, err := json.Marshal(requests)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal batch episodes request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

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
		return nil, fmt.Errorf("tvdb-proxy returned status %d", resp.StatusCode)
	}

	var results []EpisodesResponse
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&results); err != nil {
		return nil, fmt.Errorf("failed to decode batch episodes response: %w", err)
	}

	return results, nil
}

type Genre struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

func (c *TVDBProxyClient) GetGenres(ctx context.Context) ([]Genre, error) {
	url := fmt.Sprintf("http://%s/genres", c.Host)
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
	// tvdb-proxy returns {"data": [...]} structure
	var response struct {
		Data []Genre `json:"data"`
	}
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&response); err != nil {
		return nil, err
	}
	return response.Data, nil
}
