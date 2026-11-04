package interactors

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
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
)

// connectionRefused is what the HTTP client returns when nothing listens on
// the torrenter's port: a URL error around a dial failure.
func connectionRefused() error {
	return &url.Error{Op: "Post", URL: "http://torrenter:22001/download",
		Err: &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connect: connection refused")}}
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func anyNotification(t *testing.T) *repoMocks.NotificationRepository {
	n := repoMocks.NewNotificationRepository(t)
	n.On("GetNotification", mock.Anything, mock.Anything).
		Return(&notifications.Notification{ID: 1, Status: notifications.StatusSearching}, nil).Maybe()
	n.On("UpdateNotification", mock.Anything, mock.Anything).Return(nil).Maybe()
	return n
}

func TestTorrenterDown(t *testing.T) {
	assert.False(t, torrenterDown(nil))
	assert.True(t, torrenterDown(connectionRefused()), "nothing answered on the port")
	assert.True(t, torrenterDown(&url.Error{Op: "Post", Err: errors.New("EOF")}), "the connection was dropped")
	assert.False(t, torrenterDown(&clients.StatusError{StatusCode: 500}), "the torrenter answered, badly")
	assert.False(t, torrenterDown(&url.Error{Op: "Post", Err: timeoutErr{}}), "a slow torrenter is up")
	assert.False(t, torrenterDown(context.DeadlineExceeded))
	assert.False(t, torrenterDown(errors.New("invalid character")), "a bad body is not an outage")
	assert.True(t, torrenterDown(fmt.Errorf("dispatch: %w", connectionRefused())), "wrapping does not hide the outage")
	assert.False(t, torrenterDown(&url.Error{Op: "Post", Err: context.Canceled}), "a cancelled dispatch is not an outage")
}

// The torrenter is restarting: the group's first per-episode call is refused.
// Every row of the group is held for two minutes without an attempt counted,
// with one call made, not one per episode.
func TestProcessDueGroup_TorrenterDownDefersTheWholeGroup(t *testing.T) {
	first, siblings := threeRows()
	repo := &fakeSchedulerRepo{}
	disp := &fakeDispatcher{err: connectionRefused()}
	i := groupInteractor(repo, disp, anyNotification(t))

	before := time.Now()
	i.processDueGroup(context.Background(), append([]repository.DueItem{first}, siblings...))

	assert.Equal(t, 1, disp.callCount(), "one refused call is enough to know")
	assert.Equal(t, []int{1, 2, 3}, repo.deferredIDs)
	assert.Equal(t, string(dlstatus.CodeTorrenterUnreachable), repo.deferCode)
	assert.WithinDuration(t, before.Add(torrenterRetryDelay), repo.deferNext, 5*time.Second)
	assert.Empty(t, repo.recordedIDs, "no attempt is counted")
	assert.Empty(t, repo.completedIDs)
	assert.Empty(t, repo.permFailedIDs)
}

// The torrenter goes away part-way through a group: the episodes already
// answered keep their outcome and only the rest is deferred.
func TestProcessDueGroup_TorrenterDownMidGroupDefersTheRest(t *testing.T) {
	first, siblings := threeRows()
	repo := &fakeSchedulerRepo{}
	calls := 0
	disp := &fakeDispatcher{}
	disp.respFor = func(req tvdb.Media) dlstatus.DownloadResponse {
		calls++
		if calls == 1 {
			return dlstatus.DownloadResponse{Results: []dlstatus.EpisodeResult{epResult("11", dlstatus.OutcomeDownloading, "")}}
		}
		disp.err = connectionRefused()
		return dlstatus.DownloadResponse{}
	}
	i := groupInteractor(repo, disp, anyNotification(t))

	i.processDueGroup(context.Background(), append([]repository.DueItem{first}, siblings...))

	assert.Equal(t, 2, disp.callCount())
	assert.Equal(t, []int{1}, repo.completedIDs)
	assert.Equal(t, []int{2, 3}, repo.deferredIDs)
	assert.Empty(t, repo.recordedIDs)
}

// A lone row (a movie, say) is deferred the same way.
func TestProcessDueItem_TorrenterDownDefers(t *testing.T) {
	repo := &fakeSchedulerRepo{}
	i := groupInteractor(repo, &fakeDispatcher{err: connectionRefused()}, anyNotification(t))

	i.processDueItem(context.Background(), repository.DueItem{ID: 5, Media: tvdb.Media{Id: "9", Name: "M", Category: "movie"}})

	assert.Equal(t, []int{5}, repo.deferredIDs)
	assert.Empty(t, repo.recordedIDs)
}

// An error answer, a timeout or a cancelled dispatch means the torrenter was
// there: the usual ladder applies and an attempt is counted.
func TestProcessDueItem_TorrenterAnsweredKeepsTheLadder(t *testing.T) {
	unreachable := dlstatus.DownloadResponse{Results: []dlstatus.EpisodeResult{
		epResult("9", dlstatus.OutcomeFailed, dlstatus.CodeTorrenterUnreachable),
	}}
	for name, err := range map[string]error{
		"status 500": &clients.StatusError{StatusCode: 500},
		"timeout":    &url.Error{Op: "Post", Err: timeoutErr{}},
		"deadline":   context.DeadlineExceeded,
	} {
		t.Run(name, func(t *testing.T) {
			repo := &fakeSchedulerRepo{}
			i := groupInteractor(repo, &fakeDispatcher{resp: unreachable, err: err}, anyNotification(t))

			i.processDueItem(context.Background(), repository.DueItem{ID: 5, Media: tvdb.Media{Id: "9", Name: "M", Category: "movie"}})

			assert.Equal(t, []int{5}, repo.recordedIDs, "the row takes a step on the ladder")
			assert.Equal(t, string(dlstatus.CodeTorrenterUnreachable), repo.recordedCode)
			assert.Empty(t, repo.deferredIDs)
		})
	}
}

// While a group works through its episodes, the rows still waiting are kept
// fresh so the stale-queued sweep does not hand them to a second dispatch.
func TestProcessDueGroup_KeepsWaitingRowsFresh(t *testing.T) {
	first, siblings := threeRows()
	repo := &fakeSchedulerRepo{}
	disp := &fakeDispatcher{respFor: func(req tvdb.Media) dlstatus.DownloadResponse {
		ep := req.Metadata.Episodes[0]
		return dlstatus.DownloadResponse{Results: []dlstatus.EpisodeResult{epResult(strconv.Itoa(ep.Id), dlstatus.OutcomeDownloading, "")}}
	}}
	i := groupInteractor(repo, disp, repoMocks.NewNotificationRepository(t))

	i.processDueGroup(context.Background(), append([]repository.DueItem{first}, siblings...))

	assert.Equal(t, [][]int{{2, 3}, {3}}, repo.touched, "after each answer the rows still waiting are touched")
	assert.Equal(t, []int{1, 2, 3}, repo.completedIDs)
}

// A row the scheduler fires on its first attempt searches at "scheduled"
// priority whoever set it up, a person's request stays "manual", and a row a
// request dispatches directly keeps the priority it was made with.
func TestDispatchSource(t *testing.T) {
	ok := func(req tvdb.Media) dlstatus.DownloadResponse {
		return dlstatus.DownloadResponse{Results: []dlstatus.EpisodeResult{epResult("9", dlstatus.OutcomeDownloading, "")}}
	}
	movie := func(source string) repository.DueItem {
		return repository.DueItem{ID: 5, Media: tvdb.Media{Id: "9", Name: "M", Category: "movie", Source: source}}
	}

	disp := &fakeDispatcher{respFor: ok}
	groupInteractor(&fakeSchedulerRepo{}, disp, repoMocks.NewNotificationRepository(t)).processDueBatch(movie("follow"))
	assert.Equal(t, "scheduled", disp.payload().Source)

	disp = &fakeDispatcher{respFor: ok}
	groupInteractor(&fakeSchedulerRepo{}, disp, repoMocks.NewNotificationRepository(t)).processDueBatch(movie("manual"))
	assert.Equal(t, "manual", disp.payload().Source)

	disp = &fakeDispatcher{respFor: ok}
	retry := movie("follow")
	retry.Attempts = 2
	groupInteractor(&fakeSchedulerRepo{}, disp, repoMocks.NewNotificationRepository(t)).processDueBatch(retry)
	assert.Equal(t, "follow", disp.payload().Source, "a retry keeps the priority of the request it came from")

	disp = &fakeDispatcher{respFor: ok}
	groupInteractor(&fakeSchedulerRepo{}, disp, repoMocks.NewNotificationRepository(t)).
		processDueItem(context.Background(), movie("follow"))
	assert.Equal(t, "follow", disp.payload().Source, "a direct dispatch is not promoted")
}

// Twenty-five rows of one show come due at the same instant: every timer
// fires and queues its row, the first item dispatched claims the rest as
// siblings and settles them, and the other twenty-four items reach the front
// of the queue later. Those are not dispatched again: their rows are no longer
// queued. A row still queued (its own timer's claim, nobody else's) is.
func TestProcessDueBatch_SkipsRowsAnotherDispatchSettled(t *testing.T) {
	for status, dispatched := range map[string]bool{
		repository.StatusCompleted: false,
		repository.StatusPending:   false,
		repository.StatusFailed:    false,
		repository.StatusCancelled: false,
		repository.StatusQueued:    true,
	} {
		t.Run(status, func(t *testing.T) {
			repo := &fakeSchedulerRepo{rowStatus: map[int]string{5: status}}
			disp := &fakeDispatcher{respFor: func(tvdb.Media) dlstatus.DownloadResponse {
				return dlstatus.DownloadResponse{Results: []dlstatus.EpisodeResult{epResult("9", dlstatus.OutcomeDownloading, "")}}
			}}
			i := groupInteractor(repo, disp, repoMocks.NewNotificationRepository(t))

			i.processDueBatch(repository.DueItem{ID: 5, Media: tvdb.Media{Id: "9", Name: "M", Category: "movie"}})

			assert.Equal(t, dispatched, disp.callCount() == 1, "dispatched=%v for status %s", dispatched, status)
			assert.Equal(t, 0, repo.claimCalls, "a movie claims no siblings")
		})
	}
}

// When the status cannot be read the item is dispatched as before.
func TestProcessDueBatch_StatusLookupFailureStillDispatches(t *testing.T) {
	repo := &fakeSchedulerRepo{rowStatusErr: errors.New("db down")}
	disp := &fakeDispatcher{respFor: func(tvdb.Media) dlstatus.DownloadResponse {
		return dlstatus.DownloadResponse{Results: []dlstatus.EpisodeResult{epResult("9", dlstatus.OutcomeDownloading, "")}}
	}}
	i := groupInteractor(repo, disp, repoMocks.NewNotificationRepository(t))

	i.processDueBatch(repository.DueItem{ID: 5, Media: tvdb.Media{Id: "9", Name: "M", Category: "movie"}})

	assert.Equal(t, 1, disp.callCount())
}
