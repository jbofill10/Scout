package interactors

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jbofill10/scout/backend/internal/webserver/repository"
	repoMocks "github.com/jbofill10/scout/backend/internal/webserver/repository/mocks"
	"github.com/jbofill10/scout/backend/pkg/notifications"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func notif(tvdbID string, status notifications.NotificationStatus, updated time.Time) notifications.Notification {
	return notifications.Notification{
		TvdbID:     tvdbID,
		MediaTitle: "Test",
		Category:   "series",
		Status:     status,
		UpdatedAt:  updated,
	}
}

func TestGetActivity_SplitsInFlightFromFinished(t *testing.T) {
	now := time.Now()
	notifRepo := repoMocks.NewNotificationRepository(t)
	notifRepo.On("GetNotifications", mock.Anything, 200, "", "").Return([]notifications.Notification{
		notif("1", notifications.StatusSearching, now.Add(-1*time.Minute)),
		notif("2", notifications.StatusDownloading, now),
		notif("3", notifications.StatusScheduled, now.Add(-5*time.Minute)),
		notif("4", notifications.StatusCompleted, now.Add(-2*time.Minute)),
		notif("5", notifications.StatusFailed, now.Add(-10*time.Minute)),
	}, nil)

	a := NewActivityInteractor(notifRepo, &fakeSchedulerRepo{}, testLogger())

	snapshot, err := a.GetActivity(context.Background(), 200)
	require.NoError(t, err)

	require.Len(t, snapshot.Active, 3)
	require.Len(t, snapshot.Recent, 2)

	// Both lists are ordered most-recently-updated first.
	assert.Equal(t, "2", snapshot.Active[0].TvdbID)
	assert.Equal(t, "1", snapshot.Active[1].TvdbID)
	assert.Equal(t, "3", snapshot.Active[2].TvdbID)
	assert.Equal(t, "4", snapshot.Recent[0].TvdbID)

	assert.Equal(t, map[string]int{
		"searching": 1, "downloading": 1, "scheduled": 1, "completed": 1, "failed": 1,
	}, snapshot.Counts)
}

func TestGetActivity_EnrichesWithRetryState(t *testing.T) {
	now := time.Now()
	next := now.Add(15 * time.Minute)
	release := now.Add(-2 * time.Hour)

	notifRepo := repoMocks.NewNotificationRepository(t)
	notifRepo.On("GetNotifications", mock.Anything, 50, "", "").Return([]notifications.Notification{
		notif("1", notifications.StatusSearching, now),
		notif("2", notifications.StatusSearching, now),
	}, nil)

	repo := &fakeSchedulerRepo{pendingMeta: map[string]repository.ScheduleMeta{
		"1": {
			Attempts:          2,
			NextAttemptAt:     &next,
			ReleaseTime:       release,
			ScheduleStatus:    repository.StatusPending,
			LastFailureCode:   "no_torrent_found",
			LastFailureReason: "No torrent found yet",
		},
	}}

	a := NewActivityInteractor(notifRepo, repo, testLogger())

	snapshot, err := a.GetActivity(context.Background(), 50)
	require.NoError(t, err)
	require.Len(t, snapshot.Active, 2)

	byID := map[string]ActivityItem{}
	for _, item := range snapshot.Active {
		byID[item.TvdbID] = item
	}

	retried := byID["1"]
	assert.Equal(t, 2, retried.Attempts)
	require.NotNil(t, retried.NextAttemptAt)
	assert.WithinDuration(t, next, *retried.NextAttemptAt, time.Second)
	require.NotNil(t, retried.ReleaseTime)
	assert.Equal(t, "no_torrent_found", retried.LastFailureCode)

	// An item with no scheduled row carries no retry metadata.
	plain := byID["2"]
	assert.Zero(t, plain.Attempts)
	assert.Nil(t, plain.NextAttemptAt)
	assert.Nil(t, plain.ReleaseTime)
}

func TestGetActivity_SurvivesScheduleMetadataFailure(t *testing.T) {
	notifRepo := repoMocks.NewNotificationRepository(t)
	notifRepo.On("GetNotifications", mock.Anything, 10, "", "").Return([]notifications.Notification{
		notif("1", notifications.StatusSearching, time.Now()),
	}, nil)

	repo := &fakeSchedulerRepo{pendingMetaErr: errors.New("db down")}
	a := NewActivityInteractor(notifRepo, repo, testLogger())

	snapshot, err := a.GetActivity(context.Background(), 10)
	require.NoError(t, err, "retry metadata is best-effort and must not fail the whole view")
	require.Len(t, snapshot.Active, 1)
	assert.Zero(t, snapshot.Active[0].Attempts)
}

func TestGetActivity_PropagatesNotificationFailure(t *testing.T) {
	notifRepo := repoMocks.NewNotificationRepository(t)
	notifRepo.On("GetNotifications", mock.Anything, 10, "", "").
		Return([]notifications.Notification(nil), errors.New("db down"))

	a := NewActivityInteractor(notifRepo, &fakeSchedulerRepo{}, testLogger())

	_, err := a.GetActivity(context.Background(), 10)
	assert.Error(t, err)
}

func TestClearFinished_KeepsFailuresWithPendingRetries(t *testing.T) {
	notifRepo := repoMocks.NewNotificationRepository(t)
	// The retrying ids reach the repository sorted, so the query is deterministic.
	notifRepo.On("DismissFinished", mock.Anything, []string{"1", "2"}).Return(int64(4), nil)
	sched := &fakeSchedulerRepo{pendingMeta: map[string]repository.ScheduleMeta{
		"2": {ScheduleStatus: repository.StatusPending},
		"1": {ScheduleStatus: repository.StatusPending},
	}}

	a := NewActivityInteractor(notifRepo, sched, testLogger())

	count, err := a.ClearFinished(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(4), count)
}

func TestClearFinished_NoPendingRetries(t *testing.T) {
	notifRepo := repoMocks.NewNotificationRepository(t)
	notifRepo.On("DismissFinished", mock.Anything, []string{}).Return(int64(2), nil)

	a := NewActivityInteractor(notifRepo, &fakeSchedulerRepo{}, testLogger())

	count, err := a.ClearFinished(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)
}

func TestClearFinished_AbortsWhenScheduleLookupFails(t *testing.T) {
	// No DismissFinished expectation: the mock fails the test if it is called.
	notifRepo := repoMocks.NewNotificationRepository(t)
	sched := &fakeSchedulerRepo{pendingMetaErr: errors.New("db down")}

	a := NewActivityInteractor(notifRepo, sched, testLogger())

	_, err := a.ClearFinished(context.Background())
	require.Error(t, err)
}

func TestGetActivity_ReportsRetryTimeOnlyForPendingRows(t *testing.T) {
	now := time.Now()
	stale := now.Add(-6 * time.Hour)
	next := now.Add(time.Hour)

	notifRepo := repoMocks.NewNotificationRepository(t)
	notifRepo.On("GetNotifications", mock.Anything, 50, "", "").Return([]notifications.Notification{
		notif("done", notifications.StatusCompleted, now),
		notif("gone", notifications.StatusFailed, now),
		notif("sent", notifications.StatusSearching, now),
		notif("wait", notifications.StatusSearching, now),
	}, nil)

	// Every row kept the time of the attempt that moved it on; only the
	// pending one is still waiting for it.
	repo := &fakeSchedulerRepo{pendingMeta: map[string]repository.ScheduleMeta{
		"done": {Attempts: 3, NextAttemptAt: &stale, ScheduleStatus: repository.StatusCompleted},
		"gone": {Attempts: 5, NextAttemptAt: &stale, ScheduleStatus: repository.StatusFailed},
		"sent": {Attempts: 2, NextAttemptAt: &stale, ScheduleStatus: repository.StatusQueued},
		"wait": {Attempts: 2, NextAttemptAt: &next, ScheduleStatus: repository.StatusPending},
	}}

	a := NewActivityInteractor(notifRepo, repo, testLogger())

	snapshot, err := a.GetActivity(context.Background(), 50)
	require.NoError(t, err)

	byID := map[string]ActivityItem{}
	for _, item := range append(snapshot.Active, snapshot.Recent...) {
		byID[item.TvdbID] = item
	}

	for _, id := range []string{"done", "gone", "sent"} {
		assert.Nil(t, byID[id].NextAttemptAt, "%s is not waiting for a retry", id)
		assert.NotZero(t, byID[id].Attempts, "%s keeps its attempt history", id)
	}
	// Retry stays on offer for a failed item (fresh budget), not for one that
	// completed or is being dispatched.
	assert.False(t, byID["done"].CanRetry)
	assert.False(t, byID["sent"].CanRetry)
	assert.True(t, byID["gone"].CanRetry)

	require.NotNil(t, byID["wait"].NextAttemptAt)
	assert.WithinDuration(t, next, *byID["wait"].NextAttemptAt, time.Second)
	assert.True(t, byID["wait"].CanRetry)
}
