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
