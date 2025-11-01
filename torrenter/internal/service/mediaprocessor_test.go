package service

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
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
		},
	}
	ts.repo.On("GetPreferredLibrary", mock.Anything, "show").Return(models.PlexLibrary{Path: "/shows"}, nil)
	ts.fs.On("ReadDir", "/downloads").Return([]string{"file.mkv"}, nil)
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
	ts.repo.On("GetPreferredLibrary", mock.Anything, "movie").Return(models.PlexLibrary{Path: "/movies"}, nil)
	ts.fs.On("ReadDir", "/downloads").Return([]string{"file.txt", "readme.md"}, nil)
	err := ts.svc.ProcessDownloadedTorrent(context.Background(), event)
	ts.Error(err)
	ts.repo.AssertExpectations(ts.T())
	ts.fs.AssertExpectations(ts.T())
}
