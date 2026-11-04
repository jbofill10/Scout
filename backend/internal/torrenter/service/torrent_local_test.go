package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	repomocks "github.com/jbofill10/scout/backend/internal/torrenter/repository/mocks"
	"github.com/jbofill10/scout/backend/pkg/dlstatus"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"

	qbittorrent "github.com/autobrr/go-qbittorrent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const dexterSavePath = "/data/Downloads/Dexter (2006) S01-8"

func finishedTorrent(hash, name string) map[string]any {
	return map[string]any{
		"hash": hash, "name": name, "progress": 1.0, "size": 5_000_000_000, "completed": 5_000_000_000,
		"state": "stalledUP", "category": "scout", "save_path": dexterSavePath,
	}
}

func packFile(idx int, name string) map[string]any {
	return map[string]any{"index": idx, "name": name, "progress": 1.0, "priority": 1}
}

func dexterEpisodes(season int, numbers ...int) []tvdb.Episode {
	eps := make([]tvdb.Episode, 0, len(numbers))
	for _, n := range numbers {
		eps = append(eps, tvdb.Episode{Id: season*1000 + n, SeasonNumber: season, Number: n})
	}
	return eps
}

func newLocalHandler(t *testing.T, repo *repomocks.Repository, torrents, files []map[string]any) *QbittHandler {
	t.Helper()
	qb := &fakeQbittorrent{files: files, info: func(int) []map[string]any { return torrents }}
	q := newCancelTestHandler(t, repo, qb)
	q.minQualityTier = QualityTier("1080p")
	return q
}

func stubRepo(t *testing.T, active []*models.ActiveTorrent) *repomocks.Repository {
	repo := repomocks.NewRepository(t)
	repo.On("GetActiveTorrents", mock.Anything).Return(active, nil).Maybe()
	repo.On("InsertActiveTorrent", mock.Anything, mock.Anything).Return(nil).Maybe()
	repo.On("GetNotification", mock.Anything, mock.Anything).Return(nil, nil).Maybe()
	return repo
}

func dexterReq() *tvdb.Media {
	return &tvdb.Media{Id: "79349", Name: "Dexter", Category: "series"}
}

func TestCoverFromFinished_PackCoversEpisodesWithFiles(t *testing.T) {
	repo := repomocks.NewRepository(t)
	repo.On("GetActiveTorrents", mock.Anything).Return([]*models.ActiveTorrent{}, nil)
	var inserted *models.ActiveTorrent
	repo.On("InsertActiveTorrent", mock.Anything, mock.Anything).Run(func(a mock.Arguments) {
		inserted = a.Get(1).(*models.ActiveTorrent)
	}).Return(nil)
	repo.On("GetNotification", mock.Anything, mock.Anything).Return(nil, nil).Maybe()

	q := newLocalHandler(t, repo,
		[]map[string]any{finishedTorrent("FEC8D3A3", "Dexter (2006) S01-S08 (1080p BluRay x265 HEVC 10bit AAC 5.1)")},
		[]map[string]any{
			packFile(0, "Dexter (2006) S01-S08/Dexter (2006) - S01E02 - Crocodile (1080p BluRay x265).mkv"),
			packFile(1, "Dexter (2006) S01-S08/Dexter (2006) - S01E03 - Popping Cherry (1080p BluRay x265).mkv"),
			packFile(2, "Dexter (2006) S01-S08/Dexter (2006) - S01E05 - Love American Style (1080p BluRay x265).nfo"),
		})

	done := make(chan models.TorrentCompleteEvent, 4)
	var wg sync.WaitGroup
	wanted := dexterEpisodes(1, 2, 3, 4, 5)
	results, covered := q.coverFromFinished(context.Background(), dexterReq(), wanted, done, &wg)

	require.Len(t, results, 2)
	assert.Equal(t, map[int]bool{1002: true, 1003: true}, covered)
	for _, r := range results {
		assert.Equal(t, dlstatus.OutcomeDownloading, r.Outcome)
	}
	select {
	case ev := <-done:
		assert.Equal(t, dexterSavePath, ev.SavePath)
		assert.Equal(t, "FEC8D3A3", ev.Hash)
		require.NotNil(t, ev.Req.Pack)
		assert.ElementsMatch(t, []string{"1002", "1003"}, ev.Req.Pack.EpisodeIDs())
	case <-time.After(2 * time.Second):
		t.Fatal("no completion event")
	}
	wg.Wait()
	require.NotNil(t, inserted)
	assert.Equal(t, "FEC8D3A3", inserted.InfoHash)
	assert.Empty(t, inserted.TrackingUUID)
}

func TestCoverFromFinished_Rejections(t *testing.T) {
	file := []map[string]any{packFile(0, "Dexter - S01E02 - Crocodile.mkv")}
	cases := map[string]string{
		"other show":      "Dexter Resurrection S01 2160p WEB-DL",
		"below the floor": "Dexter (2006) S01-S08 720p BluRay x265",
	}
	for name, title := range cases {
		t.Run(name, func(t *testing.T) {
			q := newLocalHandler(t, stubRepo(t, nil), []map[string]any{finishedTorrent("AAAA", title)}, file)
			done := make(chan models.TorrentCompleteEvent, 1)
			var wg sync.WaitGroup
			results, covered := q.coverFromFinished(context.Background(), dexterReq(), dexterEpisodes(1, 2), done, &wg)
			assert.Empty(t, results)
			assert.Empty(t, covered)
			assert.Empty(t, done)
		})
	}
}

func TestCoverFromFinished_SingleEpisode(t *testing.T) {
	q := newLocalHandler(t, stubRepo(t, nil),
		[]map[string]any{finishedTorrent("BBBB", "Dexter S08E11 1080p WEB-DL DD5.1 H.264-GRP")}, nil)
	done := make(chan models.TorrentCompleteEvent, 2)
	var wg sync.WaitGroup
	wanted := dexterEpisodes(8, 10, 11, 12)
	results, covered := q.coverFromFinished(context.Background(), dexterReq(), wanted, done, &wg)

	require.Len(t, results, 1)
	assert.Equal(t, map[int]bool{8011: true}, covered)
	ev := <-done
	assert.Nil(t, ev.Req.Pack)
	assert.Equal(t, "8011", ev.Req.EpisodeTvdbID)
	assert.NotNil(t, ev.Req.EpisodeMeta)
	wg.Wait()
}

func TestCoverFromFinished_SkipsTorrentsStillMonitored(t *testing.T) {
	active := []*models.ActiveTorrent{{InfoHash: "bbbb"}}
	q := newLocalHandler(t, stubRepo(t, active),
		[]map[string]any{finishedTorrent("BBBB", "Dexter S08E11 1080p WEB-DL DD5.1 H.264-GRP")}, nil)
	done := make(chan models.TorrentCompleteEvent, 1)
	var wg sync.WaitGroup
	results, covered := q.coverFromFinished(context.Background(), dexterReq(), dexterEpisodes(8, 11), done, &wg)
	assert.Empty(t, results)
	assert.Nil(t, covered)
}

func TestCoverFromFinished_QbittUnreachable(t *testing.T) {
	repo := stubRepo(t, nil)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/auth/login" {
			_, _ = w.Write([]byte("Ok."))
			return
		}
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	q := newCancelTestHandler(t, repo, &fakeQbittorrent{})
	q.c = qbittorrent.NewClient(qbittorrent.Config{Host: srv.URL, Timeout: 5})
	var wg sync.WaitGroup
	results, covered := q.coverFromFinished(context.Background(), dexterReq(), dexterEpisodes(1, 2),
		make(chan models.TorrentCompleteEvent, 1), &wg)
	assert.Nil(t, results)
	assert.Nil(t, covered)
}

func TestCoverFromFinished_DeselectedButDownloadedFileCounts(t *testing.T) {
	// A trim after the fact leaves a file at priority 0 without removing it;
	// once fully downloaded it is as good as any other.
	file := packFile(0, "Dexter (2006) S01-S08/Dexter (2006) - S01E02 - Crocodile (1080p BluRay x265).mkv")
	file["priority"] = 0
	q := newLocalHandler(t, stubRepo(t, nil),
		[]map[string]any{finishedTorrent("FEC8D3A3", "Dexter (2006) S01-S08 (1080p BluRay x265 HEVC 10bit AAC 5.1)")},
		[]map[string]any{file})
	done := make(chan models.TorrentCompleteEvent, 1)
	var wg sync.WaitGroup
	_, covered := q.coverFromFinished(context.Background(), dexterReq(), dexterEpisodes(1, 2), done, &wg)
	assert.Equal(t, map[int]bool{1002: true}, covered)
	<-done
	wg.Wait()
}

func TestCoverFromFinished_SingleOfAnotherShowIgnored(t *testing.T) {
	q := newLocalHandler(t, stubRepo(t, nil),
		[]map[string]any{finishedTorrent("CCCC", "Dexter Resurrection S01E02 1080p WEB-DL DD5.1 H.264-GRP")}, nil)
	done := make(chan models.TorrentCompleteEvent, 1)
	var wg sync.WaitGroup
	results, covered := q.coverFromFinished(context.Background(), dexterReq(), dexterEpisodes(1, 2), done, &wg)
	assert.Empty(t, results)
	assert.Nil(t, covered)
	assert.Empty(t, done)
}
