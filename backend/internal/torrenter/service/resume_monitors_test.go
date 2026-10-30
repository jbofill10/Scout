package service

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	repomocks "github.com/jbofill10/scout/backend/internal/torrenter/repository/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
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
