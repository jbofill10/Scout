package ratings

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubTVDB answers GETs from a table of uri → body and counts them.
type stubTVDB struct {
	mu     sync.Mutex
	bodies map[string]string
	err    error
	uris   []string
}

func (s *stubTVDB) Get(_ context.Context, uri string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.uris = append(s.uris, uri)
	if s.err != nil {
		return nil, s.err
	}
	return []byte(s.bodies[uri]), nil
}

func TestResolverReadsShortExtendedRecords(t *testing.T) {
	client := &stubTVDB{bodies: map[string]string{
		"/series/371980/extended?short=true": `{"data":{"id":371980,"remoteIds":[
			{"id":"10.5240/3E33","type":13,"sourceName":"EIDR"},{"id":"tt11280740","type":2,"sourceName":"IMDB"}]}}`,
		"/movies/358595/extended?short=true": `{"data":{"id":358595,"remoteIds":[{"id":"tt31193180","type":2,"sourceName":"IMDB"}]}}`,
		"/series/1/extended?short=true":      `{"data":{"id":1,"remoteIds":[]}}`,
	}}
	r := NewResolver(client, slog.Default())

	assert.Equal(t, "tt11280740", r.ImdbID(context.Background(), "series", "371980"))
	assert.Equal(t, "tt31193180", r.ImdbID(context.Background(), "movie", "358595"))
	assert.Equal(t, "", r.ImdbID(context.Background(), "series", "1"))
	assert.Equal(t, "", r.ImdbID(context.Background(), "series", ""))

	assert.Equal(t, "tt11280740", r.ImdbID(context.Background(), "series", "371980"))
	assert.Equal(t, "", r.ImdbID(context.Background(), "series", "1"))
	assert.Len(t, client.uris, 3, "answers and misses are cached")
}

func TestResolverRetriesAfterAnError(t *testing.T) {
	client := &stubTVDB{err: errors.New("boom")}
	r := NewResolver(client, slog.Default())

	assert.Equal(t, "", r.ImdbID(context.Background(), "movie", "5"))
	client.mu.Lock()
	client.err = nil
	client.bodies = map[string]string{"/movies/5/extended?short=true": `{"data":{"id":5,"remoteIds":[{"id":"tt0000005","type":2}]}}`}
	client.mu.Unlock()
	assert.Equal(t, "tt0000005", r.ImdbID(context.Background(), "movie", "5"))
	assert.Len(t, client.uris, 2)
}

func TestResolverPersistsResolvedIDs(t *testing.T) {
	client := &stubTVDB{bodies: map[string]string{
		"/movies/358595/extended?short=true": `{"data":{"id":358595,"remoteIds":[{"id":"tt31193180","type":2,"sourceName":"IMDB"}]}}`,
	}}
	store := newStubStore()
	r := NewResolver(client, slog.Default())
	r.SetStore(store)

	assert.Equal(t, "tt31193180", r.ImdbID(context.Background(), "movie", "358595"))
	entry, found := store.entry("imdb-id:movies:358595")
	require.True(t, found)
	assert.JSONEq(t, `"tt31193180"`, string(entry.Value))

	// A fresh resolver with the same store does not ask TVDB.
	fresh := NewResolver(client, slog.Default())
	fresh.SetStore(store)
	assert.Equal(t, "tt31193180", fresh.ImdbID(context.Background(), "movie", "358595"))
	assert.Len(t, client.uris, 1)
}
