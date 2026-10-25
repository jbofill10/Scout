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

func (s *TVDBProxyClientTestSuite) TestExtendedToMedia_Series() {
	data := tvdb.TVDBSeriesExtendedData{
		Id:               121361,
		Name:             "Game of Thrones",
		Slug:             "game-of-thrones",
		Image:            "https://example.com/poster.jpg",
		Overview:         "Seven noble families fight for control.",
		Year:             "2011",
		Status:           tvdb.Status{Name: "Ended"},
		Genres:           []tvdb.Genres{{Name: "Drama"}, {Name: "Fantasy"}},
		Aliases:          []tvdb.Alias{{Language: "eng", Name: "GoT"}},
		FirstAired:       "2011-04-17",
		LastAired:        "2019-05-19",
		AverageRuntime:   60,
		Runtime:          0,
		OriginalCountry:  "usa",
		OriginalLanguage: "eng",
		ContentRatings: []tvdb.ContentRating{
			{Country: "aus", Name: "MA15+"},
			{Country: "usa", Name: "TV-MA"},
		},
		Episodes: []tvdb.Episode{{Id: 1, Number: 1, SeasonNumber: 1}},
	}

	media := ExtendedToMedia(data, "series")

	s.Equal("121361", media.Id)
	s.Equal("Game of Thrones", media.Name)
	s.Equal("series", media.Category)
	s.False(media.Anime)
	s.Equal("Seven noble families fight for control.", media.Overview)
	s.Equal([]string{"Drama", "Fantasy"}, media.Genres)
	s.Equal([]string{"GoT"}, media.Aliases)
	s.Equal(60, media.Runtime) // series use the average episode runtime
	s.Equal("TV-MA", media.ContentRating)
	s.Equal("usa", media.OriginalCountry)
	s.Equal("2011-04-17", media.Metadata.FirstAired)
	s.Equal("2019-05-19", media.Metadata.LastAired)
	s.Len(media.Metadata.Episodes, 1)
}

func (s *TVDBProxyClientTestSuite) TestExtendedToMedia_MovieUsesOwnRuntime() {
	data := tvdb.TVDBSeriesExtendedData{
		Id:             12345,
		Name:           "Spirited Away",
		Genres:         []tvdb.Genres{{Name: "Animation"}, {Name: "Fantasy"}},
		FirstAired:     "2001-07-20",
		Runtime:        125,
		AverageRuntime: 0,
	}

	media := ExtendedToMedia(data, "movie")

	s.Equal(125, media.Runtime)
	s.True(media.Anime, "Animation genre marks a movie as anime")
	s.Equal("2001-07-20", media.Metadata.FirstAired)
	s.Empty(media.ContentRating)
}

func (s *TVDBProxyClientTestSuite) TestExtendedToMedia_ContentRatingFallsBackToFirst() {
	data := tvdb.TVDBSeriesExtendedData{
		Id: 1,
		ContentRatings: []tvdb.ContentRating{
			{Country: "gbr", Name: "15"},
			{Country: "aus", Name: "MA15+"},
		},
	}

	s.Equal("15", ExtendedToMedia(data, "series").ContentRating)
}

func (s *TVDBProxyClientTestSuite) TestGetMediaDetails_Success() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Equal("/series/999/extended", r.URL.Path)
		s.Equal("movie", r.URL.Query().Get("mediaType"))

		_ = json.NewEncoder(w).Encode(tvdb.TVDBSeriesExtendedResponse{
			Data: tvdb.TVDBSeriesExtendedData{
				Id:       999,
				Name:     "Arrival",
				Overview: "A linguist works with the military.",
				Runtime:  116,
				Genres:   []tvdb.Genres{{Name: "Science Fiction"}},
			},
		})
	}))
	defer server.Close()

	client := NewTVDBProxyClient(server.URL[7:])

	media, err := client.GetMediaDetails(context.Background(), "999", "movie")

	s.NoError(err)
	s.Equal("999", media.Id)
	s.Equal("A linguist works with the military.", media.Overview)
	s.Equal(116, media.Runtime)
	s.Equal([]string{"Science Fiction"}, media.Genres)
}

// An empty payload means tvdb-proxy had nothing for the id; surfacing that as an
// error keeps the handler from returning a hollow record to the UI.
func (s *TVDBProxyClientTestSuite) TestGetMediaDetails_EmptyPayloadErrors() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(tvdb.TVDBSeriesExtendedResponse{})
	}))
	defer server.Close()

	client := NewTVDBProxyClient(server.URL[7:])

	_, err := client.GetMediaDetails(context.Background(), "404", "series")

	s.Error(err)
}
