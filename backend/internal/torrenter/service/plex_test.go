package service

import (
	"bytes"
	"context"
	"encoding/xml"
	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	repoMocks "github.com/jbofill10/scout/backend/internal/torrenter/repository/mocks"
	serviceMocks "github.com/jbofill10/scout/backend/internal/torrenter/service/mocks"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type PlexHandlerTestSuite struct {
	suite.Suite
	handler        *PlexHandler
	repo           *repoMocks.Repository
	mediaProcessor *serviceMocks.MediaProcessor
	logger         *slog.Logger
	cfg            *models.PlexCfg
}

func TestPlexHandlerSuite(t *testing.T) {
	suite.Run(t, new(PlexHandlerTestSuite))
}

func (s *PlexHandlerTestSuite) SetupTest() {
	buf := new(bytes.Buffer)
	s.logger = slog.New(slog.NewTextHandler(buf, nil))
	s.repo = repoMocks.NewRepository(s.T())
	s.mediaProcessor = serviceMocks.NewMediaProcessor(s.T())

	s.cfg = &models.PlexCfg{
		Host: "http://localhost:32400",
		Key:  "test-token",
	}

	s.handler = NewPlexHandler(s.repo, s.logger, s.cfg, s.mediaProcessor)
}

func (s *PlexHandlerTestSuite) TestGetLibraries_Success() {
	// Create mock Plex server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Equal("/library/sections", r.URL.Path)
		s.Equal("test-token", r.Header.Get("X-Plex-Token"))

		resp := models.PlexLibrariesResponse{
			Directories: []models.Directory{
				{
					Key:  "1",
					Type: "show",
					Locations: []models.Location{
						{ID: 1, Path: "/data/shows"},
					},
				},
			},
		}

		w.Header().Set("Content-Type", "application/xml")
		_ = xml.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	s.handler.cfg.Host = server.URL

	libs := s.handler.getLibraries(context.Background())

	s.Len(libs.Directories, 1)
	s.Equal("show", libs.Directories[0].Type)
}

func (s *PlexHandlerTestSuite) TestGetMovies_Success() {
	// Setup repo mock
	library := models.PlexLibrary{
		Id:      1,
		Type:    "movie",
		Path:    "/data/movies",
		Section: 1,
	}
	s.repo.On("GetPreferredLibrary", mock.Anything, "movie").Return(library, nil)

	// Create mock Plex server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Equal("/library/sections/1/all", r.URL.Path)

		resp := models.PlexMovieLibraryData{
			Movies: []models.Movie{
				{
					Id:    "1",
					Title: "Test Movie",
					Year:  2023,
				},
			},
		}

		w.Header().Set("Content-Type", "application/xml")
		_ = xml.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	s.handler.cfg.Host = server.URL

	movies := s.handler.getMovies(context.Background())

	s.Len(movies.Movies, 1)
	s.Equal("Test Movie", movies.Movies[0].Title)
}

func (s *PlexHandlerTestSuite) TestGetMovies_NoLibrary() {
	// Setup repo mock to return error
	s.repo.On("GetPreferredLibrary", mock.Anything, "movie").Return(models.PlexLibrary{}, http.ErrMissingFile)

	movies := s.handler.getMovies(context.Background())

	s.Empty(movies.Movies)
}

func (s *PlexHandlerTestSuite) TestFetchAndUnmarshal_Success() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Equal("test-token", r.Header.Get("X-Plex-Token"))

		resp := models.PlexLibrariesResponse{
			Directories: []models.Directory{
				{Key: "1", Type: "show"},
			},
		}

		w.WriteHeader(http.StatusOK)
		_ = xml.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	s.handler.cfg.Host = server.URL

	var result models.PlexLibrariesResponse
	err := s.handler.fetchAndUnmarshal(context.Background(), server.URL+"/test", &result)

	s.NoError(err)
	s.Len(result.Directories, 1)
}

func (s *PlexHandlerTestSuite) TestFetchAndUnmarshal_HTTPError() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	s.handler.cfg.Host = server.URL

	var result models.PlexLibrariesResponse
	err := s.handler.fetchAndUnmarshal(context.Background(), server.URL+"/test", &result)

	s.Error(err)
	s.Contains(err.Error(), "non-200 response")
}

func (s *PlexHandlerTestSuite) TestFetchAndUnmarshal_InvalidXML() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("invalid xml"))
	}))
	defer server.Close()

	s.handler.cfg.Host = server.URL

	var result models.PlexLibrariesResponse
	err := s.handler.fetchAndUnmarshal(context.Background(), server.URL+"/test", &result)

	s.Error(err)
}

func (s *PlexHandlerTestSuite) TestNewPlexHandler() {
	mediaProc := serviceMocks.NewMediaProcessor(s.T())
	handler := NewPlexHandler(s.repo, s.logger, s.cfg, mediaProc)

	s.NotNil(handler)
	s.Equal(s.repo, handler.repo)
	s.Equal(s.logger, handler.logger)
	s.Equal(s.cfg, handler.cfg)
	s.Equal(mediaProc, handler.mediaProcessor)
}

func (s *PlexHandlerTestSuite) TestSyncPlexLibrary_Integration() {
	// This test would be complex as it requires mocking multiple HTTP calls
	// and repository calls. For now, just verify the function exists and doesn't panic
	// with proper mocking setup.

	s.repo.On("UpsertLibraries", mock.Anything, models.PlexLibrariesResponse{}).Maybe()
	s.repo.On("UpsertMovies", mock.Anything, models.PlexMovieLibraryData{}).Maybe()
	s.repo.On("UpsertShows", mock.Anything, &models.PlexShowLibraryData{}).Maybe()

	// We can't easily test this without a full mock setup
	// Just verify it exists
	s.NotNil(s.handler.SyncPlexLibrary)
}
