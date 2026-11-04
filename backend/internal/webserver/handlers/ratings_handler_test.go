package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jbofill10/scout/backend/internal/webserver/clients"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubRatingsClient struct {
	got     []clients.RatingsRequest
	results []clients.RatingsResponse
	err     error
}

func (s *stubRatingsClient) GetRatingsBatch(
	_ context.Context, requests []clients.RatingsRequest,
) ([]clients.RatingsResponse, error) {
	s.got = requests
	return s.results, s.err
}

func ratingsRouter(client *stubRatingsClient) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/ratings/batch", NewRatingsHandler(client, slog.Default()).PostBatch)
	return r
}

func postRatings(r *gin.Engine, body any) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/ratings/batch", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func TestRatingsHandlerRelaysBatch(t *testing.T) {
	client := &stubRatingsClient{results: []clients.RatingsResponse{
		{Id: "1", MediaType: "movie", ImdbId: "tt0000001", Ratings: []tvdb.Rating{{Source: "imdb", Value: 8}}},
	}}
	w := postRatings(ratingsRouter(client), []clients.RatingsRequest{{Id: "1", MediaType: "movie"}})

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, []clients.RatingsRequest{{Id: "1", MediaType: "movie"}}, client.got)
	var body []clients.RatingsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, client.results, body)
}

func TestRatingsHandlerValidatesAndReportsErrors(t *testing.T) {
	client := &stubRatingsClient{}
	r := ratingsRouter(client)

	assert.Equal(t, http.StatusBadRequest, postRatings(r, []clients.RatingsRequest{}).Code)
	assert.Equal(t, http.StatusBadRequest, postRatings(r, []map[string]string{{"id": "1"}}).Code)
	assert.Nil(t, client.got, "nothing is relayed for a bad batch")

	client.err = errors.New("down")
	assert.Equal(t, http.StatusBadGateway, postRatings(r, []clients.RatingsRequest{{Id: "1", MediaType: "movie"}}).Code)
}
