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

const sampleSPARQL = `{"head":{"vars":["imdb","score","by","date","article","film"]},"results":{"bindings":[
 {"imdb":{"value":"tt0090605"},"score":{"value":"8.9/10"},"by":{"value":"http://www.wikidata.org/entity/Q105584"},"date":{"value":"2024-07-30T00:00:00Z"},"article":{"value":"https://en.wikipedia.org/wiki/Aliens_(film)"},"film":{"value":"true"}},
 {"imdb":{"value":"tt0000009"},"article":{"value":"https://en.wikipedia.org/wiki/Brand_New_(2026_film)"},"film":{"value":"true"}},
 {"imdb":{"value":"tt0000010"},"film":{"value":"false"}},
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
	return NewWikidata(server.URL, server.URL, server.Client(), slog.Default()), &seen
}

// findingServer answers wbsearchentities with hits and SPARQL with bindings.
func findingServer(t *testing.T, hits string, sparql string) *Wikidata {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("action") == "wbsearchentities" {
			_, _ = w.Write([]byte(hits))
			return
		}
		_, _ = w.Write([]byte(sparql))
	}))
	t.Cleanup(server.Close)
	return NewWikidata(server.URL, server.URL, server.Client(), slog.Default())
}

const findHits = `{"search":[{"id":"Q1","label":"SK8 the Infinity"},{"id":"Q2","label":"Mononoke the Movie"},{"id":"Q3","label":"Mononoke (1997)"}]}`
const findSPARQL = `{"results":{"bindings":[
 {"item":{"value":"http://www.wikidata.org/entity/Q2"},"imdb":{"value":"tt35301440"},"date":{"value":"2025-03-14T00:00:00Z"}},
 {"item":{"value":"http://www.wikidata.org/entity/Q3"},"imdb":{"value":"tt0119698"},"date":{"value":"1997-07-12T00:00:00Z"}}
]}}`

func TestWikidataFindIMDbID(t *testing.T) {
	source := findingServer(t, findHits, findSPARQL)

	id, ok := source.FindIMDbID(context.Background(), "Mononoke the Movie", "2025")
	require.True(t, ok)
	assert.Equal(t, "tt35301440", id, "the TV series (no film binding) is skipped, the 2025 film taken")

	id, ok = source.FindIMDbID(context.Background(), "Mononoke the Movie", "2026")
	require.True(t, ok)
	assert.Equal(t, "tt35301440", id, "a year off by one still matches")

	id, ok = source.FindIMDbID(context.Background(), "Mononoke", "1997")
	require.True(t, ok)
	assert.Equal(t, "tt0119698", id)

	_, ok = source.FindIMDbID(context.Background(), "Mononoke", "2010")
	assert.False(t, ok, "no film of that year among the candidates")

	id, ok = source.FindIMDbID(context.Background(), "Mononoke", "")
	require.True(t, ok)
	assert.Equal(t, "tt35301440", id, "without a year the best-ranked film wins")
}

func TestWikidataFindIMDbIDNoHits(t *testing.T) {
	source := findingServer(t, `{"search":[]}`, findSPARQL)
	_, ok := source.FindIMDbID(context.Background(), "Nothing", "2025")
	assert.False(t, ok)

	source = findingServer(t, `{"search":[{"id":"not-a-qid"}]}`, findSPARQL)
	_, ok = source.FindIMDbID(context.Background(), "Nothing", "2025")
	assert.False(t, ok, "hits that are not items are dropped before any query goes out")

	source, _ = wikidataServer(t, http.StatusBadGateway, "down")
	_, ok = source.FindIMDbID(context.Background(), "Nothing", "2025")
	assert.False(t, ok)
}

func TestWikidataScores(t *testing.T) {
	source, seen := wikidataServer(t, http.StatusOK, sampleSPARQL)

	records, err := source.Records(context.Background(),
		[]string{"tt0090605", "tt26581740", "tt7777777", "tt0000009", "tt0000010", "tt0000011", "nm1"})

	require.NoError(t, err)
	query := seen.URL.Query().Get("query")
	assert.Contains(t, query, `"tt0090605" "tt26581740" "tt7777777" "tt0000009" `)
	assert.NotContains(t, query, "nm1", "only title ids are asked about")
	assert.Contains(t, query, "P444")
	assert.Contains(t, query, "schema:about")
	assert.Equal(t, "application/sparql-results+json", seen.Header.Get("Accept"))
	assert.Contains(t, seen.Header.Get("User-Agent"), "Scout")

	assert.Equal(t, Record{Ratings: []tvdb.Rating{
		{Source: tvdb.RatingSourceRottenTomatoes, Value: 93},
		{Source: tvdb.RatingSourceMetacritic, Value: 84},
	}, Article: "Aliens (film)", Film: true}, records["tt0090605"],
		"the latest Tomatometer wins; the average and IMDb entries are skipped")
	assert.Equal(t, []tvdb.Rating{{Source: tvdb.RatingSourceMetacritic, Value: 81}}, records["tt26581740"].Ratings)
	assert.Equal(t, []tvdb.Rating{{Source: tvdb.RatingSourceRottenTomatoes, Value: 50}}, records["tt7777777"].Ratings)
	assert.Equal(t, Record{Article: "Brand New (2026 film)", Film: true}, records["tt0000009"],
		"an item with an article but no scores still comes back, for the prose fallback")
	assert.Equal(t, Record{}, records["tt0000010"], "a series item with nothing recorded")
	_, ok := records["tt0000011"]
	assert.False(t, ok, "an id Wikidata has no item for is absent")
}

func TestWikidataScoresAsksNothingWithoutTitleIDs(t *testing.T) {
	source, seen := wikidataServer(t, http.StatusOK, sampleSPARQL)

	records, err := source.Records(context.Background(), []string{"", "nm1"})

	require.NoError(t, err)
	assert.Empty(t, records)
	assert.Nil(t, seen.URL, "no request was made")
}

func TestWikidataScoresErrors(t *testing.T) {
	source, _ := wikidataServer(t, http.StatusTooManyRequests, "slow down")
	_, err := source.Records(context.Background(), []string{"tt0090605"})
	assert.Error(t, err)

	source, _ = wikidataServer(t, http.StatusOK, "<html>")
	_, err = source.Records(context.Background(), []string{"tt0090605"})
	assert.Error(t, err)
}

func TestArticleTitle(t *testing.T) {
	assert.Equal(t, "Sinners (2025 film)", articleTitle("https://en.wikipedia.org/wiki/Sinners_(2025_film)"))
	assert.Equal(t, "Amélie", articleTitle("https://en.wikipedia.org/wiki/Am%C3%A9lie"))
	assert.Equal(t, "", articleTitle("https://en.wikipedia.org/"))
}
