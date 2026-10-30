package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrRotateRefused is returned when the agent declines to rotate (cooldown or
// no relay left in its pool).
var ErrRotateRefused = errors.New("relay agent refused to rotate")

// RelayStatus is the host's tunnel as the relay agent reports it.
type RelayStatus struct {
	Connected      bool     `json:"connected"`
	State          string   `json:"state"`
	IP             string   `json:"ip"`
	Relay          string   `json:"relay"`
	City           string   `json:"city"`
	Country        string   `json:"country"`
	LastRotationAt *float64 `json:"last_rotation_at"`
	History        []string `json:"history"`
	Pool           []string `json:"pool"`
}

// RelayPick is the relay the agent switched to.
type RelayPick struct {
	Hostname string `json:"hostname"`
	IP       string `json:"ip"`
	Provider string `json:"provider"`
	Country  string `json:"country"`
	City     string `json:"city"`
}

// RelayRotation is the agent's answer to a rotation.
type RelayRotation struct {
	Previous RelayStatus `json:"previous"`
	Relay    RelayPick   `json:"relay"`
	Status   RelayStatus `json:"status"`
	Reason   string      `json:"reason"`
}

// RelayAgentClient talks to tools/relay-agent on the host. A zero URL means
// no agent is configured and every call reports that.
type RelayAgentClient struct {
	baseURL string
	token   string
	http    *http.Client
}

func NewRelayAgentClient(baseURL, token string) *RelayAgentClient {
	return &RelayAgentClient{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		token:   strings.TrimSpace(token),
		http:    &http.Client{Timeout: 45 * time.Second},
	}
}

// Configured reports whether an agent URL is set.
func (c *RelayAgentClient) Configured() bool {
	return c != nil && c.baseURL != ""
}

func (c *RelayAgentClient) Status(ctx context.Context) (RelayStatus, error) {
	var st RelayStatus
	err := c.call(ctx, http.MethodGet, "/status", nil, &st)
	return st, err
}

// Rotate asks for a different relay, never one of exclude.
func (c *RelayAgentClient) Rotate(ctx context.Context, exclude []string, reason string) (RelayRotation, error) {
	var rot RelayRotation
	body := map[string]any{"exclude": exclude, "reason": reason}
	err := c.call(ctx, http.MethodPost, "/rotate", body, &rot)
	return rot, err
}

func (c *RelayAgentClient) call(ctx context.Context, method, path string, body any, out any) error {
	if !c.Configured() {
		return errors.New("relay agent is not configured")
	}
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("relay agent unreachable: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusConflict {
		return fmt.Errorf("%w: %s", ErrRotateRefused, agentError(raw))
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("relay agent answered %d: %s", resp.StatusCode, agentError(raw))
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("decode relay agent response: %w", err)
		}
	}
	return nil
}

func agentError(raw []byte) string {
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(raw, &e) == nil && e.Error != "" {
		return e.Error
	}
	return strings.TrimSpace(string(raw))
}
