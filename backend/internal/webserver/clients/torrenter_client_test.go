package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jbofill10/scout/backend/pkg/dlstatus"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"

	"github.com/stretchr/testify/suite"
)

type TorrenterClientTestSuite struct {
	suite.Suite
	logger *log.Logger
}

func TestTorrenterClientSuite(t *testing.T) {
	suite.Run(t, new(TorrenterClientTestSuite))
}

func (s *TorrenterClientTestSuite) SetupTest() {
	buf := new(bytes.Buffer)
	s.logger = log.New(buf, "[TEST] ", log.LstdFlags|log.Lshortfile)
}

func (s *TorrenterClientTestSuite) TestDownload_Success() {
	requestReceived := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Equal("POST", r.Method)
		s.Equal("/download", r.URL.Path)
		s.Equal("application/json", r.Header.Get("Content-Type"))

		var media tvdb.Media
		err := json.NewDecoder(r.Body).Decode(&media)
		s.NoError(err)
		s.Equal("Test Show", media.Name)

		requestReceived = true
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(dlstatus.DownloadResponse{
			Results: []dlstatus.EpisodeResult{
				{TvdbID: "98765", Season: 1, Episode: 1, Outcome: dlstatus.OutcomeDownloading},
			},
		})
	}))
	defer server.Close()

	client := NewTorrenterClient(server.URL[7:])

	req := tvdb.Media{
		Name: "Test Show",
		Metadata: tvdb.TVDBSeriesMetadata{
			Episodes: []tvdb.Episode{
				{SeasonNumber: 1, Number: 1},
			},
		},
	}

	resp, err := client.Download(context.Background(), req)

	s.NoError(err)
	s.True(requestReceived)
	s.Len(resp.Results, 1)
	s.Equal(dlstatus.OutcomeDownloading, resp.Results[0].Outcome)
	s.Equal("98765", resp.Results[0].TvdbID)
}

func (s *TorrenterClientTestSuite) TestDownload_Non200Status() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewTorrenterClient(server.URL[7:])

	req := tvdb.Media{
		Name: "Test Show",
		Metadata: tvdb.TVDBSeriesMetadata{
			Episodes: []tvdb.Episode{
				{Id: 11, SeasonNumber: 1, Number: 1},
				{Id: 12, SeasonNumber: 1, Number: 2},
			},
		},
	}

	resp, err := client.Download(context.Background(), req)

	s.Error(err)
	s.Contains(err.Error(), "torrenter returned status 500")
	// Synthesized response: every episode failed as torrenter-unreachable (transient).
	s.Len(resp.Results, 2)
	for _, r := range resp.Results {
		s.Equal(dlstatus.OutcomeFailed, r.Outcome)
		s.Equal(dlstatus.CodeTorrenterUnreachable, r.Code)
		s.Equal(dlstatus.CategoryTransient, r.Code.Category())
	}
	s.Equal("11", resp.Results[0].TvdbID)
	s.Equal("12", resp.Results[1].TvdbID)
}

func (s *TorrenterClientTestSuite) TestDownload_NetworkError() {
	client := NewTorrenterClient("invalid-host:9999")

	req := tvdb.Media{Name: "Test Show"}

	resp, err := client.Download(context.Background(), req)

	s.Error(err)
	// No episodes in request -> single failed result keyed by media id.
	s.Len(resp.Results, 1)
	s.Equal(dlstatus.OutcomeFailed, resp.Results[0].Outcome)
	s.Equal(dlstatus.CodeTorrenterUnreachable, resp.Results[0].Code)
}

func (s *TorrenterClientTestSuite) TestDownload_MovieUnreachable() {
	client := NewTorrenterClient("invalid-host:9999")

	req := tvdb.Media{Id: "555", Name: "Test Movie", Category: "movie"}

	resp, err := client.Download(context.Background(), req)

	s.Error(err)
	s.Len(resp.Results, 1)
	s.Equal("555", resp.Results[0].TvdbID)
	s.Equal(dlstatus.OutcomeFailed, resp.Results[0].Outcome)
	s.Equal(dlstatus.CodeTorrenterUnreachable, resp.Results[0].Code)
}

func (s *TorrenterClientTestSuite) TestDownload_InvalidResponseBody() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not json"))
	}))
	defer server.Close()

	client := NewTorrenterClient(server.URL[7:])

	req := tvdb.Media{
		Name: "Test Show",
		Metadata: tvdb.TVDBSeriesMetadata{
			Episodes: []tvdb.Episode{{Id: 7, SeasonNumber: 1, Number: 1}},
		},
	}

	resp, err := client.Download(context.Background(), req)

	s.Error(err)
	s.Len(resp.Results, 1)
	s.Equal(dlstatus.CodeTorrenterUnreachable, resp.Results[0].Code)
}

func (s *TorrenterClientTestSuite) TestDownload_InvalidJSONEncoding() {
	// This test verifies JSON encoding works correctly
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(dlstatus.DownloadResponse{})
	}))
	defer server.Close()

	client := NewTorrenterClient(server.URL[7:])

	// Valid media should encode successfully
	req := tvdb.Media{
		Name: "Test Show",
		Id:   "123",
		Metadata: tvdb.TVDBSeriesMetadata{
			Episodes: []tvdb.Episode{
				{SeasonNumber: 1, Number: 1, Aired: "2023-01-01"},
			},
		},
	}

	_, err := client.Download(context.Background(), req)
	s.NoError(err)
}

func (s *TorrenterClientTestSuite) TestMediaExists_Exists() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Equal("GET", r.Method)
		s.Equal("/media/abc123", r.URL.Path)

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewTorrenterClient(server.URL[7:])

	exists, err := client.MediaExists(context.Background(), "abc123")

	s.NoError(err)
	s.True(exists)
}

func (s *TorrenterClientTestSuite) TestMediaExists_NotFound() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewTorrenterClient(server.URL[7:])

	exists, err := client.MediaExists(context.Background(), "nonexistent")

	s.NoError(err)
	s.False(exists)
}

func (s *TorrenterClientTestSuite) TestMediaExists_UnexpectedStatus() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewTorrenterClient(server.URL[7:])

	exists, err := client.MediaExists(context.Background(), "test123")

	s.Error(err)
	s.False(exists)
	s.Contains(err.Error(), "unexpected status: 500")
}

func (s *TorrenterClientTestSuite) TestMediaExists_NetworkError() {
	client := NewTorrenterClient("invalid-host:9999")

	exists, err := client.MediaExists(context.Background(), "test123")

	s.Error(err)
	s.False(exists)
}

func (s *TorrenterClientTestSuite) TestMediaExistsBatch_Success() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Equal("POST", r.Method)
		s.Equal("/media/exists", r.URL.Path)

		var items []tvdb.Media
		err := json.NewDecoder(r.Body).Decode(&items)
		s.NoError(err)
		s.Len(items, 2)

		response := struct {
			Exists     map[string]bool `json:"exists"`
			InProgress map[string]bool `json:"in_progress"`
		}{
			Exists: map[string]bool{
				"hash1": true,
				"hash2": false,
			},
			InProgress: map[string]bool{
				"hash3": true,
			},
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewTorrenterClient(server.URL[7:])

	items := []tvdb.Media{
		{Id: "1", Name: "Show 1"},
		{Id: "2", Name: "Show 2"},
	}

	exists, inProgress, err := client.MediaExistsBatch(context.Background(), items)

	s.NoError(err)
	s.NotNil(exists)
	s.NotNil(inProgress)
	s.True(exists["hash1"])
	s.False(exists["hash2"])
	s.True(inProgress["hash3"])
}

func (s *TorrenterClientTestSuite) TestMediaExistsBatch_EmptyMaps() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return response with nil maps
		response := struct {
			Exists     map[string]bool `json:"exists"`
			InProgress map[string]bool `json:"in_progress"`
		}{
			Exists:     nil,
			InProgress: nil,
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewTorrenterClient(server.URL[7:])

	items := []tvdb.Media{{Id: "1", Name: "Show 1"}}

	exists, inProgress, err := client.MediaExistsBatch(context.Background(), items)

	s.NoError(err)
	s.NotNil(exists)
	s.NotNil(inProgress)
	s.Empty(exists)
	s.Empty(inProgress)
}

func (s *TorrenterClientTestSuite) TestMediaExistsBatch_Non200Status() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewTorrenterClient(server.URL[7:])

	items := []tvdb.Media{{Id: "1", Name: "Show 1"}}

	exists, inProgress, err := client.MediaExistsBatch(context.Background(), items)

	s.Error(err)
	s.Nil(exists)
	s.Nil(inProgress)
	s.Contains(err.Error(), "torrenter returned status 500")
}

func (s *TorrenterClientTestSuite) TestMediaExistsBatch_InvalidJSON() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	client := NewTorrenterClient(server.URL[7:])

	items := []tvdb.Media{{Id: "1", Name: "Show 1"}}

	exists, inProgress, err := client.MediaExistsBatch(context.Background(), items)

	s.Error(err)
	s.Nil(exists)
	s.Nil(inProgress)
}

func (s *TorrenterClientTestSuite) TestMediaExistsBatch_NetworkError() {
	client := NewTorrenterClient("invalid-host:9999")

	items := []tvdb.Media{{Id: "1", Name: "Show 1"}}

	exists, inProgress, err := client.MediaExistsBatch(context.Background(), items)

	s.Error(err)
	s.Nil(exists)
	s.Nil(inProgress)
}

func (s *TorrenterClientTestSuite) TestMediaExistsBatch_EncodingError() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(struct {
			Exists     map[string]bool `json:"exists"`
			InProgress map[string]bool `json:"in_progress"`
		}{})
	}))
	defer server.Close()

	client := NewTorrenterClient(server.URL[7:])

	// Create valid items that should encode properly
	items := []tvdb.Media{{Id: "1", Name: "Show 1"}}

	exists, inProgress, err := client.MediaExistsBatch(context.Background(), items)

	s.NoError(err)
	s.NotNil(exists)
	s.NotNil(inProgress)
}
