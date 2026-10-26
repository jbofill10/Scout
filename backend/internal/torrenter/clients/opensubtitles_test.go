package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"

	"github.com/stretchr/testify/require"
)

func newTestClient(t *testing.T, handler http.Handler, user bool) *OpenSubtitlesClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	cfg := &models.SubtitlesCfg{APIKey: "key"}
	if user {
		cfg.Username, cfg.Password = "scout", "secret"
	}
	return NewOpenSubtitlesClient(cfg, slog.New(slog.NewTextHandler(new(bytes.Buffer), nil))).WithBaseURL(srv.URL)
}

func TestSearchBuildsExpectedQuery(t *testing.T) {
	var gotQuery, gotKey, gotUA string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		gotKey = r.Header.Get("Api-Key")
		gotUA = r.Header.Get("User-Agent")
		_, _ = w.Write([]byte(`{"total_count":2,"data":[
			{"attributes":{"language":"en","download_count":12,"hearing_impaired":true,"moviehash_match":true,"release":"Movie.YIFY",
			  "from_trusted":true,"feature_details":{"feature_type":"Episode","season_number":18,"episode_number":3,"year":2014},
			  "files":[{"file_id":77,"cd_number":1,"file_name":"a.srt"}]}},
			{"attributes":{"language":"en","files":[{"file_id":1},{"file_id":2}]}}
		]}`))
	})
	c := newTestClient(t, h, false)

	got, err := c.Search(context.Background(), models.SubtitleSearchParams{
		Languages: []string{"es", "EN"}, IsMovie: false, ParentImdbID: "tt0121955", Season: 18, Episode: 3, MovieHash: "ABCDEF0123456789",
	})
	require.NoError(t, err)
	require.Equal(t, "key", gotKey)
	require.Equal(t, openSubtitlesUserAgent, gotUA)
	require.Equal(t,
		"ai_translated=exclude&episode_number=3&languages=en%2Ces&machine_translated=exclude&moviehash=abcdef0123456789&parent_imdb_id=0121955&season_number=18&type=episode",
		gotQuery)

	require.Len(t, got, 1, "multi-CD entries are dropped")
	require.Equal(t, models.SubtitleCandidate{
		FileID: 77, Language: "en", Release: "Movie.YIFY", HearingImpaired: true, FromTrusted: true, HashMatch: true,
		DownloadCount: 12, FeatureType: "episode", Season: 18, Episode: 3, Year: 2014,
	}, got[0])
}

func TestSearchMovieQueryUsesImdbAndYear(t *testing.T) {
	var gotQuery string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"data":[]}`))
	})
	c := newTestClient(t, h, false)
	_, err := c.Search(context.Background(), models.SubtitleSearchParams{
		Languages: []string{"en"}, IsMovie: true, ImdbID: "tt3387648", Query: "The Taking of Deborah Logan", Year: "2014",
		// Season/episode are ignored for movies even if set.
		Season: 1, Episode: 2,
	})
	require.NoError(t, err)
	require.Equal(t,
		"ai_translated=exclude&imdb_id=3387648&languages=en&machine_translated=exclude&query=the+taking+of+deborah+logan&type=movie&year=2014",
		gotQuery)
}

func TestSearchErrorSurfacesStatus(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"message":"slow down"}`))
	})
	c := newTestClient(t, h, false)
	_, err := c.Search(context.Background(), models.SubtitleSearchParams{Languages: []string{"en"}})
	var apiErr *APIError
	require.True(t, errors.As(err, &apiErr))
	require.Equal(t, 429, apiErr.Status)
	require.Equal(t, "slow down", apiErr.Body)
}

func TestDownloadLogsInOnceAndFollowsLink(t *testing.T) {
	var logins, downloads int32
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&logins, 1)
		var body map[string]string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "scout", body["username"])
		require.Equal(t, "secret", body["password"])
		_, _ = w.Write([]byte(`{"token":"tok123","status":200}`))
	})
	var srvURL string
	mux.HandleFunc("/download", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&downloads, 1)
		if r.Header.Get("Authorization") != "Bearer tok123" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.EqualValues(t, 77, body["file_id"])
		require.Equal(t, "srt", body["sub_format"])
		_, _ = w.Write([]byte(`{"link":"` + srvURL + `/file.srt","remaining":19}`))
	})
	mux.HandleFunc("/file.srt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("1\n00:00:01,000 --> 00:00:02,000\nhi\n"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	srvURL = srv.URL
	c := NewOpenSubtitlesClient(&models.SubtitlesCfg{APIKey: "key", Username: "scout", Password: "secret"},
		slog.New(slog.NewTextHandler(new(bytes.Buffer), nil))).WithBaseURL(srv.URL)

	data, remaining, err := c.Download(context.Background(), 77)
	require.NoError(t, err)
	require.Equal(t, 19, remaining)
	require.Contains(t, string(data), "-->")

	_, _, err = c.Download(context.Background(), 77)
	require.NoError(t, err)
	require.EqualValues(t, 1, atomic.LoadInt32(&logins), "token is reused across downloads")
	require.EqualValues(t, 2, atomic.LoadInt32(&downloads))
}

func TestDownloadReLogsInOnExpiredToken(t *testing.T) {
	var logins int32
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&logins, 1)
		_, _ = w.Write([]byte(`{"token":"tok` + string(rune('0'+n)) + `"}`))
	})
	var srvURL string
	mux.HandleFunc("/download", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok2" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"expired"}`))
			return
		}
		_, _ = w.Write([]byte(`{"link":"` + srvURL + `/f","remaining":3}`))
	})
	mux.HandleFunc("/f", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("srt")) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	srvURL = srv.URL
	c := NewOpenSubtitlesClient(&models.SubtitlesCfg{APIKey: "key", Username: "u", Password: "p"},
		slog.New(slog.NewTextHandler(new(bytes.Buffer), nil))).WithBaseURL(srv.URL)

	data, _, err := c.Download(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, "srt", string(data))
	require.EqualValues(t, 2, atomic.LoadInt32(&logins))
}

func TestDownloadWithoutCredentialsIsAnonymous(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) { t.Fatal("no login expected") })
	var srvURL string
	mux.HandleFunc("/download", func(w http.ResponseWriter, r *http.Request) {
		require.Empty(t, r.Header.Get("Authorization"))
		require.Equal(t, "key", r.Header.Get("Api-Key"))
		_, _ = w.Write([]byte(`{"link":"` + srvURL + `/f","remaining":4}`))
	})
	mux.HandleFunc("/f", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("srt")) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	srvURL = srv.URL
	c := NewOpenSubtitlesClient(&models.SubtitlesCfg{APIKey: "key"},
		slog.New(slog.NewTextHandler(new(bytes.Buffer), nil))).WithBaseURL(srv.URL)

	data, remaining, err := c.Download(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, "srt", string(data))
	require.Equal(t, 4, remaining)
}

func TestDownloadQuotaExceeded(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"token":"t"}`)) })
	mux.HandleFunc("/download", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotAcceptable)
		_, _ = w.Write([]byte(`{"message":"You have downloaded your allowed 20 subtitles for 24h.","remaining":0}`))
	})
	c := newTestClient(t, mux, true)
	_, _, err := c.Download(context.Background(), 1)
	var apiErr *APIError
	require.True(t, errors.As(err, &apiErr))
	require.True(t, apiErr.QuotaExceeded())
}

func TestImdbDigits(t *testing.T) {
	require.Equal(t, "0121955", imdbDigits("tt0121955"))
	require.Equal(t, "0121955", imdbDigits(" TT0121955 "))
	require.Equal(t, "3387648", imdbDigits("3387648"))
	require.Equal(t, "", imdbDigits(""))
	require.Equal(t, "", imdbDigits("tt12ab"))
}
