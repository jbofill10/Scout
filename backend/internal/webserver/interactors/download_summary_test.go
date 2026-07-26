package interactors

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jbofill10/scout/backend/internal/webserver/clients"
	"github.com/jbofill10/scout/backend/internal/webserver/repository"
	repoMocks "github.com/jbofill10/scout/backend/internal/webserver/repository/mocks"
	"github.com/jbofill10/scout/backend/pkg/dlstatus"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// newSummaryInteractor wires an interactor against a stub tvdb-proxy so the
// extended-info lookup succeeds. Dispatch runs inline unless the caller opts
// back into the goroutine behaviour.
func newSummaryInteractor(
	t *testing.T,
	repo *fakeSchedulerRepo,
	notifRepo repository.NotificationRepository,
	dispatcher DownloadDispatcher,
) *DownloadInteractor {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(tvdb.TVDBSeriesExtendedResponse{})
	}))
	t.Cleanup(srv.Close)

	tvdbClient := clients.NewTVDBProxyClient(strings.TrimPrefix(srv.URL, "http://"))
	i := NewDownloadInteractor(repo, notifRepo, nil, nil, testLogger(), tvdbClient, dispatcher)
	i.RunDispatchInline()
	return i
}

func aired(daysFromNow int) string {
	return time.Now().AddDate(0, 0, daysFromNow).Format("2006-01-02")
}

func TestDownloadShow_SummaryCountsEachEpisode(t *testing.T) {
	repo := &fakeSchedulerRepo{}
	notifRepo := repoMocks.NewNotificationRepository(t)
	notifRepo.On("CreateNotification", mock.Anything, mock.Anything).Return(nil)
	dispatcher := &fakeDispatcher{resp: dlstatus.DownloadResponse{
		Results: []dlstatus.EpisodeResult{epResult("1", dlstatus.OutcomeDownloading, "")},
	}}

	i := newSummaryInteractor(t, repo, notifRepo, dispatcher)

	req := tvdb.Media{Id: "555", Name: "Test Show", Category: "series"}
	req.Metadata.Episodes = []tvdb.Episode{
		{Id: 1, SeasonNumber: 1, Number: 1, Aired: aired(-30)},   // aired -> queued now
		{Id: 2, SeasonNumber: 1, Number: 2, Aired: aired(7)},     // future -> scheduled
		{Id: 3, SeasonNumber: 0, Number: 1, Aired: aired(-30)},   // special -> skipped
		{Id: 4, SeasonNumber: 1, Number: 4, Aired: "not-a-date"}, // unparseable -> skipped
	}

	summary, err := i.DownloadShow(context.Background(), req)
	require.NoError(t, err)

	assert.Equal(t, "Test Show", summary.MediaTitle)
	assert.Equal(t, "series", summary.Category)
	assert.Equal(t, 1, summary.QueuedNow)
	assert.Equal(t, 1, summary.Scheduled)
	assert.Equal(t, 2, summary.Skipped)
	require.NotNil(t, summary.NextRelease)
	assert.True(t, summary.NextRelease.After(time.Now()), "next release should be the future episode")

	// Only the aired episode is handed to the torrenter.
	assert.Equal(t, 1, dispatcher.callCount())
	assert.Len(t, dispatcher.payload().Metadata.Episodes, 1)
	assert.Equal(t, 1, dispatcher.payload().Metadata.Episodes[0].Id)
}

func TestDownloadShow_AlreadyScheduledCountsAsSkipped(t *testing.T) {
	repo := &fakeSchedulerRepo{scheduleErr: repository.ErrDuplicateScheduled}
	notifRepo := repoMocks.NewNotificationRepository(t)
	dispatcher := &fakeDispatcher{}

	i := newSummaryInteractor(t, repo, notifRepo, dispatcher)

	req := tvdb.Media{Id: "555", Name: "Test Show", Category: "series"}
	req.Metadata.Episodes = []tvdb.Episode{{Id: 2, SeasonNumber: 1, Number: 2, Aired: aired(7)}}

	summary, err := i.DownloadShow(context.Background(), req)
	require.NoError(t, err)

	assert.Equal(t, 0, summary.Scheduled)
	assert.Equal(t, 1, summary.Skipped)
	assert.Equal(t, 0, dispatcher.callCount())
}

func TestDownloadShow_ReturnsBeforeTorrenterFinishes(t *testing.T) {
	repo := &fakeSchedulerRepo{}
	notifRepo := repoMocks.NewNotificationRepository(t)
	notifRepo.On("CreateNotification", mock.Anything, mock.Anything).Return(nil)

	release := make(chan struct{})
	started := make(chan struct{})
	dispatcher := &fakeDispatcher{blockOn: release, started: started}

	i := newSummaryInteractor(t, repo, notifRepo, dispatcher)
	// Restore the production behaviour: dispatch on its own goroutine.
	i.dispatch = func(fn func()) { go fn() }

	req := tvdb.Media{Id: "555", Name: "Test Show", Category: "series"}
	req.Metadata.Episodes = []tvdb.Episode{{Id: 1, SeasonNumber: 1, Number: 1, Aired: aired(-30)}}

	done := make(chan DownloadSummary, 1)
	go func() {
		summary, err := i.DownloadShow(context.Background(), req)
		assert.NoError(t, err)
		done <- summary
	}()

	// The request must complete while the torrenter call is still blocked.
	select {
	case summary := <-done:
		assert.Equal(t, 1, summary.QueuedNow)
	case <-time.After(2 * time.Second):
		t.Fatal("DownloadShow blocked on the torrenter dispatch")
	}

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("background dispatch never reached the torrenter")
	}
	close(release)
}

func TestDownloadMovie_ReleasedQueuesImmediately(t *testing.T) {
	repo := &fakeSchedulerRepo{}
	notifRepo := repoMocks.NewNotificationRepository(t)
	notifRepo.On("CreateNotification", mock.Anything, mock.Anything).Return(nil)
	dispatcher := &fakeDispatcher{resp: dlstatus.DownloadResponse{
		Results: []dlstatus.EpisodeResult{epResult("777", dlstatus.OutcomeDownloading, "")},
	}}

	i := newSummaryInteractor(t, repo, notifRepo, dispatcher)

	req := tvdb.Media{Id: "777", Name: "Test Movie", Category: "movie"}
	req.Metadata.FirstAired = aired(-400)

	summary, err := i.DownloadMovie(context.Background(), req)
	require.NoError(t, err)

	assert.Equal(t, "movie", summary.Category)
	assert.Equal(t, 1, summary.QueuedNow)
	assert.Equal(t, 0, summary.Scheduled)
	assert.Equal(t, 1, dispatcher.callCount())
}

func TestDownloadMovie_UnreleasedSchedules(t *testing.T) {
	repo := &fakeSchedulerRepo{}
	notifRepo := repoMocks.NewNotificationRepository(t)
	notifRepo.On("CreateNotification", mock.Anything, mock.Anything).Return(nil)
	dispatcher := &fakeDispatcher{}

	i := newSummaryInteractor(t, repo, notifRepo, dispatcher)

	req := tvdb.Media{Id: "777", Name: "Test Movie", Category: "movie"}
	req.Metadata.FirstAired = aired(30)

	summary, err := i.DownloadMovie(context.Background(), req)
	require.NoError(t, err)

	assert.Equal(t, 1, summary.Scheduled)
	assert.Equal(t, 0, summary.QueuedNow)
	require.NotNil(t, summary.NextRelease)
	assert.Equal(t, 0, dispatcher.callCount())
}

func TestDownloadMovie_AlreadyScheduledIsNotAnError(t *testing.T) {
	repo := &fakeSchedulerRepo{scheduleErr: repository.ErrDuplicateScheduled}
	notifRepo := repoMocks.NewNotificationRepository(t)
	dispatcher := &fakeDispatcher{}

	i := newSummaryInteractor(t, repo, notifRepo, dispatcher)

	req := tvdb.Media{Id: "777", Name: "Test Movie", Category: "movie"}
	req.Metadata.FirstAired = aired(30)

	summary, err := i.DownloadMovie(context.Background(), req)
	require.NoError(t, err, "an already-scheduled movie is not a user-facing failure")

	assert.Equal(t, 1, summary.Skipped)
	assert.Equal(t, 0, summary.Scheduled)
	assert.Equal(t, 0, dispatcher.callCount())
}
