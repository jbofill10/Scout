package service

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	repomocks "github.com/jbofill10/scout/backend/internal/torrenter/repository/mocks"
	"github.com/jbofill10/scout/backend/pkg/dlstatus"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/notifications"

	qbittorrent "github.com/autobrr/go-qbittorrent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golift.io/starr"
	"golift.io/starr/prowlarr"
)

// fakeQbittorrent answers the qBittorrent Web API calls the cancel path makes
// and records the hashes it was asked to delete.
type fakeQbittorrent struct {
	mu      sync.Mutex
	deleted []string
	// addStatus is what /torrents/add answers; 0 means 200.
	addStatus int
}

func (f *fakeQbittorrent) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "test"})
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/delete":
			_ = r.ParseForm()
			f.mu.Lock()
			f.deleted = append(f.deleted, r.Form.Get("hashes"))
			f.mu.Unlock()
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/add":
			if f.addStatus != 0 {
				http.Error(w, "add failed", f.addStatus)
				return
			}
			_, _ = w.Write([]byte("Ok."))
		default:
			http.NotFound(w, r)
		}
	}
}

func (f *fakeQbittorrent) deletedHashes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.deleted...)
}

func newCancelTestHandler(t *testing.T, repo *repomocks.Repository, qb *fakeQbittorrent) *QbittHandler {
	t.Helper()
	server := httptest.NewServer(qb.handler())
	t.Cleanup(server.Close)
	client := qbittorrent.NewClient(qbittorrent.Config{Host: server.URL, Timeout: 5})
	require.NoError(t, client.LoginCtx(context.Background()))
	return &QbittHandler{
		c:        client,
		repo:     repo,
		logger:   slog.New(slog.NewTextHandler(new(bytes.Buffer), nil)),
		parser:   NewTorrentParser(),
		inflight: newInflightRegistry(),
	}
}

func TestInflightRegistry_AddTakeAndNilSafety(t *testing.T) {
	var nilReg *inflightRegistry
	assert.NotPanics(t, func() { nilReg.add("1", &inflightWork{})() })
	assert.Nil(t, nilReg.take("1"))

	reg := newInflightRegistry()
	a := &inflightWork{}
	b := &inflightWork{infoHash: "hash"}
	removeA := reg.add("12", a)
	reg.add("12", b)
	reg.add("13", &inflightWork{})

	removeA()
	got := reg.take("12")
	require.Len(t, got, 1, "a removed entry must not be returned")
	assert.Same(t, b, got[0])
	assert.Empty(t, reg.take("12"), "take empties the item")
	assert.Len(t, reg.take("13"), 1, "other items are untouched")
}

func TestSleepCtx(t *testing.T) {
	assert.True(t, sleepCtx(context.Background(), time.Millisecond), "a full wait reports true")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	assert.False(t, sleepCtx(ctx, time.Hour), "a cancelled context cuts the wait short")
	assert.Less(t, time.Since(start), time.Second)
}

// Cancelling an item aborts its search, stops its monitor, deletes the
// monitored torrent and any leftover in-flight row for the item from
// qBittorrent, and marks the notification cancelled. Other items are untouched.
func TestCancelItem_StopsEverythingForTheItem(t *testing.T) {
	repo := repomocks.NewRepository(t)
	repo.On("GetActiveTorrents", mock.Anything).Return([]*models.ActiveTorrent{
		{InfoHash: "monitored", TorrentTitle: "Some Show S01E05", Strategy: &models.SearchStrategy{EpisodeTvdbID: "12"}},
		{InfoHash: "orphan", TorrentTitle: "Some Show S01E05 again", Strategy: &models.SearchStrategy{EpisodeTvdbID: "12"}},
		{InfoHash: "other", TorrentTitle: "Other Show", Strategy: &models.SearchStrategy{EpisodeTvdbID: "99"}},
		{InfoHash: "unreadable", Strategy: nil},
	}, nil)
	repo.On("DeleteActiveTorrent", mock.Anything, "monitored").Return(nil).Once()
	repo.On("DeleteActiveTorrent", mock.Anything, "orphan").Return(nil).Once()
	repo.On("GetNotification", mock.Anything, "12").Return(
		&notifications.Notification{ID: 7, TvdbID: "12", Status: notifications.StatusDownloading}, nil)
	repo.On("UpdateNotification", mock.Anything, mock.MatchedBy(func(n *notifications.Notification) bool {
		return n.ID == 7 && n.Status == notifications.StatusCancelled && n.Reason == "Cancelled"
	})).Return(nil)

	qb := &fakeQbittorrent{}
	q := newCancelTestHandler(t, repo, qb)

	searchCtx, abortSearch := context.WithCancel(context.Background())
	search := &inflightWork{cancel: abortSearch}
	q.inflight.add("12", search)
	monitorCtx, stopMonitor := context.WithCancel(context.Background())
	q.inflight.add("12", &inflightWork{cancel: stopMonitor, infoHash: "monitored", title: "Some Show S01E05"})
	otherCtx, stopOther := context.WithCancel(context.Background())
	defer stopOther()
	q.inflight.add("99", &inflightWork{cancel: stopOther, infoHash: "other"})

	resp, err := q.CancelItem(context.Background(), "12")

	require.NoError(t, err)
	assert.True(t, resp.SearchCancelled)
	assert.Equal(t, 2, resp.TorrentsRemoved, "the monitored torrent and the orphan row, not the other item's")
	assert.True(t, resp.NotificationUpdated)
	assert.True(t, resp.Found())
	assert.ErrorIs(t, searchCtx.Err(), context.Canceled, "the search context was cancelled")
	assert.ErrorIs(t, monitorCtx.Err(), context.Canceled, "the monitor context was cancelled")
	assert.NoError(t, otherCtx.Err(), "another item's monitor keeps running")
	assert.True(t, search.cancelled.Load(), "the owner can tell it was a user cancel")
	assert.ElementsMatch(t, []string{"monitored", "orphan"}, qb.deletedHashes())
	assert.Empty(t, q.inflight.take("12"), "nothing is left registered for the item")
	assert.Len(t, q.inflight.take("99"), 1)
}

func TestCancelItem_NothingInFlight(t *testing.T) {
	repo := repomocks.NewRepository(t)
	repo.On("GetActiveTorrents", mock.Anything).Return([]*models.ActiveTorrent{}, nil)
	repo.On("GetNotification", mock.Anything, "12").Return(nil, nil)
	q := newCancelTestHandler(t, repo, &fakeQbittorrent{})

	resp, err := q.CancelItem(context.Background(), "12")

	require.NoError(t, err)
	assert.False(t, resp.Found())
}

// A search that is cancelled while Prowlarr is being queried reports the item
// as cancelled instead of as an unreachable indexer, so the webserver schedules
// no retry for it.
func TestProcessEpisodeDownload_CancelledSearchReportsCancelled(t *testing.T) {
	origBackoffs := prowlarrSearchBackoffs
	prowlarrSearchBackoffs = nil
	defer func() { prowlarrSearchBackoffs = origBackoffs }()

	// Every Prowlarr call stalls until the test cancels the item.
	release := make(chan struct{})
	prowlarrSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
		http.Error(w, "gone", http.StatusServiceUnavailable)
	}))
	defer prowlarrSrv.Close()
	defer close(release)

	repo := repomocks.NewRepository(t)
	repo.On("GetFailedTorrents", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Maybe()
	repo.On("GetActiveTorrents", mock.Anything).Return([]*models.ActiveTorrent{}, nil).Maybe()
	repo.On("GetNotification", mock.Anything, "55").Return(nil, nil).Maybe()
	repo.On("InsertDownloadHistory", mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()

	q := newCancelTestHandler(t, repo, &fakeQbittorrent{})
	q.p = prowlarr.New(starr.New("test-key", prowlarrSrv.URL, 5*time.Second))

	req := &tvdb.Media{Id: "9001", Name: "Some Show", Category: "series"}
	strategies := q.createSearchStrategy(req, &tvdb.Episode{Id: 55, SeasonNumber: 1, Number: 5})
	done := make(chan models.TorrentCompleteEvent, 1)
	var wg sync.WaitGroup

	results := make(chan dlstatus.EpisodeResult, 1)
	go func() { results <- q.processEpisodeDownload(context.Background(), 0, strategies, req, done, &wg) }()

	// Wait for the search to register itself, then cancel it.
	require.Eventually(t, func() bool {
		q.inflight.mu.Lock()
		defer q.inflight.mu.Unlock()
		return len(q.inflight.work["55"]) == 1
	}, 2*time.Second, 5*time.Millisecond)
	_, err := q.CancelItem(context.Background(), "55")
	require.NoError(t, err)

	select {
	case res := <-results:
		assert.Equal(t, dlstatus.OutcomeFailed, res.Outcome)
		assert.Equal(t, dlstatus.CodeCancelled, res.Code)
		assert.Equal(t, "55", res.TvdbID)
	case <-time.After(5 * time.Second):
		t.Fatal("search did not return after cancel")
	}
	repo.AssertNotCalled(t, "UpdateNotification", mock.Anything, mock.MatchedBy(func(n *notifications.Notification) bool {
		return n.Status == notifications.StatusFailed
	}))
}
