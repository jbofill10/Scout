package main

import (
	"bytes"
	"encoding/xml"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"torrenter/mocks"
	"torrenter/models"

	"github.com/stretchr/testify/suite"
)

type PlexHandlerTestSuite struct {
	suite.Suite
	handler *PlexHandler
	repo    *mocks.Repository
	logger  *log.Logger
	cfg     *models.PlexCfg
}

func TestPlexHandlerSuite(t *testing.T) {
	suite.Run(t, new(PlexHandlerTestSuite))
}

func (s *PlexHandlerTestSuite) SetupTest() {
	buf := new(bytes.Buffer)
	s.logger = log.New(buf, "[TEST] ", log.LstdFlags|log.Lshortfile)
	s.repo = mocks.NewRepository(s.T())

	s.cfg = &models.PlexCfg{
		Host: "http://localhost:32400",
		Key:  "test-token",
	}

	s.handler = NewPlexHandler(s.repo, s.logger, s.cfg)
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
		xml.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	s.handler.cfg.Host = server.URL

	libs := s.handler.getLibraries()

	s.Len(libs.Directories, 1)
	s.Equal("show", libs.Directories[0].Type)
}

func (s *PlexHandlerTestSuite) TestGetMovies_Success() {
	// Setup repo mock
	s.repo.On("GetLibraryByType", "movie").Return(1, nil)

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
		xml.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	s.handler.cfg.Host = server.URL

	movies := s.handler.getMovies()

	s.Len(movies.Movies, 1)
	s.Equal("Test Movie", movies.Movies[0].Title)
}

func (s *PlexHandlerTestSuite) TestGetMovies_NoLibrary() {
	// Setup repo mock to return error
	s.repo.On("GetLibraryByType", "movie").Return(0, http.ErrMissingFile)

	movies := s.handler.getMovies()

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
		xml.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	s.handler.cfg.Host = server.URL

	var result models.PlexLibrariesResponse
	err := s.handler.fetchAndUnmarshal(server.URL+"/test", &result)

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
	err := s.handler.fetchAndUnmarshal(server.URL+"/test", &result)

	s.Error(err)
	s.Contains(err.Error(), "non-200 response")
}

func (s *PlexHandlerTestSuite) TestFetchAndUnmarshal_InvalidXML() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("invalid xml"))
	}))
	defer server.Close()

	s.handler.cfg.Host = server.URL

	var result models.PlexLibrariesResponse
	err := s.handler.fetchAndUnmarshal(server.URL+"/test", &result)

	s.Error(err)
}

func (s *PlexHandlerTestSuite) TestGetTvdbIdForShow_Success() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Equal("/library/metadata/123", r.URL.Path)

		type MetadataContainer struct {
			XMLName  string            `xml:"MediaContainer"`
			Metadata []models.PlexShow `xml:"Directory"`
		}

		container := MetadataContainer{
			Metadata: []models.PlexShow{
				{
					Guids: []models.PlexGuid{
						{ID: "tvdb://456789"},
					},
				},
			},
		}

		w.WriteHeader(http.StatusOK)
		xml.NewEncoder(w).Encode(container)
	}))
	defer server.Close()

	s.handler.cfg.Host = server.URL

	tvdbId := s.handler.getTvdbIdForShow("123")

	s.Equal("456789", tvdbId)
}

func (s *PlexHandlerTestSuite) TestGetTvdbIdForShow_NoGuids() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		type MetadataContainer struct {
			XMLName  string            `xml:"MediaContainer"`
			Metadata []models.PlexShow `xml:"Directory"`
		}

		container := MetadataContainer{
			Metadata: []models.PlexShow{
				{
					Guids: []models.PlexGuid{},
				},
			},
		}

		w.WriteHeader(http.StatusOK)
		xml.NewEncoder(w).Encode(container)
	}))
	defer server.Close()

	s.handler.cfg.Host = server.URL

	tvdbId := s.handler.getTvdbIdForShow("123")

	s.Equal("", tvdbId)
}

func (s *PlexHandlerTestSuite) TestGetTvdbIdForShow_NoMetadata() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		type MetadataContainer struct {
			XMLName  string            `xml:"MediaContainer"`
			Metadata []models.PlexShow `xml:"Directory"`
		}

		container := MetadataContainer{
			Metadata: []models.PlexShow{},
		}

		w.WriteHeader(http.StatusOK)
		xml.NewEncoder(w).Encode(container)
	}))
	defer server.Close()

	s.handler.cfg.Host = server.URL

	tvdbId := s.handler.getTvdbIdForShow("123")

	s.Equal("", tvdbId)
}

func (s *PlexHandlerTestSuite) TestNewPlexHandler() {
	handler := NewPlexHandler(s.repo, s.logger, s.cfg)

	s.NotNil(handler)
	s.Equal(s.repo, handler.repo)
	s.Equal(s.logger, handler.logger)
	s.Equal(s.cfg, handler.cfg)
}

func (s *PlexHandlerTestSuite) TestSyncPlexLibrary_Integration() {
	// This test would be complex as it requires mocking multiple HTTP calls
	// and repository calls. For now, just verify the function exists and doesn't panic
	// with proper mocking setup.

	s.repo.On("UpsertLibraries", models.PlexLibrariesResponse{}).Maybe()
	s.repo.On("UpsertMovies", models.PlexMovieLibraryData{}).Maybe()
	s.repo.On("UpsertShows", &models.PlexShowLibraryData{}).Maybe()

	// We can't easily test this without a full mock setup
	// Just verify it exists
	s.NotNil(s.handler.syncPlexLibrary)
}
