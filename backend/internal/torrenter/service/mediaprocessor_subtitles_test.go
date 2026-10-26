package service

import (
	"context"
	"errors"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	servicemocks "github.com/jbofill10/scout/backend/internal/torrenter/service/mocks"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"

	"github.com/stretchr/testify/mock"
)

func (ts *testSuite) withSubtitles() *servicemocks.SubtitleFetcher {
	subs := servicemocks.NewSubtitleFetcher(ts.T())
	ts.svc = NewMediaProcessSvc(ts.logger, ts.repo, ts.fs, subs).(*MediaProcessSvc)
	return subs
}

func (ts *testSuite) TestProcessDownloadedTorrent_FetchesSubtitlesForEpisode() {
	subs := ts.withSubtitles()
	event := &models.TorrentCompleteEvent{
		SavePath: "/downloads",
		Req: &models.SearchStrategy{
			MediaName: "TestShow", Season: 1, Episode: 14, TvdbId: "12345", ImdbID: "tt0121955", RuntimeMinutes: 22,
			// Anime strategy searched by absolute number; subtitles must use seasonal.
			EpisodeMeta: &tvdb.Episode{SeasonNumber: 2, Number: 1, Runtime: 22},
		},
	}
	ts.fs.On("WalkFiles", "/downloads").Return([]models.FileEntry{{Path: "Show.S02E01.1080p.WEB-GROUP.mkv", Size: 100}}, nil)
	ts.repo.On("GetShowBaseDirectory", mock.Anything, "12345").Return("/shows/TestShow", nil)
	ts.fs.On("MkDir", "/shows/TestShow/Season 02").Return(nil)
	ts.fs.On("HardLink", "/downloads/Show.S02E01.1080p.WEB-GROUP.mkv", "/shows/TestShow/Season 02/TestShow - S02E01.mkv").Return(nil)
	subs.On("Fetch", mock.Anything, models.SubtitleRequest{
		VideoPath:      "/downloads/Show.S02E01.1080p.WEB-GROUP.mkv",
		LibraryPath:    "/shows/TestShow/Season 02/TestShow - S02E01.mkv",
		ReleaseName:    "Show.S02E01.1080p.WEB-GROUP",
		ImdbID:         "tt0121955",
		Title:          "TestShow",
		IsMovie:        false,
		Season:         2,
		Episode:        1,
		RuntimeMinutes: 22,
	}).Return([]models.SubtitleResult{{Path: "/shows/TestShow/Season 02/TestShow - S02E01.en.srt", Language: "en"}}, nil)

	ts.NoError(ts.svc.ProcessDownloadedTorrent(context.Background(), event))
}

func (ts *testSuite) TestProcessDownloadedTorrent_SubtitleFailureIsNotFatal() {
	subs := ts.withSubtitles()
	event := &models.TorrentCompleteEvent{
		SavePath: "/downloads",
		Req:      &models.SearchStrategy{MediaName: "TestMovie", ReleaseYear: "2023", TvdbId: "67890", IsMovie: true, ImdbID: "tt1"},
	}
	ts.fs.On("WalkFiles", "/downloads").Return([]models.FileEntry{{Path: "movie.mkv", Size: 100}}, nil)
	ts.repo.On("GetMovieBaseDirectory", mock.Anything, "67890").Return("/movies/TestMovie (2023)", nil)
	ts.fs.On("MkDir", "/movies/TestMovie (2023)").Return(nil)
	ts.fs.On("HardLink", "/downloads/movie.mkv", "/movies/TestMovie (2023)/movie.mkv").Return(nil)
	subs.On("Fetch", mock.Anything, mock.MatchedBy(func(r models.SubtitleRequest) bool {
		return r.IsMovie && r.Year == "2023" && r.ReleaseName == "movie" && r.Season == 0
	})).Return(nil, errors.New("provider down"))

	ts.NoError(ts.svc.ProcessDownloadedTorrent(context.Background(), event))
}

func (ts *testSuite) TestProcessDownloadedTorrent_NoSubtitlesWhenLinkFails() {
	subs := ts.withSubtitles()
	event := &models.TorrentCompleteEvent{
		SavePath: "/downloads",
		Req:      &models.SearchStrategy{MediaName: "TestMovie", ReleaseYear: "2023", TvdbId: "67890", IsMovie: true},
	}
	ts.fs.On("WalkFiles", "/downloads").Return([]models.FileEntry{{Path: "movie.mkv", Size: 100}}, nil)
	ts.repo.On("GetMovieBaseDirectory", mock.Anything, "67890").Return("/movies/TestMovie (2023)", nil)
	ts.fs.On("MkDir", "/movies/TestMovie (2023)").Return(nil)
	ts.fs.On("HardLink", mock.Anything, mock.Anything).Return(errors.New("exists"))
	ts.repo.On("InsertDownloadHistory", mock.Anything, "TestMovie", 0, 0, 0, "", "failure", mock.Anything).Return(nil)

	ts.Error(ts.svc.ProcessDownloadedTorrent(context.Background(), event))
	subs.AssertNotCalled(ts.T(), "Fetch", mock.Anything, mock.Anything)
}
