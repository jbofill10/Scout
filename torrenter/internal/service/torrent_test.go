package service

import (
	"bytes"
	"log/slog"
	tvdb "shared/media"
	"testing"
	"torrenter/internal/models"

	"github.com/stretchr/testify/suite"
	qbittorrent "github.com/autobrr/go-qbittorrent"
	"golift.io/starr/prowlarr"
)

type QbittHandlerTestSuite struct {
	suite.Suite
	handler *QbittHandler
	logger  *slog.Logger
}

func TestQbittHandlerSuite(t *testing.T) {
	suite.Run(t, new(QbittHandlerTestSuite))
}

func (s *QbittHandlerTestSuite) SetupTest() {
	buf := new(bytes.Buffer)
	s.logger = slog.New(slog.NewTextHandler(buf, nil))
	// For isCorrectTorrent, we don't need the full handler, but since it's a method, we need an instance
	// We'll create a minimal handler with just logger
	s.handler = &QbittHandler{logger: s.logger}
}

func (s *QbittHandlerTestSuite) TestIsCorrectTorrent_ExcludesBatch() {
	torrent := &prowlarr.Search{
		SortTitle: "Some Show Batch 01",
		Title:     "Some Show",
	}
	strategy := &models.SearchStrategy{
		MediaName: "Some Show",
		Episode:   1,
		Exclude:   []string{},
	}

	result := s.handler.isCorrectTorrent(torrent, strategy)
	s.False(result)
}

func (s *QbittHandlerTestSuite) TestIsCorrectTorrent_ExcludesByWord() {
	torrent := &prowlarr.Search{
		SortTitle: "Some Show Season 1",
		Title:     "Some Show",
	}
	strategy := &models.SearchStrategy{
		MediaName: "Some Show",
		Episode:   1,
		Exclude:   []string{"season"},
	}

	result := s.handler.isCorrectTorrent(torrent, strategy)
	s.False(result)
}

func (s *QbittHandlerTestSuite) TestIsCorrectTorrent_NoEpisode() {
	torrent := &prowlarr.Search{
		SortTitle: "Some Show Other Words",
		Title:     "Some Show",
	}
	strategy := &models.SearchStrategy{
		MediaName: "Some Show",
		Episode:   1,
		Exclude:   []string{},
	}

	result := s.handler.isCorrectTorrent(torrent, strategy)
	s.False(result)
}

func (s *QbittHandlerTestSuite) TestIsCorrectTorrent_NoMediaName() {
	torrent := &prowlarr.Search{
		SortTitle: "Other Show 01",
		Title:     "Other Show",
	}
	strategy := &models.SearchStrategy{
		MediaName: "Some Show",
		Episode:   1,
		Exclude:   []string{},
	}

	result := s.handler.isCorrectTorrent(torrent, strategy)
	s.False(result)
}

func (s *QbittHandlerTestSuite) TestIsCorrectTorrent_MediaNameWithSpacesRemoved() {
	torrent := &prowlarr.Search{
		SortTitle: "Some Show 01",
		Title:     "Some Show",
	}
	strategy := &models.SearchStrategy{
		MediaName: "Some Show",
		Episode:   1,
		Exclude:   []string{},
	}

	result := s.handler.isCorrectTorrent(torrent, strategy)
	s.True(result)
}

func (s *QbittHandlerTestSuite) TestIsCorrectTorrent_CaseInsensitive() {
	torrent := &prowlarr.Search{
		SortTitle: "some show 01",
		Title:     "some show",
	}
	strategy := &models.SearchStrategy{
		MediaName: "Some Show",
		Episode:   1,
		Exclude:   []string{},
	}

	result := s.handler.isCorrectTorrent(torrent, strategy)
	s.True(result)
}

func (s *QbittHandlerTestSuite) TestIsCorrectTorrent_ValidMatch() {
	torrent := &prowlarr.Search{
		SortTitle: "Some Show 01",
		Title:     "Some Show Episode 1",
	}
	strategy := &models.SearchStrategy{
		MediaName: "Some Show",
		Episode:   1,
		Exclude:   []string{},
	}

	result := s.handler.isCorrectTorrent(torrent, strategy)
	s.True(result)
}

func (s *QbittHandlerTestSuite) TestIsCorrectTorrent_EpisodeNotPadded() {
	torrent := &prowlarr.Search{
		SortTitle: "Some Show 1",
		Title:     "Some Show",
	}
	strategy := &models.SearchStrategy{
		MediaName: "Some Show",
		Episode:   1,
		Exclude:   []string{},
	}

	result := s.handler.isCorrectTorrent(torrent, strategy)
	s.True(result)
}

func (s *QbittHandlerTestSuite) TestSortTorrentsByQuality() {
	torrents := []*models.TorrentMatch{
		{Torrent: &prowlarr.Search{SortTitle: "[SubsPlease] Dandadan - 01 (1080p) [2AB10B14].mkv"}},
		{Torrent: &prowlarr.Search{SortTitle: "[SubsPlease] Dandadan - 01 (720p) [2ECB2F39].mkv"}},
		{Torrent: &prowlarr.Search{SortTitle: "[SubsPlease] Dandadan - 01 (480p) [6A4E67F6].mkv"}},
		{Torrent: &prowlarr.Search{SortTitle: "[Erai-raws] Dan Da Dan - 01 (NF) [720p][Multiple Subtitle] [ENG][POR-BR][SPA-LA][SPA][ARA][FRE][GER][ITA][JPN][POL][TUR][IND][THA][KOR][CHI][VIE][MAY][FIL]"}},
		{Torrent: &prowlarr.Search{SortTitle: "[Erai-raws] Dan Da Dan - 01 [1080p][HEVC][Multiple Subtitle] [ENG][POR-BR][SPA-LA][SPA][ARA][FRE][GER][ITA][RUS]"}},
		{Torrent: &prowlarr.Search{SortTitle: "[Erai-raws] Dan Da Dan - 01 (NF) [1080p][HEVC][Multiple Subtitle] [ENG][POR-BR][SPA-LA][SPA][ARA][FRE][GER][ITA][JPN][POL][TUR][IND][THA][KOR][CHI][VIE][MAY][FIL]"}},
		{Torrent: &prowlarr.Search{SortTitle: "[Erai-raws] Dan Da Dan - 01 (EAC3 2.0) [1080p][HEVC][Multiple Subtitle] [ENG][POR-BR][SPA-LA][SPA][ARA][FRE][GER][ITA][RUS]"}},
		{Torrent: &prowlarr.Search{SortTitle: "[Erai-raws] Dan Da Dan - 01 (NF) [1080p][Multiple Subtitle] [ENG][POR-BR][SPA-LA][SPA][ARA][FRE][GER][ITA][JPN][POL][TUR][IND][THA][KOR][CHI][VIE][MAY][FIL]"}},
		{Torrent: &prowlarr.Search{SortTitle: "[Erai-raws] Dan Da Dan - 01 [480p][Multiple Subtitle] [ENG][POR-BR][SPA-LA][SPA][ARA][FRE][GER][ITA][RUS]"}},
		{Torrent: &prowlarr.Search{SortTitle: "[Erai-raws] Dan Da Dan - 01 [720p][Multiple Subtitle] [ENG][POR-BR][SPA-LA][SPA][ARA][FRE][GER][ITA][RUS]"}},
		{Torrent: &prowlarr.Search{SortTitle: "[Erai-raws] Dan Da Dan - 01 [1080p][Multiple Subtitle] [ENG][POR-BR][SPA-LA][SPA][ARA][FRE][GER][ITA][RUS]"}},
		{Torrent: &prowlarr.Search{SortTitle: "[Erai-raws] Dan Da Dan - 01 (EAC3 2.0) [1080p][Multiple Subtitle] [ENG][POR-BR][SPA-LA][SPA][ARA][FRE][GER][ITA][RUS]"}},
		{Torrent: &prowlarr.Search{SortTitle: "[Erai-raws] Dan Da Dan - 01 (EAC3 2.0) [720p][Multiple Subtitle] [ENG][POR-BR][SPA-LA][SPA][ARA][FRE][GER][ITA][RUS]"}},
		{Torrent: &prowlarr.Search{SortTitle: "[Erai-raws] Dan Da Dan - 01 (EAC3 2.0) [480p][Multiple Subtitle] [ENG][POR-BR][SPA-LA][SPA][ARA][FRE][GER][ITA][RUS]"}},
	}

	s.handler.sortTorrentsByQuality(torrents)

	// Check that 1080p torrents come first (indices 0-6)
	for i := 0; i < 7; i++ {
		s.Contains(torrents[i].Torrent.SortTitle, "1080p", "Torrent at index %d should be 1080p", i)
	}

	// Check that 720p torrents come next (indices 7-10)
	for i := 7; i < 11; i++ {
		s.Contains(torrents[i].Torrent.SortTitle, "720p", "Torrent at index %d should be 720p", i)
	}

	// Check that 480p torrents come last (indices 11-13)
	for i := 11; i < 14; i++ {
		s.Contains(torrents[i].Torrent.SortTitle, "480p", "Torrent at index %d should be 480p", i)
	}
}

func (s *QbittHandlerTestSuite) TestSortTorrentsByQuality_4K() {
	torrents := []*models.TorrentMatch{
		{Torrent: &prowlarr.Search{SortTitle: "Show 720p"}},
		{Torrent: &prowlarr.Search{SortTitle: "Show 4K"}},
		{Torrent: &prowlarr.Search{SortTitle: "Show 1080p"}},
	}

	s.handler.sortTorrentsByQuality(torrents)

	s.Contains(torrents[0].Torrent.SortTitle, "4K")
	s.Contains(torrents[1].Torrent.SortTitle, "1080p")
	s.Contains(torrents[2].Torrent.SortTitle, "720p")
}

func (s *QbittHandlerTestSuite) TestSortTorrentsByQuality_2160p() {
	torrents := []*models.TorrentMatch{
		{Torrent: &prowlarr.Search{SortTitle: "Show 720p"}},
		{Torrent: &prowlarr.Search{SortTitle: "Show 2160p"}},
		{Torrent: &prowlarr.Search{SortTitle: "Show 1080p"}},
	}

	s.handler.sortTorrentsByQuality(torrents)

	s.Contains(torrents[0].Torrent.SortTitle, "2160p")
}

func (s *QbittHandlerTestSuite) TestCreateSearchStrategy_Anime() {
	media := &tvdb.Media{
		Name:  "Test Anime",
		Anime: true,
	}

	episode := &tvdb.Episode{
		SeasonNumber:   1,
		Number:         5,
		AbsoluteNumber: 5,
	}

	strategies := s.handler.createSearchStrategy(media, episode)

	// Should create strategies with anime format
	s.NotEmpty(strategies)

	// Check for absolute numbering format
	found := false
	for _, strategy := range strategies {
		if strategy.Query == "Test Anime 05" {
			found = true
			s.Equal(5, strategy.Episode)
			s.Contains(strategy.Exclude, "season")
			s.Contains(strategy.Exclude, "episode")
		}
	}
	s.True(found, "Should have anime-style query")
}

func (s *QbittHandlerTestSuite) TestCreateSearchStrategy_RegularShow() {
	media := &tvdb.Media{
		Name:  "Test Show",
		Anime: false,
	}

	episode := &tvdb.Episode{
		SeasonNumber: 1,
		Number:       5,
	}

	strategies := s.handler.createSearchStrategy(media, episode)

	s.NotEmpty(strategies)

	// Should have S01E05 format
	found := false
	for _, strategy := range strategies {
		if strategy.Query == "Test Show S01E05" {
			found = true
			s.Equal(1, strategy.Season)
			s.Equal(5, strategy.Episode)
		}
	}
	s.True(found, "Should have S01E05 format")
}

func (s *QbittHandlerTestSuite) TestCreateSearchStrategy_WithSpaces() {
	media := &tvdb.Media{
		Name:  "Show With Spaces",
		Anime: false,
	}

	episode := &tvdb.Episode{
		SeasonNumber: 1,
		Number:       1,
	}

	strategies := s.handler.createSearchStrategy(media, episode)

	// Should create strategies using the provided name (no automatic permutations)
	hasSpaced := false
	for _, strategy := range strategies {
		if strategy.MediaName == "Show With Spaces" {
			hasSpaced = true
		}
	}

	s.True(hasSpaced)
	// Should have 2 strategies for non-anime (2 query formats)
	s.Equal(2, len(strategies))
}

func (s *QbittHandlerTestSuite) TestCreateSearchStrategy_WithAliases() {
	media := &tvdb.Media{
		Name:    "Attack on Titan",
		Aliases: []string{"Shingeki no Kyojin", "AoT"},
		Anime:   false,
	}

	episode := &tvdb.Episode{
		SeasonNumber: 1,
		Number:       1,
	}

	strategies := s.handler.createSearchStrategy(media, episode)

	// Should create strategies for original name + all aliases
	// 3 names × 2 query formats = 6 strategies
	s.Equal(6, len(strategies))

	// Verify all three names are present in strategies
	mediaNames := make(map[string]bool)
	for _, strategy := range strategies {
		mediaNames[strategy.MediaName] = true
	}

	s.True(mediaNames["Attack on Titan"])
	s.True(mediaNames["Shingeki no Kyojin"])
	s.True(mediaNames["AoT"])
}

func (s *QbittHandlerTestSuite) TestCreateSearchStrategy_WithDuplicateAliases() {
	media := &tvdb.Media{
		Name:    "Test Show",
		Aliases: []string{"Test Show", "TestShow", "Test Show"}, // Duplicate "Test Show"
		Anime:   false,
	}

	episode := &tvdb.Episode{
		SeasonNumber: 1,
		Number:       1,
	}

	strategies := s.handler.createSearchStrategy(media, episode)

	// Should deduplicate: "Test Show" appears once, "TestShow" appears once
	// 2 unique names × 2 query formats = 4 strategies
	s.Equal(4, len(strategies))

	// Verify only unique names are used
	mediaNames := make(map[string]bool)
	for _, strategy := range strategies {
		mediaNames[strategy.MediaName] = true
	}

	s.Equal(2, len(mediaNames))
	s.True(mediaNames["Test Show"])
	s.True(mediaNames["TestShow"])
}

func (s *QbittHandlerTestSuite) TestCreateSearchStrategy_WithEmptyAliases() {
	media := &tvdb.Media{
		Name:    "Test Show",
		Aliases: []string{"", "Valid Alias", ""}, // Empty strings should be filtered
		Anime:   false,
	}

	episode := &tvdb.Episode{
		SeasonNumber: 1,
		Number:       1,
	}

	strategies := s.handler.createSearchStrategy(media, episode)

	// Should filter empty strings: "Test Show" + "Valid Alias"
	// 2 names × 2 query formats = 4 strategies
	s.Equal(4, len(strategies))

	// Verify only non-empty names are used
	for _, strategy := range strategies {
		s.NotEmpty(strategy.MediaName)
	}
}

func (s *QbittHandlerTestSuite) TestDidTorrentComplete_Completed() {
	torrent := &qbittorrent.Torrent{
		State:     "stalledUP",
		Completed: 1000,
		Size:      1000,
	}

	result := s.handler.didTorrentComplete(torrent)
	s.True(result)
}

func (s *QbittHandlerTestSuite) TestDidTorrentComplete_Downloading() {
	torrent := &qbittorrent.Torrent{
		State:     "downloading",
		Completed: 500,
		Size:      1000,
	}

	result := s.handler.didTorrentComplete(torrent)
	s.False(result)
}

func (s *QbittHandlerTestSuite) TestDidTorrentComplete_PartialStalled() {
	torrent := &qbittorrent.Torrent{
		State:     "stalledUP",
		Completed: 500,
		Size:      1000,
	}

	result := s.handler.didTorrentComplete(torrent)
	s.False(result)
}

func (s *QbittHandlerTestSuite) TestCalcIndexerIDs_Anime() {
	ids := s.handler.calcIndexerIDs("series", true)

	s.Len(ids, 1)
	s.Equal(NYAA_ID, ids[0])
}

func (s *QbittHandlerTestSuite) TestCalcIndexerIDs_NonAnime() {
	ids := s.handler.calcIndexerIDs("series", false)

	s.Len(ids, 2)
	s.Contains(ids, NYAA_ID)
	s.Contains(ids, ONE337x_ID)
}

func (s *QbittHandlerTestSuite) TestStandardizeNumber() {
	testCases := []struct {
		input    int
		expected string
	}{
		{1, "01"},
		{5, "05"},
		{9, "09"},
		{10, "10"},
		{15, "15"},
		{99, "99"},
		{100, "100"},
	}

	for _, tc := range testCases {
		result := standardizeNumber(tc.input)
		s.Equal(tc.expected, result, "standardizeNumber(%d) should be %s", tc.input, tc.expected)
	}
}
