package service

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"torrenter/internal/models"
	"torrenter/internal/repository/mocks"
	servicemocks "torrenter/internal/service/mocks"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type testSuite struct {
	suite.Suite
	svc    *MediaProcessSvc
	repo   *mocks.Repository
	fs     *servicemocks.FileSystem
	logger *slog.Logger
}

func TestMediaProcessorSuite(t *testing.T) {
	suite.Run(t, new(testSuite))
}

func (ts *testSuite) SetupTest() {
	ts.repo = mocks.NewRepository(ts.T())

	// Use a better logger for testing: log to buffer, show date/time, and short file info
	buf := new(bytes.Buffer)
	ts.logger = slog.New(slog.NewTextHandler(buf, nil))
	ts.fs = servicemocks.NewFileSystem(ts.T())

	// Use constructor to ensure cache is properly initialized
	ts.svc = NewMediaProcessSvc(ts.logger, ts.repo, ts.fs).(*MediaProcessSvc)
}

func (ts *testSuite) TestPrepMediaPath_Show() {
	testCases := []struct {
		name     string
		req      *models.DownloadRequest
		expected string
	}{
		{
			name: "Single Digit Season/Episode",
			req: &models.DownloadRequest{
				MediaName: "TestShow",
				Season:    "1",
				Episode:   "2",
			},
			expected: "TestShow/Season 01/TestShow - S01E02",
		},
		{
			name: "Double Digit Season/Episode",
			req: &models.DownloadRequest{
				MediaName: "TestShow",
				Season:    "10",
				Episode:   "12",
			},
			expected: "TestShow/Season 10/TestShow - S10E12",
		},
	}

	for _, tc := range testCases {
		ts.Run(tc.name, func() {
			path := ts.svc.prepMediaPath(tc.req)
			ts.Equal(tc.expected, path)
		})
	}
}

func (ts *testSuite) TestPrepMediaPath_Movie() {
	req := &models.DownloadRequest{
		MediaName:   "TestMovie",
		ReleaseYear: "2023",
	}
	path := ts.svc.prepMediaPath(req)
	ts.Equal("TestMovie (2023)", path)
}

func (ts *testSuite) TestGetFile_Valid() {
	ts.fs.On("ReadDir", "/downloads").Return([]string{"file.mkv"}, nil)
	fileName, err := ts.svc.getFile("/downloads")
	ts.NoError(err)
	ts.Equal("file.mkv", fileName)
	ts.fs.AssertExpectations(ts.T())
}

func (ts *testSuite) TestGetFile_NoVideoFiles() {
	ts.fs.On("ReadDir", "/downloads").Return([]string{"file.txt", "readme.md"}, nil)
	fileName, err := ts.svc.getFile("/downloads")
	ts.Error(err)
	ts.Equal("", fileName)
	ts.fs.AssertExpectations(ts.T())
}

func (ts *testSuite) TestGetFile_MultipleVideoFiles() {
	ts.fs.On("ReadDir", "/downloads").Return([]string{"file1.mkv", "file2.mp4"}, nil)
	fileName, err := ts.svc.getFile("/downloads")
	ts.NoError(err)
	ts.Equal("file1.mkv", fileName) // Should return first video file
	ts.fs.AssertExpectations(ts.T())
}

func (ts *testSuite) TestGetFile_ReadDirError() {
	ts.fs.On("ReadDir", "/downloads").Return([]string{}, errors.New("permission denied"))
	fileName, err := ts.svc.getFile("/downloads")
	ts.Error(err)
	ts.Equal("", fileName)
	ts.fs.AssertExpectations(ts.T())
}

func (ts *testSuite) TestGetLibraryPath_Show() {
	expected := models.PlexLibrary{Path: "/shows"}
	ts.repo.On("GetPreferredLibrary", mock.Anything, "show").Return(expected, nil)
	lib, err := ts.svc.getLibraryPath(context.Background(), true)
	ts.NoError(err)
	ts.Equal(expected, lib)
	ts.repo.AssertExpectations(ts.T())
}

func (ts *testSuite) TestGetLibraryPath_Movie_Error() {
	ts.repo.On("GetPreferredLibrary", mock.Anything, "movie").Return(models.PlexLibrary{}, errors.New("fail"))
	lib, err := ts.svc.getLibraryPath(context.Background(), false)
	ts.Error(err)
	ts.Equal(models.PlexLibrary{}, lib)
	ts.repo.AssertExpectations(ts.T())
}

func (ts *testSuite) TestProcessDownloadedTorrent_Success() {
	event := &models.TorrentCompleteEvent{
		SavePath: "/downloads",
		Req: &models.SearchStrategy{
			MediaName: "TestShow",
			Season:    1,
			Episode:   2,
			TvdbId:    "12345",
		},
	}
	ts.fs.On("ReadDir", "/downloads").Return([]string{"file.mkv"}, nil)
	// Mock GetShowBaseDirectory - return error to test fallback path (uses cache/construct)
	ts.repo.On("GetShowBaseDirectory", mock.Anything, "12345").Return("", errors.New("not found"))
	ts.repo.On("GetPreferredLibrary", mock.Anything, "show").Return(models.PlexLibrary{Path: "/shows"}, nil)
	// Mock MkDir and HardLink for directory creation and hard linking
	ts.fs.On("MkDir", "/shows/TestShow/Season 01").Return(nil)
	ts.fs.On("HardLink", "/downloads/file.mkv", "/shows/TestShow/Season 01/file.mkv").Return(nil)
	// Note: SetBaseDirectoryForTvdbId no longer exists - base directories are cached in-memory only
	err := ts.svc.ProcessDownloadedTorrent(context.Background(), event)
	ts.NoError(err)
	ts.repo.AssertExpectations(ts.T())
	ts.fs.AssertExpectations(ts.T())
}

func (ts *testSuite) TestProcessDownloadedTorrent_NoVideoFileError() {
	event := &models.TorrentCompleteEvent{
		SavePath: "/downloads",
		Req: &models.SearchStrategy{
			MediaName: "TestMovie",
			Season:    0,
			Episode:   0,
		},
	}
	// Note: getFile is called first, so it fails before GetPreferredLibrary or GetMovieBaseDirectory are called
	ts.fs.On("ReadDir", "/downloads").Return([]string{"file.txt", "readme.md"}, nil)
	err := ts.svc.ProcessDownloadedTorrent(context.Background(), event)
	ts.Error(err)
	ts.fs.AssertExpectations(ts.T())
}

func (ts *testSuite) TestProcessDownloadedTorrent_Movie_Success() {
	event := &models.TorrentCompleteEvent{
		SavePath: "/downloads",
		Req: &models.SearchStrategy{
			MediaName:   "TestMovie",
			ReleaseYear: "2023",
			Season:      0,
			Episode:     0,
			TvdbId:      "67890",
		},
	}
	ts.fs.On("ReadDir", "/downloads").Return([]string{"movie.mkv"}, nil)
	// Mock GetMovieBaseDirectory - return error to test fallback path (uses cache/construct)
	ts.repo.On("GetMovieBaseDirectory", mock.Anything, "67890").Return("", errors.New("not found"))
	ts.repo.On("GetPreferredLibrary", mock.Anything, "movie").Return(models.PlexLibrary{Path: "/movies"}, nil)
	// Mock MkDir and HardLink for directory creation and hard linking
	ts.fs.On("MkDir", "/movies/TestMovie (2023)").Return(nil)
	ts.fs.On("HardLink", "/downloads/movie.mkv", "/movies/TestMovie (2023)/movie.mkv").Return(nil)
	err := ts.svc.ProcessDownloadedTorrent(context.Background(), event)
	ts.NoError(err)
	ts.repo.AssertExpectations(ts.T())
	ts.fs.AssertExpectations(ts.T())
}

func (ts *testSuite) TestProcessDownloadedTorrent_MkDirError() {
	event := &models.TorrentCompleteEvent{
		SavePath: "/downloads",
		Req: &models.SearchStrategy{
			MediaName: "TestShow",
			Season:    1,
			Episode:   2,
			TvdbId:    "12345",
		},
	}
	ts.fs.On("ReadDir", "/downloads").Return([]string{"file.mkv"}, nil)
	ts.repo.On("GetShowBaseDirectory", mock.Anything, "12345").Return("", errors.New("not found"))
	ts.repo.On("GetPreferredLibrary", mock.Anything, "show").Return(models.PlexLibrary{Path: "/shows"}, nil)
	// Mock MkDir to return error
	ts.fs.On("MkDir", "/shows/TestShow/Season 01").Return(errors.New("permission denied"))
	err := ts.svc.ProcessDownloadedTorrent(context.Background(), event)
	ts.Error(err)
	ts.Contains(err.Error(), "failed to create target directory")
	ts.repo.AssertExpectations(ts.T())
	ts.fs.AssertExpectations(ts.T())
}

func (ts *testSuite) TestProcessDownloadedTorrent_HardLinkError() {
	event := &models.TorrentCompleteEvent{
		SavePath: "/downloads",
		Req: &models.SearchStrategy{
			MediaName: "TestShow",
			Season:    1,
			Episode:   2,
			TvdbId:    "12345",
		},
	}
	ts.fs.On("ReadDir", "/downloads").Return([]string{"file.mkv"}, nil)
	ts.repo.On("GetShowBaseDirectory", mock.Anything, "12345").Return("", errors.New("not found"))
	ts.repo.On("GetPreferredLibrary", mock.Anything, "show").Return(models.PlexLibrary{Path: "/shows"}, nil)
	ts.fs.On("MkDir", "/shows/TestShow/Season 01").Return(nil)
	// Mock HardLink to return error
	ts.fs.On("HardLink", "/downloads/file.mkv", "/shows/TestShow/Season 01/file.mkv").Return(errors.New("cross-device link"))
	// Expect InsertDownloadHistory call when hard link fails
	ts.repo.On("InsertDownloadHistory", mock.Anything, "TestShow", 1, 2, 0, "", "failure", mock.MatchedBy(func(reason string) bool {
		return strings.Contains(reason, "symbolic link failed")
	})).Return(nil)
	err := ts.svc.ProcessDownloadedTorrent(context.Background(), event)
	ts.Error(err)
	ts.Contains(err.Error(), "failed to create symbolic link")
	ts.repo.AssertExpectations(ts.T())
	ts.fs.AssertExpectations(ts.T())
}
