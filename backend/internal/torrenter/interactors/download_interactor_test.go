package interactors

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	repomocks "github.com/jbofill10/scout/backend/internal/torrenter/repository/mocks"
	servicemocks "github.com/jbofill10/scout/backend/internal/torrenter/service/mocks"
	"github.com/jbofill10/scout/backend/pkg/notifications"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(new(bytes.Buffer), nil))
}

func completionEvent() models.TorrentCompleteEvent {
	return models.TorrentCompleteEvent{
		SavePath: "/data/Downloads/Some.Movie",
		Hash:     "hash-1",
		UUID:     "uuid-1",
		Req: &models.SearchStrategy{
			MediaName:     "Some Movie",
			EpisodeTvdbID: "999",
			IsMovie:       true,
		},
	}
}

// A processed torrent is accounted for and must stop being tracked as in-flight,
// or every restart would resume a monitor for a download that already landed.
func TestHandleDownloadCompletion_ClearsActiveTorrentOnSuccess(t *testing.T) {
	repo := repomocks.NewRepository(t)
	mp := servicemocks.NewMediaProcessor(t)
	qbitt := servicemocks.NewTorrentService(t)

	mp.On("ProcessDownloadedTorrent", mock.Anything, mock.Anything).Return(nil)
	repo.On("UpdateDownloadHistoryStatus", mock.Anything, "hash-1", "success", "").Return(nil)
	repo.On("GetNotification", mock.Anything, "999").Return(&notifications.Notification{ID: 1}, nil)
	repo.On("UpdateNotification", mock.Anything, mock.Anything).Return(nil)
	repo.On("DeleteActiveTorrent", mock.Anything, "hash-1").Return(nil)
	qbitt.On("RemoveUUIDTag", mock.Anything, "hash-1", "uuid-1").Return(nil)

	i := NewDownloadInteractor(qbitt, mp, repo, testLogger())

	dlComplete := make(chan models.TorrentCompleteEvent, 1)
	dlComplete <- completionEvent()
	close(dlComplete)

	i.handleDownloadCompletion(context.Background(), dlComplete)

	repo.AssertExpectations(t)
}

// Processing failure is still an outcome. The row has to go, otherwise the same
// doomed torrent is replayed on every single restart.
func TestHandleDownloadCompletion_ClearsActiveTorrentOnProcessingFailure(t *testing.T) {
	repo := repomocks.NewRepository(t)
	mp := servicemocks.NewMediaProcessor(t)
	qbitt := servicemocks.NewTorrentService(t)

	mp.On("ProcessDownloadedTorrent", mock.Anything, mock.Anything).Return(errors.New("no video files found"))
	repo.On("UpdateDownloadHistoryStatus", mock.Anything, "hash-1", "failure", "no video files found").Return(nil)
	repo.On("GetNotification", mock.Anything, "999").Return(&notifications.Notification{ID: 1}, nil)
	repo.On("UpdateNotification", mock.Anything, mock.MatchedBy(func(n *notifications.Notification) bool {
		return n.Status == notifications.StatusFailed
	})).Return(nil)
	repo.On("DeleteActiveTorrent", mock.Anything, "hash-1").Return(nil)

	i := NewDownloadInteractor(qbitt, mp, repo, testLogger())

	dlComplete := make(chan models.TorrentCompleteEvent, 1)
	dlComplete <- completionEvent()
	close(dlComplete)

	i.handleDownloadCompletion(context.Background(), dlComplete)

	repo.AssertExpectations(t)
	// The UUID tag is only cleaned up on the success path.
	qbitt.AssertNotCalled(t, "RemoveUUIDTag", mock.Anything, mock.Anything, mock.Anything)
}

// A failed delete must not take down the completion handler — the torrent is
// still processed, and a stale row only costs one wasted resume attempt.
func TestHandleDownloadCompletion_SurvivesDeleteFailure(t *testing.T) {
	repo := repomocks.NewRepository(t)
	mp := servicemocks.NewMediaProcessor(t)
	qbitt := servicemocks.NewTorrentService(t)

	mp.On("ProcessDownloadedTorrent", mock.Anything, mock.Anything).Return(nil)
	repo.On("UpdateDownloadHistoryStatus", mock.Anything, "hash-1", "success", "").Return(nil)
	repo.On("GetNotification", mock.Anything, "999").Return(&notifications.Notification{ID: 1}, nil)
	repo.On("UpdateNotification", mock.Anything, mock.Anything).Return(nil)
	repo.On("DeleteActiveTorrent", mock.Anything, "hash-1").Return(errors.New("db down"))
	qbitt.On("RemoveUUIDTag", mock.Anything, "hash-1", "uuid-1").Return(nil)

	i := NewDownloadInteractor(qbitt, mp, repo, testLogger())

	dlComplete := make(chan models.TorrentCompleteEvent, 1)
	dlComplete <- completionEvent()
	close(dlComplete)

	done := make(chan struct{})
	go func() {
		i.handleDownloadCompletion(context.Background(), dlComplete)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("completion handler hung after a failed delete")
	}

	repo.AssertExpectations(t)
}

func TestResumeInFlightDownloads_PropagatesError(t *testing.T) {
	repo := repomocks.NewRepository(t)
	mp := servicemocks.NewMediaProcessor(t)
	qbitt := servicemocks.NewTorrentService(t)

	qbitt.On("ResumeMonitors", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			// ResumeMonitors owns the channel, including on its error paths.
			close(args.Get(1).(chan<- models.TorrentCompleteEvent))
		}).
		Return(0, errors.New("db down"))

	i := NewDownloadInteractor(qbitt, mp, repo, testLogger())

	err := i.ResumeInFlightDownloads(context.Background())

	require.Error(t, err)
	qbitt.AssertExpectations(t)
}

func TestResumeInFlightDownloads_Success(t *testing.T) {
	repo := repomocks.NewRepository(t)
	mp := servicemocks.NewMediaProcessor(t)
	qbitt := servicemocks.NewTorrentService(t)

	qbitt.On("ResumeMonitors", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			close(args.Get(1).(chan<- models.TorrentCompleteEvent))
		}).
		Return(3, nil)

	i := NewDownloadInteractor(qbitt, mp, repo, testLogger())

	assert.NoError(t, i.ResumeInFlightDownloads(context.Background()))
	qbitt.AssertExpectations(t)
}
