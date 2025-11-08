package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
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
