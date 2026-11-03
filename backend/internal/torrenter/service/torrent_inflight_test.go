package service

import (
	"context"
	"errors"
	"testing"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	repomocks "github.com/jbofill10/scout/backend/internal/torrenter/repository/mocks"
	"github.com/jbofill10/scout/backend/pkg/dlstatus"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/notifications"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func inFlightTestEpisodes() []tvdb.Episode {
	return []tvdb.Episode{
		{Id: 11, SeasonNumber: 1, Number: 1},
		{Id: 12, SeasonNumber: 1, Number: 2},
		{Id: 13, SeasonNumber: 1, Number: 3},
	}
}

// An episode whose torrent is already in flight is not searched again: it is
// reported as downloading and its notification is put back on downloading,
// undoing the "searching — retry scheduled" the retry engine may have set.
func TestFilterInFlightEpisodes_SkipsEpisodesAlreadyDownloading(t *testing.T) {
	repo := repomocks.NewRepository(t)
	repo.On("GetActiveTorrents", mock.Anything).Return([]*models.ActiveTorrent{
		{InfoHash: "abc", Strategy: &models.SearchStrategy{EpisodeTvdbID: "12", Season: 1, Episode: 2}},
		{InfoHash: "orphan", Strategy: nil}, // unreadable rows must not panic
	}, nil)
	repo.On("GetNotification", mock.Anything, "12").Return(
		&notifications.Notification{ID: 7, TvdbID: "12", Status: notifications.StatusSearching, Reason: "retry scheduled"}, nil)
	repo.On("UpdateNotification", mock.Anything, mock.MatchedBy(func(n *notifications.Notification) bool {
		return n.ID == 7 && n.Status == notifications.StatusDownloading && n.Reason == ""
	})).Return(nil)

	q := newResumeTestHandler(t, repo)
	req := &tvdb.Media{Id: "555", Name: "Test Show", Category: "series"}

	toDownload, skipped := q.filterInFlightEpisodes(context.Background(), req, inFlightTestEpisodes())

	require.Len(t, toDownload, 2)
	assert.Equal(t, 11, toDownload[0].Id)
	assert.Equal(t, 13, toDownload[1].Id)

	require.Len(t, skipped, 1)
	assert.Equal(t, dlstatus.EpisodeResult{TvdbID: "12", Season: 1, Episode: 2, Outcome: dlstatus.OutcomeDownloading}, skipped[0])
}

// The in-flight check is best effort: if the table cannot be read, every
// episode is searched as before.
func TestFilterInFlightEpisodes_RepoErrorSearchesEverything(t *testing.T) {
	repo := repomocks.NewRepository(t)
	repo.On("GetActiveTorrents", mock.Anything).Return(nil, errors.New("db down"))

	q := newResumeTestHandler(t, repo)
	req := &tvdb.Media{Id: "555", Name: "Test Show", Category: "series"}

	toDownload, skipped := q.filterInFlightEpisodes(context.Background(), req, inFlightTestEpisodes())

	assert.Len(t, toDownload, 3)
	assert.Empty(t, skipped)
}

func TestFilterInFlightEpisodes_NothingInFlightLeavesEpisodesAlone(t *testing.T) {
	repo := repomocks.NewRepository(t)
	repo.On("GetActiveTorrents", mock.Anything).Return([]*models.ActiveTorrent{}, nil)

	q := newResumeTestHandler(t, repo)
	req := &tvdb.Media{Id: "555", Name: "Test Show", Category: "series"}

	toDownload, skipped := q.filterInFlightEpisodes(context.Background(), req, inFlightTestEpisodes())

	assert.Len(t, toDownload, 3)
	assert.Empty(t, skipped)
}

// A movie is keyed by its media id, and a repeat request for one already in
// flight must not start a second search.
func TestHandleDownload_MovieAlreadyInFlightIsReportedDownloading(t *testing.T) {
	repo := repomocks.NewRepository(t)
	repo.On("MovieExistsByTvdbId", mock.Anything, "900").Return(false, nil)
	repo.On("GetActiveTorrents", mock.Anything).Return([]*models.ActiveTorrent{
		{InfoHash: "abc", Strategy: &models.SearchStrategy{EpisodeTvdbID: "900", IsMovie: true}},
	}, nil)
	repo.On("GetNotification", mock.Anything, "900").Return(nil, nil)

	q := newResumeTestHandler(t, repo)
	req := &tvdb.Media{Id: "900", Name: "Test Movie", Category: "movie"}
	done := make(chan models.TorrentCompleteEvent, 1)

	results, err := q.HandleDownload(context.Background(), req, done)

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, dlstatus.EpisodeResult{TvdbID: "900", Outcome: dlstatus.OutcomeDownloading}, results[0])
	assert.True(t, closed(t, done), "done must be closed so the completion handler exits")
}

// A search another request started counts as in flight too, so a repeat
// request made while it runs does not start a second one. The notification is
// that search's to update, so it is not touched here.
func TestFilterInFlightEpisodes_SkipsEpisodesBeingSearched(t *testing.T) {
	repo := repomocks.NewRepository(t)
	repo.On("GetActiveTorrents", mock.Anything).Return([]*models.ActiveTorrent{}, nil)

	q := newResumeTestHandler(t, repo)
	q.inflight = newInflightRegistry()
	unregister := q.inflight.add("12", &inflightWork{cancel: func() {}})
	defer unregister()
	req := &tvdb.Media{Id: "555", Name: "Test Show", Category: "series"}

	toDownload, skipped := q.filterInFlightEpisodes(context.Background(), req, inFlightTestEpisodes())

	require.Len(t, toDownload, 2)
	assert.Equal(t, 11, toDownload[0].Id)
	assert.Equal(t, 13, toDownload[1].Id)
	require.Len(t, skipped, 1)
	assert.Equal(t, dlstatus.EpisodeResult{TvdbID: "12", Season: 1, Episode: 2, Outcome: dlstatus.OutcomeDownloading}, skipped[0])
	repo.AssertNotCalled(t, "GetNotification", mock.Anything, "12")
}

// Two requests for the same movie seconds apart used to run two searches and
// leave one of their notifications stranded on "searching"; the second now
// defers to the first without touching the database.
func TestHandleDownload_MovieBeingSearchedIsReportedDownloading(t *testing.T) {
	repo := repomocks.NewRepository(t)
	repo.On("MovieExistsByTvdbId", mock.Anything, "900").Return(false, nil)

	q := newResumeTestHandler(t, repo)
	q.inflight = newInflightRegistry()
	unregister := q.inflight.add("900", &inflightWork{cancel: func() {}})
	defer unregister()
	req := &tvdb.Media{Id: "900", Name: "Test Movie", Category: "movie"}
	done := make(chan models.TorrentCompleteEvent, 1)

	results, err := q.HandleDownload(context.Background(), req, done)

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, dlstatus.EpisodeResult{TvdbID: "900", Outcome: dlstatus.OutcomeDownloading}, results[0])
	assert.True(t, closed(t, done), "done must be closed so the completion handler exits")
}

// Only a running search counts: a monitored download is found through
// ActiveTorrents, and a nil registry (handlers built by hand) is inert.
func TestInflightRegistry_Searching(t *testing.T) {
	var none *inflightRegistry
	assert.False(t, none.searching("1"))

	r := newInflightRegistry()
	assert.False(t, r.searching("1"))

	unregisterMonitor := r.add("1", &inflightWork{cancel: func() {}, infoHash: "abc"})
	assert.False(t, r.searching("1"), "a monitored torrent is not a search")

	unregisterSearch := r.add("1", &inflightWork{cancel: func() {}})
	assert.True(t, r.searching("1"))

	unregisterSearch()
	assert.False(t, r.searching("1"))
	unregisterMonitor()
	assert.False(t, r.searching("1"))
}
