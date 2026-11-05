package service

import (
	"context"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	repomocks "github.com/jbofill10/scout/backend/internal/torrenter/repository/mocks"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"

	qbittorrent "github.com/autobrr/go-qbittorrent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golift.io/starr"
	"golift.io/starr/prowlarr"
)

func TestCrawlETA(t *testing.T) {
	hour := time.Hour
	assert.Equal(t, time.Duration(1<<63-1), crawlETA(hour, &qbittorrent.Torrent{Size: 1000, Completed: 0}), "nothing landed yet: forever")
	assert.Equal(t, 9*hour, crawlETA(hour, &qbittorrent.Torrent{Size: 1000, Completed: 100}), "10% in an hour: nine more")
	assert.Equal(t, time.Duration(0), crawlETA(hour, &qbittorrent.Torrent{Size: 1000, Completed: 1000}))
}

// JoJo's S05E36 on 2026-11-04: 7% after two hours from one connected seed at a
// few KB/s, days to go, never replaced because it kept moving.
func TestIsCrawling(t *testing.T) {
	rules := crawlRules{grace: 30 * time.Minute, maxETA: 24 * time.Hour, recheck: 30 * time.Minute, maxSeeds: 1}
	crawler := &qbittorrent.Torrent{Size: 950_000_000, Completed: 66_000_000, Progress: 0.07, NumSeeds: 1, DlSpeed: 3000}

	assert.True(t, isCrawling(2*time.Hour, crawler, rules), "7% in two hours is a 27-hour download")
	assert.False(t, isCrawling(10*time.Minute, crawler, rules), "too early to judge")
	wellSeeded := *crawler
	wellSeeded.NumSeeds = 12
	assert.False(t, isCrawling(2*time.Hour, &wellSeeded, rules), "a seeded swarm that is slow is our own bandwidth")
	nearlyDone := *crawler
	nearlyDone.Completed, nearlyDone.Progress = 600_000_000, 0.63
	assert.False(t, isCrawling(2*time.Hour, &nearlyDone, rules), "past halfway it is kept")
	healthy := &qbittorrent.Torrent{Size: 950_000_000, Completed: 400_000_000, Progress: 0.42, NumSeeds: 1}
	assert.False(t, isCrawling(time.Hour, healthy, rules), "42% an hour in finishes within the day")
	queuedThenFast := &qbittorrent.Torrent{Size: 950_000_000, Completed: 30_000_000, Progress: 0.03, NumSeeds: 1, DlSpeed: 2_000_000}
	assert.False(t, isCrawling(6*time.Hour, queuedThenFast, rules), "hours in the queue spoil the average, but 2 MB/s now finishes in minutes")
}

// The release that could have replaced the crawler names its arc: "JoJo's
// Bizarre Adventure Part 6: Stone Ocean". That is the show.
func TestValidateShowTorrent_AnimeArcDesignationIsTheShow(t *testing.T) {
	q := validateHandler()
	ep := &tvdb.Episode{Id: 9388232, SeasonNumber: 5, Number: 36, AbsoluteNumber: 188}
	names := []string{"JoJo's Bizarre Adventure", "JoJo no Kimyou na Bouken"}
	release := &prowlarr.Search{Title: "[Anime Chap] JoJo's Bizarre Adventure Part 6: Stone Ocean S05E36 [WEB 1080p] " +
		"{Corrected Stand Names & OP & ED Lyrics & Netflix Logos Removed} Episode 36 [Dual Audio]"}

	seasonal := &models.SearchStrategy{Query: "JoJo's Bizarre Adventure S05E36", MediaName: names[0], Names: names,
		Season: 5, Episode: 36, EpisodeMeta: ep}
	result := q.validateShowTorrent(release, seasonal)
	assert.True(t, result.IsValid, result.Reason)

	assert.Equal(t, []string{"JoJo's Bizarre Adventure Part 6: Stone Ocean", "JoJo's Bizarre Adventure"},
		showNameForms("JoJo's Bizarre Adventure Part 6: Stone Ocean"))
	assert.Equal(t, []string{"Dragon Ball DAIMA"}, showNameForms("Dragon Ball DAIMA"), "no arc, no second form")
	assert.Equal(t, "Attack on Titan", trimArcDesignation("Attack on Titan Part 2"))
}

func crawlEnv(t *testing.T) {
	t.Helper()
	t.Setenv("CRAWL_GRACE", "30ms")
	t.Setenv("CRAWL_MAX_ETA", "1ns")
	t.Setenv("CRAWL_RECHECK", "10ms")
	t.Setenv("MONITOR_TIMEOUT", "1h")
	t.Setenv("DEAD_SWARM_GRACE", "1h")
	fastPolls(t)
}

func crawlRepo(t *testing.T) *repomocks.Repository {
	repo := repomocks.NewRepository(t)
	repo.On("GetFailedTorrents", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Maybe()
	repo.On("InsertFailedTorrent", mock.Anything, mock.Anything).Return(nil).Maybe()
	repo.On("InsertActiveTorrent", mock.Anything, mock.Anything).Return(nil).Maybe()
	repo.On("DeleteActiveTorrent", mock.Anything, mock.Anything).Return(nil).Maybe()
	repo.On("GetNotification", mock.Anything, mock.Anything).Return(nil, nil).Maybe()
	repo.On("GetPreferredUploaders", mock.Anything, mock.Anything, mock.Anything).Return([]string{}, nil).Maybe()
	repo.On("InsertDownloadHistory", mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	return repo
}

func crawlMatch() (*models.TorrentMatch, *models.SearchRequest) {
	media := tvdb.Media{Id: "9001", Name: "Some Show", Category: "series"}
	ep := &tvdb.Episode{Id: 55, SeasonNumber: 1, Number: 5}
	match := &models.TorrentMatch{
		Strategy: &models.SearchStrategy{MediaName: "Some Show", Names: []string{"Some Show"}, Season: 1, Episode: 5,
			EpisodeMeta: ep, TvdbId: "9001", EpisodeTvdbID: "55"},
		Torrent: &prowlarr.Search{Title: "Some Show S01E05 1080p WEB-SLOW", InfoHash: "crawl-hash"},
	}
	return match, &models.SearchRequest{Media: media, Episode: ep}
}

// A crawling torrent is swapped for the copy the indexers list: the crawler is
// removed and blacklisted only once the replacement is in hand, and the
// replacement's completion comes through on the same request.
func TestMonitor_CrawlingTorrentSwappedForFasterCopy(t *testing.T) {
	crawlEnv(t)
	qb := &fakeQbittorrent{}
	qb.info = func(poll int) []map[string]any {
		for _, h := range qb.deletedHashes() {
			if h == "crawl-hash" { // the swap happened: the new torrent is what qBittorrent now reports
				return []map[string]any{qbitTorrent("someshows01e051080pweb-fast", 1, 20, 0, "stalledUP")}
			}
		}
		t := qbitTorrent("crawl-hash", 0.05, 1, 100, "downloading")
		t["name"] = "Some Show S01E05 1080p WEB-SLOW"
		return []map[string]any{t}
	}
	fake := &fakeProwlarr{t: t, results: map[int64][]*prowlarr.Search{
		ONE337x_ID: {hitWithSeeders(ONE337x_ID, "Some Show S01E05 1080p WEB-FAST", 40)},
	}}
	prowlarrServer := httptest.NewServer(fake.handler())
	t.Cleanup(prowlarrServer.Close)
	q := newCancelTestHandler(t, crawlRepo(t), qb)
	q.p = prowlarr.New(starr.New("test-key", prowlarrServer.URL, 5*time.Second))
	q.minQualityTier = QualityTier("1080p")
	tempSavePath(t)

	match, request := crawlMatch()
	var wg sync.WaitGroup
	done := make(chan models.TorrentCompleteEvent, 1)
	wg.Add(1)
	go q.monitorTorrentCompletion(context.Background(), match, request, "crawl-hash", "uuid-1", time.Now(), done, &wg)

	select {
	case ev := <-done:
		assert.Equal(t, "someshows01e051080pweb-fast", ev.Hash, "the faster copy completes on the same request")
	case <-time.After(10 * time.Second):
		t.Fatal("the crawler was never swapped")
	}
	wg.Wait()
	assert.Equal(t, []string{"crawl-hash"}, qb.deletedHashes(), "only the crawler was removed")
	assert.Equal(t, 1, request.Replaced)
}

// When the indexers list nothing else, the crawler is kept: slow beats nothing.
func TestMonitor_CrawlingTorrentKeptWhenNothingElseListed(t *testing.T) {
	crawlEnv(t)
	qb := &fakeQbittorrent{info: func(poll int) []map[string]any {
		if poll > 25 {
			return []map[string]any{qbitTorrent("crawl-hash", 1, 1, 0, "stalledUP")}
		}
		return []map[string]any{qbitTorrent("crawl-hash", 0.05, 1, 100, "downloading")}
	}}
	fake := &fakeProwlarr{t: t, results: map[int64][]*prowlarr.Search{}}
	prowlarrServer := httptest.NewServer(fake.handler())
	t.Cleanup(prowlarrServer.Close)
	q := newCancelTestHandler(t, crawlRepo(t), qb)
	q.p = prowlarr.New(starr.New("test-key", prowlarrServer.URL, 5*time.Second))

	match, request := crawlMatch()
	var wg sync.WaitGroup
	done := make(chan models.TorrentCompleteEvent, 1)
	wg.Add(1)
	go q.monitorTorrentCompletion(context.Background(), match, request, "crawl-hash", "uuid-1", time.Now(), done, &wg)

	select {
	case ev := <-done:
		assert.Equal(t, "crawl-hash", ev.Hash, "the crawler finished in the end")
	case <-time.After(10 * time.Second):
		t.Fatal("the crawler must be waited for when nothing replaces it")
	}
	wg.Wait()
	assert.Empty(t, qb.deletedHashes())
	assert.Equal(t, 0, request.Replaced)
	require.NotEmpty(t, fake.indexersQueried(), "the indexers were asked")
}
