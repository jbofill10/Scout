package ratings

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// omdbServer answers every lookup with status and body, recording the query.
func omdbServer(t *testing.T, status int, body string) (*OMDb, *http.Request) {
	t.Helper()
	var seen http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = *r
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return NewOMDb(server.URL, "key123", server.Client(), slog.Default()), &seen
}

func TestOMDbLookupMovie(t *testing.T) {
	source, seen := omdbServer(t, http.StatusOK, `{
		"Title":"Sinners","Type":"movie",
		"Ratings":[
			{"Source":"Rotten Tomatoes","Value":"97%"},
			{"Source":"Internet Movie Database","Value":"7.7/10"},
			{"Source":"Metacritic","Value":"84/100"}
		],
		"Metascore":"84","imdbRating":"7.7","imdbVotes":"151,234","imdbID":"tt31193180","Response":"True"}`)

	ratings, err := source.Lookup(context.Background(), "tt31193180")

	require.NoError(t, err)
	assert.Equal(t, []tvdb.Rating{
		{Source: tvdb.RatingSourceIMDb, Value: 7.7, Votes: 151234},
		{Source: tvdb.RatingSourceRottenTomatoes, Value: 97},
		{Source: tvdb.RatingSourceMetacritic, Value: 84},
	}, ratings, "sources come back in a fixed order whatever OMDb's")
	assert.Equal(t, "tt31193180", seen.URL.Query().Get("i"))
	assert.Equal(t, "key123", seen.URL.Query().Get("apikey"))
}

func TestOMDbLookupSeriesHasOnlyIMDb(t *testing.T) {
	source, _ := omdbServer(t, http.StatusOK, `{
		"Title":"Severance","Type":"series",
		"Ratings":[{"Source":"Internet Movie Database","Value":"8.7/10"}],
		"Metascore":"N/A","imdbRating":"8.7","imdbVotes":"N/A","Response":"True"}`)

	ratings, err := source.Lookup(context.Background(), "tt11280740")

	require.NoError(t, err)
	assert.Equal(t, []tvdb.Rating{{Source: tvdb.RatingSourceIMDb, Value: 8.7}}, ratings)
}

func TestOMDbLookupFallsBackToTopLevelFields(t *testing.T) {
	source, _ := omdbServer(t, http.StatusOK,
		`{"Title":"Old","Ratings":[],"Metascore":"61","imdbRating":"6.4","imdbVotes":"900","Response":"True"}`)

	ratings, err := source.Lookup(context.Background(), "tt1")

	require.NoError(t, err)
	assert.Equal(t, []tvdb.Rating{
		{Source: tvdb.RatingSourceIMDb, Value: 6.4, Votes: 900},
		{Source: tvdb.RatingSourceMetacritic, Value: 61},
	}, ratings)
}

func TestOMDbLookupUnratedTitle(t *testing.T) {
	source, _ := omdbServer(t, http.StatusOK,
		`{"Title":"New","Ratings":[],"Metascore":"N/A","imdbRating":"N/A","imdbVotes":"N/A","Response":"True"}`)

	ratings, err := source.Lookup(context.Background(), "tt2")

	require.NoError(t, err)
	assert.Nil(t, ratings)
}

func TestOMDbLookupUnknownID(t *testing.T) {
	source, _ := omdbServer(t, http.StatusOK, `{"Response":"False","Error":"Incorrect IMDb ID."}`)

	ratings, err := source.Lookup(context.Background(), "tt3")

	require.NoError(t, err)
	assert.Nil(t, ratings)
}

func TestOMDbLookupRateLimited(t *testing.T) {
	source, _ := omdbServer(t, http.StatusUnauthorized, `{"Response":"False","Error":"Request limit reached!"}`)

	_, err := source.Lookup(context.Background(), "tt4")

	assert.ErrorIs(t, err, ErrRateLimited)
}

func TestOMDbLookupBadKey(t *testing.T) {
	source, _ := omdbServer(t, http.StatusUnauthorized, `{"Response":"False","Error":"Invalid API key!"}`)

	_, err := source.Lookup(context.Background(), "tt5")

	assert.ErrorIs(t, err, ErrUnauthorized)
	assert.NotErrorIs(t, err, ErrRateLimited)
}

func TestOMDbLookupServerError(t *testing.T) {
	source, _ := omdbServer(t, http.StatusBadGateway, `<html>bad gateway</html>`)

	_, err := source.Lookup(context.Background(), "tt6")

	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrRateLimited)
}

func TestOMDbLookupMalformedBody(t *testing.T) {
	source, _ := omdbServer(t, http.StatusOK, `{"Response":"True",`)

	_, err := source.Lookup(context.Background(), "tt7")

	require.Error(t, err)
}

func TestParseVotes(t *testing.T) {
	assert.Equal(t, 1234567, parseVotes("1,234,567"))
	assert.Equal(t, 0, parseVotes("N/A"))
	assert.Equal(t, 0, parseVotes(""))
	assert.Equal(t, 0, parseVotes("lots"))
}

func TestParseRatingEntry(t *testing.T) {
	cases := []struct {
		source, raw string
		want        float64
		ok          bool
	}{
		{"Internet Movie Database", "8.5/10", 8.5, true},
		{"Rotten Tomatoes", "94%", 94, true},
		{"Metacritic", "80/100", 80, true},
		{"Metacritic", "N/A", 0, false},
		{"Letterboxd", "4/5", 0, false},
	}
	for _, c := range cases {
		_, got, ok := parseRatingEntry(c.source, c.raw)
		assert.Equal(t, c.ok, ok, c.source+" "+c.raw)
		assert.Equal(t, c.want, got, c.source+" "+c.raw)
	}
}
