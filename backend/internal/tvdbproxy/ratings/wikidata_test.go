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

const sampleSPARQL = `{"head":{"vars":["imdb","score","by","date"]},"results":{"bindings":[
 {"imdb":{"value":"tt0090605"},"score":{"value":"8.9/10"},"by":{"value":"http://www.wikidata.org/entity/Q105584"},"date":{"value":"2024-07-30T00:00:00Z"}},
 {"imdb":{"value":"tt0090605"},"score":{"value":"91%"},"by":{"value":"http://www.wikidata.org/entity/Q105584"},"date":{"value":"2024-07-30T00:00:00Z"}},
 {"imdb":{"value":"tt0090605"},"score":{"value":"93%"},"by":{"value":"http://www.wikidata.org/entity/Q105584"},"date":{"value":"2026-06-25T00:00:00Z"}},
 {"imdb":{"value":"tt0090605"},"score":{"value":"84/100"},"by":{"value":"http://www.wikidata.org/entity/Q150248"},"date":{"value":"2026-01-15T00:00:00Z"}},
 {"imdb":{"value":"tt0090605"},"score":{"value":"8.4/10"},"by":{"value":"http://www.wikidata.org/entity/Q37312"},"date":{"value":"2026-02-18T00:00:00Z"}},
 {"imdb":{"value":"tt26581740"},"score":{"value":"3.5/5"},"by":{"value":"http://www.wikidata.org/entity/Q117741407"}},
 {"imdb":{"value":"tt26581740"},"score":{"value":"81/100"},"by":{"value":"http://www.wikidata.org/entity/Q150248"}},
 {"imdb":{"value":"tt7777777"},"score":{"value":"50 %"},"by":{"value":"http://www.wikidata.org/entity/Q105584"}}
]}}`

func wikidataServer(t *testing.T, status int, body string) (*Wikidata, *http.Request) {
	t.Helper()
	var seen http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = *r
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return NewWikidata(server.URL, server.Client(), slog.Default()), &seen
}

func TestWikidataScores(t *testing.T) {
	source, seen := wikidataServer(t, http.StatusOK, sampleSPARQL)

	scores, err := source.Scores(context.Background(), []string{"tt0090605", "tt26581740", "tt7777777", "tt0000009", "nm1"})

	require.NoError(t, err)
	query := seen.URL.Query().Get("query")
	assert.Contains(t, query, `"tt0090605" "tt26581740" "tt7777777" "tt0000009" `)
	assert.NotContains(t, query, "nm1", "only title ids are asked about")
	assert.Contains(t, query, "P444")
	assert.Equal(t, "application/sparql-results+json", seen.Header.Get("Accept"))
	assert.Contains(t, seen.Header.Get("User-Agent"), "Scout")

	assert.Equal(t, []tvdb.Rating{
		{Source: tvdb.RatingSourceRottenTomatoes, Value: 93},
		{Source: tvdb.RatingSourceMetacritic, Value: 84},
	}, scores["tt0090605"], "the latest Tomatometer wins; the average and IMDb entries are skipped")
	assert.Equal(t, []tvdb.Rating{{Source: tvdb.RatingSourceMetacritic, Value: 81}}, scores["tt26581740"])
	assert.Equal(t, []tvdb.Rating{{Source: tvdb.RatingSourceRottenTomatoes, Value: 50}}, scores["tt7777777"])
	_, ok := scores["tt0000009"]
	assert.False(t, ok, "an id with no scores is absent")
}

func TestWikidataScoresAsksNothingWithoutTitleIDs(t *testing.T) {
	source, seen := wikidataServer(t, http.StatusOK, sampleSPARQL)

	scores, err := source.Scores(context.Background(), []string{"", "nm1"})

	require.NoError(t, err)
	assert.Empty(t, scores)
	assert.Nil(t, seen.URL, "no request was made")
}

func TestWikidataScoresErrors(t *testing.T) {
	source, _ := wikidataServer(t, http.StatusTooManyRequests, "slow down")
	_, err := source.Scores(context.Background(), []string{"tt0090605"})
	assert.Error(t, err)

	source, _ = wikidataServer(t, http.StatusOK, "<html>")
	_, err = source.Scores(context.Background(), []string{"tt0090605"})
	assert.Error(t, err)
}
