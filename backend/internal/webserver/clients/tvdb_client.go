package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/search"
	"log/slog"
	"net/http"
	"time"

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
			Timeout:   30 * time.Second,
		},
	}
}

// Search asks tvdb-proxy for media matching a parsed query: the title plus any
// year, person, director, studio/network, country, language or IMDb filters.
func (c *TVDBProxyClient) Search(ctx context.Context, mediaType string, query search.Query) ([]tvdb.Media, error) {
	searchURL := fmt.Sprintf("http://%s/series?%s", c.Host, query.ProxyParams(mediaType).Encode())
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
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tvdb-proxy search returned status %d", resp.StatusCode)
	}
	var results []tvdb.Media
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&results); err != nil {
		return nil, err
	}
	if results == nil {
		results = []tvdb.Media{}
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

		results = append(results, ExtendedToMedia(batchResp.Data.Data, batchResp.Request.MediaType))
	}

	return results, nil
}

// GetMediaDetails fetches the extended record for a single media item and maps
// it to the Media shape the UI consumes for its detail view.
func (c *TVDBProxyClient) GetMediaDetails(ctx context.Context, mediaId, mediaType string) (tvdb.Media, error) {
	info, err := c.GetExtendedInfo(ctx, mediaId, mediaType)
	if err != nil {
		return tvdb.Media{}, err
	}

	if info.Data.Id == 0 {
		return tvdb.Media{}, fmt.Errorf("no extended data for media %s", mediaId)
	}

	return ExtendedToMedia(info.Data, mediaType), nil
}

// ExtendedToMedia converts a TVDB extended record into a Media object. Movie and
// series extended responses share the struct, so runtime is taken from whichever
// field the media type populates.
func ExtendedToMedia(data tvdb.TVDBSeriesExtendedData, mediaType string) tvdb.Media {
	aliases := make([]string, 0, len(data.Aliases))
	for _, alias := range data.Aliases {
		aliases = append(aliases, alias.Name)
	}

	genres := make([]string, 0, len(data.Genres))
	anime := false
	for _, genre := range data.Genres {
		genres = append(genres, genre.Name)
		if genre.Name == "Anime" || (mediaType == "movie" && genre.Name == "Animation") {
			anime = true
		}
	}

	runtime := data.AverageRuntime
	if mediaType == "movie" {
		runtime = data.Runtime
	}

	return tvdb.Media{
		Id:               fmt.Sprintf("%d", data.Id),
		Name:             data.Name,
		OriginalName:     data.Name,
		Category:         mediaType,
		Anime:            anime,
		ImageUrl:         data.Image,
		Slug:             data.Slug,
		Status:           data.Status,
		Overview:         data.Overview,
		Tagline:          data.Tagline,
		Year:             data.Year,
		Aliases:          aliases,
		Genres:           genres,
		Runtime:          runtime,
		ContentRating:    pickContentRating(data.ContentRatings),
		OriginalCountry:  data.OriginalCountry,
		OriginalLanguage: data.OriginalLanguage,
		Score:            data.Score,
		Metadata: tvdb.TVDBSeriesMetadata{
			FirstAired: data.FirstAired,
			LastAired:  data.LastAired,
			Episodes:   data.Episodes,
			Score:      data.Score,
		},
	}
}

// pickContentRating prefers the US rating, which is the one the UI's audience
// recognises, and otherwise falls back to the first rating TVDB returned.
func pickContentRating(ratings []tvdb.ContentRating) string {
	if len(ratings) == 0 {
		return ""
	}
	for _, rating := range ratings {
		if rating.Country == "usa" {
			return rating.Name
		}
	}
	return ratings[0].Name
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
