package service

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ParsedTorrent represents structured information extracted from a torrent title
type ParsedTorrent struct {
	ShowName      string  // Cleaned show name
	Season        int     // Season number (0 if not present)
	Episode       int     // Episode number
	EpisodeEnd    int     // End episode for multi-episode (0 if single)
	Quality       string  // Quality (1080p, 720p, etc.)
	ReleaseGroup  string  // Release group name
	Version       int     // Version number (v2, v3, etc. - 0 if not present)
	IsAbsolute    bool    // True if using absolute numbering
	IsSeasonal    bool    // True if using S##E## format
	MatchConfidence float64 // Confidence score for this parse (0.0 - 1.0)
	Raw           string  // Original title
}

// TorrentParser handles parsing of torrent titles
type TorrentParser struct {
	patterns []torrentPattern
}

type torrentPattern struct {
	name    string
	regex   *regexp.Regexp
	parser  func([]string, string) *ParsedTorrent
	confidence float64 // Base confidence for this pattern
}

// NewTorrentParser creates a new torrent parser with predefined patterns
func NewTorrentParser() *TorrentParser {
	tp := &TorrentParser{}
	tp.initPatterns()
	return tp
}

// initPatterns initializes all regex patterns for different torrent naming conventions
func (tp *TorrentParser) initPatterns() {
	tp.patterns = []torrentPattern{
		// NYAA Anime Format: [Group] Show - Episode [Quality][Hash]
		// Example: [SubsPlease] Attack on Titan - 05 [1080p][ABC123].mkv
		{
			name:       "nyaa-absolute",
			regex:      regexp.MustCompile(`(?i)^\[([^\]]+)\]\s*(.+?)\s*-\s*(\d{1,3})(?:v(\d+))?\s*\[([^\]]*(?:1080p|720p|480p|2160p|4k)[^\]]*)\]`),
			parser:     parseNYAAAbsolute,
			confidence: 0.95,
		},
		// NYAA Seasonal Format: [Group] Show - S##E## [Quality]
		// Example: [Group] Show - S02E05 [1080p]
		{
			name:       "nyaa-seasonal",
			regex:      regexp.MustCompile(`(?i)^\[([^\]]+)\]\s*(.+?)\s*-\s*S(\d{1,2})E(\d{1,3})(?:v(\d+))?\s*\[([^\]]*(?:1080p|720p|480p|2160p|4k)[^\]]*)\]`),
			parser:     parseNYAASeasonal,
			confidence: 0.95,
		},
		// Scene Format: Show.S##E##.Title.Quality.Codec-Group
		// Example: Breaking.Bad.S01E05.Gray.Matter.1080p.BluRay.x264-DEMAND
		{
			name:       "scene-standard",
			regex:      regexp.MustCompile(`(?i)^(.+?)\.S(\d{1,2})E(\d{1,3})(?:E(\d{1,3}))?(?:v(\d+))?\..*?(1080p|720p|480p|2160p|4k).*?-([^\s\.]+)`),
			parser:     parseScene,
			confidence: 0.90,
		},
		// Alternative Scene Format: Show.S##E##.Quality
		// Example: Show.S01E05.1080p.mkv
		{
			name:       "scene-simple",
			regex:      regexp.MustCompile(`(?i)^(.+?)\.S(\d{1,2})E(\d{1,3})(?:E(\d{1,3}))?(?:v(\d+))?\..*?(1080p|720p|480p|2160p|4k)`),
			parser:     parseSceneSimple,
			confidence: 0.85,
		},
		// Absolute Numbering with Parentheses: Show - Episode (Quality)
		// Example: Show - 05 (1080p)
		{
			name:       "absolute-paren",
			regex:      regexp.MustCompile(`(?i)^(.+?)\s*-\s*(\d{1,3})(?:v(\d+))?\s*\(([^\)]*(?:1080p|720p|480p|2160p|4k)[^\)]*)\)`),
			parser:     parseAbsoluteParen,
			confidence: 0.88,
		},
		// Absolute Numbering Simple: Show - Episode Quality
		// Example: Show - 05 1080p
		{
			name:       "absolute-simple",
			regex:      regexp.MustCompile(`(?i)^(.+?)\s*-\s*(\d{1,3})(?:v(\d+))?\s+.*?(1080p|720p|480p|2160p|4k)`),
			parser:     parseAbsoluteSimple,
			confidence: 0.82,
		},
		// Alternative Format: Show ##x## Quality
		// Example: Show 2x05 1080p
		{
			name:       "alternative-format",
			regex:      regexp.MustCompile(`(?i)^(.+?)\s+(\d{1,2})x(\d{1,3})(?:v(\d+))?\s+.*?(1080p|720p|480p|2160p|4k)`),
			parser:     parseAlternative,
			confidence: 0.80,
		},
		// Fallback: Try to extract any S##E## pattern
		{
			name:       "fallback-seasonal",
			regex:      regexp.MustCompile(`(?i)S(\d{1,2})E(\d{1,3})(?:E(\d{1,3}))?(?:v(\d+))?`),
			parser:     parseFallbackSeasonal,
			confidence: 0.60,
		},
	}
}

// Parse attempts to parse a torrent title using all available patterns
func (tp *TorrentParser) Parse(title string) *ParsedTorrent {
	// Clean the title first
	cleanTitle := strings.TrimSpace(title)

	// Try each pattern in order of confidence
	for _, pattern := range tp.patterns {
		if matches := pattern.regex.FindStringSubmatch(cleanTitle); matches != nil {
			parsed := pattern.parser(matches, cleanTitle)
			if parsed != nil {
				parsed.MatchConfidence = pattern.confidence
				parsed.Raw = title
				return parsed
			}
		}
	}

	// If no pattern matched, return nil
	return nil
}

// Parser functions for each pattern type

func parseNYAAAbsolute(matches []string, raw string) *ParsedTorrent {
	// matches: [full, group, show, episode, version?, quality]
	if len(matches) < 6 {
		return nil
	}

	episode, _ := strconv.Atoi(matches[3])
	version := 0
	if matches[4] != "" {
		version, _ = strconv.Atoi(matches[4])
	}

	return &ParsedTorrent{
		ShowName:     cleanShowName(matches[2]),
		Episode:      episode,
		Quality:      extractQuality(matches[5]),
		ReleaseGroup: matches[1],
		Version:      version,
		IsAbsolute:   true,
		IsSeasonal:   false,
	}
}

func parseNYAASeasonal(matches []string, raw string) *ParsedTorrent {
	// matches: [full, group, show, season, episode, version?, quality]
	if len(matches) < 7 {
		return nil
	}

	season, _ := strconv.Atoi(matches[3])
	episode, _ := strconv.Atoi(matches[4])
	version := 0
	if matches[5] != "" {
		version, _ = strconv.Atoi(matches[5])
	}

	return &ParsedTorrent{
		ShowName:     cleanShowName(matches[2]),
		Season:       season,
		Episode:      episode,
		Quality:      extractQuality(matches[6]),
		ReleaseGroup: matches[1],
		Version:      version,
		IsAbsolute:   false,
		IsSeasonal:   true,
	}
}

func parseScene(matches []string, raw string) *ParsedTorrent {
	// matches: [full, show, season, episode, episodeEnd?, version?, quality, group]
	if len(matches) < 8 {
		return nil
	}

	season, _ := strconv.Atoi(matches[2])
	episode, _ := strconv.Atoi(matches[3])
	episodeEnd := 0
	if matches[4] != "" {
		episodeEnd, _ = strconv.Atoi(matches[4])
	}
	version := 0
	if matches[5] != "" {
		version, _ = strconv.Atoi(matches[5])
	}

	return &ParsedTorrent{
		ShowName:     cleanShowName(matches[1]),
		Season:       season,
		Episode:      episode,
		EpisodeEnd:   episodeEnd,
		Quality:      matches[6],
		ReleaseGroup: matches[7],
		Version:      version,
		IsAbsolute:   false,
		IsSeasonal:   true,
	}
}

func parseSceneSimple(matches []string, raw string) *ParsedTorrent {
	// matches: [full, show, season, episode, episodeEnd?, version?, quality]
	if len(matches) < 7 {
		return nil
	}

	season, _ := strconv.Atoi(matches[2])
	episode, _ := strconv.Atoi(matches[3])
	episodeEnd := 0
	if matches[4] != "" {
		episodeEnd, _ = strconv.Atoi(matches[4])
	}
	version := 0
	if matches[5] != "" {
		version, _ = strconv.Atoi(matches[5])
	}

	return &ParsedTorrent{
		ShowName:     cleanShowName(matches[1]),
		Season:       season,
		Episode:      episode,
		EpisodeEnd:   episodeEnd,
		Quality:      matches[6],
		Version:      version,
		IsAbsolute:   false,
		IsSeasonal:   true,
	}
}

func parseAbsoluteParen(matches []string, raw string) *ParsedTorrent {
	// matches: [full, show, episode, version?, quality]
	if len(matches) < 5 {
		return nil
	}

	episode, _ := strconv.Atoi(matches[2])
	version := 0
	if len(matches) > 3 && matches[3] != "" {
		version, _ = strconv.Atoi(matches[3])
	}

	qualityIdx := 4
	if len(matches) > 4 {
		qualityIdx = len(matches) - 1
	}

	return &ParsedTorrent{
		ShowName:   cleanShowName(matches[1]),
		Episode:    episode,
		Quality:    extractQuality(matches[qualityIdx]),
		Version:    version,
		IsAbsolute: true,
		IsSeasonal: false,
	}
}

func parseAbsoluteSimple(matches []string, raw string) *ParsedTorrent {
	// matches: [full, show, episode, version?, quality]
	if len(matches) < 5 {
		return nil
	}

	episode, _ := strconv.Atoi(matches[2])
	version := 0
	if matches[3] != "" {
		version, _ = strconv.Atoi(matches[3])
	}

	return &ParsedTorrent{
		ShowName:   cleanShowName(matches[1]),
		Episode:    episode,
		Quality:    matches[4],
		Version:    version,
		IsAbsolute: true,
		IsSeasonal: false,
	}
}

func parseAlternative(matches []string, raw string) *ParsedTorrent {
	// matches: [full, show, season, episode, version?, quality]
	if len(matches) < 6 {
		return nil
	}

	season, _ := strconv.Atoi(matches[2])
	episode, _ := strconv.Atoi(matches[3])
	version := 0
	if matches[4] != "" {
		version, _ = strconv.Atoi(matches[4])
	}

	return &ParsedTorrent{
		ShowName:   cleanShowName(matches[1]),
		Season:     season,
		Episode:    episode,
		Quality:    matches[5],
		Version:    version,
		IsAbsolute: false,
		IsSeasonal: true,
	}
}

func parseFallbackSeasonal(matches []string, raw string) *ParsedTorrent {
	// matches: [full, season, episode, episodeEnd?, version?]
	if len(matches) < 3 {
		return nil
	}

	season, _ := strconv.Atoi(matches[1])
	episode, _ := strconv.Atoi(matches[2])
	episodeEnd := 0
	if len(matches) > 3 && matches[3] != "" {
		episodeEnd, _ = strconv.Atoi(matches[3])
	}
	version := 0
	if len(matches) > 4 && matches[4] != "" {
		version, _ = strconv.Atoi(matches[4])
	}

	// Try to extract show name from raw title (everything before S##E##)
	showNameRegex := regexp.MustCompile(`(?i)^(.+?)[\s\._-]*S\d`)
	if showMatches := showNameRegex.FindStringSubmatch(raw); len(showMatches) > 1 {
		return &ParsedTorrent{
			ShowName:   cleanShowName(showMatches[1]),
			Season:     season,
			Episode:    episode,
			EpisodeEnd: episodeEnd,
			Version:    version,
			IsAbsolute: false,
			IsSeasonal: true,
		}
	}

	return nil
}

// Helper functions

func cleanShowName(name string) string {
	// Remove release groups in brackets/parentheses at the beginning
	releaseGroupRegex := regexp.MustCompile(`(?i)^[\[\(][^\]\)]+[\]\)]\s*`)
	cleaned := releaseGroupRegex.ReplaceAllString(name, "")

	// Replace dots, underscores, and multiple spaces with single space
	cleaned = strings.ReplaceAll(cleaned, ".", " ")
	cleaned = strings.ReplaceAll(cleaned, "_", " ")
	cleaned = regexp.MustCompile(`\s+`).ReplaceAllString(cleaned, " ")
	return strings.TrimSpace(cleaned)
}

func extractQuality(qualityString string) string {
	// Extract quality from a string that might contain additional info
	qualityRegex := regexp.MustCompile(`(?i)(4k|2160p|1080p|720p|480p)`)
	if matches := qualityRegex.FindStringSubmatch(qualityString); len(matches) > 1 {
		return strings.ToLower(matches[1])
	}
	return ""
}

// GetQualityTier returns a numeric tier for quality comparison
func (pt *ParsedTorrent) GetQualityTier() int {
	switch strings.ToLower(pt.Quality) {
	case "4k", "2160p":
		return 5
	case "1080p":
		return 4
	case "720p":
		return 3
	case "480p":
		return 2
	default:
		return 1
	}
}

// IsMultiEpisode returns true if this torrent contains multiple episodes
func (pt *ParsedTorrent) IsMultiEpisode() bool {
	return pt.EpisodeEnd > 0 && pt.EpisodeEnd > pt.Episode
}

// ContainsEpisode checks if this torrent contains a specific episode number
func (pt *ParsedTorrent) ContainsEpisode(episode int) bool {
	if pt.IsMultiEpisode() {
		return episode >= pt.Episode && episode <= pt.EpisodeEnd
	}
	return pt.Episode == episode
}

// String returns a human-readable representation
func (pt *ParsedTorrent) String() string {
	if pt.IsAbsolute {
		return fmt.Sprintf("%s - %d [%s] (Absolute)", pt.ShowName, pt.Episode, pt.Quality)
	}
	if pt.IsMultiEpisode() {
		return fmt.Sprintf("%s S%02dE%02d-E%02d [%s]", pt.ShowName, pt.Season, pt.Episode, pt.EpisodeEnd, pt.Quality)
	}
	return fmt.Sprintf("%s S%02dE%02d [%s]", pt.ShowName, pt.Season, pt.Episode, pt.Quality)
}
