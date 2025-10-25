package service

import (
	"bytes"
	"errors"
	"log"
	"testing"
	"torrenter/internal/models"
	"torrenter/internal/repository/mocks"
	servicemocks "torrenter/internal/service/mocks"

	"github.com/stretchr/testify/suite"
)

type testSuite struct {
	suite.Suite
	svc    *MediaProcessSvc
	repo   *mocks.Repository
	fs     *servicemocks.FileSystem
	logger *log.Logger
}

func TestMediaProcessorSuite(t *testing.T) {
	suite.Run(t, new(testSuite))
}

func (ts *testSuite) SetupTest() {
	ts.repo = mocks.NewRepository(ts.T())

	// Use a better logger for testing: log to buffer, show date/time, and short file info
	buf := new(bytes.Buffer)
	ts.logger = log.New(buf, "[TEST] ", log.LstdFlags|log.Lshortfile)
	ts.fs = servicemocks.NewFileSystem(ts.T())

	ts.svc = &MediaProcessSvc{Logger: ts.logger, repo: ts.repo, fs: ts.fs}
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

func (ts *testSuite) TestGetFileExtension_Valid() {
	ext, err := ts.svc.getFileExtension("file.mkv")
	ts.NoError(err)
	ts.Equal(".mkv", ext)
}

func (ts *testSuite) TestGetFileExtension_Invalid() {
	ext, err := ts.svc.getFileExtension("file.txt")
	ts.Error(err)
	ts.Equal("", ext)
}

func (ts *testSuite) TestGetLibraryPath_Show() {
	expected := models.PlexLibrary{Path: "/shows"}
	ts.repo.On("GetPreferredLibrary", "show").Return(expected, nil)
	lib, err := ts.svc.getLibraryPath(true)
	ts.NoError(err)
	ts.Equal(expected, lib)
	ts.repo.AssertExpectations(ts.T())
}

func (ts *testSuite) TestGetLibraryPath_Movie_Error() {
	ts.repo.On("GetPreferredLibrary", "movie").Return(models.PlexLibrary{}, errors.New("fail"))
	lib, err := ts.svc.getLibraryPath(false)
	ts.Error(err)
	ts.Equal(models.PlexLibrary{}, lib)
	ts.repo.AssertExpectations(ts.T())
}

func (ts *testSuite) TestProcessDownloadedTorrent_Success() {
	event := &models.TorrentCompleteEvent{
		SavePath: "/downloads/file.mkv",
		Req: &models.SearchStrategy{
			MediaName: "TestShow",
			Season:    1,
			Episode:   2,
		},
	}
	ts.repo.On("GetPreferredLibrary", "show").Return(models.PlexLibrary{Path: "/shows"}, nil)
	err := ts.svc.ProcessDownloadedTorrent(event)
	ts.NoError(err)
	ts.repo.AssertExpectations(ts.T())
}

func (ts *testSuite) TestProcessDownloadedTorrent_FileExtError() {
	event := &models.TorrentCompleteEvent{
		SavePath: "/downloads/file.txt",
		Req: &models.SearchStrategy{
			MediaName: "TestMovie",
			Season:    0,
			Episode:   0,
		},
	}
	ts.repo.On("GetPreferredLibrary", "movie").Return(models.PlexLibrary{Path: "/movies"}, nil)
	err := ts.svc.ProcessDownloadedTorrent(event)
	ts.Error(err)
	ts.repo.AssertExpectations(ts.T())
}
