package service

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jbofill10/scout/backend/internal/torrenter/clients"
	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	servicemocks "github.com/jbofill10/scout/backend/internal/torrenter/service/mocks"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// ---- hashing ----

func writeTempVideo(t *testing.T, size int, edit func([]byte)) string {
	t.Helper()
	data := make([]byte, size)
	if edit != nil {
		edit(data)
	}
	path := filepath.Join(t.TempDir(), "video.mkv")
	require.NoError(t, os.WriteFile(path, data, 0644))
	return path
}

func TestOpenSubtitlesHash(t *testing.T) {
	// All zeros: the hash is just the size.
	path := writeTempVideo(t, 3*hashChunkSize, nil)
	hash, size, err := OpenSubtitlesHash(path)
	require.NoError(t, err)
	require.Equal(t, int64(3*hashChunkSize), size)
	require.Equal(t, fmt.Sprintf("%016x", 3*hashChunkSize), hash)

	// One uint64 of 5 in the head and one of 7 in the tail add to the size;
	// bytes in the middle do not count.
	path = writeTempVideo(t, 3*hashChunkSize, func(b []byte) {
		binary.LittleEndian.PutUint64(b[8:16], 5)
		binary.LittleEndian.PutUint64(b[len(b)-16:len(b)-8], 7)
		b[hashChunkSize+100] = 0xff // middle, ignored
	})
	hash, _, err = OpenSubtitlesHash(path)
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("%016x", 3*hashChunkSize+12), hash)

	// Overflow wraps mod 2^64 rather than erroring.
	path = writeTempVideo(t, 2*hashChunkSize, func(b []byte) {
		binary.LittleEndian.PutUint64(b[0:8], ^uint64(0))
	})
	hash, _, err = OpenSubtitlesHash(path)
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("%016x", uint64(2*hashChunkSize)-1), hash)

	// Too small to hash.
	path = writeTempVideo(t, 1000, nil)
	_, _, err = OpenSubtitlesHash(path)
	require.Error(t, err)
}

// ---- sanity check ----

func srtWithCues(n int, lastSec int) []byte {
	var b strings.Builder
	for i := 0; i < n; i++ {
		start := lastSec * i / n
		if i == n-1 {
			start = lastSec
		}
		fmt.Fprintf(&b, "%d\n%02d:%02d:%02d,000 --> %02d:%02d:%02d,500\nline %d\n\n",
			i+1, start/3600, start%3600/60, start%60, start/3600, start%3600/60, start%60, i)
	}
	return []byte(b.String())
}

func TestCheckSubtitle(t *testing.T) {
	cases := []struct {
		name    string
		data    []byte
		runtime int
		movie   bool
		wantErr string
	}{
		{"movie ok", srtWithCues(800, 85*60), 90, true, ""},
		{"episode ok", srtWithCues(300, 20*60), 22, false, ""},
		{"unknown runtime only counts cues", srtWithCues(60, 3*3600), 0, true, ""},
		{"too few cues for a movie", srtWithCues(30, 85*60), 90, true, "only 30 cues"},
		{"too few cues for an episode", srtWithCues(10, 20*60), 22, false, "only 10 cues"},
		{"runs past the runtime", srtWithCues(800, 130*60), 90, true, "past the"},
		{"covers under half", srtWithCues(800, 30*60), 90, true, "under half"},
		{"not srt at all", []byte("<html>quota exceeded</html>"), 90, true, "only 0 cues"},
		{"dot millisecond separator accepted", bytes.ReplaceAll(srtWithCues(800, 85*60), []byte(","), []byte(".")), 90, true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckSubtitle(tc.data, tc.runtime, tc.movie)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

// ---- scoring ----

func TestScoreCandidate(t *testing.T) {
	movie := models.SubtitleRequest{IsMovie: true, ReleaseName: "The.Taking.of.Deborah.Logan.2014.1080p.BluRay.x264.YIFY"}
	episode := models.SubtitleRequest{IsMovie: false, Season: 18, Episode: 3, ReleaseName: "South.Park.S18E03.1080p.WEB.x264-GROUP"}

	t.Run("rejections", func(t *testing.T) {
		rejected := []models.SubtitleCandidate{
			{Language: "es"},
			{Language: "en", MachineTranslated: true},
			{Language: "en", AITranslated: true},
			{Language: "en", ForeignPartsOnly: true},
		}
		for _, c := range rejected {
			_, ok := scoreCandidate(c, movie, "en", false)
			require.False(t, ok, "%+v", c)
		}
		_, ok := scoreCandidate(models.SubtitleCandidate{Language: "en", Season: 18, Episode: 4}, episode, "en", false)
		require.False(t, ok, "wrong episode must be rejected")
		_, ok = scoreCandidate(models.SubtitleCandidate{Language: "en", Season: 18, Episode: 3}, episode, "en", false)
		require.True(t, ok)
		_, ok = scoreCandidate(models.SubtitleCandidate{Language: "en"}, episode, "en", false)
		require.True(t, ok, "provider that omits feature details is still usable")
	})

	t.Run("ordering", func(t *testing.T) {
		hash := models.SubtitleCandidate{FileID: 1, Language: "en", HashMatch: true, DownloadCount: 10}
		sameGroup := models.SubtitleCandidate{FileID: 2, Language: "en", Release: "The.Taking.of.Deborah.Logan.2014.720p.BluRay.x264.[YTS.MX]", DownloadCount: 500}
		popular := models.SubtitleCandidate{FileID: 3, Language: "en", Release: "Deborah.Logan.2014.WEBRip", DownloadCount: 100000}
		sdhSameGroup := sameGroup
		sdhSameGroup.FileID = 4
		sdhSameGroup.HearingImpaired = true

		ranked := rankCandidates([]models.SubtitleCandidate{popular, sdhSameGroup, sameGroup, hash}, movie, "en", false)
		ids := []int{}
		for _, r := range ranked {
			ids = append(ids, r.FileID)
		}
		require.Equal(t, []int{1, 2, 4, 3}, ids,
			"hash match first, then same release group, plain before SDH, popularity last")

		ranked = rankCandidates([]models.SubtitleCandidate{sameGroup, sdhSameGroup}, movie, "en", true)
		require.Equal(t, 4, ranked[0].FileID, "preferSDH flips the SDH penalty")
	})

	t.Run("sdh alone still wins", func(t *testing.T) {
		only := models.SubtitleCandidate{FileID: 9, Language: "en", HearingImpaired: true}
		ranked := rankCandidates([]models.SubtitleCandidate{only}, movie, "en", false)
		require.Len(t, ranked, 1)
	})
}

func TestSubtitlePath(t *testing.T) {
	require.Equal(t, "/m/Movie (2014)/Movie.2014.1080p.en.srt", subtitlePath("/m/Movie (2014)/Movie.2014.1080p.mkv", "en", false))
	require.Equal(t, "/m/Movie (2014)/Movie.2014.1080p.en.sdh.srt", subtitlePath("/m/Movie (2014)/Movie.2014.1080p.mkv", "en", true))
	require.Equal(t, "/s/Show/Season 01/Show - S01E02.es.srt", subtitlePath("/s/Show/Season 01/Show - S01E02.mp4", "es", false))
}

// ---- fetch flow ----

type subtitleSuite struct {
	suite.Suite
	provider *servicemocks.SubtitleProvider
	fs       *servicemocks.FileSystem
	plex     *servicemocks.PlexRefresher
	cfg      *models.SubtitlesCfg
	svc      *SubtitleSvc
	video    string
	library  string
}

func TestSubtitleSuite(t *testing.T) { suite.Run(t, new(subtitleSuite)) }

func (s *subtitleSuite) SetupTest() {
	s.provider = servicemocks.NewSubtitleProvider(s.T())
	s.fs = servicemocks.NewFileSystem(s.T())
	s.plex = servicemocks.NewPlexRefresher(s.T())
	s.cfg = &models.SubtitlesCfg{Languages: []string{"en"}, MaxDownloadsPerItem: 2, Timeout: 10 * time.Second}
	s.svc = NewSubtitleSvc(s.cfg, s.provider, s.fs, s.plex, slog.New(slog.NewTextHandler(new(bytes.Buffer), nil)))
	s.video = writeTempVideo(s.T(), 3*hashChunkSize, nil)
	s.library = "/movies/Movie (2014)/Movie.2014.1080p.BluRay.x264.YIFY.mkv"
}

func (s *subtitleSuite) movieReq() models.SubtitleRequest {
	return models.SubtitleRequest{
		VideoPath: s.video, LibraryPath: s.library, ReleaseName: "Movie.2014.1080p.BluRay.x264.YIFY",
		ImdbID: "tt3387648", Title: "Movie", Year: "2014", IsMovie: true, RuntimeMinutes: 90,
	}
}

func (s *subtitleSuite) expectNoSubtitlesYet() {
	s.fs.On("Exists", subtitlePath(s.library, "en", false)).Return(false)
	s.fs.On("Exists", subtitlePath(s.library, "en", true)).Return(false)
}

func (s *subtitleSuite) TestWritesBestCandidateAndRefreshesPlex() {
	s.expectNoSubtitlesYet()
	expectedHash := fmt.Sprintf("%016x", 3*hashChunkSize)
	s.provider.On("Search", mock.Anything, mock.MatchedBy(func(p models.SubtitleSearchParams) bool {
		return p.ImdbID == "tt3387648" && p.IsMovie && p.MovieHash == expectedHash && p.Query == "" &&
			len(p.Languages) == 1 && p.Languages[0] == "en"
	})).Return([]models.SubtitleCandidate{
		{FileID: 10, Language: "en", Release: "Movie.2014.720p.WEBRip", DownloadCount: 5000},
		{FileID: 11, Language: "en", HashMatch: true, Release: "Movie.2014.1080p.BluRay.x264.YIFY"},
	}, nil)
	good := srtWithCues(900, 86*60)
	s.provider.On("Download", mock.Anything, 11).Return(good, 17, nil)
	s.fs.On("WriteFile", subtitlePath(s.library, "en", false), good).Return(nil)
	s.plex.On("RefreshPath", mock.Anything, true, "/movies/Movie (2014)").Return(nil)

	results, err := s.svc.Fetch(context.Background(), s.movieReq())
	s.NoError(err)
	s.Require().Len(results, 1)
	s.True(results[0].HashMatch)
	s.Equal(17, results[0].Remaining)
	s.Equal(subtitlePath(s.library, "en", false), results[0].Path)
}

func (s *subtitleSuite) TestFallsToNextCandidateWhenSanityCheckFails() {
	s.expectNoSubtitlesYet()
	s.provider.On("Search", mock.Anything, mock.Anything).Return([]models.SubtitleCandidate{
		{FileID: 1, Language: "en", HashMatch: true},
		{FileID: 2, Language: "en", HearingImpaired: true, DownloadCount: 50},
	}, nil)
	s.provider.On("Download", mock.Anything, 1).Return(srtWithCues(900, 200*60), 10, nil) // wrong cut
	good := srtWithCues(900, 80*60)
	s.provider.On("Download", mock.Anything, 2).Return(good, 9, nil)
	s.fs.On("WriteFile", subtitlePath(s.library, "en", true), good).Return(nil)
	s.plex.On("RefreshPath", mock.Anything, true, mock.Anything).Return(nil)

	results, err := s.svc.Fetch(context.Background(), s.movieReq())
	s.NoError(err)
	s.Require().Len(results, 1)
	s.True(results[0].HearingImpaired)
}

func (s *subtitleSuite) TestStopsAtDownloadCap() {
	s.expectNoSubtitlesYet()
	s.provider.On("Search", mock.Anything, mock.Anything).Return([]models.SubtitleCandidate{
		{FileID: 1, Language: "en", DownloadCount: 300},
		{FileID: 2, Language: "en", DownloadCount: 200},
		{FileID: 3, Language: "en", DownloadCount: 100},
	}, nil)
	s.provider.On("Download", mock.Anything, 1).Return(srtWithCues(10, 80*60), 10, nil)
	s.provider.On("Download", mock.Anything, 2).Return(srtWithCues(10, 80*60), 9, nil)
	// Third is never downloaded: MaxDownloadsPerItem is 2.

	results, err := s.svc.Fetch(context.Background(), s.movieReq())
	s.NoError(err)
	s.Empty(results)
}

func (s *subtitleSuite) TestSkipsWhenSubtitleAlreadyPresent() {
	s.fs.On("Exists", subtitlePath(s.library, "en", false)).Return(true)
	results, err := s.svc.Fetch(context.Background(), s.movieReq())
	s.NoError(err)
	s.Empty(results)
	s.provider.AssertNotCalled(s.T(), "Search", mock.Anything, mock.Anything)
}

func (s *subtitleSuite) TestEpisodeWithoutImdbUsesTitleSearch() {
	req := s.movieReq()
	req.IsMovie = false
	req.ImdbID = ""
	req.Title = "South Park"
	req.Season = 18
	req.Episode = 3
	req.RuntimeMinutes = 22
	req.LibraryPath = "/shows/South Park/Season 18/South Park - S18E03.mp4"
	s.fs.On("Exists", mock.Anything).Return(false)
	s.provider.On("Search", mock.Anything, mock.MatchedBy(func(p models.SubtitleSearchParams) bool {
		return !p.IsMovie && p.Query == "South Park" && p.Season == 18 && p.Episode == 3 && p.ImdbID == "" && p.ParentImdbID == ""
	})).Return([]models.SubtitleCandidate{
		{FileID: 5, Language: "en", Season: 18, Episode: 4}, // neighbour, rejected
		{FileID: 6, Language: "en", Season: 18, Episode: 3},
	}, nil)
	good := srtWithCues(300, 21*60)
	s.provider.On("Download", mock.Anything, 6).Return(good, 1, nil)
	s.fs.On("WriteFile", "/shows/South Park/Season 18/South Park - S18E03.en.srt", good).Return(nil)
	s.plex.On("RefreshPath", mock.Anything, false, "/shows/South Park/Season 18").Return(nil)

	results, err := s.svc.Fetch(context.Background(), req)
	s.NoError(err)
	s.Len(results, 1)
}

func (s *subtitleSuite) TestIdSearchEmptyFallsBackToTitle() {
	s.expectNoSubtitlesYet()
	s.provider.On("Search", mock.Anything, mock.MatchedBy(func(p models.SubtitleSearchParams) bool { return p.ImdbID != "" })).
		Return(nil, nil).Once()
	s.provider.On("Search", mock.Anything, mock.MatchedBy(func(p models.SubtitleSearchParams) bool { return p.Query == "Movie" && p.Year == "2014" })).
		Return(nil, nil).Once()
	results, err := s.svc.Fetch(context.Background(), s.movieReq())
	s.NoError(err)
	s.Empty(results)
}

func (s *subtitleSuite) TestQuotaExceededPausesUntilNextDay() {
	s.expectNoSubtitlesYet()
	now := time.Date(2026, 10, 25, 20, 0, 0, 0, time.UTC)
	s.svc.now = func() time.Time { return now }
	s.provider.On("Search", mock.Anything, mock.Anything).Return([]models.SubtitleCandidate{{FileID: 1, Language: "en"}}, nil).Once()
	s.provider.On("Download", mock.Anything, 1).Return(nil, 0, &clients.APIError{Status: 406, Body: "quota"}).Once()

	_, err := s.svc.Fetch(context.Background(), s.movieReq())
	s.Error(err)
	s.Contains(err.Error(), "quota")

	// The next fetch the same day does not touch the provider at all.
	_, err = s.svc.Fetch(context.Background(), s.movieReq())
	s.Error(err)
	s.provider.AssertNumberOfCalls(s.T(), "Search", 1)

	// After midnight UTC it searches again (by id, then the title fallback).
	s.svc.now = func() time.Time { return now.Add(5 * time.Hour) }
	s.provider.On("Search", mock.Anything, mock.Anything).Return(nil, nil).Twice()
	_, err = s.svc.Fetch(context.Background(), s.movieReq())
	s.NoError(err)
}

func (s *subtitleSuite) TestSearchErrorIsReturned() {
	s.expectNoSubtitlesYet()
	s.provider.On("Search", mock.Anything, mock.Anything).Return(nil, errors.New("boom"))
	_, err := s.svc.Fetch(context.Background(), s.movieReq())
	s.Error(err)
}

func (s *subtitleSuite) TestUnhashableVideoStillSearches() {
	s.expectNoSubtitlesYet()
	req := s.movieReq()
	req.VideoPath = filepath.Join(s.T().TempDir(), "missing.mkv")
	s.provider.On("Search", mock.Anything, mock.MatchedBy(func(p models.SubtitleSearchParams) bool { return p.MovieHash == "" })).
		Return(nil, nil)
	_, err := s.svc.Fetch(context.Background(), req)
	s.NoError(err)
}
