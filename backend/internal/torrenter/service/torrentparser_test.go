package service

import (
	"testing"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"
)

func TestTorrentParser_Parse_NYAAAbsolute(t *testing.T) {
	parser := NewTorrentParser()

	testCases := []struct {
		name           string
		title          string
		expectedShow   string
		expectedEp     int
		expectedQual   string
		expectedGroup  string
		expectedVer    int
		shouldBeAbs    bool
		shouldBeSeas   bool
	}{
		{
			name:          "SubsPlease standard",
			title:         "[SubsPlease] Attack on Titan - 05 [1080p][ABC123].mkv",
			expectedShow:  "Attack on Titan",
			expectedEp:    5,
			expectedQual:  "1080p",
			expectedGroup: "SubsPlease",
			shouldBeAbs:   true,
			shouldBeSeas:  false,
		},
		{
			name:          "EMBER with v2",
			title:         "[EMBER] Jujutsu Kaisen - 12v2 [720p][DEF456]",
			expectedShow:  "Jujutsu Kaisen",
			expectedEp:    12,
			expectedQual:  "720p",
			expectedGroup: "EMBER",
			expectedVer:   2,
			shouldBeAbs:   true,
			shouldBeSeas:  false,
		},
		{
			name:          "Three digit episode",
			title:         "[Group] One Piece - 123 [1080p][HASH]",
			expectedShow:  "One Piece",
			expectedEp:    123,
			expectedQual:  "1080p",
			expectedGroup: "Group",
			shouldBeAbs:   true,
			shouldBeSeas:  false,
		},
		{
			name:          "4K quality",
			title:         "[Group] Show - 01 [4K][HASH]",
			expectedShow:  "Show",
			expectedEp:    1,
			expectedQual:  "4k",
			expectedGroup: "Group",
			shouldBeAbs:   true,
			shouldBeSeas:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := parser.Parse(tc.title)
			if result == nil {
				t.Fatal("Parser returned nil")
			}

			if result.ShowName != tc.expectedShow {
				t.Errorf("ShowName: expected %q, got %q", tc.expectedShow, result.ShowName)
			}
			if result.Episode != tc.expectedEp {
				t.Errorf("Episode: expected %d, got %d", tc.expectedEp, result.Episode)
			}
			if result.Quality != tc.expectedQual {
				t.Errorf("Quality: expected %q, got %q", tc.expectedQual, result.Quality)
			}
			if result.ReleaseGroup != tc.expectedGroup {
				t.Errorf("ReleaseGroup: expected %q, got %q", tc.expectedGroup, result.ReleaseGroup)
			}
			if result.Version != tc.expectedVer {
				t.Errorf("Version: expected %d, got %d", tc.expectedVer, result.Version)
			}
			if result.IsAbsolute != tc.shouldBeAbs {
				t.Errorf("IsAbsolute: expected %v, got %v", tc.shouldBeAbs, result.IsAbsolute)
			}
			if result.IsSeasonal != tc.shouldBeSeas {
				t.Errorf("IsSeasonal: expected %v, got %v", tc.shouldBeSeas, result.IsSeasonal)
			}
		})
	}
}

func TestTorrentParser_Parse_NYAASeasonal(t *testing.T) {
	parser := NewTorrentParser()

	testCases := []struct {
		name           string
		title          string
		expectedShow   string
		expectedSeason int
		expectedEp     int
		expectedQual   string
		expectedGroup  string
		shouldBeAbs    bool
		shouldBeSeas   bool
	}{
		{
			name:           "Standard seasonal format",
			title:          "[Group] Breaking Bad - S02E05 [1080p]",
			expectedShow:   "Breaking Bad",
			expectedSeason: 2,
			expectedEp:     5,
			expectedQual:   "1080p",
			expectedGroup:  "Group",
			shouldBeAbs:    false,
			shouldBeSeas:   true,
		},
		{
			name:           "Single digit season",
			title:          "[Group] Show - S1E10 [720p]",
			expectedShow:   "Show",
			expectedSeason: 1,
			expectedEp:     10,
			expectedQual:   "720p",
			expectedGroup:  "Group",
			shouldBeAbs:    false,
			shouldBeSeas:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := parser.Parse(tc.title)
			if result == nil {
				t.Fatal("Parser returned nil")
			}

			if result.ShowName != tc.expectedShow {
				t.Errorf("ShowName: expected %q, got %q", tc.expectedShow, result.ShowName)
			}
			if result.Season != tc.expectedSeason {
				t.Errorf("Season: expected %d, got %d", tc.expectedSeason, result.Season)
			}
			if result.Episode != tc.expectedEp {
				t.Errorf("Episode: expected %d, got %d", tc.expectedEp, result.Episode)
			}
			if result.Quality != tc.expectedQual {
				t.Errorf("Quality: expected %q, got %q", tc.expectedQual, result.Quality)
			}
			if result.ReleaseGroup != tc.expectedGroup {
				t.Errorf("ReleaseGroup: expected %q, got %q", tc.expectedGroup, result.ReleaseGroup)
			}
			if result.IsAbsolute != tc.shouldBeAbs {
				t.Errorf("IsAbsolute: expected %v, got %v", tc.shouldBeAbs, result.IsAbsolute)
			}
			if result.IsSeasonal != tc.shouldBeSeas {
				t.Errorf("IsSeasonal: expected %v, got %v", tc.shouldBeSeas, result.IsSeasonal)
			}
		})
	}
}

func TestTorrentParser_Parse_SceneFormat(t *testing.T) {
	parser := NewTorrentParser()

	testCases := []struct {
		name           string
		title          string
		expectedShow   string
		expectedSeason int
		expectedEp     int
		expectedEpEnd  int
		expectedQual   string
		expectedGroup  string
		shouldBeAbs    bool
		shouldBeSeas   bool
	}{
		{
			name:           "Scene standard format",
			title:          "Breaking.Bad.S01E05.Gray.Matter.1080p.BluRay.x264-DEMAND",
			expectedShow:   "Breaking Bad",
			expectedSeason: 1,
			expectedEp:     5,
			expectedEpEnd:  0,
			expectedQual:   "1080p",
			expectedGroup:  "DEMAND",
			shouldBeAbs:    false,
			shouldBeSeas:   true,
		},
		{
			name:           "Scene simple format",
			title:          "Show.Name.S02E10.1080p.mkv",
			expectedShow:   "Show Name",
			expectedSeason: 2,
			expectedEp:     10,
			expectedEpEnd:  0,
			expectedQual:   "1080p",
			shouldBeAbs:    false,
			shouldBeSeas:   true,
		},
		{
			name:           "Multi-episode scene format",
			title:          "Show.S01E05E06.1080p.WEB.x264-GROUP",
			expectedShow:   "Show",
			expectedSeason: 1,
			expectedEp:     5,
			expectedEpEnd:  6,
			expectedQual:   "1080p",
			expectedGroup:  "GROUP",
			shouldBeAbs:    false,
			shouldBeSeas:   true,
		},
		{
			name:           "Scene with dots in show name",
			title:          "Mr.Robot.S01E01.1080p.BluRay.x264-GROUP",
			expectedShow:   "Mr Robot",
			expectedSeason: 1,
			expectedEp:     1,
			expectedQual:   "1080p",
			expectedGroup:  "GROUP",
			shouldBeAbs:    false,
			shouldBeSeas:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := parser.Parse(tc.title)
			if result == nil {
				t.Fatal("Parser returned nil")
			}

			if result.ShowName != tc.expectedShow {
				t.Errorf("ShowName: expected %q, got %q", tc.expectedShow, result.ShowName)
			}
			if result.Season != tc.expectedSeason {
				t.Errorf("Season: expected %d, got %d", tc.expectedSeason, result.Season)
			}
			if result.Episode != tc.expectedEp {
				t.Errorf("Episode: expected %d, got %d", tc.expectedEp, result.Episode)
			}
			if result.EpisodeEnd != tc.expectedEpEnd {
				t.Errorf("EpisodeEnd: expected %d, got %d", tc.expectedEpEnd, result.EpisodeEnd)
			}
			if result.Quality != tc.expectedQual {
				t.Errorf("Quality: expected %q, got %q", tc.expectedQual, result.Quality)
			}
			if tc.expectedGroup != "" && result.ReleaseGroup != tc.expectedGroup {
				t.Errorf("ReleaseGroup: expected %q, got %q", tc.expectedGroup, result.ReleaseGroup)
			}
			if result.IsAbsolute != tc.shouldBeAbs {
				t.Errorf("IsAbsolute: expected %v, got %v", tc.shouldBeAbs, result.IsAbsolute)
			}
			if result.IsSeasonal != tc.shouldBeSeas {
				t.Errorf("IsSeasonal: expected %v, got %v", tc.shouldBeSeas, result.IsSeasonal)
			}
		})
	}
}

func TestTorrentParser_Parse_AbsoluteFormats(t *testing.T) {
	parser := NewTorrentParser()

	testCases := []struct {
		name         string
		title        string
		expectedShow string
		expectedEp   int
		expectedQual string
		shouldBeAbs  bool
	}{
		{
			name:         "Absolute with parentheses",
			title:        "Show Name - 05 (1080p)",
			expectedShow: "Show Name",
			expectedEp:   5,
			expectedQual: "1080p",
			shouldBeAbs:  true,
		},
		{
			name:         "Absolute simple format",
			title:        "Anime Title - 12 1080p HEVC",
			expectedShow: "Anime Title",
			expectedEp:   12,
			expectedQual: "1080p",
			shouldBeAbs:  true,
		},
		{
			name:         "Absolute with version",
			title:        "Show - 08v2 (720p)",
			expectedShow: "Show",
			expectedEp:   8,
			expectedQual: "720p",
			shouldBeAbs:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := parser.Parse(tc.title)
			if result == nil {
				t.Fatal("Parser returned nil")
			}

			if result.ShowName != tc.expectedShow {
				t.Errorf("ShowName: expected %q, got %q", tc.expectedShow, result.ShowName)
			}
			if result.Episode != tc.expectedEp {
				t.Errorf("Episode: expected %d, got %d", tc.expectedEp, result.Episode)
			}
			if result.Quality != tc.expectedQual {
				t.Errorf("Quality: expected %q, got %q", tc.expectedQual, result.Quality)
			}
			if result.IsAbsolute != tc.shouldBeAbs {
				t.Errorf("IsAbsolute: expected %v, got %v", tc.shouldBeAbs, result.IsAbsolute)
			}
		})
	}
}

func TestTorrentParser_Parse_AlternativeFormat(t *testing.T) {
	parser := NewTorrentParser()

	testCases := []struct {
		name           string
		title          string
		expectedShow   string
		expectedSeason int
		expectedEp     int
		expectedQual   string
	}{
		{
			name:           "Alternative 2x05 format",
			title:          "Breaking Bad 2x05 1080p",
			expectedShow:   "Breaking Bad",
			expectedSeason: 2,
			expectedEp:     5,
			expectedQual:   "1080p",
		},
		{
			name:           "Alternative 1x10 format",
			title:          "Show 1x10 720p HDTV",
			expectedShow:   "Show",
			expectedSeason: 1,
			expectedEp:     10,
			expectedQual:   "720p",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := parser.Parse(tc.title)
			if result == nil {
				t.Fatal("Parser returned nil")
			}

			if result.ShowName != tc.expectedShow {
				t.Errorf("ShowName: expected %q, got %q", tc.expectedShow, result.ShowName)
			}
			if result.Season != tc.expectedSeason {
				t.Errorf("Season: expected %d, got %d", tc.expectedSeason, result.Season)
			}
			if result.Episode != tc.expectedEp {
				t.Errorf("Episode: expected %d, got %d", tc.expectedEp, result.Episode)
			}
			if result.Quality != tc.expectedQual {
				t.Errorf("Quality: expected %q, got %q", tc.expectedQual, result.Quality)
			}
		})
	}
}

func TestParsedTorrent_IsMultiEpisode(t *testing.T) {
	testCases := []struct {
		name      string
		torrent   ParsedTorrent
		expected  bool
	}{
		{
			name:     "Single episode",
			torrent:  ParsedTorrent{Episode: 5, EpisodeEnd: 0},
			expected: false,
		},
		{
			name:     "Multi episode",
			torrent:  ParsedTorrent{Episode: 5, EpisodeEnd: 8},
			expected: true,
		},
		{
			name:     "Same start and end",
			torrent:  ParsedTorrent{Episode: 5, EpisodeEnd: 5},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.torrent.IsMultiEpisode()
			if result != tc.expected {
				t.Errorf("IsMultiEpisode: expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestParsedTorrent_ContainsEpisode(t *testing.T) {
	testCases := []struct {
		name       string
		torrent    ParsedTorrent
		checkEp    int
		expected   bool
	}{
		{
			name:     "Single episode - match",
			torrent:  ParsedTorrent{Episode: 5, EpisodeEnd: 0},
			checkEp:  5,
			expected: true,
		},
		{
			name:     "Single episode - no match",
			torrent:  ParsedTorrent{Episode: 5, EpisodeEnd: 0},
			checkEp:  6,
			expected: false,
		},
		{
			name:     "Multi episode - first episode",
			torrent:  ParsedTorrent{Episode: 5, EpisodeEnd: 8},
			checkEp:  5,
			expected: true,
		},
		{
			name:     "Multi episode - middle episode",
			torrent:  ParsedTorrent{Episode: 5, EpisodeEnd: 8},
			checkEp:  6,
			expected: true,
		},
		{
			name:     "Multi episode - last episode",
			torrent:  ParsedTorrent{Episode: 5, EpisodeEnd: 8},
			checkEp:  8,
			expected: true,
		},
		{
			name:     "Multi episode - outside range",
			torrent:  ParsedTorrent{Episode: 5, EpisodeEnd: 8},
			checkEp:  9,
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.torrent.ContainsEpisode(tc.checkEp)
			if result != tc.expected {
				t.Errorf("ContainsEpisode(%d): expected %v, got %v", tc.checkEp, tc.expected, result)
			}
		})
	}
}

func TestParsedTorrent_GetQualityTier(t *testing.T) {
	testCases := []struct {
		quality  string
		expected int
	}{
		{"4k", 5},
		{"4K", 5},
		{"2160p", 5},
		{"1080p", 4},
		{"720p", 3},
		{"480p", 2},
		{"unknown", 1},
		{"", 1},
	}

	for _, tc := range testCases {
		t.Run(tc.quality, func(t *testing.T) {
			pt := ParsedTorrent{Quality: tc.quality}
			result := pt.GetQualityTier()
			if result != tc.expected {
				t.Errorf("GetQualityTier for %q: expected %d, got %d", tc.quality, tc.expected, result)
			}
		})
	}
}

func TestTorrentParser_Parse_EdgeCases(t *testing.T) {
	parser := NewTorrentParser()

	testCases := []struct {
		name       string
		title      string
		shouldFail bool
	}{
		{
			name:       "Batch torrent (should parse but IsMultiEpisode should handle rejection)",
			title:      "[Group] Show - Batch [1080p]",
			shouldFail: true,
		},
		{
			name:       "Empty string",
			title:      "",
			shouldFail: true,
		},
		{
			name:       "Invalid format",
			title:      "Some Random Text Without Episode Info",
			shouldFail: true,
		},
		{
			name:       "Show name with special characters",
			title:      "[Group] Re:Zero - 05 [1080p][HASH]",
			shouldFail: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := parser.Parse(tc.title)
			if tc.shouldFail && result != nil {
				t.Errorf("Expected parse to fail, but got result: %+v", result)
			}
			if !tc.shouldFail && result == nil {
				t.Error("Expected parse to succeed, but got nil")
			}
		})
	}
}

func TestCleanShowName(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"Show.Name.With.Dots", "Show Name With Dots"},
		{"Show_Name_With_Underscores", "Show Name With Underscores"},
		{"Show  With   Multiple    Spaces", "Show With Multiple Spaces"},
		{"  Show With Leading Trailing  ", "Show With Leading Trailing"},
		{"Normal Show Name", "Normal Show Name"},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := cleanShowName(tc.input)
			if result != tc.expected {
				t.Errorf("cleanShowName(%q): expected %q, got %q", tc.input, tc.expected, result)
			}
		})
	}
}

func TestExtractQuality(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"[1080p][HASH]", "1080p"},
		{"720p HEVC", "720p"},
		{"4K HDR", "4k"},
		{"2160p", "2160p"},
		{"No quality here", ""},
		{"", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := extractQuality(tc.input)
			if result != tc.expected {
				t.Errorf("extractQuality(%q): expected %q, got %q", tc.input, tc.expected, result)
			}
		})
	}
}

// Test real-world torrent names
func TestTorrentParser_Parse_RealWorldExamples(t *testing.T) {
	parser := NewTorrentParser()

	testCases := []struct {
		name         string
		title        string
		shouldParse  bool
		expectedShow string
		expectedEp   int
	}{
		{
			name:         "SubsPlease Frieren",
			title:        "[SubsPlease] Sousou no Frieren - 05 (1080p) [ABC12345].mkv",
			shouldParse:  true,
			expectedShow: "Sousou no Frieren",
			expectedEp:   5,
		},
		{
			name:         "Erai-raws with brackets",
			title:        "[Erai-raws] Jujutsu Kaisen - 12 [1080p][Multiple Subtitle].mkv",
			shouldParse:  true,
			expectedShow: "Jujutsu Kaisen",
			expectedEp:   12,
		},
		{
			name:         "RARBG scene release",
			title:        "The.Mandalorian.S02E05.1080p.WEB.h264-KOGi[rarbg]",
			shouldParse:  true,
			expectedShow: "The Mandalorian",
			expectedEp:   5,
		},
		{
			name:         "YTS movie (should not parse as episode)",
			title:        "Inception.2010.1080p.BluRay.x264-[YTS.AM]",
			shouldParse:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := parser.Parse(tc.title)
			if tc.shouldParse && result == nil {
				t.Fatal("Expected parse to succeed, but got nil")
			}
			if !tc.shouldParse && result != nil {
				t.Fatalf("Expected parse to fail, but got result: %+v", result)
			}
			if tc.shouldParse {
				if result.ShowName != tc.expectedShow {
					t.Errorf("ShowName: expected %q, got %q", tc.expectedShow, result.ShowName)
				}
				if result.Episode != tc.expectedEp {
					t.Errorf("Episode: expected %d, got %d", tc.expectedEp, result.Episode)
				}
			}
		})
	}
}

// Test that different formats all parse to the same episode
// This validates parser consistency across format variations
func TestTorrentParser_Parse_SameEpisodeDifferentFormats(t *testing.T) {
	parser := NewTorrentParser()

	t.Run("Seasonal formats - Breaking Bad S02E05", func(t *testing.T) {
		testCases := []struct {
			name   string
			title  string
			format string
		}{
			{
				name:   "NYAA seasonal format",
				title:  "[Group] Breaking Bad - S02E05 [1080p]",
				format: "NYAA S##E##",
			},
			{
				name:   "Scene format simple",
				title:  "Breaking.Bad.S02E05.1080p.mkv",
				format: "Scene dots",
			},
			{
				name:   "Scene format full",
				title:  "Breaking.Bad.S02E05.Gray.Matter.1080p.BluRay.x264-DEMAND",
				format: "Scene full",
			},
			{
				name:   "Alternative 2x05 format",
				title:  "Breaking Bad 2x05 1080p",
				format: "Alternative",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := parser.Parse(tc.title)
				if result == nil {
					t.Fatalf("Parser returned nil for %s format: %s", tc.format, tc.title)
				}

				// All should parse to Breaking Bad Season 2 Episode 5
				if result.ShowName != "Breaking Bad" {
					t.Errorf("ShowName: expected %q, got %q", "Breaking Bad", result.ShowName)
				}
				if result.Season != 2 {
					t.Errorf("Season: expected %d, got %d", 2, result.Season)
				}
				if result.Episode != 5 {
					t.Errorf("Episode: expected %d, got %d", 5, result.Episode)
				}
				if !result.IsSeasonal {
					t.Errorf("IsSeasonal: expected true, got false")
				}
				if result.IsAbsolute {
					t.Errorf("IsAbsolute: expected false, got true")
				}
			})
		}
	})

	t.Run("Absolute formats - Attack on Titan Episode 17", func(t *testing.T) {
		testCases := []struct {
			name   string
			title  string
			format string
		}{
			{
				name:   "NYAA absolute with brackets",
				title:  "[SubsPlease] Attack on Titan - 17 [1080p][HASH]",
				format: "NYAA brackets",
			},
			{
				name:   "Absolute with parentheses",
				title:  "Attack on Titan - 17 (1080p)",
				format: "Parentheses",
			},
			{
				name:   "Absolute simple format",
				title:  "Attack on Titan - 17 1080p HEVC",
				format: "Simple",
			},
			{
				name:   "Absolute with version",
				title:  "Attack on Titan - 17v2 [720p]",
				format: "With version",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := parser.Parse(tc.title)
				if result == nil {
					t.Fatalf("Parser returned nil for %s format: %s", tc.format, tc.title)
				}

				// All should parse to Attack on Titan Episode 17 (absolute)
				if result.ShowName != "Attack on Titan" {
					t.Errorf("ShowName: expected %q, got %q", "Attack on Titan", result.ShowName)
				}
				if result.Episode != 17 {
					t.Errorf("Episode: expected %d, got %d", 17, result.Episode)
				}
				if !result.IsAbsolute {
					t.Errorf("IsAbsolute: expected true, got false")
				}
				if result.IsSeasonal {
					t.Errorf("IsSeasonal: expected false, got true")
				}
			})
		}
	})
}

// Helper function to check if a parsed torrent matches episode metadata
// This simulates the validation logic in torrent.go that checks equivalence
func isEquivalentEpisode(parsed *ParsedTorrent, meta *tvdb.Episode) bool {
	if parsed == nil || meta == nil {
		return false
	}

	// If parsed as absolute, check against AbsoluteNumber
	if parsed.IsAbsolute && !parsed.IsSeasonal {
		return parsed.Episode == meta.AbsoluteNumber
	}

	// If parsed as seasonal, check against Season/Episode
	if parsed.IsSeasonal {
		seasonMatch := parsed.Season == meta.SeasonNumber
		episodeMatch := parsed.Episode == meta.Number
		return seasonMatch && episodeMatch
	}

	return false
}

// Test that absolute and seasonal formats can represent the same episode
// This validates the system's ability to match different torrent formats
// to the same episode using TVDB metadata as the source of truth
func TestTorrentParser_Parse_AbsoluteSeasonalEquivalence(t *testing.T) {
	parser := NewTorrentParser()

	// Mock TVDB metadata for Attack on Titan
	// Season 2, Episode 5 is absolute episode 17 (assuming 12 episodes in S1)
	episodeMeta := &tvdb.Episode{
		SeasonNumber:   2,
		Number:         5,
		AbsoluteNumber: 17,
		Name:           "Test Episode",
	}

	t.Run("Same episode - different formats", func(t *testing.T) {
		testCases := []struct {
			name           string
			title          string
			format         string
			expectedIsAbs  bool
			expectedIsSeas bool
		}{
			{
				name:           "Absolute format",
				title:          "[SubsPlease] Attack on Titan - 17 [1080p][HASH]",
				format:         "NYAA absolute",
				expectedIsAbs:  true,
				expectedIsSeas: false,
			},
			{
				name:           "Seasonal format",
				title:          "Attack.on.Titan.S02E05.1080p.mkv",
				format:         "Scene seasonal",
				expectedIsAbs:  false,
				expectedIsSeas: true,
			},
			{
				name:           "Alternative seasonal",
				title:          "Attack on Titan 2x05 1080p",
				format:         "Alternative",
				expectedIsAbs:  false,
				expectedIsSeas: true, // Alternative format sets IsSeasonal flag
			},
			{
				name:           "NYAA seasonal bracket",
				title:          "[Group] Attack on Titan - S02E05 [1080p]",
				format:         "NYAA S##E##",
				expectedIsAbs:  false,
				expectedIsSeas: true,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := parser.Parse(tc.title)
				if result == nil {
					t.Fatalf("Parser returned nil for %s format: %s", tc.format, tc.title)
				}

				// Verify format flags
				if result.IsAbsolute != tc.expectedIsAbs {
					t.Errorf("IsAbsolute: expected %v, got %v", tc.expectedIsAbs, result.IsAbsolute)
				}
				if result.IsSeasonal != tc.expectedIsSeas {
					t.Errorf("IsSeasonal: expected %v, got %v", tc.expectedIsSeas, result.IsSeasonal)
				}

				// Verify show name
				if result.ShowName != "Attack on Titan" {
					t.Errorf("ShowName: expected %q, got %q", "Attack on Titan", result.ShowName)
				}

				// Verify equivalence using metadata
				if !isEquivalentEpisode(result, episodeMeta) {
					t.Errorf("Format %s should match episode metadata (S%02dE%02d / Abs %d): parsed as Season=%d Episode=%d IsAbsolute=%v IsSeasonal=%v",
						tc.format, episodeMeta.SeasonNumber, episodeMeta.Number, episodeMeta.AbsoluteNumber,
						result.Season, result.Episode, result.IsAbsolute, result.IsSeasonal)
				}
			})
		}
	})

	t.Run("Wrong episodes should not match", func(t *testing.T) {
		testCases := []struct {
			name   string
			title  string
			reason string
		}{
			{
				name:   "Wrong absolute episode",
				title:  "[SubsPlease] Attack on Titan - 18 [1080p]",
				reason: "Episode 18 != AbsoluteNumber 17",
			},
			{
				name:   "Wrong seasonal episode",
				title:  "Attack.on.Titan.S02E06.1080p.mkv",
				reason: "S02E06 != S02E05",
			},
			{
				name:   "Wrong season",
				title:  "Attack.on.Titan.S01E05.1080p.mkv",
				reason: "S01E05 != S02E05",
			},
			{
				name:   "Wrong absolute (off by one)",
				title:  "[SubsPlease] Attack on Titan - 16 [1080p]",
				reason: "Episode 16 != AbsoluteNumber 17",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := parser.Parse(tc.title)
				if result == nil {
					t.Fatal("Parser returned nil")
				}

				if isEquivalentEpisode(result, episodeMeta) {
					t.Errorf("Should NOT match: %s (parsed as Season=%d Episode=%d IsAbsolute=%v)",
						tc.reason, result.Season, result.Episode, result.IsAbsolute)
				}
			})
		}
	})

	t.Run("Metadata validates cross-format equivalence", func(t *testing.T) {
		// Parse absolute format
		absoluteTitle := "[SubsPlease] Attack on Titan - 17 [1080p]"
		absoluteParsed := parser.Parse(absoluteTitle)

		// Parse seasonal format
		seasonalTitle := "Attack.on.Titan.S02E05.1080p.mkv"
		seasonalParsed := parser.Parse(seasonalTitle)

		// Both should parse successfully
		if absoluteParsed == nil || seasonalParsed == nil {
			t.Fatal("One or both parsers returned nil")
		}

		// Verify they parse to different internal representations
		if absoluteParsed.IsAbsolute == seasonalParsed.IsAbsolute {
			t.Error("Formats should be parsed differently (absolute vs seasonal)")
		}

		// But both should match the same episode via metadata
		absoluteMatches := isEquivalentEpisode(absoluteParsed, episodeMeta)
		seasonalMatches := isEquivalentEpisode(seasonalParsed, episodeMeta)

		if !absoluteMatches {
			t.Errorf("Absolute format should match metadata: Episode=%d, expected AbsoluteNumber=%d",
				absoluteParsed.Episode, episodeMeta.AbsoluteNumber)
		}

		if !seasonalMatches {
			t.Errorf("Seasonal format should match metadata: S%02dE%02d, expected S%02dE%02d",
				seasonalParsed.Season, seasonalParsed.Episode,
				episodeMeta.SeasonNumber, episodeMeta.Number)
		}

		// This proves both formats represent the same episode
		if absoluteMatches && seasonalMatches {
			t.Logf("✓ Verified: Both formats match the same episode (S%02dE%02d / Abs %d)",
				episodeMeta.SeasonNumber, episodeMeta.Number, episodeMeta.AbsoluteNumber)
		}
	})
}
