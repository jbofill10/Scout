package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
