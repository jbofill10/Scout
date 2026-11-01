package interactors

import (
	"context"
	"testing"

	repoMocks "github.com/jbofill10/scout/backend/internal/webserver/repository/mocks"
	"github.com/jbofill10/scout/backend/pkg/dlstatus"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/settings"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func threeAiredEpisodes() tvdb.Media {
	req := tvdb.Media{Id: "555", Name: "Test Show", Category: "series"}
	req.Metadata.Episodes = []tvdb.Episode{
		{Id: 1, SeasonNumber: 5, Number: 1, Aired: aired(-30)},
		{Id: 2, SeasonNumber: 5, Number: 2, Aired: aired(-30)},
		{Id: 3, SeasonNumber: 6, Number: 1, Aired: aired(-30)},
	}
	return req
}

// With packs on, the whole episode set goes to the torrenter once as a
// pack-only request; what a pack covered is recorded as dispatched and the
// rest is sent one episode at a time.
func TestDownloadShow_PacksFirstThenPerEpisode(t *testing.T) {
	repo := &fakeSchedulerRepo{}
	notifRepo := repoMocks.NewNotificationRepository(t)
	notifRepo.On("CreateNotification", mock.Anything, mock.Anything).Return(nil)
	dispatcher := &fakeDispatcher{
		settingsView: &settings.View{Values: settings.Values{SeasonPacks: true, SeasonPackMinEpisodes: 2}},
		respFor: func(req tvdb.Media) dlstatus.DownloadResponse {
			if req.PackOnly {
				return dlstatus.DownloadResponse{Results: []dlstatus.EpisodeResult{
					epResult("1", dlstatus.OutcomeDownloading, ""),
					epResult("2", dlstatus.OutcomeDownloading, ""),
					epResult("3", dlstatus.OutcomeFailed, dlstatus.CodeNoPack),
				}}
			}
			return dlstatus.DownloadResponse{Results: []dlstatus.EpisodeResult{
				{TvdbID: "3", Outcome: dlstatus.OutcomeDownloading, Season: 6, Episode: 1},
			}}
		},
	}
	i := newSummaryInteractor(t, repo, notifRepo, dispatcher)

	summary, err := i.DownloadShow(context.Background(), threeAiredEpisodes())
	require.NoError(t, err)
	assert.Equal(t, 3, summary.QueuedNow)

	payloads := dispatcher.payloads()
	require.Len(t, payloads, 2, "one pack-only request, then one per uncovered episode")
	assert.True(t, payloads[0].PackOnly)
	assert.Len(t, payloads[0].Metadata.Episodes, 3)
	assert.False(t, payloads[1].PackOnly)
	require.Len(t, payloads[1].Metadata.Episodes, 1)
	assert.Equal(t, 3, payloads[1].Metadata.Episodes[0].Id)

	require.Len(t, repo.dispatched, 3, "every started episode has a row to retry from")
	assert.Equal(t, 0, repo.retryCount)
}

// With packs off, or the torrenter's settings unreadable, episodes go one at
// a time and no pack-only request is ever made.
func TestDownloadShow_PacksOffSendsOneAtATime(t *testing.T) {
	for name, view := range map[string]*settings.View{
		"off":         {Values: settings.Values{SeasonPacks: false}},
		"unreadable":  nil,
		"under limit": {Values: settings.Values{SeasonPacks: true, SeasonPackMinEpisodes: 4}},
	} {
		t.Run(name, func(t *testing.T) {
			repo := &fakeSchedulerRepo{}
			notifRepo := repoMocks.NewNotificationRepository(t)
			notifRepo.On("CreateNotification", mock.Anything, mock.Anything).Return(nil)
			dispatcher := &fakeDispatcher{settingsView: view, respFor: func(req tvdb.Media) dlstatus.DownloadResponse {
				return dlstatus.DownloadResponse{Results: []dlstatus.EpisodeResult{
					{TvdbID: "x", Outcome: dlstatus.OutcomeDownloading},
				}}
			}}
			i := newSummaryInteractor(t, repo, notifRepo, dispatcher)

			_, err := i.DownloadShow(context.Background(), threeAiredEpisodes())
			require.NoError(t, err)

			payloads := dispatcher.payloads()
			require.Len(t, payloads, 3)
			for _, p := range payloads {
				assert.False(t, p.PackOnly)
				assert.Len(t, p.Metadata.Episodes, 1)
			}
		})
	}
}

// A pack-only answer that fails an episode for another reason is settled
// like any immediate failure: a retry row, not a per-episode dispatch.
func TestDownloadShow_PackOnlyOtherFailuresScheduleRetries(t *testing.T) {
	repo := &fakeSchedulerRepo{}
	notifRepo := repoMocks.NewNotificationRepository(t)
	notifRepo.On("CreateNotification", mock.Anything, mock.Anything).Return(nil)
	notifRepo.On("GetNotification", mock.Anything, mock.Anything).Return(nil, nil).Maybe()
	notifRepo.On("UpdateNotification", mock.Anything, mock.Anything).Return(nil).Maybe()
	dispatcher := &fakeDispatcher{
		settingsView: &settings.View{Values: settings.Values{SeasonPacks: true, SeasonPackMinEpisodes: 2}},
		respFor: func(req tvdb.Media) dlstatus.DownloadResponse {
			return dlstatus.DownloadResponse{Results: []dlstatus.EpisodeResult{
				epResult("1", dlstatus.OutcomeDownloading, ""),
				epResult("2", dlstatus.OutcomeDownloading, ""),
				epResult("3", dlstatus.OutcomeFailed, dlstatus.CodeInsufficientDiskSpace),
			}}
		},
	}
	i := newSummaryInteractor(t, repo, notifRepo, dispatcher)

	_, err := i.DownloadShow(context.Background(), threeAiredEpisodes())
	require.NoError(t, err)

	assert.Equal(t, 1, dispatcher.callCount(), "nothing is sent per episode")
	assert.Equal(t, 1, repo.retryCount, "the disk-space failure gets a retry row")
	assert.Len(t, repo.dispatched, 2)
}
