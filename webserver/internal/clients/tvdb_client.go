package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	tvdb "shared/media"

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

func (c *TVDBProxyClient) GetExtendedInfo(ctx context.Context, mediaId string) (tvdb.TVDBSeriesExtendedResponse, error) {
	url := fmt.Sprintf("http://%s/series/%s/extended", c.Host, mediaId)
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
