package clients

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelayAgentClient(t *testing.T) {
	var gotAuth string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		switch r.URL.Path {
		case "/status":
			_ = json.NewEncoder(w).Encode(map[string]any{"connected": true, "relay": "ca-mtr-wg-004", "ip": "1.2.3.4"})
		case "/rotate":
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			if gotBody["reason"] == "cooldown" {
				w.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "rotated 5s ago"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"relay": map[string]string{"hostname": "ca-tor-wg-101", "ip": "5.6.7.8"}, "status": map[string]any{"connected": true}})
		}
	}))
	defer srv.Close()

	c := NewRelayAgentClient(srv.URL+"/", "tok")
	assert.True(t, c.Configured())
	assert.False(t, NewRelayAgentClient("", "x").Configured())

	st, err := c.Status(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "Bearer tok", gotAuth)
	assert.Equal(t, "ca-mtr-wg-004", st.Relay)

	rot, err := c.Rotate(context.Background(), []string{"a"}, "blocked")
	require.NoError(t, err)
	assert.Equal(t, "ca-tor-wg-101", rot.Relay.Hostname)
	assert.Equal(t, []any{"a"}, gotBody["exclude"])

	_, err = c.Rotate(context.Background(), nil, "cooldown")
	assert.ErrorIs(t, err, ErrRotateRefused)
	assert.Contains(t, err.Error(), "rotated 5s ago")

	_, err = NewRelayAgentClient("", "").Status(context.Background())
	assert.Error(t, err)
}
