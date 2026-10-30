package interactors

import (
	"context"
	"testing"
	"time"

	"github.com/jbofill10/scout/backend/internal/webserver/repository"
	repoMocks "github.com/jbofill10/scout/backend/internal/webserver/repository/mocks"
	"github.com/jbofill10/scout/backend/pkg/dlstatus"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/notifications"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func lostNotification(id, category string, code dlstatus.FailureCode) notifications.Notification {
	return notifications.Notification{ID: 1836, TvdbID: id, MediaTitle: "X", Category: category,
		Status: notifications.StatusFailed, Reason: code.HumanReason(), FailureCode: string(code)}
}

func completedRow(id, attempts int) repository.ScheduledRow {
	return repository.ScheduledRow{ID: id, ItemKey: "154561", ScheduleStatus: repository.StatusCompleted, Attempts: attempts,
		Media: tvdb.Media{Id: "154561", Name: "X", Category: "movie"}}
}

func newReconcileInteractor(repo *fakeSchedulerRepo, notifs *repoMocks.NotificationRepository) *DownloadInteractor {
	return &DownloadInteractor{repo: repo, notificationRepo: notifs, logger: testLogger(), dispatch: func(f func()) { f() }}
}

// expectSearching is the notification update the reconciler makes when it
// puts an item back on the ladder.
func expectSearching(t *testing.T, notifs *repoMocks.NotificationRepository, n notifications.Notification) *notifications.Notification {
	t.Helper()
	current := n
	notifs.On("GetNotification", mock.Anything, n.TvdbID).Return(&current, nil)
	var written *notifications.Notification
	notifs.On("UpdateNotification", mock.Anything, mock.MatchedBy(func(u *notifications.Notification) bool {
		written = u
		return true
	})).Return(nil)
	_ = written
	return &current
}

// The X case: a movie the torrenter accepted, then lost after six hours. The
// completed row it left is re-armed one hour out and the notification says so.
func TestReconcileAsyncFailures_RearmsCompletedRow(t *testing.T) {
	n := lostNotification("154561", "movie", dlstatus.CodeMonitorLost)
	repo := &fakeSchedulerRepo{rows: []repository.ScheduledRow{completedRow(40, 0)}}
	notifs := repoMocks.NewNotificationRepository(t)
	notifs.On("GetNotifications", mock.Anything, asyncReconcileBatch, "", "failed").Return([]notifications.Notification{n}, nil)
	current := expectSearching(t, notifs, n)

	acted, err := newReconcileInteractor(repo, notifs).ReconcileAsyncFailures(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 1, acted)
	assert.Equal(t, []int{40}, repo.rearmedComplete)
	assert.Equal(t, "monitor_lost", repo.rearmedCode)
	assert.WithinDuration(t, time.Now().Add(time.Hour), repo.rearmedNext, time.Minute, "first retry follows the 1h step")
	assert.Equal(t, notifications.StatusSearching, current.Status)
	assert.Contains(t, current.Reason, "attempt 1 of 5")
	assert.Empty(t, current.FailureCode, "a notification back in flight carries no failure code")
	assert.Empty(t, repo.failedComplete)
}

// With two completed rows (an old canonical one and a later re-request) the
// newest carries the payload to retry from; attempts come from that row.
func TestReconcileAsyncFailures_UsesNewestCompletedRowAndItsBackoff(t *testing.T) {
	n := lostNotification("154561", "movie", dlstatus.CodeMonitorLost)
	repo := &fakeSchedulerRepo{rows: []repository.ScheduledRow{completedRow(40, 0), completedRow(41, 2)}}
	notifs := repoMocks.NewNotificationRepository(t)
	notifs.On("GetNotifications", mock.Anything, asyncReconcileBatch, "", "failed").Return([]notifications.Notification{n}, nil)
	expectSearching(t, notifs, n)

	_, err := newReconcileInteractor(repo, notifs).ReconcileAsyncFailures(context.Background())

	require.NoError(t, err)
	assert.Equal(t, []int{41}, repo.rearmedComplete)
	assert.WithinDuration(t, time.Now().Add(12*time.Hour), repo.rearmedNext, time.Minute, "third failure waits 12h")
}

func TestReconcileAsyncFailures_ExhaustedLadderFailsForGood(t *testing.T) {
	n := lostNotification("154561", "movie", dlstatus.CodeMonitorLost)
	repo := &fakeSchedulerRepo{rows: []repository.ScheduledRow{completedRow(40, 4)}}
	notifs := repoMocks.NewNotificationRepository(t)
	notifs.On("GetNotifications", mock.Anything, asyncReconcileBatch, "", "failed").Return([]notifications.Notification{n}, nil)
	current := expectSearching(t, notifs, n)

	acted, err := newReconcileInteractor(repo, notifs).ReconcileAsyncFailures(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 1, acted)
	assert.Equal(t, []int{40}, repo.failedComplete)
	assert.Equal(t, "max_retries_exceeded", repo.failedCode)
	assert.Empty(t, repo.rearmedComplete)
	assert.Equal(t, notifications.StatusFailed, current.Status)
	assert.Equal(t, "max_retries_exceeded", current.FailureCode, "a permanent code stops the reconciler from looking again")
	assert.Contains(t, current.Reason, "Gave up after 5 attempts")
	assert.Equal(t, 1, repo.historyCount)
}

// A lost movie requested before dispatched rows were recorded has no row at
// all; its id and name are enough to build one.
func TestReconcileAsyncFailures_RebuildsRowForMovieWithoutOne(t *testing.T) {
	n := lostNotification("154561", "movie", dlstatus.CodeMonitorLost)
	repo := &fakeSchedulerRepo{}
	notifs := repoMocks.NewNotificationRepository(t)
	notifs.On("GetNotifications", mock.Anything, asyncReconcileBatch, "", "failed").Return([]notifications.Notification{n}, nil)
	expectSearching(t, notifs, n)

	acted, err := newReconcileInteractor(repo, notifs).ReconcileAsyncFailures(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 1, acted)
	require.Len(t, repo.retryMedia, 1)
	assert.Equal(t, "154561", repo.retryMedia[0].Id)
	assert.Equal(t, "movie", repo.retryMedia[0].Category)
}

func TestReconcileAsyncFailures_LeavesOthersAlone(t *testing.T) {
	cases := map[string]struct {
		n    notifications.Notification
		rows []repository.ScheduledRow
	}{
		"already pending":    {lostNotification("154561", "movie", dlstatus.CodeMonitorLost), []repository.ScheduledRow{completedRow(40, 0), {ID: 41, ScheduleStatus: repository.StatusPending}}},
		"permanent code":     {lostNotification("154561", "movie", dlstatus.CodeProcessingError), []repository.ScheduledRow{completedRow(40, 0)}},
		"no code (legacy)":   {lostNotification("154561", "movie", ""), []repository.ScheduledRow{completedRow(40, 0)}},
		"episode, no row":    {lostNotification("999", "series", dlstatus.CodeMonitorLost), nil},
		"user gave up on it": {lostNotification("154561", "movie", dlstatus.CodeCancelled), []repository.ScheduledRow{completedRow(40, 0)}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			repo := &fakeSchedulerRepo{rows: tc.rows}
			notifs := repoMocks.NewNotificationRepository(t)
			notifs.On("GetNotifications", mock.Anything, asyncReconcileBatch, "", "failed").Return([]notifications.Notification{tc.n}, nil)

			acted, err := newReconcileInteractor(repo, notifs).ReconcileAsyncFailures(context.Background())

			require.NoError(t, err)
			assert.Equal(t, 0, acted)
			assert.Empty(t, repo.rearmedComplete)
			assert.Empty(t, repo.failedComplete)
			assert.Empty(t, repo.retryMedia)
		})
	}
}

// Every accepted direct request leaves a completed row behind, so a loss
// reported later has something to retry from.
func TestDispatchMovieDownload_RecordsDispatchedRow(t *testing.T) {
	repo := &fakeSchedulerRepo{}
	notifs := repoMocks.NewNotificationRepository(t)
	disp := &fakeDispatcher{resp: dlstatus.DownloadResponse{Results: []dlstatus.EpisodeResult{{TvdbID: "154561", Outcome: dlstatus.OutcomeDownloading}}}}
	i := &DownloadInteractor{repo: repo, notificationRepo: notifs, logger: testLogger(), torrenterClient: disp, dispatch: func(f func()) { f() }}

	i.dispatchMovieDownload(context.Background(), tvdb.Media{Id: "154561", Name: "X", Category: "movie"})

	require.Len(t, repo.dispatched, 1)
	assert.Equal(t, "154561", repo.dispatched[0].Id)
	assert.Equal(t, 0, repo.retryCount, "a download that started is not a failure")
}
