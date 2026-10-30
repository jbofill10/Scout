package service

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	repomocks "github.com/jbofill10/scout/backend/internal/torrenter/repository/mocks"
	"github.com/jbofill10/scout/backend/internal/torrenter/searchgate"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golift.io/starr"
	"golift.io/starr/prowlarr"
)

// fakeProwlarr serves Prowlarr's search endpoint from canned per-indexer results and
// records which indexers each request targeted, in order.
type fakeProwlarr struct {
	t  *testing.T
	mu sync.Mutex
	// results holds the hits returned for any query against an indexer id.
	results map[int64][]*prowlarr.Search
	// failing indexer ids answer every search with HTTP 500.
	failing map[int64]bool
	// requested is the indexerIds of every search request, in order.
	requested [][]int64
}

func (f *fakeProwlarr) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/search") {
			http.NotFound(w, r)
			return
		}

		ids := []int64{}
		for _, raw := range r.URL.Query()["indexerIds"] {
			id, err := strconv.ParseInt(raw, 10, 64)
			require.NoError(f.t, err)
			ids = append(ids, id)
		}

		f.mu.Lock()
		defer f.mu.Unlock()
		f.requested = append(f.requested, ids)

		hits := []*prowlarr.Search{}
		for _, id := range ids {
			if f.failing[id] {
				http.Error(w, "indexer down", http.StatusInternalServerError)
				return
			}
			hits = append(hits, f.results[id]...)
		}

		w.Header().Set("Content-Type", "application/json")
		require.NoError(f.t, json.NewEncoder(w).Encode(hits))
	}
}

// indexersQueried flattens the recorded requests for single-indexer searches.
func (f *fakeProwlarr) indexersQueried() []int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []int64{}
	for _, ids := range f.requested {
		require.Len(f.t, ids, 1, "these tests only search one indexer per request")
		out = append(out, ids[0])
	}
	return out
}

func newFallbackTestHandler(t *testing.T, fake *fakeProwlarr) *QbittHandler {
	t.Helper()
	server := httptest.NewServer(fake.handler())
	t.Cleanup(server.Close)

	repo := repomocks.NewRepository(t)
	repo.On("GetPreferredUploaders", mock.Anything, "series", false).Return([]string{}, nil).Maybe()
	repo.On("InsertDownloadHistory", mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()

	return &QbittHandler{
		p:      prowlarr.New(starr.New("test-key", server.URL, 5*time.Second)),
		repo:   repo,
		logger: slog.New(slog.NewTextHandler(new(bytes.Buffer), nil)),
		parser: NewTorrentParser(),
	}
}

func hitFrom(indexerID int64, title string) *prowlarr.Search {
	return &prowlarr.Search{
		Title:     title,
		GUID:      "magnet:?xt=urn:btih:" + strings.ToLower(strings.ReplaceAll(title, " ", "")),
		InfoHash:  strings.ToLower(strings.ReplaceAll(title, " ", "")),
		Seeders:   12,
		IndexerID: indexerID,
	}
}

// hitWithSeeders is hitFrom with an explicit reported seeder count.
func hitWithSeeders(indexerID int64, title string, seeders int) *prowlarr.Search {
	h := hitFrom(indexerID, title)
	h.Seeders = seeders
	return h
}

// repeat returns id n times, for building expected indexer sequences.
func repeat(id int64, n int) []int64 {
	out := make([]int64, n)
	for i := range out {
		out[i] = id
	}
	return out
}

func someShowStrategies(q *QbittHandler) (*tvdb.Media, []*models.SearchStrategy) {
	req := &tvdb.Media{Id: "9001", Name: "Some Show", Category: "series"}
	return req, q.createSearchStrategy(req, &tvdb.Episode{Id: 55, SeasonNumber: 1, Number: 5})
}

func TestSearchForMatch_PreferredIndexerWinsWithoutConsultingPirateBay(t *testing.T) {
	fake := &fakeProwlarr{t: t, results: map[int64][]*prowlarr.Search{
		ONE337x_ID: {hitFrom(ONE337x_ID, "Some Show S01E05 720p WEB h264-GRACE")},
		TPB_ID:     {hitFrom(TPB_ID, "Some Show S01E05 2160p WEB h265-GRACE")},
	}}
	q := newFallbackTestHandler(t, fake)
	req, strategies := someShowStrategies(q)

	outcome := q.searchForMatch(context.Background(), strategies, req, nil)

	require.NotNil(t, outcome.match)
	assert.Equal(t, ONE337x_ID, outcome.match.Torrent.IndexerID,
		"a release on the preferred indexer is taken even when the fallback has a better one")
	assert.NotContains(t, fake.indexersQueried(), TPB_ID, "The Pirate Bay must not be searched")
	assert.Equal(t, repeat(ONE337x_ID, len(queriesAtLevel(strategies, 0))), fake.indexersQueried(),
		"the strict tier matched, so no relaxed tier ran")
}

func TestSearchForMatch_FallsBackToPirateBayWhenPreferredHasNothing(t *testing.T) {
	fake := &fakeProwlarr{t: t, results: map[int64][]*prowlarr.Search{
		TPB_ID: {hitFrom(TPB_ID, "Some Show S01E05 1080p WEB h264-GRACE")},
	}}
	q := newFallbackTestHandler(t, fake)
	req, strategies := someShowStrategies(q)

	outcome := q.searchForMatch(context.Background(), strategies, req, nil)

	require.NotNil(t, outcome.match)
	assert.Equal(t, TPB_ID, outcome.match.Torrent.IndexerID)
	assert.Equal(t, "Some Show S01E05 1080p WEB h264-GRACE", outcome.match.Torrent.Title)
	assert.True(t, outcome.attemptedAny)
	assert.False(t, outcome.erroredAll)

	// Every relax tier ran against 1337x first; only then was The Pirate Bay searched,
	// and it stopped at the strict tier because that already produced a match.
	want := append(repeat(ONE337x_ID, len(strategies)), repeat(TPB_ID, len(queriesAtLevel(strategies, 0)))...)
	assert.Equal(t, want, fake.indexersQueried())
}

func TestSearchForMatch_NoTorrentAnywhere(t *testing.T) {
	fake := &fakeProwlarr{t: t, results: map[int64][]*prowlarr.Search{}}
	q := newFallbackTestHandler(t, fake)
	req, strategies := someShowStrategies(q)

	outcome := q.searchForMatch(context.Background(), strategies, req, nil)

	assert.Nil(t, outcome.match)
	assert.True(t, outcome.attemptedAny)
	assert.False(t, outcome.erroredAll, "empty results are not an unreachable indexer")
	want := append(repeat(ONE337x_ID, len(strategies)), repeat(TPB_ID, len(strategies))...)
	assert.Equal(t, want, fake.indexersQueried(), "both indexer tiers exhaust every relax tier")
}

func TestSearchForMatch_PreferredIndexerDownStillTriesPirateBay(t *testing.T) {
	// Skip the retry backoffs so the failing indexer does not slow the test down.
	origBackoffs := prowlarrSearchBackoffs
	prowlarrSearchBackoffs = nil
	defer func() { prowlarrSearchBackoffs = origBackoffs }()

	fake := &fakeProwlarr{
		t:       t,
		failing: map[int64]bool{ONE337x_ID: true},
		results: map[int64][]*prowlarr.Search{
			TPB_ID: {hitFrom(TPB_ID, "Some Show S01E05 1080p WEB h264-GRACE")},
		},
	}
	q := newFallbackTestHandler(t, fake)
	req, strategies := someShowStrategies(q)

	outcome := q.searchForMatch(context.Background(), strategies, req, nil)

	require.NotNil(t, outcome.match)
	assert.Equal(t, TPB_ID, outcome.match.Torrent.IndexerID)
	assert.False(t, outcome.erroredAll, "a working fallback means the indexers are not all unreachable")
}

// A release on the preferred indexer whose reported seeders are suspect is not
// taken at face value: the fallback indexer is consulted, and its live release
// wins. Only the strict tier runs against the preferred indexer in that case.
func TestSearchForMatch_WeaklySeededPreferredPickYieldsToPirateBay(t *testing.T) {
	fake := &fakeProwlarr{t: t, results: map[int64][]*prowlarr.Search{
		ONE337x_ID: {hitWithSeeders(ONE337x_ID, "Some.Show.S01E05.1080p.WEB.x264-GRACE", 3)},
		TPB_ID:     {hitWithSeeders(TPB_ID, "Some Show S01E05 1080p WEB h264-OTHER", 40)},
	}}
	q := newFallbackTestHandler(t, fake)
	req, strategies := someShowStrategies(q)

	outcome := q.searchForMatch(context.Background(), strategies, req, nil)

	require.NotNil(t, outcome.match)
	assert.Equal(t, TPB_ID, outcome.match.Torrent.IndexerID)
	assert.Equal(t, 40, outcome.match.Torrent.Seeders)
	strict := len(queriesAtLevel(strategies, 0))
	want := append(repeat(ONE337x_ID, strict), repeat(TPB_ID, strict)...)
	assert.Equal(t, want, fake.indexersQueried(),
		"a weak strict pick skips the preferred indexer's relaxed tiers and goes straight to the fallback")
}

// When every indexer only has weakly seeded releases, the best of them is still
// returned rather than reporting no torrent at all.
func TestSearchForMatch_WeaklySeededPickIsLastResort(t *testing.T) {
	fake := &fakeProwlarr{t: t, results: map[int64][]*prowlarr.Search{
		ONE337x_ID: {hitWithSeeders(ONE337x_ID, "Some.Show.S01E05.1080p.WEB.x264-GRACE", 2)},
		TPB_ID:     {hitWithSeeders(TPB_ID, "Some Show S01E05 1080p WEB h264-OTHER", 4)},
	}}
	q := newFallbackTestHandler(t, fake)
	req, strategies := someShowStrategies(q)

	outcome := q.searchForMatch(context.Background(), strategies, req, nil)

	require.NotNil(t, outcome.match)
	assert.Equal(t, TPB_ID, outcome.match.Torrent.IndexerID, "the better-seeded of the weak picks is kept")
	assert.Equal(t, 4, outcome.match.Torrent.Seeders)
}

func TestSearchForMatch_WeaklySeededPickWhenFallbackHasNothing(t *testing.T) {
	fake := &fakeProwlarr{t: t, results: map[int64][]*prowlarr.Search{
		ONE337x_ID: {hitWithSeeders(ONE337x_ID, "Some.Show.S01E05.1080p.WEB.x264-GRACE", 2)},
	}}
	q := newFallbackTestHandler(t, fake)
	req, strategies := someShowStrategies(q)

	outcome := q.searchForMatch(context.Background(), strategies, req, nil)

	require.NotNil(t, outcome.match)
	assert.Equal(t, ONE337x_ID, outcome.match.Torrent.IndexerID)
	strict := len(queriesAtLevel(strategies, 0))
	want := append(repeat(ONE337x_ID, strict), repeat(TPB_ID, len(strategies))...)
	assert.Equal(t, want, fake.indexersQueried(),
		"the fallback indexer exhausts every relax tier before the weak pick is accepted")
}

// An indexer the health monitor has paused (Prowlarr disabled it, usually a
// ban) is not searched at all: the request falls through to the fallback
// tier at once instead of burning queries on a dead exit.
func TestSearchForMatch_PausedPreferredIndexerFallsThroughImmediately(t *testing.T) {
	fake := &fakeProwlarr{t: t, results: map[int64][]*prowlarr.Search{
		ONE337x_ID: {hitFrom(ONE337x_ID, "Some Show S01E05 1080p WEB h264-NTb")},
		TPB_ID:     {hitFrom(TPB_ID, "Some Show S01E05 1080p WEB h264-GRACE")},
	}}
	q := newFallbackTestHandler(t, fake)
	q.gate = searchgate.New(nil)
	q.gate.Pause(ONE337x_ID, time.Now().Add(time.Hour), "Cloudflare 1006")
	req, strategies := someShowStrategies(q)

	outcome := q.searchForMatch(context.Background(), strategies, req, nil)

	require.NotNil(t, outcome.match)
	assert.Equal(t, TPB_ID, outcome.match.Torrent.IndexerID)
	for _, ids := range fake.requested {
		assert.NotContains(t, ids, ONE337x_ID, "the paused indexer must not be queried")
	}
	assert.False(t, outcome.erroredAll)
}

// With the gate in place every search still reaches Prowlarr, in order, and
// the gate's snapshot accounts for them.
func TestSearchForMatch_GateCountsSearches(t *testing.T) {
	fake := &fakeProwlarr{t: t, results: map[int64][]*prowlarr.Search{
		ONE337x_ID: {hitFrom(ONE337x_ID, "Some Show S01E05 1080p WEB h264-NTb")},
	}}
	q := newFallbackTestHandler(t, fake)
	q.gate = searchgate.New(func(int64) searchgate.Limits { return searchgate.Limits{PerMinute: 600, Burst: 10} })
	req, strategies := someShowStrategies(q)
	ctx := searchgate.WithRequest(context.Background(), searchgate.Follow, "Some Show S01E05")

	outcome := q.searchForMatch(ctx, strategies, req, nil)

	require.NotNil(t, outcome.match)
	snap := q.gate.Snapshot()
	require.NotEmpty(t, snap)
	assert.Equal(t, ONE337x_ID, snap[0].ID)
	assert.Equal(t, len(fake.requested), snap[0].SearchesLastMinute)
	assert.Empty(t, snap[0].InFlight, "every search released its slot")
}
