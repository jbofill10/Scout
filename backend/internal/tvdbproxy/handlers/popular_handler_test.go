package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jbofill10/scout/backend/internal/tvdbproxy/popular"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubRanker struct {
	results []tvdb.Media
	err     error
	kind    popular.Kind
	genre   string
	limit   int
	calls   int
}

func (s *stubRanker) Rank(_ context.Context, kind popular.Kind, genre string, limit int) ([]tvdb.Media, error) {
	s.calls++
	s.kind, s.genre, s.limit = kind, genre, limit
	return s.results, s.err
}

type stubGenres map[string]int

func (g stubGenres) LookupID(name string) (int, bool) {
	id, ok := g[name]
	return id, ok
}

func newPopularRouter(ranker *stubRanker) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewPopularHandler(ranker, stubGenres{"Drama": 12, "Anime": 27}, slog.Default())
	r := gin.New()
	r.GET("/series/popular", h.GetPopularSeries)
	r.GET("/movies/popular", h.GetPopularMovies)
	return r
}

func get(r *gin.Engine, target string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
	return w
}

func TestPopularHandlerSeries(t *testing.T) {
	ranker := &stubRanker{results: []tvdb.Media{{Id: "1", Name: "a"}}}
	w := get(newPopularRouter(ranker), "/series/popular?genre=Drama&limit=7")

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, popular.KindSeries, ranker.kind)
	assert.Equal(t, "Drama", ranker.genre)
	assert.Equal(t, 7, ranker.limit)
	var body []tvdb.Media
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, ranker.results, body)
}

func TestPopularHandlerMoviesDefaultsAndCaps(t *testing.T) {
	ranker := &stubRanker{results: nil}
	r := newPopularRouter(ranker)

	w := get(r, "/movies/popular")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, popular.KindMovie, ranker.kind)
	assert.Equal(t, "", ranker.genre)
	assert.Equal(t, popular.DefaultLimit, ranker.limit)
	assert.Equal(t, "[]", w.Body.String(), "no results is an empty array, not null")

	get(r, "/movies/popular?limit=900")
	assert.Equal(t, popular.MaxLimit, ranker.limit)
	get(r, "/movies/popular?limit=abc")
	assert.Equal(t, popular.DefaultLimit, ranker.limit)
}

func TestPopularHandlerUnknownGenre(t *testing.T) {
	ranker := &stubRanker{}
	w := get(newPopularRouter(ranker), "/series/popular?genre=Nope")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, 0, ranker.calls)

	ranker = &stubRanker{err: popular.ErrUnknownGenre}
	w = get(newPopularRouter(ranker), "/series/popular?genre=Anime")
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPopularHandlerRankerError(t *testing.T) {
	ranker := &stubRanker{err: errors.New("boom")}
	w := get(newPopularRouter(ranker), "/movies/popular")
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Failed to fetch popular movie")
}
