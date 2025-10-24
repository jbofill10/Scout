package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	tvdb "shared/media"
	status "shared/status"
	"webserver/mocks"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/suite"
)

type InteractorTestSuite struct {
	suite.Suite
	interactor *Interactor
	repo       *mocks.SchedulerRepository
	logger     *log.Logger
	mediaQueue chan tvdb.Media
}

func TestInteractorSuite(t *testing.T) {
	suite.Run(t, new(InteractorTestSuite))
}

func (s *InteractorTestSuite) SetupTest() {
	gin.SetMode(gin.TestMode)
	buf := new(bytes.Buffer)
	s.logger = log.New(buf, "[TEST] ", log.LstdFlags|log.Lshortfile)
	s.repo = mocks.NewSchedulerRepository(s.T())
	s.mediaQueue = make(chan tvdb.Media, 10)

	// Create mock scheduler
	scheduler := NewScheduler(s.repo, s.logger)

	s.interactor = &Interactor{
		s:          scheduler,
		repo:       s.repo,
		logger:     s.logger,
		mediaQueue: s.mediaQueue,
	}
}

func (s *InteractorTestSuite) TearDownTest() {
	close(s.mediaQueue)
}

func (s *InteractorTestSuite) TestIsMediaAnime_WithAnimeGenre() {
	resp := tvdb.TVDBSeriesExtendedResponse{
		Data: tvdb.TVDBSeriesExtendedData{
			Genres: []tvdb.Genres{
				{Name: "Action"},
				{Name: "Anime"},
			},
		},
	}

	result := isMediaAnime(resp)
	s.True(result)
}

func (s *InteractorTestSuite) TestIsMediaAnime_WithoutAnimeGenre() {
	resp := tvdb.TVDBSeriesExtendedResponse{
		Data: tvdb.TVDBSeriesExtendedData{
			Genres: []tvdb.Genres{
				{Name: "Action"},
				{Name: "Drama"},
			},
		},
	}

	result := isMediaAnime(resp)
	s.False(result)
}

func (s *InteractorTestSuite) TestIsMediaAnime_EmptyGenres() {
	resp := tvdb.TVDBSeriesExtendedResponse{
		Data: tvdb.TVDBSeriesExtendedData{
			Genres: []tvdb.Genres{},
		},
	}

	result := isMediaAnime(resp)
	s.False(result)
}

func (s *InteractorTestSuite) TestCorsMiddleware() {
	middleware := corsMiddleware()
	s.NotNil(middleware)

	// Test CORS headers are set
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/test", nil)

	middleware(c)

	s.Equal("*", w.Header().Get("Access-Control-Allow-Origin"))
	s.Equal("GET, POST, OPTIONS", w.Header().Get("Access-Control-Allow-Methods"))
	s.Equal("Content-Type", w.Header().Get("Access-Control-Allow-Headers"))
}

func (s *InteractorTestSuite) TestCorsMiddleware_OptionsRequest() {
	middleware := corsMiddleware()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("OPTIONS", "/test", nil)

	middleware(c)

	s.Equal(200, w.Code)
}

func (s *InteractorTestSuite) TestHandleSearch_Success() {
	// Create mock TVDB client that returns search results
	mockResults := []tvdb.Media{
		{
			Id:   "123",
			Name: "Test Show",
		},
	}

	// Setup HTTP test server to mock TVDB proxy
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Equal("/series", r.URL.Path)
		s.Equal("series", r.URL.Query().Get("mediaType"))
		s.Equal("test", r.URL.Query().Get("mediaName"))
		json.NewEncoder(w).Encode(mockResults)
	}))
	defer server.Close()

	s.interactor.tvdbClient = NewTVDBProxyClient(server.URL[7:]) // Remove http://

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/search?query=test&media_type=series", nil)

	s.interactor.handleSearch(c)

	s.Equal(http.StatusOK, w.Code)

	var results []tvdb.Media
	err := json.NewDecoder(w.Body).Decode(&results)
	s.NoError(err)
	s.Len(results, 1)
	s.Equal("Test Show", results[0].Name)
}

func (s *InteractorTestSuite) TestHandleSearch_TVDBClientError() {
	// Setup HTTP test server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	s.interactor.tvdbClient = NewTVDBProxyClient(server.URL[7:])

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/search?query=test&media_type=series", nil)

	s.interactor.handleSearch(c)

	s.Equal(http.StatusInternalServerError, w.Code)
}

func (s *InteractorTestSuite) TestDownloadShow_AiredEpisodes() {
	// Setup mock TVDB server for extended info
	tvdbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := tvdb.TVDBSeriesExtendedResponse{
			Data: tvdb.TVDBSeriesExtendedData{
				Genres: []tvdb.Genres{{Name: "Drama"}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer tvdbServer.Close()

	// Setup mock torrenter server
	torrenterServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Equal("/download", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer torrenterServer.Close()

	s.interactor.tvdbClient = NewTVDBProxyClient(tvdbServer.URL[7:])
	s.interactor.torrenterClient = NewTorrenterClient(torrenterServer.URL[7:])

	// Expect InsertDownloadHistory to be called
	s.repo.On("InsertDownloadHistory", "Test Show", 1, 1, 0, status.Searching, "").Return(nil)

	req := tvdb.Media{
		Id:   "123",
		Name: "Test Show",
		Metadata: tvdb.TVDBSeriesMetadata{
			Episodes: []tvdb.Episode{
				{
					SeasonNumber: 1,
					Number:       1,
					Aired:        time.Now().AddDate(0, 0, -1).Format("2006-01-02"), // Yesterday
				},
			},
		},
	}

	body, _ := json.Marshal(req)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/shows", bytes.NewBuffer(body))

	s.interactor.downloadShow(c)

	s.Equal(http.StatusOK, w.Code)
}

func (s *InteractorTestSuite) TestDownloadShow_FutureEpisodes() {
	// Setup mock TVDB server
	tvdbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := tvdb.TVDBSeriesExtendedResponse{
			Data: tvdb.TVDBSeriesExtendedData{
				Genres: []tvdb.Genres{{Name: "Drama"}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer tvdbServer.Close()

	s.interactor.tvdbClient = NewTVDBProxyClient(tvdbServer.URL[7:])

	futureDate := time.Now().AddDate(0, 0, 7) // Next week
	s.repo.On("Schedule", tvdb.Media{
		Id:       "123",
		Name:     "Test Show",
		Category: "",
		Anime:    false,
		Metadata: tvdb.TVDBSeriesMetadata{
			Episodes: []tvdb.Episode{
				{
					SeasonNumber: 1,
					Number:       2,
					Aired:        futureDate.Format("2006-01-02"),
				},
			},
		},
	}, futureDate).Return(nil)

	req := tvdb.Media{
		Id:   "123",
		Name: "Test Show",
		Metadata: tvdb.TVDBSeriesMetadata{
			Episodes: []tvdb.Episode{
				{
					SeasonNumber: 1,
					Number:       2,
					Aired:        futureDate.Format("2006-01-02"),
				},
			},
		},
	}

	body, _ := json.Marshal(req)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/shows", bytes.NewBuffer(body))

	s.interactor.downloadShow(c)

	s.Equal(http.StatusOK, w.Code)
}

func (s *InteractorTestSuite) TestDownloadShow_AnimeAbsoluteNumbering() {
	// Setup mock TVDB server that returns anime genre
	tvdbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := tvdb.TVDBSeriesExtendedResponse{
			Data: tvdb.TVDBSeriesExtendedData{
				Genres: []tvdb.Genres{{Name: "Anime"}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer tvdbServer.Close()

	// Setup mock torrenter server
	torrenterServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var media tvdb.Media
		json.NewDecoder(r.Body).Decode(&media)

		// Verify absolute numbering is set
		s.True(media.Anime)
		s.Equal(1, media.Metadata.Episodes[0].AbsoluteNumber)
		s.Equal(2, media.Metadata.Episodes[1].AbsoluteNumber)

		w.WriteHeader(http.StatusOK)
	}))
	defer torrenterServer.Close()

	s.interactor.tvdbClient = NewTVDBProxyClient(tvdbServer.URL[7:])
	s.interactor.torrenterClient = NewTorrenterClient(torrenterServer.URL[7:])

	s.repo.On("InsertDownloadHistory", "Anime Show", 1, 1, 1, status.Searching, "").Return(nil)
	s.repo.On("InsertDownloadHistory", "Anime Show", 1, 2, 2, status.Searching, "").Return(nil)

	req := tvdb.Media{
		Id:   "456",
		Name: "Anime Show",
		Metadata: tvdb.TVDBSeriesMetadata{
			Episodes: []tvdb.Episode{
				{
					SeasonNumber: 1,
					Number:       1,
					Aired:        time.Now().AddDate(0, 0, -2).Format("2006-01-02"),
				},
				{
					SeasonNumber: 1,
					Number:       2,
					Aired:        time.Now().AddDate(0, 0, -1).Format("2006-01-02"),
				},
			},
		},
	}

	body, _ := json.Marshal(req)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/shows", bytes.NewBuffer(body))

	s.interactor.downloadShow(c)

	s.Equal(http.StatusOK, w.Code)
}

func (s *InteractorTestSuite) TestDownloadShow_DuplicateScheduling() {
	tvdbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := tvdb.TVDBSeriesExtendedResponse{
			Data: tvdb.TVDBSeriesExtendedData{
				Genres: []tvdb.Genres{{Name: "Drama"}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer tvdbServer.Close()

	s.interactor.tvdbClient = NewTVDBProxyClient(tvdbServer.URL[7:])

	futureDate := time.Now().AddDate(0, 0, 7)

	// Return duplicate error
	s.repo.On("Schedule", tvdb.Media{
		Id:       "123",
		Name:     "Test Show",
		Category: "",
		Anime:    false,
		Metadata: tvdb.TVDBSeriesMetadata{
			Episodes: []tvdb.Episode{
				{
					SeasonNumber: 1,
					Number:       2,
					Aired:        futureDate.Format("2006-01-02"),
				},
			},
		},
	}, futureDate).Return(ErrDuplicateScheduled)

	req := tvdb.Media{
		Id:   "123",
		Name: "Test Show",
		Metadata: tvdb.TVDBSeriesMetadata{
			Episodes: []tvdb.Episode{
				{
					SeasonNumber: 1,
					Number:       2,
					Aired:        futureDate.Format("2006-01-02"),
				},
			},
		},
	}

	body, _ := json.Marshal(req)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/shows", bytes.NewBuffer(body))

	s.interactor.downloadShow(c)

	// Should still return 200 even with duplicate
	s.Equal(http.StatusOK, w.Code)
}

func (s *InteractorTestSuite) TestDownloadShow_SchedulingError() {
	tvdbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := tvdb.TVDBSeriesExtendedResponse{
			Data: tvdb.TVDBSeriesExtendedData{
				Genres: []tvdb.Genres{{Name: "Drama"}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer tvdbServer.Close()

	s.interactor.tvdbClient = NewTVDBProxyClient(tvdbServer.URL[7:])

	futureDate := time.Now().AddDate(0, 0, 7)

	// Return error and expect InsertDownloadHistory to be called
	s.repo.On("Schedule", tvdb.Media{
		Id:       "123",
		Name:     "Test Show",
		Category: "",
		Anime:    false,
		Metadata: tvdb.TVDBSeriesMetadata{
			Episodes: []tvdb.Episode{
				{
					SeasonNumber: 1,
					Number:       2,
					Aired:        futureDate.Format("2006-01-02"),
				},
			},
		},
	}, futureDate).Return(errors.New("scheduling error"))

	s.repo.On("InsertDownloadHistory", "Test Show", 1, 2, 0, status.Failure, "[Scheduling Error]: scheduling error").Return(nil)

	req := tvdb.Media{
		Id:   "123",
		Name: "Test Show",
		Metadata: tvdb.TVDBSeriesMetadata{
			Episodes: []tvdb.Episode{
				{
					SeasonNumber: 1,
					Number:       2,
					Aired:        futureDate.Format("2006-01-02"),
				},
			},
		},
	}

	body, _ := json.Marshal(req)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/shows", bytes.NewBuffer(body))

	s.interactor.downloadShow(c)

	s.Equal(http.StatusOK, w.Code)
}

func (s *InteractorTestSuite) TestDownloadShow_InvalidJSON() {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/shows", bytes.NewBuffer([]byte("invalid json")))

	s.interactor.downloadShow(c)

	s.Equal(http.StatusBadRequest, w.Code)
}

func (s *InteractorTestSuite) TestDownloadShow_GetExtendedInfoError() {
	// Setup mock TVDB server that returns error
	tvdbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer tvdbServer.Close()

	s.interactor.tvdbClient = NewTVDBProxyClient(tvdbServer.URL[7:])

	req := tvdb.Media{
		Id:   "123",
		Name: "Test Show",
		Metadata: tvdb.TVDBSeriesMetadata{
			Episodes: []tvdb.Episode{
				{
					SeasonNumber: 1,
					Number:       1,
					Aired:        time.Now().AddDate(0, 0, -1).Format("2006-01-02"),
				},
			},
		},
	}

	body, _ := json.Marshal(req)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/shows", bytes.NewBuffer(body))

	s.interactor.downloadShow(c)

	s.Equal(http.StatusInternalServerError, w.Code)
}

func (s *InteractorTestSuite) TestDownloadShow_TorrenterError() {
	tvdbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := tvdb.TVDBSeriesExtendedResponse{
			Data: tvdb.TVDBSeriesExtendedData{
				Genres: []tvdb.Genres{{Name: "Drama"}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer tvdbServer.Close()

	// Torrenter returns error
	torrenterServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer torrenterServer.Close()

	s.interactor.tvdbClient = NewTVDBProxyClient(tvdbServer.URL[7:])
	s.interactor.torrenterClient = NewTorrenterClient(torrenterServer.URL[7:])

	s.repo.On("InsertDownloadHistory", "Test Show", 1, 1, 0, status.Searching, "").Return(nil)

	req := tvdb.Media{
		Id:   "123",
		Name: "Test Show",
		Metadata: tvdb.TVDBSeriesMetadata{
			Episodes: []tvdb.Episode{
				{
					SeasonNumber: 1,
					Number:       1,
					Aired:        time.Now().AddDate(0, 0, -1).Format("2006-01-02"),
				},
			},
		},
	}

	body, _ := json.Marshal(req)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/shows", bytes.NewBuffer(body))

	s.interactor.downloadShow(c)

	s.Equal(http.StatusInternalServerError, w.Code)
}

func (s *InteractorTestSuite) TestDownloadShow_InvalidAiredDate() {
	tvdbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := tvdb.TVDBSeriesExtendedResponse{
			Data: tvdb.TVDBSeriesExtendedData{
				Genres: []tvdb.Genres{{Name: "Drama"}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer tvdbServer.Close()

	s.interactor.tvdbClient = NewTVDBProxyClient(tvdbServer.URL[7:])

	req := tvdb.Media{
		Id:   "123",
		Name: "Test Show",
		Metadata: tvdb.TVDBSeriesMetadata{
			Episodes: []tvdb.Episode{
				{
					SeasonNumber: 1,
					Number:       1,
					Aired:        "invalid-date",
				},
			},
		},
	}

	body, _ := json.Marshal(req)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/shows", bytes.NewBuffer(body))

	s.interactor.downloadShow(c)

	// Should still return 200, just skip the invalid episode
	s.Equal(http.StatusOK, w.Code)
}

func (s *InteractorTestSuite) TestWatchForDueMedia_Success() {
	torrenterServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer torrenterServer.Close()

	s.interactor.torrenterClient = NewTorrenterClient(torrenterServer.URL[7:])

	// Start watching
	s.interactor.WatchForDueMedia()

	// Send a media to the queue
	media := tvdb.Media{
		Name: "Test Show",
		Metadata: tvdb.TVDBSeriesMetadata{
			Episodes: []tvdb.Episode{
				{SeasonNumber: 1, Number: 1},
			},
		},
	}

	s.mediaQueue <- media

	// Give goroutine time to process
	time.Sleep(100 * time.Millisecond)
}

func (s *InteractorTestSuite) TestWatchForDueMedia_DownloadError() {
	torrenterServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer torrenterServer.Close()

	s.interactor.torrenterClient = NewTorrenterClient(torrenterServer.URL[7:])

	// Expect failure to be logged
	s.repo.On("InsertDownloadHistory", "Test Show", 1, 1, 0, status.Failure, "[Scheduled Download Error]: torrenter returned status 500").Return(nil)

	s.interactor.WatchForDueMedia()

	media := tvdb.Media{
		Name: "Test Show",
		Metadata: tvdb.TVDBSeriesMetadata{
			Episodes: []tvdb.Episode{
				{SeasonNumber: 1, Number: 1},
			},
		},
	}

	s.mediaQueue <- media

	time.Sleep(100 * time.Millisecond)
}

func (s *InteractorTestSuite) TestDownloadMovie() {
	// Just verify it doesn't crash since it's TODO
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/movies", nil)

	s.interactor.downloadMovie(c)
	// No assertions - just checking it doesn't panic
}
