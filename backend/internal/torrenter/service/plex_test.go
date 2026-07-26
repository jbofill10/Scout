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

	"github.com/stretchr/testify/assert"
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

	libs, err := s.handler.getLibraries(context.Background())

	s.NoError(err)
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

	movies, complete := s.handler.getMovies(context.Background())

	s.True(complete)
	s.Len(movies.Movies, 1)
	s.Equal("Test Movie", movies.Movies[0].Title)
}

func (s *PlexHandlerTestSuite) TestGetMovies_NoLibrary() {
	// Setup repo mock to return error
	s.repo.On("GetPreferredLibrary", mock.Anything, "movie").Return(models.PlexLibrary{}, http.ErrMissingFile)

	movies, complete := s.handler.getMovies(context.Background())

	// A failed fetch must report incomplete so the caller never prunes against it.
	s.False(complete)
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


// TestSyncPlexLibrary_ShowFetchFailureReturnsError verifies that when the show library
// cannot be resolved, getShows returns nil and SyncPlexLibrary surfaces an error rather
// than reporting success. Reporting success left the mirror stale with no retry.
func (s *PlexHandlerTestSuite) TestSyncPlexLibrary_ShowFetchFailureReturnsError() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<MediaContainer size="0"></MediaContainer>`))
	}))
	defer server.Close()

	s.handler.cfg.Host = server.URL

	s.repo.On("UpsertLibraries", mock.Anything, mock.Anything).Return().Maybe()
	s.repo.On("UpsertMovies", mock.Anything, mock.Anything).Return().Maybe()
	s.repo.On("GetPreferredLibrary", mock.Anything, libraryTypeMovie).
		Return(models.PlexLibrary{}, assert.AnError).Maybe()
	s.repo.On("GetPreferredLibrary", mock.Anything, libraryTypeShow).
		Return(models.PlexLibrary{}, assert.AnError)

	err := s.handler.SyncPlexLibrary(context.Background())

	s.Error(err)
	s.Contains(err.Error(), "failed to fetch shows")
	// The mirror must not be touched when the fetch failed.
	s.repo.AssertNotCalled(s.T(), "UpsertShows", mock.Anything, mock.Anything)
}

// TestGetLibraries_Non200ReturnsError guards against the silent-failure mode where a 401
// body unmarshalled into an empty MediaContainer and the sync carried on with 0 libraries.
func (s *PlexHandlerTestSuite) TestGetLibraries_Non200ReturnsError() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`<MediaContainer size="0"></MediaContainer>`))
	}))
	defer server.Close()

	s.handler.cfg.Host = server.URL

	_, err := s.handler.getLibraries(context.Background())

	s.Error(err)
	s.Contains(err.Error(), "401")
}

// TestSyncPlexLibrary_PartialShowFetchSkipsPrune is the safety property that makes pruning
// tolerable: when one show's seasons fail to fetch, that show is missing from the result,
// so pruning against it would delete a show that actually still exists in Plex. The sync
// must upsert what it got and skip the prune entirely.
func (s *PlexHandlerTestSuite) TestSyncPlexLibrary_PartialShowFetchSkipsPrune() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")

		switch r.URL.Path {
		case "/library/sections":
			_ = xml.NewEncoder(w).Encode(models.PlexLibrariesResponse{})

		case "/library/sections/3/all": // movies
			_ = xml.NewEncoder(w).Encode(models.PlexMovieLibraryData{})

		case "/library/sections/4/all": // shows
			_ = xml.NewEncoder(w).Encode(models.PlexShowsResponse{
				Shows: []models.PlexShow{
					{ShowKey: "show1", Title: "Healthy Show", Key: "/show1/children"},
					{ShowKey: "show2", Title: "Broken Show", Key: "/show2/children"},
				},
			})

		case "/show1/children":
			_ = xml.NewEncoder(w).Encode(models.PlexSeasonsResponse{
				Seasons: []models.PlexSeason{
					{SeasonKey: "s1", Key: "/s1/children", Index: 1},
				},
			})

		case "/s1/children":
			_ = xml.NewEncoder(w).Encode(models.PlexEpisodesResponse{
				Videos: []models.PlexEpisode{{EpisodeKey: "e1", Key: "/e1", Index: 1}},
			})

		case "/show2/children": // the failure that makes the fetch partial
			w.WriteHeader(http.StatusInternalServerError)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	s.handler.cfg.Host = server.URL

	s.repo.On("GetPreferredLibrary", mock.Anything, libraryTypeMovie).
		Return(models.PlexLibrary{Section: 3, Path: "/data/movies"}, nil)
	s.repo.On("GetPreferredLibrary", mock.Anything, libraryTypeShow).
		Return(models.PlexLibrary{Section: 4, Path: "/data/shows"}, nil)
	s.repo.On("UpsertLibraries", mock.Anything, mock.Anything).Return()
	s.repo.On("UpsertMovies", mock.Anything, mock.Anything).Return()
	s.repo.On("UpsertShows", mock.Anything, mock.Anything).Return()
	// Movies fetched cleanly, so that prune is allowed to run.
	s.repo.On("PruneMissingMovies", mock.Anything, mock.Anything).Return(nil)
	s.mediaProcessor.On("InvalidateCache", mock.Anything).Return().Maybe()

	err := s.handler.SyncPlexLibrary(context.Background())

	s.NoError(err)
	// The whole point: show2 is absent from the fetch, so nothing may be pruned.
	s.repo.AssertNotCalled(s.T(), "PruneMissingShows", mock.Anything, mock.Anything)
}

// TestSyncPlexLibrary_CompleteFetchPrunes is the counterpart guard: without it, a
// permanently-false complete flag would silently disable pruning and nobody would notice.
func (s *PlexHandlerTestSuite) TestSyncPlexLibrary_CompleteFetchPrunes() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")

		switch r.URL.Path {
		case "/library/sections":
			_ = xml.NewEncoder(w).Encode(models.PlexLibrariesResponse{})

		case "/library/sections/3/all":
			_ = xml.NewEncoder(w).Encode(models.PlexMovieLibraryData{})

		case "/library/sections/4/all":
			_ = xml.NewEncoder(w).Encode(models.PlexShowsResponse{
				Shows: []models.PlexShow{{ShowKey: "show1", Title: "Healthy Show", Key: "/show1/children"}},
			})

		case "/show1/children":
			_ = xml.NewEncoder(w).Encode(models.PlexSeasonsResponse{
				Seasons: []models.PlexSeason{{SeasonKey: "s1", Key: "/s1/children", Index: 1}},
			})

		case "/s1/children":
			_ = xml.NewEncoder(w).Encode(models.PlexEpisodesResponse{
				Videos: []models.PlexEpisode{{EpisodeKey: "e1", Key: "/e1", Index: 1}},
			})

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	s.handler.cfg.Host = server.URL

	s.repo.On("GetPreferredLibrary", mock.Anything, libraryTypeMovie).
		Return(models.PlexLibrary{Section: 3, Path: "/data/movies"}, nil)
	s.repo.On("GetPreferredLibrary", mock.Anything, libraryTypeShow).
		Return(models.PlexLibrary{Section: 4, Path: "/data/shows"}, nil)
	s.repo.On("UpsertLibraries", mock.Anything, mock.Anything).Return()
	s.repo.On("UpsertMovies", mock.Anything, mock.Anything).Return()
	s.repo.On("UpsertShows", mock.Anything, mock.Anything).Return()
	s.repo.On("PruneMissingMovies", mock.Anything, mock.Anything).Return(nil)
	s.repo.On("PruneMissingShows", mock.Anything, mock.Anything).Return(nil)
	s.mediaProcessor.On("InvalidateCache", mock.Anything).Return().Maybe()

	err := s.handler.SyncPlexLibrary(context.Background())

	s.NoError(err)
	s.repo.AssertCalled(s.T(), "PruneMissingShows", mock.Anything, mock.Anything)
	s.repo.AssertCalled(s.T(), "PruneMissingMovies", mock.Anything, mock.Anything)
}

// TestGetShows_SkipsSyntheticAllEpisodesDirectory ensures Plex's keyless "All episodes"
// aggregate is ignored: it previously became a season with an empty id and caused every
// show's episodes to be fetched twice.
func (s *PlexHandlerTestSuite) TestGetShows_SkipsSyntheticAllEpisodesDirectory() {
	allLeavesFetched := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")

		switch r.URL.Path {
		case "/library/sections/4/all":
			_ = xml.NewEncoder(w).Encode(models.PlexShowsResponse{
				Shows: []models.PlexShow{{ShowKey: "show1", Title: "Show", Key: "/show1/children"}},
			})

		case "/show1/children":
			_ = xml.NewEncoder(w).Encode(models.PlexSeasonsResponse{
				Seasons: []models.PlexSeason{
					{SeasonKey: "", Key: "/show1/allLeaves", Title: "All episodes"},
					{SeasonKey: "s1", Key: "/s1/children", Index: 1},
				},
			})

		case "/show1/allLeaves":
			allLeavesFetched = true
			_ = xml.NewEncoder(w).Encode(models.PlexEpisodesResponse{})

		case "/s1/children":
			_ = xml.NewEncoder(w).Encode(models.PlexEpisodesResponse{
				Videos: []models.PlexEpisode{{EpisodeKey: "e1", Key: "/e1", Index: 1}},
			})

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	s.handler.cfg.Host = server.URL
	s.repo.On("GetPreferredLibrary", mock.Anything, libraryTypeShow).
		Return(models.PlexLibrary{Section: 4, Path: "/data/shows"}, nil)

	shows, complete := s.handler.getShows(context.Background())

	s.True(complete)
	s.Require().Len(shows.Shows, 1)
	s.Require().Len(shows.Shows[0].Seasons, 1)
	s.Equal("s1", shows.Shows[0].Seasons[0].Id)
	s.False(allLeavesFetched, "the synthetic aggregate must not be fetched")
}
