package service

import (
	"context"
	"testing"
	"time"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	repomocks "github.com/jbofill10/scout/backend/internal/torrenter/repository/mocks"
	"github.com/jbofill10/scout/backend/pkg/controls"
	"github.com/jbofill10/scout/backend/pkg/dlstatus"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golift.io/starr/prowlarr"
)

func TestCandidateKey(t *testing.T) {
	assert.Equal(t, "h:abc", candidateKey(&prowlarr.Search{InfoHash: "ABC", GUID: "g", Title: "t"}), "hash wins")
	assert.Equal(t, "g:g", candidateKey(&prowlarr.Search{GUID: "g", Title: "t"}), "then guid")
	assert.Equal(t, "t:some show s01e05", candidateKey(&prowlarr.Search{Title: "Some  Show S01E05"}), "then normalised title")
}

func TestSortCandidates(t *testing.T) {
	cs := []controls.ReleaseCandidate{
		{Title: "rejected-many-seeds", Seeders: 500},
		{Title: "accepted-low", Accepted: true, Score: 1.0, Seeders: 10},
		{Title: "rejected-few-seeds", Seeders: 2},
		{Title: "accepted-high", Accepted: true, Score: 2.0, Seeders: 1},
	}
	sortCandidates(cs)
	got := []string{}
	for _, c := range cs {
		got = append(got, c.Title)
	}
	assert.Equal(t, []string{"accepted-high", "accepted-low", "rejected-many-seeds", "rejected-few-seeds"}, got)
}

func TestResolveItemMedia(t *testing.T) {
	t.Run("posted series media must carry one episode", func(t *testing.T) {
		q := newResumeTestHandler(t, repomocks.NewRepository(t))
		_, err := q.ResolveItemMedia(context.Background(), "55", &tvdb.Media{Category: "series"})
		assert.Error(t, err)
		m, err := q.ResolveItemMedia(context.Background(), "55",
			&tvdb.Media{Category: "series", Metadata: tvdb.TVDBSeriesMetadata{Episodes: []tvdb.Episode{{Id: 55}}}})
		require.NoError(t, err)
		assert.Len(t, m.Metadata.Episodes, 1)
	})

	t.Run("falls back to the in-flight torrent's request", func(t *testing.T) {
		repo := repomocks.NewRepository(t)
		repo.On("GetActiveTorrents", mock.Anything).Return([]*models.ActiveTorrent{
			{InfoHash: "x", Strategy: &models.SearchStrategy{EpisodeTvdbID: "55"}, Request: &models.SearchRequest{
				Media:   tvdb.Media{Id: "9001", Name: "Some Show", Category: "series"},
				Episode: &tvdb.Episode{Id: 55, SeasonNumber: 1, Number: 5},
			}},
		}, nil)
		q := newResumeTestHandler(t, repo)
		m, err := q.ResolveItemMedia(context.Background(), "55", nil)
		require.NoError(t, err)
		assert.Equal(t, "Some Show", m.Name)
		require.Len(t, m.Metadata.Episodes, 1)
		assert.Equal(t, 5, m.Metadata.Episodes[0].Number)
	})

	t.Run("unknown item", func(t *testing.T) {
		repo := repomocks.NewRepository(t)
		repo.On("GetActiveTorrents", mock.Anything).Return([]*models.ActiveTorrent{}, nil)
		q := newResumeTestHandler(t, repo)
		_, err := q.ResolveItemMedia(context.Background(), "55", nil)
		assert.ErrorIs(t, err, ErrUnknownItem)
	})
}

// The release listing consults every indexer tier, collapses duplicates, and
// reports the picker's verdict on each hit: accepted with a score, or the
// reason it was passed over, plus whether it already failed for the item.
func TestListReleases_ReportsVerdictsAcrossIndexerTiers(t *testing.T) {
	good := hitWithSeeders(ONE337x_ID, "Some.Show.S01E05.1080p.WEB.x264-GRACE", 40)
	sd := hitWithSeeders(ONE337x_ID, "Some.Show.S01E05.720p.WEB.x264-GRACE", 300)
	wrongEpisode := hitWithSeeders(ONE337x_ID, "Some.Show.S01E06.1080p.WEB.x264-GRACE", 20)
	tpbGood := hitWithSeeders(TPB_ID, "Some Show S01E05 2160p WEB h265-OTHER", 15)
	tpbDead := hitWithSeeders(TPB_ID, "Some.Show.S01E05.1080p.WEB.x264-DEAD", 1)
	fake := &fakeProwlarr{t: t, results: map[int64][]*prowlarr.Search{
		ONE337x_ID: {good, sd, wrongEpisode},
		TPB_ID:     {tpbGood, tpbDead, good}, // good again: must collapse
	}}
	q := newFallbackTestHandler(t, fake)
	q.minQualityTier = QualityTier("1080p")
	repo := q.repo.(*repomocks.Repository)
	repo.On("GetFailedTorrents", mock.Anything, "9001", 1, 5).Return([]*models.FailedTorrent{
		{InfoHash: tpbDead.InfoHash, Title: tpbDead.Title},
	}, nil)

	media := &tvdb.Media{Id: "9001", Name: "Some Show", Category: "series",
		Metadata: tvdb.TVDBSeriesMetadata{Episodes: []tvdb.Episode{{Id: 55, SeasonNumber: 1, Number: 5}}}}

	resp, err := q.ListReleases(context.Background(), "55", media)

	require.NoError(t, err)
	assert.Equal(t, "55", resp.ItemID)
	assert.Contains(t, fake.indexersQueried(), TPB_ID, "the fallback tier is searched even though 1337x had a match")
	assert.Greater(t, resp.Queries, 1)

	byTitle := map[string]controls.ReleaseCandidate{}
	for _, c := range resp.Candidates {
		byTitle[c.Title] = c
	}
	require.Len(t, byTitle, 5, "the duplicate hit collapses into one candidate")

	assert.True(t, byTitle[good.Title].Accepted)
	assert.Greater(t, byTitle[good.Title].Score, 0.0)
	assert.Equal(t, "1080p", byTitle[good.Title].Quality)
	assert.True(t, byTitle[tpbGood.Title].Accepted)

	assert.False(t, byTitle[sd.Title].Accepted)
	assert.Contains(t, byTitle[sd.Title].Reason, "minimum quality")
	assert.False(t, byTitle[wrongEpisode.Title].Accepted)
	assert.NotEmpty(t, byTitle[wrongEpisode.Title].Reason)
	assert.True(t, byTitle[tpbDead.Title].PreviouslyFailed)

	assert.True(t, resp.Candidates[0].Accepted, "accepted candidates come first")
	for _, c := range resp.Candidates {
		assert.NotEmpty(t, c.GUID)
		assert.NotEmpty(t, c.Query)
	}
}

func TestDownloadRelease_RejectsMediaForAnotherItem(t *testing.T) {
	q := newResumeTestHandler(t, repomocks.NewRepository(t))
	media := &tvdb.Media{Id: "9001", Name: "Some Show", Category: "series",
		Metadata: tvdb.TVDBSeriesMetadata{Episodes: []tvdb.Episode{{Id: 55, SeasonNumber: 1, Number: 5}}}}
	done := make(chan models.TorrentCompleteEvent, 1)

	_, err := q.DownloadRelease(context.Background(), "56", media, controls.ReleaseSelection{Title: "x", GUID: "magnet:?x"}, done)

	assert.Error(t, err)
	assert.True(t, closed(t, done), "done is closed on every path")
}

// A manual grab first clears what was in flight for the item, then hands the
// chosen release to qBittorrent. When the add fails the result says so and the
// completion channel still closes.
func TestDownloadRelease_AddFailureIsReported(t *testing.T) {
	origSavePath := baseSavePath
	baseSavePath = t.TempDir()
	defer func() { baseSavePath = origSavePath }()

	repo := repomocks.NewRepository(t)
	repo.On("GetActiveTorrents", mock.Anything).Return([]*models.ActiveTorrent{
		{InfoHash: "old", TorrentTitle: "old pick", Strategy: &models.SearchStrategy{EpisodeTvdbID: "55"}},
	}, nil)
	repo.On("DeleteActiveTorrent", mock.Anything, "old").Return(nil)
	repo.On("GetNotification", mock.Anything, "55").Return(nil, nil).Maybe()
	repo.On("InsertDownloadHistory", mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	qb := &fakeQbittorrent{addStatus: 500}
	q := newCancelTestHandler(t, repo, qb)

	media := &tvdb.Media{Id: "9001", Name: "Some Show", Category: "series",
		Metadata: tvdb.TVDBSeriesMetadata{Episodes: []tvdb.Episode{{Id: 55, SeasonNumber: 1, Number: 5}}}}
	done := make(chan models.TorrentCompleteEvent, 1)
	sel := controls.ReleaseSelection{Title: "Some.Show.S01E05.1080p.WEB.x264-GRACE", GUID: "magnet:?xt=urn:btih:abc", Seeders: 40}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := q.DownloadRelease(ctx, "55", media, sel, done)

	require.NoError(t, err)
	assert.Equal(t, dlstatus.OutcomeFailed, result.Outcome)
	assert.Equal(t, dlstatus.CodeTorrentClientError, result.Code)
	assert.Equal(t, []string{"old"}, qb.deletedHashes(), "the previous torrent for the item was removed first")
	assert.True(t, closed(t, done))
	repo.AssertNotCalled(t, "InsertActiveTorrent", mock.Anything, mock.Anything)
}
