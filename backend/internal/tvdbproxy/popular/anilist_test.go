package popular

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeAniList answers the GraphQL query with canned media per format set.
type fakeAniList struct {
	t        *testing.T
	srv      *httptest.Server
	mu       sync.Mutex
	requests []map[string]any
	media    map[string][]map[string]any // first format -> media
	status   int
}

func newFakeAniList(t *testing.T) *fakeAniList {
	f := &fakeAniList{t: t, media: map[string][]map[string]any{}, status: http.StatusOK}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		require.NoError(t, json.Unmarshal(body, &req))
		f.mu.Lock()
		f.requests = append(f.requests, req)
		status := f.status
		f.mu.Unlock()
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		formats := req["variables"].(map[string]any)["formats"].([]any)
		media := f.media[formats[0].(string)]
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"Page": map[string]any{"media": media}}})
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func anilistMedia(english, romaji string, seasonYear, startYear, trending int) map[string]any {
	return map[string]any{
		"trending": trending, "seasonYear": seasonYear, "startDate": map[string]any{"year": startYear},
		"title": map[string]any{"english": english, "romaji": romaji},
	}
}

func TestAniListTrendingPerKind(t *testing.T) {
	f := newFakeAniList(t)
	f.media["TV"] = []map[string]any{
		anilistMedia("Gachiakuta", "Gachiakuta", 2025, 2025, 300),
		anilistMedia("", "Sakamoto Days", 0, 2025, 250),
		anilistMedia("", "", 2026, 2026, 100),
	}
	f.media["MOVIE"] = []map[string]any{
		anilistMedia("Chainsaw Man – The Movie: Reze Arc", "Chainsaw Man Movie: Reze-hen", 2025, 2025, 11),
	}
	a := NewAniList(f.srv.URL, f.srv.Client(), slog.Default())

	series, err := a.Trending(context.Background(), KindSeries)
	require.NoError(t, err)
	assert.Equal(t, []AnimeTrend{
		{Title: "Gachiakuta", Romaji: "Gachiakuta", Year: "2025", Trending: 300},
		{Title: "Sakamoto Days", Romaji: "Sakamoto Days", Year: "2025", Trending: 250},
	}, series, "English title first, romaji when there is none, untitled entries dropped")

	movies, err := a.Trending(context.Background(), KindMovie)
	require.NoError(t, err)
	require.Len(t, movies, 1)
	assert.Equal(t, "Chainsaw Man – The Movie: Reze Arc", movies[0].Title)
	assert.Equal(t, "Chainsaw Man Movie: Reze-hen", movies[0].Romaji)

	f.mu.Lock()
	require.Len(t, f.requests, 2)
	tvVars := f.requests[0]["variables"].(map[string]any)
	movieVars := f.requests[1]["variables"].(map[string]any)
	f.mu.Unlock()
	assert.Equal(t, "ANIME", tvVars["type"])
	assert.Equal(t, []any{"TV", "TV_SHORT", "ONA"}, tvVars["formats"])
	assert.Equal(t, []any{"MOVIE"}, movieVars["formats"])
	assert.Contains(t, f.requests[0]["query"], "TRENDING_DESC")

	_, err = a.Trending(context.Background(), KindSeries)
	require.NoError(t, err)
	assert.Len(t, f.requests, 2, "the list is cached")
}

func TestAniListErrors(t *testing.T) {
	f := newFakeAniList(t)
	f.status = http.StatusServiceUnavailable
	a := NewAniList(f.srv.URL, f.srv.Client(), slog.Default())
	_, err := a.Trending(context.Background(), KindMovie)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "503")
}
