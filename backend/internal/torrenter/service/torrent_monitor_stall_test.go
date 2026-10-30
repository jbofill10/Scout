package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	repomocks "github.com/jbofill10/scout/backend/internal/torrenter/repository/mocks"
	"github.com/jbofill10/scout/backend/pkg/dlstatus"
	"github.com/jbofill10/scout/backend/pkg/notifications"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golift.io/starr/prowlarr"
)

// fastPolls makes the monitor poll every few milliseconds for one test.
func fastPolls(t *testing.T) {
	t.Helper()
	prev := monitorPoll
	monitorPoll = func(time.Duration) time.Duration { return 5 * time.Millisecond }
	t.Cleanup(func() { monitorPoll = prev })
}

func qbitTorrent(hash string, progress float64, seeds, speed int64, state string) map[string]any {
	return map[string]any{
		"hash": hash, "name": "Big.Movie.2022.2160p", "progress": progress, "state": state,
		"num_seeds": seeds, "dlspeed": speed, "save_path": "/dl/Big.Movie", "size": int64(1000), "completed": int64(progress * 1000),
	}
}

func stallTestMatch() *models.TorrentMatch {
	return &models.TorrentMatch{
		Strategy: &models.SearchStrategy{MediaName: "Big Movie", TvdbId: "77", EpisodeTvdbID: "77", IsMovie: true, ReleaseYear: "2022"},
		Torrent:  &prowlarr.Search{Title: "Big.Movie.2022.2160p", InfoHash: "stall-hash"},
	}
}

// A download that keeps moving is never given up on, however long it has
// been running: a resumed monitor of an 11-hour-old torrent at 89% must see
// it through to completion, not expire on arrival.
func TestMonitor_SlowButMovingDownloadCompletes(t *testing.T) {
	t.Setenv("MONITOR_TIMEOUT", "1h")
	fastPolls(t)
	qb := &fakeQbittorrent{info: func(poll int) []map[string]any {
		progress := 0.89 + float64(poll)*0.03
		if progress >= 1 {
			progress = 1
		}
		return []map[string]any{qbitTorrent("stall-hash", progress, 5, 2_000_000, "downloading")}
	}}
	repo := repomocks.NewRepository(t)
	q := newCancelTestHandler(t, repo, qb)

	var wg sync.WaitGroup
	done := make(chan models.TorrentCompleteEvent, 1)
	wg.Add(1)
	go q.monitorTorrentCompletion(context.Background(), stallTestMatch(), nil, "stall-hash", "uuid-1",
		time.Now().Add(-11*time.Hour), done, &wg)

	select {
	case ev := <-done:
		assert.Equal(t, "stall-hash", ev.Hash)
		assert.Equal(t, "/dl/Big.Movie", ev.SavePath)
	case <-time.After(5 * time.Second):
		t.Fatal("a progressing download must complete, not time out")
	}
	wg.Wait()
	assert.Equal(t, []string{"stall-hash"}, qb.rechecked, "completion is confirmed with one recheck")
	assert.Empty(t, qb.deletedHashes())
}

// A download that has not moved for MONITOR_TIMEOUT is given up on even with
// seeds attached, and treated like a dead swarm: removed, remembered, and
// replaced when the request is known.
func TestMonitor_NoProgressForTimeoutGivesUpAndMarksLost(t *testing.T) {
	t.Setenv("MONITOR_TIMEOUT", "40ms")
	fastPolls(t)
	qb := &fakeQbittorrent{info: func(int) []map[string]any {
		return []map[string]any{qbitTorrent("stall-hash", 0.42, 3, 0, "stalledDL")}
	}}
	notification := &notifications.Notification{ID: 9, TvdbID: "77"}
	repo := repomocks.NewRepository(t)
	repo.On("InsertDownloadHistory", mock.Anything, "Big Movie", 0, 0, 0, "", "failure", mock.MatchedBy(func(r string) bool {
		return len(r) > 0 && r[:11] == "no progress"
	})).Return(nil)
	repo.On("InsertFailedTorrent", mock.Anything, mock.MatchedBy(func(ft *models.FailedTorrent) bool {
		return ft.InfoHash == "stall-hash" && ft.TvdbID == "77"
	})).Return(nil)
	repo.On("DeleteActiveTorrent", mock.Anything, "stall-hash").Return(nil)
	repo.On("GetNotification", mock.Anything, "77").Return(notification, nil)
	var written *notifications.Notification
	repo.On("UpdateNotification", mock.Anything, mock.MatchedBy(func(n *notifications.Notification) bool {
		written = n
		return true
	})).Return(nil)
	q := newCancelTestHandler(t, repo, qb)

	var wg sync.WaitGroup
	done := make(chan models.TorrentCompleteEvent, 1)
	wg.Add(1)
	finished := make(chan struct{})
	go func() {
		// No request recorded: the replacement path reports the loss instead.
		q.monitorTorrentCompletion(context.Background(), stallTestMatch(), nil, "stall-hash", "uuid-1", time.Now(), done, &wg)
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("a stalled download must be given up on after MONITOR_TIMEOUT")
	}

	assert.Empty(t, done, "no completion event for a lost download")
	assert.Equal(t, []string{"stall-hash"}, qb.deletedHashes(), "the stalled torrent is removed from qBittorrent")
	require.NotNil(t, written)
	assert.Equal(t, notifications.StatusFailed, written.Status)
	assert.Equal(t, string(dlstatus.CodeMonitorLost), written.FailureCode, "the webserver needs the code to retry")
	assert.Contains(t, written.Reason, "made no progress for 40ms")
	repo.AssertExpectations(t)
}

// The stall window is measured from when this monitor started, not from the
// torrent's original start: an old torrent resumed after a restart gets a
// full window before it can be called stalled.
func TestMonitor_ResumedMonitorMeasuresStallFromNow(t *testing.T) {
	t.Setenv("MONITOR_TIMEOUT", "300ms")
	fastPolls(t)
	qb := &fakeQbittorrent{info: func(poll int) []map[string]any {
		// Stuck for the first polls, then finishes: inside the window.
		if poll < 8 {
			return []map[string]any{qbitTorrent("stall-hash", 0.5, 2, 0, "stalledDL")}
		}
		return []map[string]any{qbitTorrent("stall-hash", 1, 2, 0, "uploading")}
	}}
	repo := repomocks.NewRepository(t)
	q := newCancelTestHandler(t, repo, qb)

	var wg sync.WaitGroup
	done := make(chan models.TorrentCompleteEvent, 1)
	wg.Add(1)
	go q.monitorTorrentCompletion(context.Background(), stallTestMatch(), nil, "stall-hash", "uuid-1",
		time.Now().Add(-48*time.Hour), done, &wg)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("a resumed monitor must not inherit an expired window")
	}
	wg.Wait()
	assert.Empty(t, qb.deletedHashes())
}
