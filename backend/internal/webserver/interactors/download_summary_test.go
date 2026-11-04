package interactors

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jbofill10/scout/backend/internal/webserver/clients"
	"github.com/jbofill10/scout/backend/internal/webserver/repository"
	repoMocks "github.com/jbofill10/scout/backend/internal/webserver/repository/mocks"
	"github.com/jbofill10/scout/backend/pkg/dlstatus"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/notifications"

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
	notifRepo.On("ActiveTvdbIDs", mock.Anything, mock.Anything).Return(map[string]bool{}, nil)
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

	// Only the aired episode is handed to the torrenter, and its row was
	// written first and completed on the answer.
	assert.Equal(t, 1, dispatcher.callCount())
	assert.Len(t, dispatcher.payload().Metadata.Episodes, 1)
	assert.Equal(t, 1, dispatcher.payload().Metadata.Episodes[0].Id)
	require.Len(t, repo.enqueued, 1)
	assert.Equal(t, 1, repo.enqueued[0].Metadata.Episodes[0].Id)
	assert.Equal(t, []int{101}, repo.completedIDs)
}

// A season's episodes go to the torrenter one per call. The torrenter searches
// them sequentially, so a whole-season request used to outlive the client's
// deadline and come back as "unreachable" for every episode.
func TestDownloadShow_DispatchesOneEpisodePerTorrenterCall(t *testing.T) {
	repo := &fakeSchedulerRepo{}
	notifRepo := repoMocks.NewNotificationRepository(t)
	notifRepo.On("ActiveTvdbIDs", mock.Anything, mock.Anything).Return(map[string]bool{}, nil)
	notifRepo.On("CreateNotification", mock.Anything, mock.Anything).Return(nil)
	dispatcher := &fakeDispatcher{respFor: func(req tvdb.Media) dlstatus.DownloadResponse {
		ep := req.Metadata.Episodes[0]
		return dlstatus.DownloadResponse{Results: []dlstatus.EpisodeResult{
			epResult(strconv.Itoa(ep.Id), dlstatus.OutcomeDownloading, ""),
		}}
	}}

	i := newSummaryInteractor(t, repo, notifRepo, dispatcher)

	req := tvdb.Media{Id: "555", Name: "Test Show", Category: "series", Aliases: []string{"TS"}}
	req.Metadata.Episodes = []tvdb.Episode{
		{Id: 1, SeasonNumber: 1, Number: 1, Aired: aired(-30)},
		{Id: 2, SeasonNumber: 1, Number: 2, Aired: aired(-23)},
		{Id: 3, SeasonNumber: 1, Number: 3, Aired: aired(-16)},
	}

	summary, err := i.DownloadShow(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, 3, summary.QueuedNow)

	payloads := dispatcher.payloads()
	require.Len(t, payloads, 3)
	for idx, p := range payloads {
		require.Len(t, p.Metadata.Episodes, 1, "call %d must carry exactly one episode", idx)
		assert.Equal(t, idx+1, p.Metadata.Episodes[0].Id, "episodes are dispatched in order")
		assert.Equal(t, "555", p.Id)
		assert.Equal(t, "Test Show", p.Name)
	}
	assert.Empty(t, repo.recordedIDs, "nothing failed, so no row is put on the retry ladder")
	assert.Equal(t, []int{101, 102, 103}, repo.completedIDs, "each episode's row is completed on its own answer")
}

// Each call's outcome is reconciled on its own: the failed episode's row goes
// on the retry ladder, and the others are completed.
func TestDownloadShow_RetriesOnlyTheEpisodeThatFailed(t *testing.T) {
	repo := &fakeSchedulerRepo{}
	notifRepo := repoMocks.NewNotificationRepository(t)
	notifRepo.On("ActiveTvdbIDs", mock.Anything, mock.Anything).Return(map[string]bool{}, nil)
	notifRepo.On("CreateNotification", mock.Anything, mock.Anything).Return(nil)
	notifRepo.On("GetNotification", mock.Anything, "2").Return(nil, nil)
	dispatcher := &fakeDispatcher{respFor: func(req tvdb.Media) dlstatus.DownloadResponse {
		ep := req.Metadata.Episodes[0]
		if ep.Id == 2 {
			return dlstatus.DownloadResponse{Results: []dlstatus.EpisodeResult{
				epResult("2", dlstatus.OutcomeFailed, dlstatus.CodeNoTorrentFound),
			}}
		}
		return dlstatus.DownloadResponse{Results: []dlstatus.EpisodeResult{
			epResult(strconv.Itoa(ep.Id), dlstatus.OutcomeDownloading, ""),
		}}
	}}

	i := newSummaryInteractor(t, repo, notifRepo, dispatcher)

	req := tvdb.Media{Id: "555", Name: "Test Show", Category: "series"}
	req.Metadata.Episodes = []tvdb.Episode{
		{Id: 1, SeasonNumber: 1, Number: 1, Aired: aired(-30)},
		{Id: 2, SeasonNumber: 1, Number: 2, Aired: aired(-23)},
		{Id: 3, SeasonNumber: 1, Number: 3, Aired: aired(-16)},
	}

	_, err := i.DownloadShow(context.Background(), req)
	require.NoError(t, err)

	assert.Equal(t, 3, dispatcher.callCount())
	assert.Equal(t, []int{102}, repo.recordedIDs, "only episode 2's row is rescheduled")
	assert.Equal(t, string(dlstatus.CodeNoTorrentFound), repo.recordedCode)
	assert.Equal(t, []int{101, 103}, repo.completedIDs)
}

// The rows are written before the torrenter is asked: a restart while the
// search runs finds them queued and sends them again, instead of leaving the
// notifications on "searching" with nothing to recover from.
func TestDownloadShow_RowsExistBeforeTheTorrenterIsAsked(t *testing.T) {
	repo := &fakeSchedulerRepo{}
	notifRepo := repoMocks.NewNotificationRepository(t)
	notifRepo.On("ActiveTvdbIDs", mock.Anything, mock.Anything).Return(map[string]bool{}, nil)
	notifRepo.On("CreateNotification", mock.Anything, mock.Anything).Return(nil)

	release := make(chan struct{})
	started := make(chan struct{})
	dispatcher := &fakeDispatcher{blockOn: release, started: started}
	defer close(release)

	i := newSummaryInteractor(t, repo, notifRepo, dispatcher)
	i.dispatch = func(fn func()) { go fn() }

	req := tvdb.Media{Id: "555", Name: "Test Show", Category: "series", Source: "manual"}
	req.Metadata.Episodes = []tvdb.Episode{
		{Id: 1, SeasonNumber: 1, Number: 1, Aired: aired(-30)},
		{Id: 2, SeasonNumber: 1, Number: 2, Aired: aired(-23)},
	}

	summary, err := i.DownloadShow(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, 2, summary.QueuedNow)

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("background dispatch never reached the torrenter")
	}
	// The torrenter is still working on the first episode; both rows are down already.
	rows := repo.enqueuedRows()
	require.Len(t, rows, 2)
	for _, m := range rows {
		assert.Equal(t, "555", m.Id)
		assert.Equal(t, "manual", m.Source, "the row keeps the request's priority")
		assert.Len(t, m.Metadata.Episodes, 1)
	}
}

// An episode whose row is already queued is being dispatched by someone else
// (a scheduled row that just fired, say): it is reported in progress and not
// searched for twice.
func TestDownloadShow_AlreadyQueuedRowCountsAsInProgress(t *testing.T) {
	repo := &fakeSchedulerRepo{alreadyRun: map[string]bool{"2": true}}
	notifRepo := repoMocks.NewNotificationRepository(t)
	notifRepo.On("ActiveTvdbIDs", mock.Anything, mock.Anything).Return(map[string]bool{}, nil)
	notifRepo.On("CreateNotification", mock.Anything, mock.MatchedBy(func(n *notifications.Notification) bool {
		return n.TvdbID != "2"
	})).Return(nil)
	dispatcher := &fakeDispatcher{respFor: func(req tvdb.Media) dlstatus.DownloadResponse {
		ep := req.Metadata.Episodes[0]
		return dlstatus.DownloadResponse{Results: []dlstatus.EpisodeResult{
			epResult(strconv.Itoa(ep.Id), dlstatus.OutcomeDownloading, ""),
		}}
	}}

	i := newSummaryInteractor(t, repo, notifRepo, dispatcher)

	req := tvdb.Media{Id: "555", Name: "Test Show", Category: "series"}
	req.Metadata.Episodes = []tvdb.Episode{
		{Id: 1, SeasonNumber: 1, Number: 1, Aired: aired(-30)},
		{Id: 2, SeasonNumber: 1, Number: 2, Aired: aired(-23)},
	}

	summary, err := i.DownloadShow(context.Background(), req)
	require.NoError(t, err)

	assert.Equal(t, 1, summary.QueuedNow)
	assert.Equal(t, 1, summary.InProgress)
	require.Len(t, dispatcher.payloads(), 1)
	assert.Equal(t, 1, dispatcher.payloads()[0].Metadata.Episodes[0].Id)
}

// When the request cannot be written down there is nothing a restart could
// recover, so the episode is not started half-tracked: it is skipped and the
// history says why.
func TestDownloadShow_UnrecordableEpisodeIsSkipped(t *testing.T) {
	repo := &fakeSchedulerRepo{enqueueErr: errors.New("db down")}
	notifRepo := repoMocks.NewNotificationRepository(t)
	notifRepo.On("ActiveTvdbIDs", mock.Anything, mock.Anything).Return(map[string]bool{}, nil)
	dispatcher := &fakeDispatcher{}

	i := newSummaryInteractor(t, repo, notifRepo, dispatcher)

	req := tvdb.Media{Id: "555", Name: "Test Show", Category: "series"}
	req.Metadata.Episodes = []tvdb.Episode{{Id: 1, SeasonNumber: 1, Number: 1, Aired: aired(-30)}}

	summary, err := i.DownloadShow(context.Background(), req)
	require.NoError(t, err)

	assert.Equal(t, 0, summary.QueuedNow)
	assert.Equal(t, 1, summary.Skipped)
	assert.Equal(t, 0, dispatcher.callCount())
	assert.Equal(t, 1, repo.historyCount)
	notifRepo.AssertNotCalled(t, "CreateNotification", mock.Anything, mock.Anything)
}

func TestDownloadShow_AlreadyScheduledCountsAsSkipped(t *testing.T) {
	repo := &fakeSchedulerRepo{scheduleErr: repository.ErrDuplicateScheduled}
	notifRepo := repoMocks.NewNotificationRepository(t)
	notifRepo.On("ActiveTvdbIDs", mock.Anything, mock.Anything).Return(map[string]bool{}, nil)
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
	notifRepo.On("ActiveTvdbIDs", mock.Anything, mock.Anything).Return(map[string]bool{}, nil)
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
	notifRepo.On("ActiveTvdbIDs", mock.Anything, mock.Anything).Return(map[string]bool{}, nil)
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
	// The movie's row was written before the torrenter was asked and completed on its answer.
	require.Len(t, repo.enqueued, 1)
	assert.Equal(t, "777", repo.enqueued[0].Id)
	assert.Equal(t, []int{101}, repo.completedIDs)
}

// A movie whose request cannot be written down is not started half-tracked:
// the request fails so the caller knows, and nothing goes to the torrenter.
func TestDownloadMovie_UnrecordableRequestFails(t *testing.T) {
	repo := &fakeSchedulerRepo{enqueueErr: errors.New("db down")}
	notifRepo := repoMocks.NewNotificationRepository(t)
	notifRepo.On("ActiveTvdbIDs", mock.Anything, mock.Anything).Return(map[string]bool{}, nil)
	dispatcher := &fakeDispatcher{}

	i := newSummaryInteractor(t, repo, notifRepo, dispatcher)

	req := tvdb.Media{Id: "777", Name: "Test Movie", Category: "movie"}
	req.Metadata.FirstAired = aired(-400)

	_, err := i.DownloadMovie(context.Background(), req)
	require.Error(t, err)
	assert.Equal(t, 0, dispatcher.callCount())
	notifRepo.AssertNotCalled(t, "CreateNotification", mock.Anything, mock.Anything)
}

// A movie whose row is already queued is being dispatched already.
func TestDownloadMovie_AlreadyQueuedRowCountsAsInProgress(t *testing.T) {
	repo := &fakeSchedulerRepo{alreadyRun: map[string]bool{"777": true}}
	notifRepo := repoMocks.NewNotificationRepository(t)
	notifRepo.On("ActiveTvdbIDs", mock.Anything, mock.Anything).Return(map[string]bool{}, nil)
	dispatcher := &fakeDispatcher{}

	i := newSummaryInteractor(t, repo, notifRepo, dispatcher)

	req := tvdb.Media{Id: "777", Name: "Test Movie", Category: "movie"}
	req.Metadata.FirstAired = aired(-400)

	summary, err := i.DownloadMovie(context.Background(), req)
	require.NoError(t, err)

	assert.Equal(t, 1, summary.InProgress)
	assert.Equal(t, 0, summary.QueuedNow)
	assert.Equal(t, 0, dispatcher.callCount())
	notifRepo.AssertNotCalled(t, "CreateNotification", mock.Anything, mock.Anything)
}

func TestDownloadMovie_UnreleasedSchedules(t *testing.T) {
	repo := &fakeSchedulerRepo{}
	notifRepo := repoMocks.NewNotificationRepository(t)
	notifRepo.On("ActiveTvdbIDs", mock.Anything, mock.Anything).Return(map[string]bool{}, nil)
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
	notifRepo.On("ActiveTvdbIDs", mock.Anything, mock.Anything).Return(map[string]bool{}, nil)
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

// An episode Scout is already searching for or downloading is not requested
// again: no second notification, no second search. The other episodes go
// through as usual.
func TestDownloadShow_EpisodesInProgressAreSkipped(t *testing.T) {
	repo := &fakeSchedulerRepo{}
	notifRepo := repoMocks.NewNotificationRepository(t)
	notifRepo.On("ActiveTvdbIDs", mock.Anything, []string{"1", "2", "3"}).Return(map[string]bool{"2": true}, nil)
	notifRepo.On("CreateNotification", mock.Anything, mock.MatchedBy(func(n *notifications.Notification) bool {
		return n.TvdbID != "2"
	})).Return(nil)
	dispatcher := &fakeDispatcher{respFor: func(req tvdb.Media) dlstatus.DownloadResponse {
		ep := req.Metadata.Episodes[0]
		return dlstatus.DownloadResponse{Results: []dlstatus.EpisodeResult{
			epResult(strconv.Itoa(ep.Id), dlstatus.OutcomeDownloading, ""),
		}}
	}}

	i := newSummaryInteractor(t, repo, notifRepo, dispatcher)

	req := tvdb.Media{Id: "555", Name: "Test Show", Category: "series"}
	req.Metadata.Episodes = []tvdb.Episode{
		{Id: 1, SeasonNumber: 1, Number: 1, Aired: aired(-30)},
		{Id: 2, SeasonNumber: 1, Number: 2, Aired: aired(-23)},
		{Id: 3, SeasonNumber: 1, Number: 3, Aired: aired(-16)},
	}

	summary, err := i.DownloadShow(context.Background(), req)
	require.NoError(t, err)

	assert.Equal(t, 2, summary.QueuedNow)
	assert.Equal(t, 1, summary.InProgress)
	assert.Equal(t, 0, summary.Skipped)

	payloads := dispatcher.payloads()
	require.Len(t, payloads, 2)
	assert.Equal(t, 1, payloads[0].Metadata.Episodes[0].Id)
	assert.Equal(t, 3, payloads[1].Metadata.Episodes[0].Id)
}

// The in-progress check is best effort: when it cannot be made, the request
// proceeds as before and the torrenter's own in-flight check is the backstop.
func TestDownloadShow_InProgressLookupFailureStillDownloads(t *testing.T) {
	repo := &fakeSchedulerRepo{}
	notifRepo := repoMocks.NewNotificationRepository(t)
	notifRepo.On("ActiveTvdbIDs", mock.Anything, mock.Anything).Return(nil, errors.New("db down"))
	notifRepo.On("CreateNotification", mock.Anything, mock.Anything).Return(nil)
	dispatcher := &fakeDispatcher{resp: dlstatus.DownloadResponse{
		Results: []dlstatus.EpisodeResult{epResult("1", dlstatus.OutcomeDownloading, "")},
	}}

	i := newSummaryInteractor(t, repo, notifRepo, dispatcher)

	req := tvdb.Media{Id: "555", Name: "Test Show", Category: "series"}
	req.Metadata.Episodes = []tvdb.Episode{{Id: 1, SeasonNumber: 1, Number: 1, Aired: aired(-30)}}

	summary, err := i.DownloadShow(context.Background(), req)
	require.NoError(t, err)

	assert.Equal(t, 1, summary.QueuedNow)
	assert.Equal(t, 0, summary.InProgress)
	assert.Equal(t, 1, dispatcher.callCount())
}

// A movie Scout is already working on is answered without a TVDB lookup, a
// second notification or a torrenter call. Two requests seconds apart used to
// run two searches and strand one notification on "searching".
func TestDownloadMovie_InProgressIsNotRequeued(t *testing.T) {
	repo := &fakeSchedulerRepo{}
	notifRepo := repoMocks.NewNotificationRepository(t)
	notifRepo.On("ActiveTvdbIDs", mock.Anything, []string{"777"}).Return(map[string]bool{"777": true}, nil)
	dispatcher := &fakeDispatcher{}

	i := newSummaryInteractor(t, repo, notifRepo, dispatcher)

	req := tvdb.Media{Id: "777", Name: "Test Movie", Category: "movie"}
	req.Metadata.FirstAired = aired(-400)

	summary, err := i.DownloadMovie(context.Background(), req)
	require.NoError(t, err)

	assert.Equal(t, 1, summary.InProgress)
	assert.Equal(t, 0, summary.QueuedNow)
	assert.Equal(t, 0, summary.Skipped)
	assert.Equal(t, 0, dispatcher.callCount())
}

// The movie path is best effort too: a failed lookup does not block the request.
func TestDownloadMovie_InProgressLookupFailureStillDownloads(t *testing.T) {
	repo := &fakeSchedulerRepo{}
	notifRepo := repoMocks.NewNotificationRepository(t)
	notifRepo.On("ActiveTvdbIDs", mock.Anything, []string{"777"}).Return(nil, errors.New("db down"))
	notifRepo.On("CreateNotification", mock.Anything, mock.Anything).Return(nil)
	dispatcher := &fakeDispatcher{resp: dlstatus.DownloadResponse{
		Results: []dlstatus.EpisodeResult{epResult("777", dlstatus.OutcomeDownloading, "")},
	}}

	i := newSummaryInteractor(t, repo, notifRepo, dispatcher)

	req := tvdb.Media{Id: "777", Name: "Test Movie", Category: "movie"}
	req.Metadata.FirstAired = aired(-400)

	summary, err := i.DownloadMovie(context.Background(), req)
	require.NoError(t, err)

	assert.Equal(t, 1, summary.QueuedNow)
	assert.Equal(t, 0, summary.InProgress)
	assert.Equal(t, 1, dispatcher.callCount())
}

// A season whose every episode is already in progress starts nothing and says
// so, rather than reporting "nothing to download".
func TestDownloadShow_AllEpisodesInProgressStartsNothing(t *testing.T) {
	repo := &fakeSchedulerRepo{}
	notifRepo := repoMocks.NewNotificationRepository(t)
	notifRepo.On("ActiveTvdbIDs", mock.Anything, []string{"1", "2"}).Return(map[string]bool{"1": true, "2": true}, nil)
	dispatcher := &fakeDispatcher{}

	i := newSummaryInteractor(t, repo, notifRepo, dispatcher)

	req := tvdb.Media{Id: "555", Name: "Test Show", Category: "series"}
	req.Metadata.Episodes = []tvdb.Episode{
		{Id: 1, SeasonNumber: 1, Number: 1, Aired: aired(-30)},
		{Id: 2, SeasonNumber: 1, Number: 2, Aired: aired(7)}, // future, but already scheduled
	}

	summary, err := i.DownloadShow(context.Background(), req)
	require.NoError(t, err)

	assert.Equal(t, 2, summary.InProgress)
	assert.Equal(t, 0, summary.QueuedNow)
	assert.Equal(t, 0, summary.Scheduled)
	assert.Equal(t, 0, dispatcher.callCount())
	assert.Equal(t, 0, repo.scheduleCount, "an already-scheduled episode is not scheduled again")
}
