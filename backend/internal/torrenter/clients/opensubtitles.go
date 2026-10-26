package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
)

const (
	// DefaultOpenSubtitlesURL is the public REST API. Logged-in VIP accounts
	// are told to use a different base URL in the login response; it is
	// honoured when present.
	DefaultOpenSubtitlesURL = "https://api.opensubtitles.com/api/v1"
	openSubtitlesUserAgent  = "Scout v1.0"
	// Downloaded subtitle bodies are small; this bounds a misbehaving link.
	maxSubtitleBytes = 4 << 20
)

// OpenSubtitlesClient talks to the OpenSubtitles REST API
// (https://opensubtitles.stoplight.io/docs/opensubtitles-api). Searching needs
// only the API key; downloading needs a user token, which is fetched lazily
// and refreshed when the API says it has expired.
type OpenSubtitlesClient struct {
	baseURL  string
	apiKey   string
	username string
	password string
	http     *http.Client
	logger   *slog.Logger

	mu          sync.Mutex
	token       string
	tokenExpiry time.Time
}

func NewOpenSubtitlesClient(cfg *models.SubtitlesCfg, logger *slog.Logger) *OpenSubtitlesClient {
	return &OpenSubtitlesClient{
		baseURL:  DefaultOpenSubtitlesURL,
		apiKey:   cfg.APIKey,
		username: cfg.Username,
		password: cfg.Password,
		http:     &http.Client{Timeout: 30 * time.Second},
		logger:   logger,
	}
}

// WithBaseURL points the client at another server; tests use it.
func (c *OpenSubtitlesClient) WithBaseURL(base string) *OpenSubtitlesClient {
	c.baseURL = strings.TrimRight(base, "/")
	return c
}

type osSearchResponse struct {
	TotalCount int `json:"total_count"`
	Data       []struct {
		Attributes struct {
			Language          string  `json:"language"`
			DownloadCount     int     `json:"download_count"`
			HearingImpaired   bool    `json:"hearing_impaired"`
			FPS               float64 `json:"fps"`
			Ratings           float64 `json:"ratings"`
			FromTrusted       bool    `json:"from_trusted"`
			ForeignPartsOnly  bool    `json:"foreign_parts_only"`
			AITranslated      bool    `json:"ai_translated"`
			MachineTranslated bool    `json:"machine_translated"`
			Release           string  `json:"release"`
			MovieHashMatch    bool    `json:"moviehash_match"`
			FeatureDetails    struct {
				FeatureType   string `json:"feature_type"`
				Year          int    `json:"year"`
				SeasonNumber  int    `json:"season_number"`
				EpisodeNumber int    `json:"episode_number"`
			} `json:"feature_details"`
			Files []struct {
				FileID   int    `json:"file_id"`
				CDNumber int    `json:"cd_number"`
				FileName string `json:"file_name"`
			} `json:"files"`
		} `json:"attributes"`
	} `json:"data"`
}

type osLoginResponse struct {
	Token   string `json:"token"`
	BaseURL string `json:"base_url"`
	Status  int    `json:"status"`
}

type osDownloadResponse struct {
	Link      string `json:"link"`
	FileName  string `json:"file_name"`
	Requests  int    `json:"requests"`
	Remaining int    `json:"remaining"`
	Message   string `json:"message"`
	ResetTime string `json:"reset_time"`
}

// Search runs one /subtitles query and flattens the response into candidates.
// Multi-CD subtitles are skipped: nothing downstream can use them.
func (c *OpenSubtitlesClient) Search(ctx context.Context,
	p models.SubtitleSearchParams) ([]models.SubtitleCandidate, error) {
	q := url.Values{}
	// The API wants languages sorted and lowercase; Encode() already sorts keys.
	langs := append([]string(nil), p.Languages...)
	for i := range langs {
		langs[i] = strings.ToLower(langs[i])
	}
	sort.Strings(langs)
	q.Set("languages", strings.Join(langs, ","))
	q.Set("ai_translated", "exclude")
	q.Set("machine_translated", "exclude")
	if p.IsMovie {
		q.Set("type", "movie")
	} else {
		q.Set("type", "episode")
	}
	if id := imdbDigits(p.ImdbID); id != "" {
		q.Set("imdb_id", id)
	}
	if id := imdbDigits(p.ParentImdbID); id != "" {
		q.Set("parent_imdb_id", id)
	}
	if !p.IsMovie && p.Season > 0 {
		q.Set("season_number", strconv.Itoa(p.Season))
	}
	if !p.IsMovie && p.Episode > 0 {
		q.Set("episode_number", strconv.Itoa(p.Episode))
	}
	if p.Query != "" {
		q.Set("query", strings.ToLower(p.Query))
	}
	if p.Year != "" && p.IsMovie {
		q.Set("year", p.Year)
	}
	if p.MovieHash != "" {
		q.Set("moviehash", strings.ToLower(p.MovieHash))
	}

	var resp osSearchResponse
	if err := c.do(ctx, http.MethodGet, "/subtitles?"+q.Encode(), nil, false, &resp); err != nil {
		return nil, err
	}

	out := make([]models.SubtitleCandidate, 0, len(resp.Data))
	for _, d := range resp.Data {
		a := d.Attributes
		if len(a.Files) != 1 {
			continue
		}
		out = append(out, models.SubtitleCandidate{
			FileID:            a.Files[0].FileID,
			Language:          strings.ToLower(a.Language),
			Release:           a.Release,
			HearingImpaired:   a.HearingImpaired,
			ForeignPartsOnly:  a.ForeignPartsOnly,
			MachineTranslated: a.MachineTranslated,
			AITranslated:      a.AITranslated,
			FromTrusted:       a.FromTrusted,
			HashMatch:         a.MovieHashMatch,
			DownloadCount:     a.DownloadCount,
			Ratings:           a.Ratings,
			FPS:               a.FPS,
			FeatureType:       strings.ToLower(a.FeatureDetails.FeatureType),
			Season:            a.FeatureDetails.SeasonNumber,
			Episode:           a.FeatureDetails.EpisodeNumber,
			Year:              a.FeatureDetails.Year,
		})
	}
	return out, nil
}

// Download asks for a download link for fileID, follows it, and returns the
// subtitle body as SRT along with the remaining daily quota (-1 if unknown).
func (c *OpenSubtitlesClient) Download(ctx context.Context, fileID int) ([]byte, int, error) {
	body, _ := json.Marshal(map[string]any{"file_id": fileID, "sub_format": "srt"})

	var dl osDownloadResponse
	if err := c.do(ctx, http.MethodPost, "/download", body, true, &dl); err != nil {
		return nil, -1, err
	}
	if dl.Link == "" {
		return nil, dl.Remaining, fmt.Errorf("opensubtitles returned no download link: %s", dl.Message)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dl.Link, nil)
	if err != nil {
		return nil, dl.Remaining, err
	}
	req.Header.Set("User-Agent", openSubtitlesUserAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, dl.Remaining, fmt.Errorf("fetch subtitle file: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, dl.Remaining, fmt.Errorf("fetch subtitle file: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxSubtitleBytes))
	if err != nil {
		return nil, dl.Remaining, err
	}
	return data, dl.Remaining, nil
}

// do performs one API call, decoding a JSON body into out. With auth set it
// attaches the user token, logging in first if needed, and retries once on a
// 401 in case the token expired.
func (c *OpenSubtitlesClient) do(ctx context.Context, method, path string, body []byte, auth bool, out any) error {
	for attempt := 0; attempt < 2; attempt++ {
		// A user token is only needed for downloads, and only when an account
		// is configured; some consumer keys are allowed to download anonymously.
		var token string
		if auth && c.hasCredentials() {
			var err error
			if token, err = c.currentToken(ctx, attempt > 0); err != nil {
				return err
			}
		}

		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Api-Key", c.apiKey)
		req.Header.Set("User-Agent", openSubtitlesUserAgent)
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		resp, err := c.http.Do(req)
		if err != nil {
			return fmt.Errorf("opensubtitles %s %s: %w", method, path, err)
		}
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, maxSubtitleBytes))
		resp.Body.Close()
		if readErr != nil {
			return readErr
		}

		if resp.StatusCode == http.StatusUnauthorized && auth && c.hasCredentials() && attempt == 0 {
			c.logger.InfoContext(ctx, "OpenSubtitles token rejected, logging in again")
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return &APIError{Status: resp.StatusCode, Body: apiMessage(data)}
		}
		if out == nil {
			return nil
		}
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("opensubtitles %s %s: decode: %w", method, path, err)
		}
		return nil
	}
	return fmt.Errorf("opensubtitles %s %s: unauthorized after re-login", method, path)
}

// APIError is a non-2xx answer from OpenSubtitles. Status 406 is the daily
// download quota; 401/403 are credential problems.
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("opensubtitles HTTP %d: %s", e.Status, e.Body)
}

// QuotaExceeded reports whether the error is the daily download limit.
func (e *APIError) QuotaExceeded() bool { return e.Status == http.StatusNotAcceptable }

func apiMessage(data []byte) string {
	var m struct {
		Message string `json:"message"`
		Errors  []string
	}
	if json.Unmarshal(data, &m) == nil && m.Message != "" {
		return m.Message
	}
	s := strings.TrimSpace(string(data))
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

func (c *OpenSubtitlesClient) hasCredentials() bool {
	return c.username != "" && c.password != ""
}

// currentToken returns a usable user token, logging in when there is none,
// when it is about to expire, or when the caller says the last one was stale.
func (c *OpenSubtitlesClient) currentToken(ctx context.Context, stale bool) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !stale && c.token != "" && time.Now().Before(c.tokenExpiry) {
		return c.token, nil
	}

	body, _ := json.Marshal(map[string]string{"username": c.username, "password": c.password})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/login", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Api-Key", c.apiKey)
	req.Header.Set("User-Agent", openSubtitlesUserAgent)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("opensubtitles login: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxSubtitleBytes))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", &APIError{Status: resp.StatusCode, Body: "login: " + apiMessage(data)}
	}
	var login osLoginResponse
	if err := json.Unmarshal(data, &login); err != nil || login.Token == "" {
		return "", fmt.Errorf("opensubtitles login: no token in response")
	}
	c.token = login.Token
	// Tokens last 24h; renew a little early so a call never straddles expiry.
	c.tokenExpiry = time.Now().Add(23 * time.Hour)
	// The login response names the host to use from now on (VIP accounts get
	// a dedicated one). Only honour a host, never a scheme-less or odd value.
	if login.BaseURL != "" && !strings.Contains(login.BaseURL, "/") && c.baseURL == DefaultOpenSubtitlesURL {
		c.baseURL = "https://" + login.BaseURL + "/api/v1"
	}
	c.logger.InfoContext(ctx, "Logged in to OpenSubtitles", "user", c.username)
	return c.token, nil
}

// imdbDigits turns "tt0121955" into "0121955" (what the API wants) and leaves
// a bare number alone. Anything else yields "".
func imdbDigits(id string) string {
	id = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(id)), "tt")
	if id == "" {
		return ""
	}
	for _, r := range id {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return id
}
