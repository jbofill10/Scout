package service

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	repomocks "github.com/jbofill10/scout/backend/internal/torrenter/repository/mocks"
	"github.com/jbofill10/scout/backend/pkg/notifications"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golift.io/starr/prowlarr"
)

func newResumeTestHandler(t *testing.T, repo *repomocks.Repository) *QbittHandler {
	t.Helper()
	return &QbittHandler{
		logger: slog.New(slog.NewTextHandler(new(bytes.Buffer), nil)),
		parser: NewTorrentParser(),
		repo:   repo,
	}
}

// closed reports whether done was closed, giving the goroutine a moment to run.
func closed(t *testing.T, done <-chan models.TorrentCompleteEvent) bool {
	t.Helper()
	select {
	case _, ok := <-done:
		return !ok
	case <-time.After(2 * time.Second):
		return false
	}
}

func TestResumeMonitors_NothingToResume(t *testing.T) {
	repo := repomocks.NewRepository(t)
	repo.On("GetActiveTorrents", mock.Anything).Return([]*models.ActiveTorrent{}, nil)

	q := newResumeTestHandler(t, repo)
	done := make(chan models.TorrentCompleteEvent, 1)

	resumed, err := q.ResumeMonitors(context.Background(), done)

	require.NoError(t, err)
	assert.Equal(t, 0, resumed)
	assert.True(t, closed(t, done), "done must be closed so the consumer exits")
}

// The channel must be closed on the error path too, or the completion handler
// goroutine started alongside it leaks for the life of the process.
func TestResumeMonitors_RepoErrorStillClosesChannel(t *testing.T) {
	repo := repomocks.NewRepository(t)
	repo.On("GetActiveTorrents", mock.Anything).Return(nil, errors.New("db down"))

	q := newResumeTestHandler(t, repo)
	done := make(chan models.TorrentCompleteEvent, 1)

	resumed, err := q.ResumeMonitors(context.Background(), done)

	require.Error(t, err)
	assert.Equal(t, 0, resumed)
	assert.True(t, closed(t, done))
}

func TestResumeMonitors_SkipsAndClearsRowWithNoStrategy(t *testing.T) {
	repo := repomocks.NewRepository(t)
	repo.On("GetActiveTorrents", mock.Anything).Return([]*models.ActiveTorrent{
		{InfoHash: "orphan", TrackingUUID: "uuid-1", TorrentTitle: "Broken"},
	}, nil)
	repo.On("DeleteActiveTorrent", mock.Anything, "orphan").Return(nil)

	q := newResumeTestHandler(t, repo)
	done := make(chan models.TorrentCompleteEvent, 1)

	resumed, err := q.ResumeMonitors(context.Background(), done)

	require.NoError(t, err)
	assert.Equal(t, 0, resumed, "a row with no strategy can never be resumed")
	assert.True(t, closed(t, done))
	repo.AssertExpectations(t)
}

// A resumed monitor inherits the deadline it was born with. Without this a
// restart would hand every in-flight torrent a fresh timeout window, so a
// torrent that never completes could be watched indefinitely across restarts.
func TestMonitorTorrentCompletion_ResumedMonitorInheritsDeadline(t *testing.T) {
	t.Setenv("MONITOR_TIMEOUT", "1h")

	repo := repomocks.NewRepository(t)
	// Expired on arrival: started well beyond the timeout, so the monitor gives
	// up on its first pass without ever polling qBittorrent.
	repo.On("GetNotification", mock.Anything, "999").Return(nil, nil)
	repo.On("InsertDownloadHistory", mock.Anything, "Some Show", 1, 2, 0, "", "failure", "monitor timeout").Return(nil)
	repo.On("DeleteActiveTorrent", mock.Anything, "stale-hash").Return(nil)

	q := newResumeTestHandler(t, repo)
	match := &models.TorrentMatch{
		Strategy: &models.SearchStrategy{
			MediaName:     "Some Show",
			Season:        1,
			Episode:       2,
			EpisodeTvdbID: "999",
		},
		Torrent: &prowlarr.Search{Title: "Some.Show.S01E02", InfoHash: "stale-hash"},
	}

	var wg sync.WaitGroup
	done := make(chan models.TorrentCompleteEvent, 1)
	wg.Add(1)

	finished := make(chan struct{})
	go func() {
		q.monitorTorrentCompletion(context.Background(), match, "stale-hash", "uuid-1",
			time.Now().Add(-2*time.Hour), done, &wg)
		close(finished)
	}()

	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("resumed monitor should have timed out immediately, not started polling")
	}

	assert.Empty(t, done, "an expired monitor must not emit a completion event")
	repo.AssertExpectations(t)
}

// A resumed monitor measures its deadline from the original start; a zero value
// means "starting now" and must not be read as the zero time, which would expire
// the monitor on its first pass.
func TestEffectiveStart(t *testing.T) {
	original := time.Date(2026, 7, 26, 20, 0, 0, 0, time.UTC)
	assert.True(t, effectiveStart(original).Equal(original), "a resumed monitor keeps its original start")

	now := effectiveStart(time.Time{})
	assert.False(t, now.IsZero(), "a zero start must resolve to now, not the zero time")
	assert.WithinDuration(t, time.Now(), now, time.Minute)
}

// Guards the status the monitor writes when it gives up, since that is what the
// UI shows for a download that will never land.
func TestMonitorTorrentCompletion_TimeoutMarksNotificationFailed(t *testing.T) {
	t.Setenv("MONITOR_TIMEOUT", "1s")

	notification := &notifications.Notification{ID: 7, TvdbID: "999"}

	repo := repomocks.NewRepository(t)
	repo.On("GetNotification", mock.Anything, "999").Return(notification, nil)
	repo.On("UpdateNotification", mock.Anything, mock.MatchedBy(func(n *notifications.Notification) bool {
		return n.Status == notifications.StatusFailed
	})).Return(nil)
	repo.On("InsertDownloadHistory", mock.Anything, "Some Show", 0, 0, 0, "", "failure", "monitor timeout").Return(nil)
	repo.On("DeleteActiveTorrent", mock.Anything, "stale-hash").Return(nil)

	q := newResumeTestHandler(t, repo)
	match := &models.TorrentMatch{
		Strategy: &models.SearchStrategy{MediaName: "Some Show", EpisodeTvdbID: "999"},
		Torrent:  &prowlarr.Search{Title: "Some.Show", InfoHash: "stale-hash"},
	}

	var wg sync.WaitGroup
	done := make(chan models.TorrentCompleteEvent, 1)
	wg.Add(1)
	q.monitorTorrentCompletion(context.Background(), match, "stale-hash", "uuid-1",
		time.Now().Add(-2*time.Hour), done, &wg)

	repo.AssertExpectations(t)
}
