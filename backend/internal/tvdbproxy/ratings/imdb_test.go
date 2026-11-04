package ratings

import (
	"bytes"
	"compress/gzip"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gzipped(t *testing.T, text string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, err := w.Write([]byte(text))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return buf.Bytes()
}

const sampleTable = "tconst\taverageRating\tnumVotes\n" +
	"tt0111161\t9.3\t2900000\n" +
	"tt9999999\t6.1\t40\n" +
	"tt10000000\t7.0\t12\n" +
	"broken line\n" +
	"tt0000001\t5.7\t2135\n"

func TestDatasetLoadAndRating(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(gzipped(t, sampleTable))
	}))
	defer server.Close()
	d := NewDataset(server.URL, server.Client(), slog.Default())
	assert.False(t, d.Loaded())
	_, ok := d.Rating("tt0111161")
	assert.False(t, ok, "nothing before the table is loaded")

	require.NoError(t, d.Load(context.Background()))

	assert.True(t, d.Loaded())
	rating, ok := d.Rating("tt0111161")
	require.True(t, ok)
	assert.Equal(t, 9.3, rating.Value)
	assert.Equal(t, 2900000, rating.Votes)
	assert.Equal(t, "imdb", rating.Source)
	rating, ok = d.Rating("tt10000000")
	require.True(t, ok, "ids longer than seven digits sort numerically, not as text")
	assert.Equal(t, 7.0, rating.Value)
	rating, ok = d.Rating("tt9999999")
	require.True(t, ok)
	assert.Equal(t, 6.1, rating.Value)
	_, ok = d.Rating("tt0000002")
	assert.False(t, ok)
	_, ok = d.Rating("nm0000001")
	assert.False(t, ok, "not a title id")
}

func TestDatasetKeepsTheOldTableWhenARefreshFails(t *testing.T) {
	var fail atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write(gzipped(t, sampleTable))
	}))
	defer server.Close()
	d := NewDataset(server.URL, server.Client(), slog.Default())
	require.NoError(t, d.Load(context.Background()))

	fail.Store(true)
	err := d.Load(context.Background())

	require.Error(t, err)
	_, ok := d.Rating("tt0111161")
	assert.True(t, ok, "the previous table still answers")
}

func TestDatasetRejectsAnEmptyTable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(gzipped(t, "tconst\taverageRating\tnumVotes\n"))
	}))
	defer server.Close()
	d := NewDataset(server.URL, server.Client(), slog.Default())

	assert.Error(t, d.Load(context.Background()))
	assert.False(t, d.Loaded())
}

func TestDatasetRejectsNonGzip(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(sampleTable))
	}))
	defer server.Close()
	d := NewDataset(server.URL, server.Client(), slog.Default())

	assert.Error(t, d.Load(context.Background()))
}

func TestParseDatasetLine(t *testing.T) {
	row, ok := parseDatasetLine("tt0111161\t9.3\t2900000")
	require.True(t, ok)
	assert.Equal(t, imdbRow{id: 111161, rating: 93, votes: 2900000}, row)

	for _, line := range []string{"", "tt1", "tt1\t9.3", "tt1\tnine\t3", "tt1\t9.3\tmany", "tt1\t11\t3", "x1\t9.3\t3"} {
		_, ok := parseDatasetLine(line)
		assert.False(t, ok, line)
	}
}
