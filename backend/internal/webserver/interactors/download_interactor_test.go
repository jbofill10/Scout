package interactors

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jbofill10/scout/backend/internal/webserver/repository"
	repoMocks "github.com/jbofill10/scout/backend/internal/webserver/repository/mocks"
	"github.com/jbofill10/scout/backend/internal/webserver/retry"
	"github.com/jbofill10/scout/backend/pkg/dlstatus"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/notifications"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// fakeDispatcher is a minimal DownloadDispatcher returning a canned response.
type fakeDispatcher struct {
	resp dlstatus.DownloadResponse
	err  error
}

func (f *fakeDispatcher) Download(_ context.Context, _ tvdb.Media) (dlstatus.DownloadResponse, error) {
	return f.resp, f.err
}

// fakeSchedulerRepo records the terminal-status transitions made by the
// interactor. It implements repository.SchedulerRepository; methods unused by
// the dispatch path are no-ops.
type fakeSchedulerRepo struct {
	completedID      *int
	recordedID       *int
	recordedCode     string
	recordedReason   string
	recordedNext     time.Time
	permFailedID     *int
	permFailedCode   string
	permFailedReason string
	historyCount     int
	pendingMeta      map[string]repository.ScheduleMeta
	pendingMetaErr   error
}

func (f *fakeSchedulerRepo) GetPendingScheduleMeta(context.Context) (map[string]repository.ScheduleMeta, error) {
	return f.pendingMeta, f.pendingMetaErr
}

func (f *fakeSchedulerRepo) MarkCompleted(_ context.Context, id int) error {
	f.completedID = &id
	return nil
}

func (f *fakeSchedulerRepo) RecordFailure(_ context.Context, id int, code, reason string, next time.Time) error {
	f.recordedID = &id
	f.recordedCode = code
	f.recordedReason = reason
	f.recordedNext = next
	return nil
}

func (f *fakeSchedulerRepo) MarkPermanentlyFailed(_ context.Context, id int, code, reason string) error {
	f.permFailedID = &id
	f.permFailedCode = code
	f.permFailedReason = reason
	return nil
}

func (f *fakeSchedulerRepo) InsertDownloadHistory(_ string, _, _, _ int, _, _, _, _ string) error {
	f.historyCount++
	return nil
}

// Unused-by-dispatch methods (present to satisfy the interface).
func (f *fakeSchedulerRepo) Schedule(context.Context, tvdb.Media, time.Time, string, string) error {
	return nil
}
func (f *fakeSchedulerRepo) ScheduleRetry(context.Context, tvdb.Media, time.Time, string, string, string, string) error {
	return nil
}
func (f *fakeSchedulerRepo) GetDueMedia(context.Context, time.Time) ([]repository.DueItem, error) {
	return nil, nil
}
func (f *fakeSchedulerRepo) MarkQueued(context.Context, int) error { return nil }
func (f *fakeSchedulerRepo) ResetStaleQueued(context.Context, time.Duration) (int64, error) {
	return 0, nil
}
func (f *fakeSchedulerRepo) GetWeeklySchedule() ([]repository.ScheduledDownload, error) {
	return nil, nil
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, nil))
}

func epResult(tvdbID string, outcome string, code dlstatus.FailureCode) dlstatus.EpisodeResult {
	return dlstatus.EpisodeResult{TvdbID: tvdbID, Outcome: outcome, Code: code, Season: 1, Episode: 1}
}

func TestProcessDueItem(t *testing.T) {
	tests := []struct {
		name              string
		results           []dlstatus.EpisodeResult
		dispatchErr       error
		attempts          int
		wantCompleted     bool
		wantRecordFailure bool
		wantPermFailed    bool
		wantPermCode      string
		wantNotifyStatus  notifications.NotificationStatus // expected notification status, "" = no update expected
		wantHistory       bool
	}{
		{
			name:          "all downloading -> completed",
			results:       []dlstatus.EpisodeResult{epResult("100", dlstatus.OutcomeDownloading, "")},
			wantCompleted: true,
		},
		{
			name:          "downloading and exists -> completed",
			results:       []dlstatus.EpisodeResult{epResult("100", dlstatus.OutcomeDownloading, ""), epResult("101", dlstatus.OutcomeExists, "")},
			wantCompleted: true,
		},
		{
			name:              "transient with retries remaining -> record failure + searching",
			results:           []dlstatus.EpisodeResult{epResult("100", dlstatus.OutcomeFailed, dlstatus.CodeNoTorrentFound)},
			attempts:          1,
			wantRecordFailure: true,
			wantNotifyStatus:  notifications.StatusSearching,
		},
		{
			name:             "transient exhausted -> permanent max_retries + failed",
			results:          []dlstatus.EpisodeResult{epResult("100", dlstatus.OutcomeFailed, dlstatus.CodeNoTorrentFound)},
			attempts:         retry.MaxAttempts - 1,
			wantPermFailed:   true,
			wantPermCode:     string(dlstatus.CodeMaxRetries),
			wantNotifyStatus: notifications.StatusFailed,
			wantHistory:      true,
		},
		{
			name:             "permanent failure -> permanent + failed",
			results:          []dlstatus.EpisodeResult{epResult("100", dlstatus.OutcomeFailed, dlstatus.CodeInvalidMedia)},
			attempts:         0,
			wantPermFailed:   true,
			wantPermCode:     string(dlstatus.CodeInvalidMedia),
			wantNotifyStatus: notifications.StatusFailed,
			wantHistory:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeSchedulerRepo{}
			notifRepo := repoMocks.NewNotificationRepository(t)
			if tt.wantNotifyStatus != "" {
				existing := &notifications.Notification{ID: 9, TvdbID: "100", Status: notifications.StatusSearching}
				notifRepo.On("GetNotification", mock.Anything, "100").Return(existing, nil)
				notifRepo.On("UpdateNotification", mock.Anything, mock.MatchedBy(func(n *notifications.Notification) bool {
					return n.Status == tt.wantNotifyStatus
				})).Return(nil)
			}

			i := &DownloadInteractor{
				repo:             repo,
				notificationRepo: notifRepo,
				logger:           testLogger(),
				torrenterClient:  &fakeDispatcher{resp: dlstatus.DownloadResponse{Results: tt.results}, err: tt.dispatchErr},
			}

			item := repository.DueItem{ID: 42, Media: tvdb.Media{Name: "Test", Id: "555"}, Attempts: tt.attempts}
			i.processDueItem(item)

			if tt.wantCompleted {
				assert.NotNil(t, repo.completedID, "expected MarkCompleted")
				assert.Equal(t, 42, *repo.completedID)
			} else {
				assert.Nil(t, repo.completedID, "did not expect MarkCompleted")
			}

			if tt.wantRecordFailure {
				assert.NotNil(t, repo.recordedID, "expected RecordFailure")
				assert.Equal(t, 42, *repo.recordedID)
				assert.Equal(t, string(dlstatus.CodeNoTorrentFound), repo.recordedCode)
				assert.True(t, repo.recordedNext.After(time.Now()), "next attempt should be in the future")
			} else {
				assert.Nil(t, repo.recordedID, "did not expect RecordFailure")
			}

			if tt.wantPermFailed {
				assert.NotNil(t, repo.permFailedID, "expected MarkPermanentlyFailed")
				assert.Equal(t, 42, *repo.permFailedID)
				assert.Equal(t, tt.wantPermCode, repo.permFailedCode)
			} else {
				assert.Nil(t, repo.permFailedID, "did not expect MarkPermanentlyFailed")
			}

			if tt.wantHistory {
				assert.Equal(t, 1, repo.historyCount, "expected one history row")
			} else {
				assert.Equal(t, 0, repo.historyCount, "did not expect history rows")
			}
		})
	}
}

func TestFailedResult_FirstPermanentWins(t *testing.T) {
	// A transient failure precedes a permanent one; permanent must win.
	results := []dlstatus.EpisodeResult{
		epResult("1", dlstatus.OutcomeFailed, dlstatus.CodeNoTorrentFound), // transient
		epResult("2", dlstatus.OutcomeFailed, dlstatus.CodeInvalidMedia),   // permanent
	}
	got := failedResult(results)
	assert.NotNil(t, got)
	assert.Equal(t, "2", got.TvdbID, "first permanent failure should win over an earlier transient")
}

func TestFailedResult_TransientWhenNoPermanent(t *testing.T) {
	results := []dlstatus.EpisodeResult{
		epResult("1", dlstatus.OutcomeExists, ""),
		epResult("2", dlstatus.OutcomeFailed, dlstatus.CodeNoTorrentFound),
		epResult("3", dlstatus.OutcomeFailed, dlstatus.CodeIndexerUnreachable),
	}
	got := failedResult(results)
	assert.NotNil(t, got)
	assert.Equal(t, "2", got.TvdbID, "first transient failure should be returned when no permanent failures")
}

func TestFailedResult_NoFailures(t *testing.T) {
	results := []dlstatus.EpisodeResult{
		epResult("1", dlstatus.OutcomeDownloading, ""),
		epResult("2", dlstatus.OutcomeExists, ""),
	}
	assert.Nil(t, failedResult(results))
}
