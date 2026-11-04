package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadReadsRatingSourcesFromEnvironment(t *testing.T) {
	t.Setenv("TVDB_HOST", "https://tvdb.test")
	t.Setenv("TVDB_API_KEY", "key")
	t.Setenv("RATINGS_IMDB", "")
	t.Setenv("IMDB_RATINGS_URL", "")
	t.Setenv("RATINGS_WIKIDATA", "")
	t.Setenv("WIKIDATA_SPARQL_URL", "")
	t.Setenv("OMDB_API_KEY", "")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, IMDb{Enabled: true}, cfg.IMDb, "the IMDb table is on by default")
	assert.Equal(t, WikidataRatings{Enabled: true}, cfg.Wikidata, "Wikidata scores are on by default")
	assert.Equal(t, OMDb{}, cfg.OMDb, "OMDb is off without a key")

	t.Setenv("RATINGS_IMDB", "false")
	t.Setenv("IMDB_RATINGS_URL", "http://imdb.test/ratings.tsv.gz")
	t.Setenv("RATINGS_WIKIDATA", "off")
	t.Setenv("WIKIDATA_SPARQL_URL", "http://wd.test/sparql")
	t.Setenv("OMDB_API_KEY", " abc ")
	t.Setenv("OMDB_HOST", "http://omdb.test")
	cfg, err = Load()
	require.NoError(t, err)
	assert.Equal(t, IMDb{Enabled: false, URL: "http://imdb.test/ratings.tsv.gz"}, cfg.IMDb)
	assert.Equal(t, WikidataRatings{Enabled: false, URL: "http://wd.test/sparql"}, cfg.Wikidata)
	assert.Equal(t, OMDb{APIKey: "abc", Host: "http://omdb.test"}, cfg.OMDb, "the key is trimmed")
}

func TestLoadReadsTraktFromEnvironment(t *testing.T) {
	t.Setenv("TVDB_HOST", "https://tvdb.test")
	t.Setenv("TVDB_API_KEY", "key")
	t.Setenv("TRAKT_CLIENT_ID", "")
	t.Setenv("TRAKT_LIST", "")
	t.Setenv("TRAKT_PERIOD", "")
	t.Setenv("TRAKT_HOST", "")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, Trakt{}, cfg.Trakt, "unset Trakt variables leave the proxy on the TVDB ranking")
	require.NoError(t, cfg.Validate())

	t.Setenv("TRAKT_CLIENT_ID", "abc")
	t.Setenv("TRAKT_LIST", "trending")
	t.Setenv("TRAKT_PERIOD", "monthly")
	t.Setenv("TRAKT_HOST", "https://trakt.test")
	cfg, err = Load()
	require.NoError(t, err)
	assert.Equal(t, Trakt{Host: "https://trakt.test", ClientID: "abc", List: "trending", Period: "monthly"}, cfg.Trakt)
}
