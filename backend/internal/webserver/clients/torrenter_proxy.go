package clients

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
)

// Proxy forwards one request to the torrenter verbatim and returns its status
// and body, so a webserver handler can relay an endpoint the torrenter owns
// (runtime settings, uploader preferences) without re-describing its errors.
// A transport failure is the only error; any HTTP status is passed back.
func (c *TorrenterClient) Proxy(ctx context.Context, method, path string, body []byte) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, fmt.Sprintf("http://%s%s", c.Host, path), reader)
	if err != nil {
		return 0, nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("torrenter unreachable: %w", err)
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("read torrenter response: %w", err)
	}
	return resp.StatusCode, out, nil
}
