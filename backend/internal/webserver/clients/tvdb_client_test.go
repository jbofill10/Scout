package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"

	"github.com/stretchr/testify/suite"
)

type TVDBProxyClientTestSuite struct {
	suite.Suite
	logger *log.Logger
}

func TestTVDBProxyClientSuite(t *testing.T) {
	suite.Run(t, new(TVDBProxyClientTestSuite))
}

func (s *TVDBProxyClientTestSuite) SetupTest() {
	buf := new(bytes.Buffer)
	s.logger = log.New(buf, "[TEST] ", log.LstdFlags|log.Lshortfile)
}

func (s *TVDBProxyClientTestSuite) TestSearch_Success() {
	expectedResults := []tvdb.Media{
		{
			Id:   "123",
			Name: "Test Show",
		},
		{
			Id:   "456",
			Name: "Another Show",
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Equal("/series", r.URL.Path)
		s.Equal("series", r.URL.Query().Get("mediaType"))
		s.Equal("test", r.URL.Query().Get("mediaName"))

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(expectedResults)
	}))
	defer server.Close()

	client := NewTVDBProxyClient(server.URL[7:]) // Remove http://

	results, err := client.Search(context.Background(), "series", "test")

	s.NoError(err)
	s.Len(results, 2)
	s.Equal("Test Show", results[0].Name)
	s.Equal("Another Show", results[1].Name)
}

func (s *TVDBProxyClientTestSuite) TestSearch_QueryEncoding_SpacesAndSpecialCharacters() {
	expectedResults := []tvdb.Media{
		{
			Id:   "123",
			Name: "The Office",
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Equal("/series", r.URL.Path)
		s.Equal("series", r.URL.Query().Get("mediaType"))
		s.Equal("The Office & Friends", r.URL.Query().Get("mediaName"))

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(expectedResults)
	}))
	defer server.Close()

	client := NewTVDBProxyClient(server.URL[7:]) // Remove http://

	results, err := client.Search(context.Background(), "series", "The Office & Friends")

	s.NoError(err)
	s.Len(results, 1)
	s.Equal("The Office", results[0].Name)
}

func (s *TVDBProxyClientTestSuite) TestSearch_EmptyResults() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode([]tvdb.Media{})
	}))
	defer server.Close()

	client := NewTVDBProxyClient(server.URL[7:])

	results, err := client.Search(context.Background(), "series", "nonexistent")

	s.NoError(err)
	s.Empty(results)
}

func (s *TVDBProxyClientTestSuite) TestSearch_HTTPError() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewTVDBProxyClient(server.URL[7:])

	results, err := client.Search(context.Background(), "series", "test")

	// JSON decoder returns EOF on empty body
	s.Error(err)
	s.Nil(results)
}

func (s *TVDBProxyClientTestSuite) TestSearch_InvalidJSON() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	client := NewTVDBProxyClient(server.URL[7:])

	results, err := client.Search(context.Background(), "series", "test")

	s.Error(err)
	s.Nil(results)
}

func (s *TVDBProxyClientTestSuite) TestSearch_NetworkError() {
	// Use invalid host to trigger network error
	client := NewTVDBProxyClient("invalid-host:9999")

	results, err := client.Search(context.Background(), "series", "test")

	s.Error(err)
	s.Nil(results)
}

func (s *TVDBProxyClientTestSuite) TestGetExtendedInfo_Success() {
	expectedInfo := tvdb.TVDBSeriesExtendedResponse{
		Data: tvdb.TVDBSeriesExtendedData{
			Slug: "test-show",
			Genres: []tvdb.Genres{
				{Name: "Drama"},
				{Name: "Action"},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Equal("/series/123/extended", r.URL.Path)
		s.Equal("series", r.URL.Query().Get("mediaType"))

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(expectedInfo)
	}))
	defer server.Close()

	client := NewTVDBProxyClient(server.URL[7:])

	info, err := client.GetExtendedInfo(context.Background(), "123", "series")

	s.NoError(err)
	s.Equal("test-show", info.Data.Slug)
	s.Len(info.Data.Genres, 2)
	s.Equal("Drama", info.Data.Genres[0].Name)
}

func (s *TVDBProxyClientTestSuite) TestGetExtendedInfo_HTTPError() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewTVDBProxyClient(server.URL[7:])

	info, err := client.GetExtendedInfo(context.Background(), "999", "series")

	// JSON decoder returns EOF on empty body
	s.Error(err)
	s.Equal(tvdb.TVDBSeriesExtendedResponse{}, info)
}

func (s *TVDBProxyClientTestSuite) TestGetExtendedInfo_InvalidJSON() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	client := NewTVDBProxyClient(server.URL[7:])

	info, err := client.GetExtendedInfo(context.Background(), "123", "series")

	s.Error(err)
	s.Equal(tvdb.TVDBSeriesExtendedResponse{}, info)
}

func (s *TVDBProxyClientTestSuite) TestGetExtendedInfo_NetworkError() {
	client := NewTVDBProxyClient("invalid-host:9999")

	info, err := client.GetExtendedInfo(context.Background(), "123", "series")

	s.Error(err)
	s.Equal(tvdb.TVDBSeriesExtendedResponse{}, info)
}

func (s *TVDBProxyClientTestSuite) TestGetExtendedInfo_DifferentMediaIDs() {
	testCases := []struct {
		name    string
		mediaID string
	}{
		{"numeric ID", "123"},
		{"alphanumeric ID", "abc123"},
		{"long ID", "123456789"},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				s.Contains(r.URL.Path, tc.mediaID)
				_ = json.NewEncoder(w).Encode(tvdb.TVDBSeriesExtendedResponse{})
			}))
			defer server.Close()

			client := NewTVDBProxyClient(server.URL[7:])
			_, err := client.GetExtendedInfo(context.Background(), tc.mediaID, "series")

			s.NoError(err)
		})
	}
}
