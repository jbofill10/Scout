// Package tvdb is tvdb-proxy's client for TVDB API v4. It owns the bearer
// token: it logs in at startup, re-issues the token on a timer, and logs in
// again once whenever TVDB answers 401.
package tvdb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jbofill10/scout/backend/internal/tvdbproxy/config"
	"github.com/jbofill10/scout/backend/pkg/media"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const (
	requestTimeout = 30 * time.Second
	// refreshTimeout bounds one scheduled token refresh.
	refreshTimeout = 30 * time.Second
)

// Client makes authenticated requests to TVDB API v4.
type Client struct {
	cfg        config.TVDB
	httpClient *http.Client
	logger     *slog.Logger

	// tokenMu guards token. TVDB v4 bearer tokens expire after roughly a
	// month, so the token is re-issued proactively on a timer and reactively
	// whenever TVDB answers 401. Before this the proxy logged in once at startup
	// and served 401s forever once the token aged out.
	tokenMu sync.RWMutex
	token   string
}

// NewHTTPClient returns the traced HTTP client used for TVDB requests.
func NewHTTPClient() *http.Client {
	return &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
		Timeout:   requestTimeout,
	}
}

// NewClient returns a client for the TVDB API at cfg.Host. It holds no token
// until Login succeeds.
func NewClient(cfg config.TVDB, httpClient *http.Client, logger *slog.Logger) *Client {
	return &Client{cfg: cfg, httpClient: httpClient, logger: logger}
}

// Login authenticates with TVDB and stores the issued bearer token.
func (c *Client) Login(ctx context.Context) error {
	token, err := c.login(ctx)
	if err != nil {
		return err
	}
	c.tokenMu.Lock()
	c.token = token
	c.tokenMu.Unlock()
	return nil
}

// CurrentToken returns the bearer token in use, or "" before the first login.
func (c *Client) CurrentToken() string {
	c.tokenMu.RLock()
	defer c.tokenMu.RUnlock()
	return c.token
}

// refreshToken re-authenticates with TVDB unless another caller already
// replaced staleToken, in which case that newer token is returned as is. The
// login runs under the write lock so concurrent 401s collapse into one login.
func (c *Client) refreshToken(ctx context.Context, staleToken string) (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	if c.token != staleToken {
		return c.token, nil
	}
	token, err := c.login(ctx)
	if err != nil {
		return "", err
	}
	c.token = token
	return token, nil
}

// StartTokenRefresher re-logins every interval so the token never reaches
// TVDB's expiry. It stops when ctx is cancelled.
func (c *Client) StartTokenRefresher(ctx context.Context, interval time.Duration) {
	c.logger.Info("TVDB token refresher started", "interval", interval.String())
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				refreshCtx, cancel := context.WithTimeout(ctx, refreshTimeout)
				if _, err := c.refreshToken(refreshCtx, c.CurrentToken()); err != nil {
					c.logger.Error("Scheduled TVDB token refresh failed, will retry next interval", "error", err)
				} else {
					c.logger.Info("Scheduled TVDB token refresh succeeded")
				}
				cancel()
			}
		}
	}()
}

// authedGet performs an authenticated GET against a full TVDB URL and
// returns the status code and body. On 401 it re-authenticates once and
// retries, so an expired token costs one extra round trip instead of an outage.
// If re-authentication itself fails the original 401 response is returned.
func (c *Client) authedGet(ctx context.Context, url string) (int, []byte, error) {
	do := func(token string) (int, []byte, error) {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return 0, nil, err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return 0, nil, err
		}
		defer func() {
			if cerr := resp.Body.Close(); cerr != nil {
				c.logger.InfoContext(ctx, "Error closing response body", "error", cerr)
			}
		}()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return resp.StatusCode, nil, err
		}
		return resp.StatusCode, body, nil
	}

	token := c.CurrentToken()
	status, body, err := do(token)
	if err != nil || status != http.StatusUnauthorized {
		return status, body, err
	}

	c.logger.WarnContext(ctx, "TVDB rejected bearer token, re-authenticating", "url", url)
	newToken, err := c.refreshToken(ctx, token)
	if err != nil {
		c.logger.ErrorContext(ctx, "TVDB re-authentication failed", "error", err)
		return status, body, nil
	}
	return do(newToken)
}

type loginResponse struct {
	Data struct {
		Token string `json:"token"`
	} `json:"data"`
	Status string `json:"status"`
}

// login exchanges the API key (and PIN, when set) for a bearer token.
func (c *Client) login(ctx context.Context) (string, error) {
	// Prepare login request body with apikey and pin
	loginReq := map[string]string{
		"apikey": c.cfg.APIKey,
	}
	// Only include pin if it's set (licensed keys don't need PIN)
	if c.cfg.Pin != "" {
		loginReq["pin"] = c.cfg.Pin
	}

	reqBody, err := json.Marshal(loginReq)
	if err != nil {
		c.logger.ErrorContext(ctx, "Error marshalling login request", "err", err)
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.cfg.Host+"/login", strings.NewReader(string(reqBody)))
	if err != nil {
		c.logger.InfoContext(ctx, "Error creating login request")
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.ErrorContext(ctx, "Error making request to TVDB", "err", err)
		return "", err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.logger.InfoContext(ctx, "Error closing response body", "err", err)
		}
	}()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.logger.ErrorContext(ctx, "Error reading response body", "err", err)
		return "", err
	}

	// Check for non-2xx status codes
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.logger.ErrorContext(ctx, "TVDB login failed", "status_code", resp.StatusCode, "response", string(body))
		return "", fmt.Errorf("TVDB login failed with status %d: %s", resp.StatusCode, string(body))
	}

	var lr loginResponse
	if err := json.Unmarshal(body, &lr); err != nil {
		c.logger.ErrorContext(ctx, "Unable to parse login response body", "err", err, "body", string(body))
		return "", err
	}

	c.logger.InfoContext(ctx, "Successfully authenticated with TVDB API")
	return lr.Data.Token, nil
}

// Get performs an authenticated GET of uri (a path plus query, relative to
// the TVDB host) and returns the body, or an error for a non-2xx status.
func (c *Client) Get(ctx context.Context, uri string) ([]byte, error) {
	c.logger.InfoContext(ctx, "Making TVDB API request", "uri", uri)
	statusCode, body, err := c.authedGet(ctx, c.cfg.Host+uri)
	if err != nil {
		c.logger.InfoContext(ctx, "Error making request to TVDB", "error", err)
		return nil, err
	}

	// Log response details for debugging
	preview := string(body)
	if len(body) > 500 {
		preview = string(body[:500]) + "..."
	}
	c.logger.InfoContext(ctx, "TVDB API response",
		"status_code", statusCode,
		"body_length", len(body),
		"body_preview", preview)

	// Check HTTP status code
	if statusCode < 200 || statusCode >= 300 {
		c.logger.ErrorContext(ctx, "TVDB API returned error",
			"status_code", statusCode,
			"response_body", string(body))
		return nil, fmt.Errorf("TVDB API error: status %d", statusCode)
	}

	return body, nil
}

// SeriesEpisodes fetches a series' official-order English episodes and score.
func (c *Client) SeriesEpisodes(ctx context.Context, seriesId string) (media.TVDBSeriesResponse, error) {
	queryUri := fmt.Sprintf("/series/%s/episodes/official/eng", seriesId)
	var seriesResponse = media.TVDBSeriesResponse{}
	res, err := c.Get(ctx, queryUri)

	if err != nil {
		c.logger.InfoContext(ctx, "Unable to get show information", "value", err)
		return seriesResponse, err
	}

	if err := json.Unmarshal(res, &seriesResponse); err != nil {
		c.logger.InfoContext(ctx, "Unable to unmarshal json response", "value", err)
		return seriesResponse, err
	}

	c.logger.InfoContext(
		ctx,
		"Series metadata querried",
		"seriesId", seriesId)
	return seriesResponse, nil
}
