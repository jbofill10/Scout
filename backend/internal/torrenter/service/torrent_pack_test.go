package service

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	repomocks "github.com/jbofill10/scout/backend/internal/torrenter/repository/mocks"
	"github.com/jbofill10/scout/backend/internal/torrenter/service/mocks"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golift.io/starr/prowlarr"
)

func TestParsePack(t *testing.T) {
	cases := []struct {
		title    string
		show     string
		season   int
		start    int
		end      int
		complete bool
		single   bool
		quality  string
	}{
		{"Severance S01 1080p WEB-DL DDP5.1 H.264-NTb", "Severance", 1, 0, 0, false, false, "1080p"},
		{"Severance.Season.2.Complete.1080p.WEB.x265-RARBG", "Severance", 2, 0, 0, true, false, "1080p"},
		{"Severance S01E01-E09 720p WEB", "Severance", 1, 1, 9, false, false, "720p"},
		{"[SubsPlease] Dan Da Dan (01-12) (1080p) [Batch]", "Dan Da Dan", 0, 1, 12, true, false, "1080p"},
		{"Severance S01E04 1080p WEB", "Severance", 1, 0, 0, false, true, "1080p"},
		{"The Bear Season 3 4K HDR", "The Bear", 3, 0, 0, false, false, "2160p"},
	}
	for _, c := range cases {
		pt := parsePack(c.title)
		assert.False(t, pt.ParseError, c.title)
		assert.Equal(t, c.show, pt.ShowName, c.title)
		assert.Equal(t, c.season, pt.Season, c.title)
		assert.Equal(t, [2]int{c.start, c.end}, [2]int{pt.EpStart, pt.EpEnd}, c.title)
		assert.Equal(t, c.complete, pt.Complete, c.title)
		assert.Equal(t, c.single, pt.SingleEp, c.title)
		assert.Equal(t, c.quality, pt.Quality, c.title)
	}
}

func packShow() (*tvdb.Media, []tvdb.Episode) {
	eps := []tvdb.Episode{}
	for i := 1; i <= 8; i++ {
		eps = append(eps, tvdb.Episode{Id: 100 + i, SeasonNumber: 1, Number: i, AbsoluteNumber: i})
	}
	return &tvdb.Media{Id: "900", Name: "Severance", Category: "series"}, eps
}

func TestValidatePack(t *testing.T) {
	q := newResumeTestHandler(t, repomocks.NewRepository(t))
	q.minQualityTier = QualityTier("1080p")
	show, eps := packShow()
	accept := map[string]bool{
		"Severance S01 1080p WEB-DL-NTb":           true,
		"Severance Season 1 Complete 1080p x265":   true,
		"Severance S01E01-E08 1080p WEB":           true,
		"Severance S01E01-E06 1080p WEB":           false, // does not cover 7 and 8
		"Severance S02 1080p WEB":                  false,
		"Severance S01E04 1080p WEB":               false, // single episode
		"Severance S01 720p WEB":                   false, // below the floor
		"Some Other Show S01 1080p WEB":            false,
		"Severance.S01.COMPLETE.2160p.WEB.x265":    true,
	}
	for title, want := range accept {
		ok, why, _, _ := q.validatePack(&prowlarr.Search{Title: title, Seeders: 20}, show, 1, eps)
		assert.Equal(t, want, ok, "%s: %s", title, why)
	}
}

func TestValidatePack_AnimeBatch(t *testing.T) {
	q := newResumeTestHandler(t, repomocks.NewRepository(t))
	q.minQualityTier = QualityTier("1080p")
	show, eps := packShow()
	show.Anime = true
	ok, why, _, _ := q.validatePack(&prowlarr.Search{Title: "[SubsPlease] Severance (01-08) (1080p) [Batch]", Seeders: 20}, show, 1, eps)
	assert.True(t, ok, why)
	ok, _, _, _ = q.validatePack(&prowlarr.Search{Title: "[SubsPlease] Severance (01-04) (1080p) [Batch]", Seeders: 20}, show, 1, eps)
	assert.False(t, ok, "a batch that stops at 4 does not cover 8 episodes")
}

// With packs on and enough episodes in a season, one pack torrent covers them
// and the rest of the pipeline sees every episode as downloading.
func TestSearchForPack_PicksLivePackAndBuildsStrategy(t *testing.T) {
	fake := &fakeProwlarr{t: t, results: map[int64][]*prowlarr.Search{
		ONE337x_ID: {
			hitWithSeeders(ONE337x_ID, "Severance S01E01 1080p WEB-NTb", 90),
			hitWithSeeders(ONE337x_ID, "Severance S01 720p WEB-NTb", 500),
			hitWithSeeders(ONE337x_ID, "Severance S01 1080p WEB-DL DDP5.1 H.264-NTb", 40),
			hitWithSeeders(ONE337x_ID, "Severance Season 1 Complete 2160p WEB x265", 2),
		},
	}}
	q := newFallbackTestHandler(t, fake)
	q.minQualityTier = QualityTier("1080p")
	show, eps := packShow()

	match := q.searchForPack(context.Background(), show, 1, eps, nil)

	require.NotNil(t, match)
	assert.Equal(t, "Severance S01 1080p WEB-DL DDP5.1 H.264-NTb", match.Torrent.Title, "the live 1080p pack beats the dead 4K one and the 720p one is below the floor")
	require.NotNil(t, match.Strategy.Pack)
	assert.Equal(t, 1, match.Strategy.Pack.Season)
	assert.Len(t, match.Strategy.Pack.Episodes, 8)
	assert.Equal(t, "101", match.Strategy.EpisodeTvdbID, "tracked under the first wanted episode")
	assert.Equal(t, []string{"101", "102", "103", "104", "105", "106", "107", "108"}, match.Strategy.Pack.EpisodeIDs())
}

func TestTryPacks_RespectsSettingAndThreshold(t *testing.T) {
	q := newResumeTestHandler(t, repomocks.NewRepository(t))
	show, eps := packShow()
	done := make(chan models.TorrentCompleteEvent, 1)

	results, packed := q.tryPacks(context.Background(), show, eps, done, nil)
	assert.Nil(t, results)
	assert.Nil(t, packed, "packs are off without settings")

	q.settings = &fakePickerSettings{}
	results, packed = q.tryPacks(context.Background(), show, eps[:3], done, nil)
	assert.Nil(t, packed, "three episodes are under the threshold of four")
	assert.Nil(t, results)
}

func TestInFlightIDs_IncludePackEpisodes(t *testing.T) {
	repo := repomocks.NewRepository(t)
	repo.On("GetActiveTorrents", mock.Anything).Return([]*models.ActiveTorrent{
		{InfoHash: "p", Strategy: &models.SearchStrategy{EpisodeTvdbID: "101", Pack: &models.PackInfo{Season: 1,
			Episodes: []tvdb.Episode{{Id: 101}, {Id: 102}, {Id: 103}}}}},
	}, nil)
	q := newResumeTestHandler(t, repo)
	ids := q.inFlightTvdbIDs(context.Background())
	assert.Equal(t, map[string]bool{"101": true, "102": true, "103": true}, ids)
}

// A landed pack links each file to its episode and reports the ones the
// pack did not contain.
func TestProcessPack_LinksMatchingFilesAndReportsMissing(t *testing.T) {
	fs := mocks.NewFileSystem(t)
	repo := repomocks.NewRepository(t)
	fs.On("WalkFiles", "/dl/Severance.S01").Return([]models.FileEntry{
		{Path: "Severance.S01/Severance.S01E01.1080p.WEB.mkv", Size: 100},
		{Path: "Severance.S01/Severance.S01E02.1080p.WEB.mkv", Size: 100},
		{Path: "Severance.S01/Sample/Severance.S01E01.sample.mkv", Size: 1},
		{Path: "Severance.S01/readme.nfo", Size: 1},
	}, nil)
	repo.On("GetShowBaseDirectory", mock.Anything, "900").Return("/lib/Severance", nil)
	fs.On("MkDir", "/lib/Severance/Season 01").Return(nil)
	fs.On("HardLink", "/dl/Severance.S01/Severance.S01/Severance.S01E01.1080p.WEB.mkv", "/lib/Severance/Season 01/Severance - S01E01.mkv").Return(nil)
	fs.On("HardLink", "/dl/Severance.S01/Severance.S01/Severance.S01E02.1080p.WEB.mkv", "/lib/Severance/Season 01/Severance - S01E02.mkv").Return(nil)

	mp := NewMediaProcessSvc(slog.New(slog.NewTextHandler(new(bytes.Buffer), nil)), repo, fs, nil)
	eps := []tvdb.Episode{{Id: 101, SeasonNumber: 1, Number: 1}, {Id: 102, SeasonNumber: 1, Number: 2}, {Id: 103, SeasonNumber: 1, Number: 3}}
	event := &models.TorrentCompleteEvent{SavePath: "/dl/Severance.S01", Req: &models.SearchStrategy{
		MediaName: "Severance", TvdbId: "900", EpisodeTvdbID: "101", Season: 1, Episode: 1, EpisodeMeta: &eps[0],
		Pack: &models.PackInfo{Season: 1, Episodes: eps},
	}}

	err := mp.ProcessDownloadedTorrent(context.Background(), event)

	require.NoError(t, err)
	assert.Equal(t, map[string]string{"101": "linked", "102": "linked", "103": "not in the season pack"}, event.PackResults)
	fs.AssertNumberOfCalls(t, "HardLink", 2)
}
