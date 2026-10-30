package interactors

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jbofill10/scout/backend/internal/webserver/clients"
	"github.com/jbofill10/scout/backend/internal/webserver/repository"
	repoMocks "github.com/jbofill10/scout/backend/internal/webserver/repository/mocks"
	"github.com/jbofill10/scout/backend/pkg/controls"
	"github.com/jbofill10/scout/backend/pkg/dlstatus"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/notifications"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type fakeControlRepo struct {
	rows      []repository.ScheduledRow
	cancelled []int
	rearmed   map[int]bool
	completed []int
	upcoming  []repository.UpcomingItem
}

func (f *fakeControlRepo) FindRowsByItem(context.Context, string) ([]repository.ScheduledRow, error) {
	return f.rows, nil
}
func (f *fakeControlRepo) CancelRow(_ context.Context, id int) error {
	f.cancelled = append(f.cancelled, id)
	return nil
}
func (f *fakeControlRepo) RearmRow(_ context.Context, id int, reset bool) error {
	if f.rearmed == nil {
		f.rearmed = map[int]bool{}
	}
	f.rearmed[id] = reset
	return nil
}
func (f *fakeControlRepo) MarkCompleted(_ context.Context, id int) error {
	f.completed = append(f.completed, id)
	return nil
}
func (f *fakeControlRepo) GetUpcoming(context.Context) ([]repository.UpcomingItem, error) {
	return f.upcoming, nil
}

type fakeTimers struct {
	armed     map[int]bool
	cancelled []int
	kicks     int
}

func (f *fakeTimers) CancelTimer(id int) bool {
	f.cancelled = append(f.cancelled, id)
	return f.armed[id]
}
func (f *fakeTimers) Kick() { f.kicks++ }

type fakeControlTorrenter struct {
	cancelResp controls.CancelResponse
	cancelErr  error
	releases   controls.ReleasesResponse
	releaseErr error
	grabResult dlstatus.EpisodeResult
	grabErr    error
	gotMedia   *tvdb.Media
	gotSel     controls.ReleaseSelection
}

func (f *fakeControlTorrenter) Cancel(context.Context, string) (controls.CancelResponse, error) {
	return f.cancelResp, f.cancelErr
}
func (f *fakeControlTorrenter) ListReleases(_ context.Context, _ string, media *tvdb.Media) (controls.ReleasesResponse, error) {
	f.gotMedia = media
	return f.releases, f.releaseErr
}
func (f *fakeControlTorrenter) ManualDownload(_ context.Context, _ string, media *tvdb.Media, sel controls.ReleaseSelection) (dlstatus.EpisodeResult, error) {
	f.gotMedia = media
	f.gotSel = sel
	return f.grabResult, f.grabErr
}

func row(id int, status string) repository.ScheduledRow {
	return repository.ScheduledRow{
		ID:             id,
		ItemKey:        "55",
		ScheduleStatus: status,
		Media:          tvdb.Media{Id: "900", Name: "Some Show", Category: "series", Metadata: tvdb.TVDBSeriesMetadata{Episodes: []tvdb.Episode{{Id: 55}}}},
	}
}

func TestControlCancel_StopsRowsTimersAndTorrenter(t *testing.T) {
	repo := &fakeControlRepo{rows: []repository.ScheduledRow{row(1, repository.StatusPending), row(2, repository.StatusFailed)}}
	timers := &fakeTimers{armed: map[int]bool{1: true}}
	torrenter := &fakeControlTorrenter{cancelResp: controls.CancelResponse{TorrentsRemoved: 1, NotificationUpdated: true}}
	notifs := repoMocks.NewNotificationRepository(t)

	c := NewDownloadControlInteractor(repo, notifs, timers, torrenter, testLogger())
	out, err := c.Cancel(context.Background(), "55")

	require.NoError(t, err)
	assert.Equal(t, []int{1, 2}, repo.cancelled)
	assert.Equal(t, 2, out.RowsCancelled)
	assert.Equal(t, 1, out.TimersCancelled)
	assert.Equal(t, 1, out.TorrentsRemoved)
	assert.True(t, out.NotificationUpdated, "the torrenter already marked it, so the notification repo is not touched")
}

func TestControlCancel_MarksNotificationWhenTorrenterHadNothing(t *testing.T) {
	repo := &fakeControlRepo{}
	torrenter := &fakeControlTorrenter{cancelErr: errors.New("torrenter down")}
	notifs := repoMocks.NewNotificationRepository(t)
	notifs.On("GetNotification", mock.Anything, "55").Return(&notifications.Notification{ID: 3, TvdbID: "55", Status: notifications.StatusScheduled}, nil)
	notifs.On("UpdateNotification", mock.Anything, mock.MatchedBy(func(n *notifications.Notification) bool {
		return n.ID == 3 && n.Status == notifications.StatusCancelled
	})).Return(nil)

	c := NewDownloadControlInteractor(repo, notifs, &fakeTimers{}, torrenter, testLogger())
	out, err := c.Cancel(context.Background(), "55")

	require.NoError(t, err, "an unreachable torrenter does not block the cancel")
	assert.True(t, out.NotificationUpdated)
}

func TestControlCancel_NothingAnywhere(t *testing.T) {
	notifs := repoMocks.NewNotificationRepository(t)
	notifs.On("GetNotification", mock.Anything, "55").Return(nil, nil)
	c := NewDownloadControlInteractor(&fakeControlRepo{}, notifs, &fakeTimers{}, &fakeControlTorrenter{}, testLogger())

	_, err := c.Cancel(context.Background(), "55")

	assert.ErrorIs(t, err, ErrNothingToControl)
}

func TestControlRetryNow(t *testing.T) {
	t.Run("no row", func(t *testing.T) {
		c := NewDownloadControlInteractor(&fakeControlRepo{}, repoMocks.NewNotificationRepository(t), &fakeTimers{}, &fakeControlTorrenter{}, testLogger())
		_, err := c.RetryNow(context.Background(), "55")
		assert.ErrorIs(t, err, ErrNothingToControl)
	})

	t.Run("dispatch in progress", func(t *testing.T) {
		repo := &fakeControlRepo{rows: []repository.ScheduledRow{row(1, repository.StatusQueued)}}
		c := NewDownloadControlInteractor(repo, repoMocks.NewNotificationRepository(t), &fakeTimers{}, &fakeControlTorrenter{}, testLogger())
		_, err := c.RetryNow(context.Background(), "55")
		assert.ErrorIs(t, err, ErrDispatchInProgress)
		assert.Nil(t, repo.rearmed)
	})

	t.Run("re-arms, resets attempts for failed rows, kicks the scheduler", func(t *testing.T) {
		repo := &fakeControlRepo{rows: []repository.ScheduledRow{row(1, repository.StatusPending), row(2, repository.StatusFailed)}}
		timers := &fakeTimers{armed: map[int]bool{1: true}}
		notifs := repoMocks.NewNotificationRepository(t)
		notifs.On("GetNotification", mock.Anything, "55").Return(&notifications.Notification{ID: 3, TvdbID: "55", Status: notifications.StatusFailed}, nil)
		notifs.On("UpdateNotification", mock.Anything, mock.MatchedBy(func(n *notifications.Notification) bool {
			return n.Status == notifications.StatusSearching && n.Reason == "Retrying now"
		})).Return(nil)

		c := NewDownloadControlInteractor(repo, notifs, timers, &fakeControlTorrenter{}, testLogger())
		out, err := c.RetryNow(context.Background(), "55")

		require.NoError(t, err)
		assert.Equal(t, 2, out.Rearmed)
		assert.Equal(t, map[int]bool{1: false, 2: true}, repo.rearmed, "only the failed row gets a fresh attempt budget")
		assert.Equal(t, []int{1, 2}, timers.cancelled)
		assert.Equal(t, 1, timers.kicks)
	})

	t.Run("only completed rows: the newest is re-armed with a fresh budget", func(t *testing.T) {
		repo := &fakeControlRepo{rows: []repository.ScheduledRow{row(1, repository.StatusCompleted), row(2, repository.StatusCompleted)}}
		notifs := repoMocks.NewNotificationRepository(t)
		notifs.On("GetNotification", mock.Anything, "55").Return(&notifications.Notification{ID: 3, TvdbID: "55", Status: notifications.StatusFailed}, nil)
		notifs.On("UpdateNotification", mock.Anything, mock.Anything).Return(nil)

		c := NewDownloadControlInteractor(repo, notifs, &fakeTimers{}, &fakeControlTorrenter{}, testLogger())
		out, err := c.RetryNow(context.Background(), "55")

		require.NoError(t, err)
		assert.Equal(t, 1, out.Rearmed)
		assert.Equal(t, map[int]bool{2: true}, repo.rearmed, "history rows other than the newest stay as they are")
	})

	t.Run("live rows win over completed ones", func(t *testing.T) {
		repo := &fakeControlRepo{rows: []repository.ScheduledRow{row(1, repository.StatusCompleted), row(2, repository.StatusFailed)}}
		notifs := repoMocks.NewNotificationRepository(t)
		notifs.On("GetNotification", mock.Anything, "55").Return(&notifications.Notification{ID: 3, TvdbID: "55"}, nil)
		notifs.On("UpdateNotification", mock.Anything, mock.Anything).Return(nil)

		c := NewDownloadControlInteractor(repo, notifs, &fakeTimers{}, &fakeControlTorrenter{}, testLogger())
		out, err := c.RetryNow(context.Background(), "55")

		require.NoError(t, err)
		assert.Equal(t, 1, out.Rearmed)
		assert.Equal(t, map[int]bool{2: true}, repo.rearmed)
	})
}

// Completed rows are history: cancelling an item never touches them.
func TestControlCancel_SkipsCompletedRows(t *testing.T) {
	repo := &fakeControlRepo{rows: []repository.ScheduledRow{row(1, repository.StatusCompleted), row(2, repository.StatusPending)}}
	notifs := repoMocks.NewNotificationRepository(t)
	notifs.On("GetNotification", mock.Anything, "55").Return(&notifications.Notification{ID: 3, TvdbID: "55"}, nil).Maybe()
	notifs.On("UpdateNotification", mock.Anything, mock.Anything).Return(nil).Maybe()

	c := NewDownloadControlInteractor(repo, notifs, &fakeTimers{}, &fakeControlTorrenter{}, testLogger())
	out, err := c.Cancel(context.Background(), "55")

	require.NoError(t, err)
	assert.Equal(t, []int{2}, repo.cancelled)
	assert.Equal(t, 1, out.RowsCancelled)
}

func TestControlGrab(t *testing.T) {
	sel := controls.ReleaseSelection{Title: "Some.Show.S01E05.1080p-GRP", GUID: "magnet:?x"}

	t.Run("a started download settles the rows", func(t *testing.T) {
		repo := &fakeControlRepo{rows: []repository.ScheduledRow{row(5, repository.StatusPending)}}
		timers := &fakeTimers{armed: map[int]bool{5: true}}
		torrenter := &fakeControlTorrenter{grabResult: dlstatus.EpisodeResult{TvdbID: "55", Outcome: dlstatus.OutcomeDownloading}}

		c := NewDownloadControlInteractor(repo, repoMocks.NewNotificationRepository(t), timers, torrenter, testLogger())
		result, err := c.Grab(context.Background(), "55", sel)

		require.NoError(t, err)
		assert.Equal(t, dlstatus.OutcomeDownloading, result.Outcome)
		require.NotNil(t, torrenter.gotMedia)
		assert.Equal(t, "Some Show", torrenter.gotMedia.Name, "the row's media is handed to the torrenter")
		assert.Equal(t, sel, torrenter.gotSel)
		assert.Equal(t, []int{5}, repo.completed)
		assert.Equal(t, []int{5}, timers.cancelled)
	})

	t.Run("a failed add leaves the rows alone", func(t *testing.T) {
		repo := &fakeControlRepo{rows: []repository.ScheduledRow{row(5, repository.StatusPending)}}
		torrenter := &fakeControlTorrenter{grabResult: dlstatus.EpisodeResult{Outcome: dlstatus.OutcomeFailed, Code: dlstatus.CodeTorrentClientError}}

		c := NewDownloadControlInteractor(repo, repoMocks.NewNotificationRepository(t), &fakeTimers{}, torrenter, testLogger())
		_, err := c.Grab(context.Background(), "55", sel)

		require.NoError(t, err)
		assert.Empty(t, repo.completed)
	})

	t.Run("unknown everywhere", func(t *testing.T) {
		torrenter := &fakeControlTorrenter{grabErr: clients.ErrItemUnknown}
		c := NewDownloadControlInteractor(&fakeControlRepo{}, repoMocks.NewNotificationRepository(t), &fakeTimers{}, torrenter, testLogger())
		_, err := c.Grab(context.Background(), "55", sel)
		assert.ErrorIs(t, err, ErrNothingToControl)
		assert.Nil(t, torrenter.gotMedia, "no row means no media; the torrenter resolves it or says unknown")
	})
}

func TestControlListReleases_PassesRowMedia(t *testing.T) {
	repo := &fakeControlRepo{rows: []repository.ScheduledRow{row(5, repository.StatusPending)}}
	torrenter := &fakeControlTorrenter{releases: controls.ReleasesResponse{ItemID: "55", Queries: 3}}
	c := NewDownloadControlInteractor(repo, repoMocks.NewNotificationRepository(t), &fakeTimers{}, torrenter, testLogger())

	resp, err := c.ListReleases(context.Background(), "55")

	require.NoError(t, err)
	assert.Equal(t, 3, resp.Queries)
	require.NotNil(t, torrenter.gotMedia)
	assert.Equal(t, "900", torrenter.gotMedia.Id)
}

func TestGetActivity_ActionFlags(t *testing.T) {
	now := time.Now()
	next := now.Add(time.Hour)
	notifRepo := repoMocks.NewNotificationRepository(t)
	notifRepo.On("GetNotifications", mock.Anything, 50, "", "").Return([]notifications.Notification{
		notif("retrying", notifications.StatusSearching, now),
		notif("downloading", notifications.StatusDownloading, now),
		notif("failed-with-row", notifications.StatusFailed, now),
		notif("failed-no-row", notifications.StatusFailed, now),
		notif("done", notifications.StatusCompleted, now),
		notif("waiting", notifications.StatusScheduled, now),
	}, nil)
	repo := &fakeSchedulerRepo{pendingMeta: map[string]repository.ScheduleMeta{
		"retrying":        {ScheduleStatus: repository.StatusPending, NextAttemptAt: &next, Attempts: 1},
		"failed-with-row": {ScheduleStatus: repository.StatusFailed, Attempts: 5},
		"waiting":         {ScheduleStatus: repository.StatusPending},
	}}

	a := NewActivityInteractor(notifRepo, repo, testLogger())
	snapshot, err := a.GetActivity(context.Background(), 50)
	require.NoError(t, err)

	byID := map[string]ActivityItem{}
	for _, item := range append(snapshot.Active, snapshot.Recent...) {
		byID[item.TvdbID] = item
	}

	assert.True(t, byID["retrying"].CanCancel)
	assert.True(t, byID["retrying"].CanRetry, "a retry wait can be skipped")
	assert.True(t, byID["retrying"].CanPickRelease)

	assert.True(t, byID["downloading"].CanCancel)
	assert.False(t, byID["downloading"].CanRetry)
	assert.True(t, byID["downloading"].CanPickRelease, "the torrenter knows the item from its active torrent")

	assert.False(t, byID["failed-with-row"].CanCancel)
	assert.True(t, byID["failed-with-row"].CanRetry)
	assert.Equal(t, 5, byID["failed-with-row"].Attempts, "failed rows now contribute their attempt count")

	assert.False(t, byID["failed-no-row"].CanRetry, "nothing to re-dispatch without a row")
	assert.False(t, byID["failed-no-row"].CanPickRelease)

	assert.False(t, byID["done"].CanCancel)
	assert.False(t, byID["done"].CanRetry)

	assert.True(t, byID["waiting"].CanCancel)
	assert.False(t, byID["waiting"].CanRetry, "a release that has not aired has nothing to retry")
}
