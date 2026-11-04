package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/search"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubRater rates every item whose IMDb id it knows.
type stubRater struct {
	mu      sync.Mutex
	known   map[string][]tvdb.Rating
	lookups []string
}

func (r *stubRater) Attach(_ context.Context, media []tvdb.Media) {
	for i := range media {
		media[i].Ratings = r.known[media[i].ImdbID]
	}
}

func (r *stubRater) Lookup(_ context.Context, imdbID string) []tvdb.Rating {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lookups = append(r.lookups, imdbID)
	return r.known[imdbID]
}

type stubSearcher struct{ results []tvdb.Media }

func (s stubSearcher) Search(context.Context, search.Query, string) ([]tvdb.Media, error) {
	return s.results, nil
}

type stubExtended struct {
	records map[string]*tvdb.TVDBSeriesExtendedResponse
}

func (s stubExtended) Extended(_ context.Context, id, _ string) (*tvdb.TVDBSeriesExtendedResponse, error) {
	return s.records[id], nil
}

var tomato = []tvdb.Rating{{Source: tvdb.RatingSourceRottenTomatoes, Value: 97}}

func TestPopularHandlerAttachesRatings(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ranker := &stubRanker{results: []tvdb.Media{{Id: "1", ImdbID: "tt0000001"}, {Id: "2"}}}
	h := NewPopularHandler(ranker, stubGenres{}, &stubRater{known: map[string][]tvdb.Rating{"tt0000001": tomato}},
		slog.Default())
	r := gin.New()
	r.GET("/movies/popular", h.GetPopularMovies)

	w := get(r, "/movies/popular")

	require.Equal(t, http.StatusOK, w.Code)
	var body []tvdb.Media
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, tomato, body[0].Ratings)
	assert.Nil(t, body[1].Ratings)
}

func TestSearchHandlerAttachesRatings(t *testing.T) {
	gin.SetMode(gin.TestMode)
	searcher := stubSearcher{results: []tvdb.Media{{Id: "1", ImdbID: "tt0000001"}}}
	h := NewSearchHandler(searcher, &stubRater{known: map[string][]tvdb.Rating{"tt0000001": tomato}}, slog.Default())
	r := gin.New()
	r.GET("/series", h.GetSeries)

	w := get(r, "/series?mediaName=sinners&mediaType=movie")

	require.Equal(t, http.StatusOK, w.Code)
	var body []tvdb.Media
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, tomato, body[0].Ratings)
}

func TestSearchHandlerWithoutRater(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewSearchHandler(stubSearcher{results: []tvdb.Media{{Id: "1", ImdbID: "tt0000001"}}}, nil, slog.Default())
	r := gin.New()
	r.GET("/series", h.GetSeries)

	w := get(r, "/series?mediaName=sinners&mediaType=movie")

	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), `"ratings"`)
}

func TestExtendedHandlerRatesRecords(t *testing.T) {
	gin.SetMode(gin.TestMode)
	records := map[string]*tvdb.TVDBSeriesExtendedResponse{
		"1": {Data: tvdb.TVDBSeriesExtendedData{Id: 1,
			RemoteIds: []tvdb.RemoteID{{Id: "tt0000001", Type: tvdb.RemoteIDTypeIMDb, SourceName: "IMDB"}}}},
		"2": {Data: tvdb.TVDBSeriesExtendedData{Id: 2}},
	}
	rater := &stubRater{known: map[string][]tvdb.Rating{"tt0000001": tomato}}
	h := NewExtendedHandler(stubExtended{records: records}, rater, slog.Default())
	r := gin.New()
	r.GET("/series/:id/extended", h.GetExtended)
	r.POST("/series/batch/extended", h.GetBatchExtended)

	w := get(r, "/series/1/extended?mediaType=movie")
	require.Equal(t, http.StatusOK, w.Code)
	var single tvdb.TVDBSeriesExtendedResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &single))
	assert.Equal(t, tomato, single.Data.Ratings)

	body, _ := json.Marshal([]BatchExtendedRequest{{Id: "1", MediaType: "movie"}, {Id: "2", MediaType: "movie"}})
	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/series/batch/extended", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var batch []BatchExtendedResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &batch))
	byID := map[string][]tvdb.Rating{}
	for _, item := range batch {
		byID[item.Request.Id] = item.Data.Data.Ratings
	}
	assert.Equal(t, tomato, byID["1"])
	assert.Nil(t, byID["2"])
	assert.Equal(t, []string{"tt0000001", "tt0000001"}, rater.lookups, "records without an IMDb id are not looked up")
}

// resolvingRater is a stubRater that also knows IMDb ids by "mediaType:id".
type resolvingRater struct {
	stubRater
	byTVDB map[string]string
}

func (r *resolvingRater) Attach(ctx context.Context, media []tvdb.Media) {
	for i := range media {
		if media[i].ImdbID == "" {
			media[i].ImdbID = r.byTVDB[media[i].Category+":"+media[i].Id]
		}
	}
	r.stubRater.Attach(ctx, media)
}

func postJSON(r *gin.Engine, target string, body any) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, target, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func TestRatingsHandlerBatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rater := &resolvingRater{
		stubRater: stubRater{known: map[string][]tvdb.Rating{"tt0000001": tomato}},
		byTVDB:    map[string]string{"movie:1": "tt0000001", "series:2": "tt0000002"},
	}
	h := NewRatingsHandler(rater, slog.Default())
	r := gin.New()
	r.POST("/ratings/batch", h.GetBatch)

	w := postJSON(r, "/ratings/batch", []RatingsRequest{
		{Id: "1", MediaType: "movie"}, {Id: "2", MediaType: "series"}, {Id: "3", MediaType: "movie"},
	})

	require.Equal(t, http.StatusOK, w.Code)
	var body []RatingsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, []RatingsResponse{
		{Id: "1", MediaType: "movie", ImdbId: "tt0000001", Ratings: tomato},
		{Id: "2", MediaType: "series", ImdbId: "tt0000002"},
		{Id: "3", MediaType: "movie"},
	}, body)
}

func TestRatingsHandlerRejectsBadBatches(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewRatingsHandler(nil, slog.Default())
	r := gin.New()
	r.POST("/ratings/batch", h.GetBatch)

	assert.Equal(t, http.StatusBadRequest, postJSON(r, "/ratings/batch", []RatingsRequest{}).Code)
	assert.Equal(t, http.StatusBadRequest, postJSON(r, "/ratings/batch", map[string]string{"id": "1"}).Code)
	assert.Equal(t, http.StatusBadRequest, postJSON(r, "/ratings/batch", []map[string]string{{"id": "1"}}).Code,
		"mediaType is required")
	big := make([]RatingsRequest, ratingsBatchMax+1)
	for i := range big {
		big[i] = RatingsRequest{Id: "1", MediaType: "movie"}
	}
	assert.Equal(t, http.StatusBadRequest, postJSON(r, "/ratings/batch", big).Code)

	w := postJSON(r, "/ratings/batch", []RatingsRequest{{Id: "1", MediaType: "movie"}})
	require.Equal(t, http.StatusOK, w.Code, "without a rater titles come back unrated")
	assert.Equal(t, `[{"id":"1","mediaType":"movie"}]`, w.Body.String())
}
