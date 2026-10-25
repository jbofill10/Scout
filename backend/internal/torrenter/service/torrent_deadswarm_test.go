package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	repomocks "github.com/jbofill10/scout/backend/internal/torrenter/repository/mocks"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/notifications"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golift.io/starr/prowlarr"
)

func TestIsDeadSwarm(t *testing.T) {
	grace := 15 * time.Minute
	assert.True(t, isDeadSwarm(15*time.Minute, 0, 0, grace), "stalled past grace with no seeds and no transfer")
	assert.True(t, isDeadSwarm(2*time.Hour, 0, 0, grace))
	assert.False(t, isDeadSwarm(14*time.Minute, 0, 0, grace), "still inside the grace period")
	assert.False(t, isDeadSwarm(time.Hour, 1, 0, grace), "a connected seed means pieces can still arrive")
	assert.False(t, isDeadSwarm(time.Hour, 0, 4096, grace), "bytes are flowing, from leechers at least")
}

func TestDeadSwarmGrace(t *testing.T) {
	t.Setenv("DEAD_SWARM_GRACE", "")
	assert.Equal(t, defaultDeadSwarmGrace, deadSwarmGrace())
	t.Setenv("DEAD_SWARM_GRACE", "3m")
	assert.Equal(t, 3*time.Minute, deadSwarmGrace())
	t.Setenv("DEAD_SWARM_GRACE", "nope")
	assert.Equal(t, defaultDeadSwarmGrace, deadSwarmGrace())
}

func TestExcludedTorrents(t *testing.T) {
	ex := newExcludedTorrents([]*models.FailedTorrent{
		{InfoHash: "ABCDEF", Title: "Some.Show.S01E05.1080p.WEB-GRP"},
		{Title: "Some Show S01E05 1080p WEB OTHER [eztv]"}, // no hash known
	})

	hits := []*prowlarr.Search{
		{Title: "Some.Show.S01E05.1080p.WEB-GRP", InfoHash: "abcdef"},    // hash match, case-insensitive
		{Title: "Some Show S01E05 1080p WEB OTHER [eztv]", InfoHash: ""}, // title match
		{Title: "Some.Show.S01E05.1080p.WEB-GRP", InfoHash: "123456"},    // same title, different hash: still the same release
		{Title: "Some.Show.S01E05.1080p.WEB-FRESH", InfoHash: "999999"},
	}
	dropped := ex.drop(&hits)

	assert.Equal(t, 3, dropped)
	require.Len(t, hits, 1)
	assert.Equal(t, "Some.Show.S01E05.1080p.WEB-FRESH", hits[0].Title)

	var none *excludedTorrents
	empty := []*prowlarr.Search{{Title: "x"}}
	assert.Equal(t, 0, none.drop(&empty), "a nil set excludes nothing")
	assert.Len(t, empty, 1)
}

// Titles recorded from qBittorrent are HTML-encoded ("Marvel&#039;s") while
// Prowlarr's are not; the two must still match.
func TestExcludedTorrents_NormalizesTitles(t *testing.T) {
	ex := newExcludedTorrents([]*models.FailedTorrent{
		{Title: "Marvel&#039;s The Punisher S01E12 (1080p NF WEBRip x265 HEVC 10bit AC3 5.1 Qman) [UTR]"},
	})
	hits := []*prowlarr.Search{
		{Title: "Marvel's The Punisher S01E12 (1080p NF WEBRip x265 HEVC 10bit AC3 5.1 Qman) [UTR]"},
		{Title: "MARVEL'S  The Punisher S01E12 (1080p NF WEBRip x265 HEVC 10bit AC3 5.1 Qman) [UTR]"},
		{Title: "Marvel's The Punisher S01E12 720p WEB x264 STRiFE [eztv]"},
	}
	assert.Equal(t, 2, ex.drop(&hits))
	require.Len(t, hits, 1)
	assert.Contains(t, hits[0].Title, "720p")
}

// A release that already failed for this item is skipped, and the search
// takes the next best one instead.
func TestSearchForMatch_SkipsPreviouslyFailedTorrents(t *testing.T) {
	fake := &fakeProwlarr{t: t, results: map[int64][]*prowlarr.Search{
		ONE337x_ID: {
			hitWithSeeders(ONE337x_ID, "Some.Show.S01E05.1080p.WEB.x264-DEAD", 60),
			hitWithSeeders(ONE337x_ID, "Some.Show.S01E05.1080p.WEB.x264-ALIVE", 20),
		},
	}}
	q := newFallbackTestHandler(t, fake)
	req, strategies := someShowStrategies(q)
	excluded := newExcludedTorrents([]*models.FailedTorrent{{Title: "Some.Show.S01E05.1080p.WEB.x264-DEAD"}})

	outcome := q.searchForMatch(context.Background(), strategies, req, excluded)

	require.NotNil(t, outcome.match)
	assert.Equal(t, "Some.Show.S01E05.1080p.WEB.x264-ALIVE", outcome.match.Torrent.Title)
}

func TestSearchRequestFor(t *testing.T) {
	req := &tvdb.Media{Id: "9001", Name: "Some Show", Category: "series", Aliases: []string{"SS"}}
	req.Metadata.Episodes = []tvdb.Episode{{Id: 55, SeasonNumber: 1, Number: 5}}
	ss := &models.SearchStrategy{EpisodeMeta: &req.Metadata.Episodes[0]}

	sr := searchRequestFor(req, ss)

	assert.Equal(t, "9001", sr.Media.Id)
	assert.Equal(t, []string{"SS"}, sr.Media.Aliases)
	assert.Empty(t, sr.Media.Metadata.Episodes, "the episode list is not carried along")
	require.NotNil(t, sr.Episode)
	assert.Equal(t, 55, sr.Episode.Id)

	movie := searchRequestFor(&tvdb.Media{Id: "77", Category: "movie"}, &models.SearchStrategy{IsMovie: true})
	assert.Nil(t, movie.Episode)
}

func deadSwarmTestStrategy() *models.SearchStrategy {
	return &models.SearchStrategy{MediaName: "Some Show", TvdbId: "9001", EpisodeTvdbID: "55", Season: 1, Episode: 5}
}

// Without a recorded search request (rows from before it existed) there is
// nothing to search with, so the item is reported failed rather than left
// silently downloading.
func TestReplaceDeadTorrent_NoRequestReportsFailed(t *testing.T) {
	repo := repomocks.NewRepository(t)
	repo.On("GetNotification", mock.Anything, "55").Return(&notifications.Notification{ID: 1, TvdbID: "55"}, nil)
	repo.On("UpdateNotification", mock.Anything, mock.MatchedBy(func(n *notifications.Notification) bool {
		return n.Status == notifications.StatusFailed
	})).Return(nil)
	q := newResumeTestHandler(t, repo)

	var wg sync.WaitGroup
	q.replaceDeadTorrent(context.Background(), nil, deadSwarmTestStrategy(), make(chan models.TorrentCompleteEvent, 1), &wg)
}

// After the replacement budget is spent the item is reported failed with the
// try count, and no further search happens.
func TestReplaceDeadTorrent_BudgetExhaustedReportsFailed(t *testing.T) {
	repo := repomocks.NewRepository(t)
	repo.On("GetNotification", mock.Anything, "55").Return(&notifications.Notification{ID: 1, TvdbID: "55"}, nil)
	repo.On("UpdateNotification", mock.Anything, mock.MatchedBy(func(n *notifications.Notification) bool {
		return n.Status == notifications.StatusFailed && n.Reason == "No copy with live seeds found after 4 tries"
	})).Return(nil)
	q := newResumeTestHandler(t, repo)

	request := &models.SearchRequest{Media: tvdb.Media{Id: "9001", Name: "Some Show", Category: "series"}, Replaced: maxDeadReplacements}
	var wg sync.WaitGroup
	q.replaceDeadTorrent(context.Background(), request, deadSwarmTestStrategy(), make(chan models.TorrentCompleteEvent, 1), &wg)
	assert.Equal(t, maxDeadReplacements, request.Replaced, "no further replacement is counted")
}

// With budget left, the replacement search runs with the failed releases
// excluded; when nothing else exists the item is reported failed.
func TestReplaceDeadTorrent_SearchesAgainExcludingFailed(t *testing.T) {
	fake := &fakeProwlarr{t: t, results: map[int64][]*prowlarr.Search{
		ONE337x_ID: {hitWithSeeders(ONE337x_ID, "Some.Show.S01E05.1080p.WEB.x264-DEAD", 60)},
	}}
	q := newFallbackTestHandler(t, fake)
	repo := q.repo.(*repomocks.Repository)
	repo.On("GetFailedTorrents", mock.Anything, "9001", 1, 5).Return([]*models.FailedTorrent{
		{TvdbID: "9001", Season: 1, Episode: 5, Title: "Some.Show.S01E05.1080p.WEB.x264-DEAD"},
	}, nil)
	repo.On("GetNotification", mock.Anything, "55").Return(&notifications.Notification{ID: 1, TvdbID: "55"}, nil)
	statuses := []notifications.NotificationStatus{}
	repo.On("UpdateNotification", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		statuses = append(statuses, args.Get(1).(*notifications.Notification).Status)
	}).Return(nil)

	req := &tvdb.Media{Id: "9001", Name: "Some Show", Category: "series"}
	ep := tvdb.Episode{Id: 55, SeasonNumber: 1, Number: 5}
	request := &models.SearchRequest{Media: *req, Episode: &ep}

	var wg sync.WaitGroup
	q.replaceDeadTorrent(context.Background(), request, deadSwarmTestStrategy(), make(chan models.TorrentCompleteEvent, 1), &wg)

	assert.Equal(t, 1, request.Replaced)
	assert.Equal(t, []notifications.NotificationStatus{notifications.StatusSearching, notifications.StatusFailed}, statuses,
		"searching while looking, failed once the only release turned out to be the dead one")
}
