package main

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeTVDB serves /login (issuing freshToken) and /thing (401 unless the
// bearer token equals freshToken). It counts calls so tests can assert on
// exactly how many logins and retries happened.
type fakeTVDB struct {
	srv        *httptest.Server
	freshToken string
	loginFails bool
	logins     atomic.Int32
	gets       atomic.Int32
}

func newFakeTVDB(t *testing.T) *fakeTVDB {
	f := &fakeTVDB{freshToken: "fresh-token"}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login":
			f.logins.Add(1)
			if f.loginFails {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"status":"failure"}`))
				return
			}
			_, _ = w.Write([]byte(`{"status":"success","data":{"token":"` + f.freshToken + `"}}`))
		case "/thing":
			f.gets.Add(1)
			if r.Header.Get("Authorization") != "Bearer "+f.freshToken {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"message":"Unauthorized"}`))
				return
			}
			_, _ = w.Write([]byte(`{"data":"ok"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

// useFakeTVDB points the package globals at the fake server and restores them
// afterwards so other tests in the package are unaffected.
func useFakeTVDB(t *testing.T, f *fakeTVDB, startingToken string) {
	prevCfg := tvDbConfig
	prevLogger := logger
	t.Cleanup(func() {
		tokenMu.Lock()
		tvDbConfig = prevCfg
		tokenMu.Unlock()
		logger = prevLogger
	})
	if logger == nil {
		logger = slog.Default()
	}
	tokenMu.Lock()
	tvDbConfig = TvDbConfig{Host: f.srv.URL, ApiKey: "key", Pin: "pin", Token: startingToken}
	tokenMu.Unlock()
}

func TestTvdbAuthedGet_ReauthenticatesOn401(t *testing.T) {
	f := newFakeTVDB(t)
	useFakeTVDB(t, f, "expired-token")

	status, body, err := tvdbAuthedGet(context.Background(), f.srv.URL+"/thing")

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)
	assert.True(t, strings.Contains(string(body), `"ok"`), "retry should return the real payload, got %s", body)
	assert.Equal(t, int32(1), f.logins.Load(), "exactly one re-login")
	assert.Equal(t, int32(2), f.gets.Load(), "original request plus one retry")
	assert.Equal(t, "fresh-token", currentToken(), "new token must be stored for later calls")
}

func TestTvdbAuthedGet_NoLoginWhenTokenValid(t *testing.T) {
	f := newFakeTVDB(t)
	useFakeTVDB(t, f, "fresh-token")

	status, _, err := tvdbAuthedGet(context.Background(), f.srv.URL+"/thing")

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, int32(0), f.logins.Load())
	assert.Equal(t, int32(1), f.gets.Load())
}

func TestTvdbAuthedGet_ReturnsOriginal401WhenReloginFails(t *testing.T) {
	f := newFakeTVDB(t)
	f.loginFails = true
	useFakeTVDB(t, f, "expired-token")

	status, body, err := tvdbAuthedGet(context.Background(), f.srv.URL+"/thing")

	require.NoError(t, err, "a failed re-login is reported through the status, not an error")
	assert.Equal(t, http.StatusUnauthorized, status)
	assert.Contains(t, string(body), "Unauthorized")
	assert.Equal(t, int32(1), f.logins.Load())
	assert.Equal(t, int32(1), f.gets.Load(), "no retry without a new token")
	assert.Equal(t, "expired-token", currentToken(), "token unchanged when login fails")
}

func TestRefreshToken_SkipsLoginWhenAlreadyReplaced(t *testing.T) {
	f := newFakeTVDB(t)
	useFakeTVDB(t, f, "already-new")

	// A caller that observed an older token asks for a refresh; the stored
	// token has moved on, so no login should happen.
	tok, err := refreshToken(context.Background(), "older-token")

	require.NoError(t, err)
	assert.Equal(t, "already-new", tok)
	assert.Equal(t, int32(0), f.logins.Load())
}
