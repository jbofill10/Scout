package clients

import (
	"context"
	"encoding/json"
	"fmt"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"log/slog"
	"net/http"

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
	url := fmt.Sprintf("http://%s/series?mediaType=%s&mediaName=%s", c.Host, mediaType, mediaName)
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
