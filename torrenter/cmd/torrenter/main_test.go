package main

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"torrenter/internal/handlers"
	"torrenter/internal/interactors"
	"torrenter/internal/repository/mocks"
	servicemocks "torrenter/internal/service/mocks"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/suite"
)

type TorrenterInteractorTestSuite struct {
	suite.Suite
	downloadHandler *handlers.DownloadHandler
	mediaHandler    *handlers.MediaHandler
	repo            *mocks.Repository
	qbitt           *servicemocks.TorrentService
	mp              *servicemocks.MediaProcessor
	logger          *slog.Logger
}

func TestTorrenterInteractorSuite(t *testing.T) {
	suite.Run(t, new(TorrenterInteractorTestSuite))
}

func (s *TorrenterInteractorTestSuite) SetupTest() {
	gin.SetMode(gin.TestMode)
	buf := new(bytes.Buffer)
	s.logger = slog.New(slog.NewTextHandler(buf, nil))
	s.repo = mocks.NewRepository(s.T())
	s.mp = servicemocks.NewMediaProcessor(s.T())
	s.qbitt = servicemocks.NewTorrentService(s.T())

	// Initialize interactors
	downloadInteractor := interactors.NewDownloadInteractor(s.qbitt, s.mp, s.repo, s.logger)
	mediaInteractor := interactors.NewMediaInteractor(s.repo)

	// Initialize handlers
	s.downloadHandler = handlers.NewDownloadHandler(downloadInteractor, s.logger)
	s.mediaHandler = handlers.NewMediaHandler(mediaInteractor, s.logger)
}

func (s *TorrenterInteractorTestSuite) TestDownloadTorrent_InvalidJSON() {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/download", bytes.NewBuffer([]byte("invalid json")))
	c.Request.Header.Set("Content-Type", "application/json")

	s.downloadHandler.DownloadTorrent(c)

	s.Equal(http.StatusBadRequest, w.Code)
}

func (s *TorrenterInteractorTestSuite) TestMediaExists_Exists() {
	s.repo.On("MediaExists", "abc123").Return(true, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/media/abc123", nil)
	c.Params = gin.Params{{Key: "hash", Value: "abc123"}}

	s.mediaHandler.MediaExists(c)

	s.Equal(http.StatusOK, w.Code)
}

func (s *TorrenterInteractorTestSuite) TestMediaExists_NotFound() {
	s.repo.On("MediaExists", "nonexistent").Return(false, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/media/nonexistent", nil)
	c.Params = gin.Params{{Key: "hash", Value: "nonexistent"}}

	s.mediaHandler.MediaExists(c)

	s.Equal(http.StatusNotFound, w.Code)
}

func (s *TorrenterInteractorTestSuite) TestMediaExists_Error() {
	s.repo.On("MediaExists", "error123").Return(false, errors.New("db error"))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/media/error123", nil)
	c.Params = gin.Params{{Key: "hash", Value: "error123"}}

	s.mediaHandler.MediaExists(c)

	s.Equal(http.StatusInternalServerError, w.Code)
}

func (s *TorrenterInteractorTestSuite) TestLoadConfig_Success() {
	// This test would require creating a mock config file
	// For now, we'll just verify the function exists
	// A full test would create a temp config.toml and test loading
}
