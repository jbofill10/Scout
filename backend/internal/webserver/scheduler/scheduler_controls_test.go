package scheduler

import (
	"bytes"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jbofill10/scout/backend/internal/webserver/repository"
	schedMocks "github.com/jbofill10/scout/backend/internal/webserver/scheduler/mocks"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func quietScheduler(t *testing.T, repo *schedMocks.SchedulerRepository) *Scheduler {
	t.Helper()
	return NewScheduler(repo, slog.New(slog.NewTextHandler(new(bytes.Buffer), nil)))
}

func futureItem(id int, in time.Duration) repository.DueItem {
	due := time.Now().Add(in)
	return repository.DueItem{
		ID:          id,
		Media:       tvdb.Media{Id: "900", Name: "Future Show", Metadata: tvdb.TVDBSeriesMetadata{Episodes: []tvdb.Episode{{Id: id, Aired: "2030-01-01"}}}},
		ReleaseTime: due,
		DueAt:       due,
	}
}

// Cancelling an armed timer stops it from dispatching and frees the row's
// dedupe slot, so a later poll can arm the row again if it is revived.
func TestCancelTimer_StopsArmedTimerAndFreesSlot(t *testing.T) {
	repo := schedMocks.NewSchedulerRepository(t)
	repo.On("ResetStaleQueued", mock.Anything, mock.Anything).Return(int64(0), nil)
	repo.On("GetDueMedia", mock.Anything, mock.Anything).Return([]repository.DueItem{futureItem(7, 200*time.Millisecond)}, nil)

	s := quietScheduler(t, repo)
	queue := make(chan repository.DueItem, 1)
	s.Start(queue)
	defer s.Stop()

	require.Eventually(t, func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		_, ok := s.timers[7]
		return ok
	}, time.Second, 5*time.Millisecond, "the poll arms a timer for the future row")

	assert.True(t, s.CancelTimer(7), "an armed timer reports cancelled")
	assert.False(t, s.CancelTimer(7), "a second cancel finds nothing")

	s.mu.Lock()
	assert.Empty(t, s.timers)
	assert.Empty(t, s.scheduledMap, "the dedupe slot is released")
	s.mu.Unlock()

	select {
	case item := <-queue:
		t.Fatalf("cancelled timer still dispatched %q", item.Media.Name)
	case <-time.After(400 * time.Millisecond):
	}
	repo.AssertNotCalled(t, "MarkQueued", mock.Anything, 7)
}

// Kick runs a poll right away instead of waiting for the next tick.
func TestKick_PollsImmediately(t *testing.T) {
	var polls atomic.Int32
	repo := schedMocks.NewSchedulerRepository(t)
	repo.On("ResetStaleQueued", mock.Anything, mock.Anything).Return(int64(0), nil)
	repo.On("GetDueMedia", mock.Anything, mock.Anything).Run(func(mock.Arguments) { polls.Add(1) }).
		Return([]repository.DueItem{}, nil)

	s := quietScheduler(t, repo)
	s.Start(make(chan repository.DueItem, 1))
	defer s.Stop()

	require.Eventually(t, func() bool { return polls.Load() == 1 }, time.Second, 5*time.Millisecond, "startup poll")

	s.Kick()
	require.Eventually(t, func() bool { return polls.Load() == 2 }, time.Second, 5*time.Millisecond, "kick polls without waiting for the ticker")

	assert.NotPanics(t, func() {
		s.Kick()
		s.Kick()
	}, "kicks never block, even back to back")
}
