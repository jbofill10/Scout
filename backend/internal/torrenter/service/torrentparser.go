package service

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ParsedTorrent represents structured information extracted from a torrent title
type ParsedTorrent struct {
	ShowName        string  // Cleaned show name
	Season          int     // Season number (0 if not present)
	Episode         int     // Episode number
	EpisodeEnd      int     // End episode for multi-episode (0 if single)
	Quality         string  // Quality (1080p, 720p, etc.)
	ReleaseGroup    string  // Release group name
	Version         int     // Version number (v2, v3, etc. - 0 if not present)
	IsAbsolute      bool    // True if using absolute numbering
	IsSeasonal      bool    // True if using S##E## format
	MatchConfidence float64 // Confidence score for this parse (0.0 - 1.0)
	Raw             string  // Original title
}

// ParsedMovie represents structured information extracted from a movie torrent title
type ParsedMovie struct {
	MovieName       string  // Cleaned movie name
	Year            int     // Release year (0 if not present)
	Quality         string  // Quality (1080p, 720p, 4K, 2160p, etc.)
	Source          string  // Source (BluRay, WEB-DL, WEBRip, HDRip, etc.)
	Codec           string  // Codec (x264, x265, HEVC, H.264, etc.)
	Audio           string  // Audio format (AAC, DTS, DD5.1, etc.)
	ReleaseGroup    string  // Release group name
	Edition         string  // Edition (Extended, Directors Cut, PROPER, etc.)
	Language        string  // Language tags (MULTI, Dual-Audio, etc.)
	Is4K            bool    // True if 4K/2160p quality
	MatchConfidence float64 // Confidence score (0.0 - 1.0)
	Raw             string  // Original title
}

// TorrentParser handles parsing of torrent titles
type TorrentParser struct {
	patterns      []torrentPattern
	moviePatterns []moviePattern
}

type torrentPattern struct {
	name       string
	regex      *regexp.Regexp
	parser     func([]string, string) *ParsedTorrent
	confidence float64 // Base confidence for this pattern
}

type moviePattern struct {
	name       string
	regex      *regexp.Regexp
	parser     func([]string, string) *ParsedMovie
	confidence float64 // Base confidence for this pattern
}

// NewTorrentParser creates a new torrent parser with predefined patterns
func NewTorrentParser() *TorrentParser {
	tp := &TorrentParser{}
	tp.initPatterns()
	tp.initMoviePatterns()
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

// initMoviePatterns initializes all regex patterns for different movie torrent naming conventions
func (tp *TorrentParser) initMoviePatterns() {
	tp.moviePatterns = []moviePattern{
		// Edition Tags Format: Movie.Title.Year.Edition.Quality.Source.Codec-Group
		// Example: Aliens.1986.Directors.Cut.1080p.BluRay.x264-MOOVEE
		{
			name: "movie-edition-tags",
			regex: regexp.MustCompile(
				`(?i)^(.+?)\.(\d{4})\.?(EXTENDED|REMASTERED|UNRATED|DIRECTORS\.CUT|DC|THEATRICAL|PROPER|REPACK|iNTERNAL)?\.?` +
					`(2160p|1080p|720p|480p|4k)\.?(BluRay|WEB-DL|WEBRip|HDRip|BrRip)?.*?\.?(x264|x265|H\.?264|H\.?265|HEVC|XviD).*?-([^\s\.]+)`),
			parser:     parseMovieEditionTags,
			confidence: 0.92,
		},
		// Scene Standard Format: Movie.Title.Year.Quality.Source.Codec-Group
		// Example: Breaking.Bad.Movie.2019.1080p.BluRay.x264-SPARKS
		{
			name: "movie-scene-standard",
			regex: regexp.MustCompile(
				`(?i)^(.+?)\.(\d{4})\..*?(2160p|1080p|720p|480p|4k)\.?(BluRay|WEB-DL|WEBRip|HDRip|BrRip|HDTV)?.*?` +
					`\.?(x264|x265|H\.?264|H\.?265|HEVC|XviD).*?-([^\s\.]+)`),
			parser:     parseMovieSceneStandard,
			confidence: 0.95,
		},
		// NYAA Anime Movie Format: [Group] Movie Title (Year) [Quality][Codec]
		// Example: [SubsPlease] Suzume (2022) [1080p][HEVC][AAC]
		{
			name: "movie-nyaa-anime",
			regex: regexp.MustCompile(
				`(?i)^\[([^\]]+)\]\s*(.+?)\s*\((\d{4})\)\s*\[([^\]]*(?:2160p|1080p|720p|480p|4k)[^\]]*)\]`),
			parser:     parseMovieNYAA,
			confidence: 0.95,
		},
		// YTS/YIFY Format: Movie Title (Year) Quality Source - Group
		// Example: Interstellar (2014) 1080p BluRay x264 - YIFY
		{
			name: "movie-yts-yify",
			regex: regexp.MustCompile(
				`(?i)^(.+?)\s*\((\d{4})\)\s*\[?(\d{3,4}p|4k)\]?\s*\[?(BluRay|WEBRip|WEB|BrRip|HDRip)\]?\s*` +
					`\[?(x264|x265|HEVC)\]?\s*-\s*([A-Z]+)`),
			parser:     parseMovieYTS,
			confidence: 0.93,
		},
		// Bracket Quality Format: Movie Title (Year) [Quality] [Language] [Codec] [Group]
		// Example: Mad Max Fury Road (2015) [1080p BluRay] [MULTI] [x264 DTS] [EXTREME]
		{
			name: "movie-bracket-quality",
			regex: regexp.MustCompile(
				`(?i)^(.+?)\s*\((\d{4})\)\s*\[([^\]]*(?:2160p|1080p|720p|480p|4k)[^\]]*)\]` +
					`(?:\s*\[([^\]]+)\])?\s*\[([^\]]+)\]\s*\[([^\]]+)\]`),
			parser:     parseMovieBracketQuality,
			confidence: 0.90,
		},
		// Simple Parentheses Format: Movie Title (Year) Quality Source
		// Example: The Martian (2015) 1080p WEB-DL x264
		{
			name: "movie-simple-parentheses",
			regex: regexp.MustCompile(
				`(?i)^(.+?)\s*\((\d{4})\)\s+.*?(2160p|1080p|720p|480p|4k).*?` +
					`(BluRay|WEB-DL|WEBRip|HDRip|BrRip|HDTV)?`),
			parser:     parseMovieSimpleParentheses,
			confidence: 0.88,
		},
		// Multi-Edition Complex: Movie.Title.Year.Quality.Source.Audio.Codec-Group
		// Example: Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1.Atmos.HEVC-CMRG
		{
			name: "movie-multi-edition-complex",
			regex: regexp.MustCompile(
				`(?i)^(.+?)\.(\d{4})\..*?(2160p|1080p|720p|480p|4k)\.?(BluRay|WEB-DL|WEBRip)?` +
					`\.?(DTS|DD5\.1|DDp5\.1|DDP5\.1|AAC|AC3|Atmos)?.*?(x264|x265|HEVC|H\.264|H\.265)` +
					`\.?(PROPER|REPACK|EXTENDED)?.*?-([^\s\.]+)`),
			parser:     parseMovieMultiEdition,
			confidence: 0.85,
		},
		// Space Delimited Format: Movie Title Year Quality Source
		// Example: The Matrix Reloaded 2003 1080p BluRay x264
		{
			name: "movie-space-delimited",
			regex: regexp.MustCompile(
				`(?i)^(.+?)\s+(\d{4})\s+(?:.*?\s+)?(2160p|1080p|720p|480p|4k)\s+` +
					`(BluRay|WEB-DL|WEBRip|HDRip|BrRip|HDTV)`),
			parser:     parseMovieSpaceDelimited,
			confidence: 0.80,
		},
		// No Year Simple Format: Movie.Title.Quality.Source.Codec-Group
		// Example: Casablanca.1080p.BluRay.x264-CLASSIC
		{
			name: "movie-no-year",
			regex: regexp.MustCompile(
				`(?i)^(.+?)\.(2160p|1080p|720p|480p|4k)\.?` +
					`(BluRay|WEB-DL|WEBRip|HDRip|BrRip)?.*?\.?(x264|x265|HEVC).*?-([^\s\.]+)`),
			parser:     parseMovieNoYear,
			confidence: 0.75,
		},
		// Minimal Format: Movie Title (Year)
		// Example: My Movie (2023)
		{
			name:       "movie-minimal",
			regex:      regexp.MustCompile(`(?i)^(.+?)\s*\((\d{4})\)\s*$`),
			parser:     parseMovieMinimal,
			confidence: 0.70,
		},
		// Fallback: Square bracket year format: Movie Name [Year] Quality
		// Example: Marley and Me [2008] 1080p BrRip
		{
			name: "movie-square-bracket-year",
			regex: regexp.MustCompile(
				`(?i)^(.+?)\s*\[(\d{4})\]\s*.*?(2160p|1080p|720p|480p|4k)`),
			parser:     parseMovieSquareBracketYear,
			confidence: 0.85,
		},
		// Fallback: Compact format with no space after year: Movie(Year)Quality
		// Example: Interstellar(2014)1080p
		{
			name: "movie-compact-no-space",
			regex: regexp.MustCompile(
				`(?i)^(.+?)\((\d{4})\)(.*?)(2160p|1080p|720p|480p|4k)`),
			parser:     parseMovieCompactNoSpace,
			confidence: 0.80,
		},
		// Fallback: Dot-delimited format with periods throughout
		// Example: Movie.Name.2024.1080p.WEB or Movie.Title.Year.Quality
		{
			name: "movie-dot-delimited-flexible",
			regex: regexp.MustCompile(
				`(?i)^(.+?)\.(\d{4})\..*?(2160p|1080p|720p|480p|4k)`),
			parser:     parseMovieDotDelimitedFlexible,
			confidence: 0.78,
		},
		// Fallback: Quality-anywhere - extract quality from ANY position in title
		// This is a last resort pattern that looks for year and quality anywhere
		// Example: Movie Title Year Quality or any variation
		{
			name: "movie-quality-anywhere",
			regex: regexp.MustCompile(
				`(?i)(.+?)\s*[\(\[]?(\d{4})[\)\]]?\s*.*(2160p|1080p|720p|480p|4k)`),
			parser:     parseMovieQualityAnywhere,
			confidence: 0.65,
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

// ParseMovie attempts to parse a movie torrent title using all available movie patterns
func (tp *TorrentParser) ParseMovie(title string) *ParsedMovie {
	// Clean the title first
	cleanTitle := strings.TrimSpace(title)

	// Try each movie pattern in order of confidence
	for _, pattern := range tp.moviePatterns {
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
		ShowName:   cleanShowName(matches[1]),
		Season:     season,
		Episode:    episode,
		EpisodeEnd: episodeEnd,
		Quality:    matches[6],
		Version:    version,
		IsAbsolute: false,
		IsSeasonal: true,
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

// Movie parser functions for each pattern type

func parseMovieEditionTags(matches []string, raw string) *ParsedMovie {
	// matches: [full, movieName, year, edition?, quality, source?, codec?, group]
	if len(matches) < 8 {
		return nil
	}

	year := extractYear(matches[2])
	edition := ""
	if matches[3] != "" {
		edition = normalizeEdition(matches[3])
	}

	return &ParsedMovie{
		MovieName:    cleanMovieName(matches[1]),
		Year:         year,
		Edition:      edition,
		Quality:      normalizeQuality(matches[4]),
		Source:       normalizeSource(matches[5]),
		Codec:        normalizeCodec(matches[6]),
		ReleaseGroup: matches[7],
		Is4K:         is4K(matches[4]),
	}
}

func parseMovieSceneStandard(matches []string, raw string) *ParsedMovie {
	// matches: [full, movieName, year, quality, source?, codec?, group]
	if len(matches) < 7 {
		return nil
	}

	year := extractYear(matches[2])

	return &ParsedMovie{
		MovieName:    cleanMovieName(matches[1]),
		Year:         year,
		Quality:      normalizeQuality(matches[3]),
		Source:       normalizeSource(matches[4]),
		Codec:        normalizeCodec(matches[5]),
		ReleaseGroup: matches[6],
		Is4K:         is4K(matches[3]),
	}
}

func parseMovieNYAA(matches []string, raw string) *ParsedMovie {
	// matches: [full, group, movieName, year, qualityString]
	if len(matches) < 5 {
		return nil
	}

	year := extractYear(matches[3])
	qualityStr := matches[4]

	// Extract quality, codec, and audio from the quality string
	quality := extractQuality(qualityStr)
	codec := extractCodecFromString(qualityStr)
	audio := extractAudioFromString(qualityStr)
	language := extractLanguageFromString(qualityStr)

	return &ParsedMovie{
		MovieName:    cleanMovieName(matches[2]),
		Year:         year,
		Quality:      quality,
		Codec:        codec,
		Audio:        audio,
		Language:     language,
		ReleaseGroup: matches[1],
		Is4K:         is4K(quality),
	}
}

func parseMovieYTS(matches []string, raw string) *ParsedMovie {
	// matches: [full, movieName, year, quality, source?, codec?, group]
	if len(matches) < 7 {
		return nil
	}

	year := extractYear(matches[2])

	return &ParsedMovie{
		MovieName:    cleanMovieName(matches[1]),
		Year:         year,
		Quality:      normalizeQuality(matches[3]),
		Source:       normalizeSource(matches[4]),
		Codec:        normalizeCodec(matches[5]),
		ReleaseGroup: matches[6],
		Is4K:         is4K(matches[3]),
	}
}

func parseMovieBracketQuality(matches []string, raw string) *ParsedMovie {
	// matches: [full, movieName, year, qualityString, language?, codecString, group]
	if len(matches) < 7 {
		return nil
	}

	year := extractYear(matches[2])
	qualityStr := matches[3]
	language := ""
	if len(matches) > 4 && matches[4] != "" {
		language = matches[4]
	}
	codecStr := matches[5]
	group := matches[6]

	// Extract from strings
	quality := extractQuality(qualityStr)
	source := extractSourceFromString(qualityStr)
	codec := extractCodecFromString(codecStr)
	audio := extractAudioFromString(codecStr)

	return &ParsedMovie{
		MovieName:    cleanMovieName(matches[1]),
		Year:         year,
		Quality:      quality,
		Source:       source,
		Codec:        codec,
		Audio:        audio,
		Language:     language,
		ReleaseGroup: group,
		Is4K:         is4K(quality),
	}
}

func parseMovieSimpleParentheses(matches []string, raw string) *ParsedMovie {
	// matches: [full, movieName, year, quality, source?]
	if len(matches) < 4 {
		return nil
	}

	year := extractYear(matches[2])
	source := ""
	if len(matches) > 4 {
		source = normalizeSource(matches[4])
	}

	return &ParsedMovie{
		MovieName: cleanMovieName(matches[1]),
		Year:      year,
		Quality:   normalizeQuality(matches[3]),
		Source:    source,
		Is4K:      is4K(matches[3]),
	}
}

func parseMovieMultiEdition(matches []string, raw string) *ParsedMovie {
	// matches: [full, movieName, year, quality, source?, audio?, codec, edition?, group]
	if len(matches) < 9 {
		return nil
	}

	year := extractYear(matches[2])
	edition := ""
	if matches[7] != "" {
		edition = normalizeEdition(matches[7])
	}

	return &ParsedMovie{
		MovieName:    cleanMovieName(matches[1]),
		Year:         year,
		Quality:      normalizeQuality(matches[3]),
		Source:       normalizeSource(matches[4]),
		Audio:        matches[5],
		Codec:        normalizeCodec(matches[6]),
		Edition:      edition,
		ReleaseGroup: matches[8],
		Is4K:         is4K(matches[3]),
	}
}

func parseMovieSpaceDelimited(matches []string, raw string) *ParsedMovie {
	// matches: [full, movieName, year, quality, source]
	if len(matches) < 5 {
		return nil
	}

	year := extractYear(matches[2])

	return &ParsedMovie{
		MovieName: cleanMovieName(matches[1]),
		Year:      year,
		Quality:   normalizeQuality(matches[3]),
		Source:    normalizeSource(matches[4]),
		Is4K:      is4K(matches[3]),
	}
}

func parseMovieNoYear(matches []string, raw string) *ParsedMovie {
	// matches: [full, movieName, quality, source?, codec?, group]
	if len(matches) < 6 {
		return nil
	}

	source := ""
	if len(matches) > 3 && matches[3] != "" {
		source = normalizeSource(matches[3])
	}

	codec := ""
	if len(matches) > 4 && matches[4] != "" {
		codec = normalizeCodec(matches[4])
	}

	return &ParsedMovie{
		MovieName:    cleanMovieName(matches[1]),
		Year:         0, // No year available
		Quality:      normalizeQuality(matches[2]),
		Source:       source,
		Codec:        codec,
		ReleaseGroup: matches[5],
		Is4K:         is4K(matches[2]),
	}
}

func parseMovieMinimal(matches []string, raw string) *ParsedMovie {
	// matches: [full, movieName, year]
	if len(matches) < 3 {
		return nil
	}

	year := extractYear(matches[2])

	return &ParsedMovie{
		MovieName: cleanMovieName(matches[1]),
		Year:      year,
		Is4K:      false,
	}
}

func parseMovieSquareBracketYear(matches []string, raw string) *ParsedMovie {
	// matches: [full, movieName, year, quality]
	if len(matches) < 4 {
		return nil
	}

	year := extractYear(matches[2])

	return &ParsedMovie{
		MovieName: cleanMovieName(matches[1]),
		Year:      year,
		Quality:   normalizeQuality(matches[3]),
		Is4K:      is4K(matches[3]),
	}
}

func parseMovieCompactNoSpace(matches []string, raw string) *ParsedMovie {
	// matches: [full, movieName, year, middleText, quality]
	if len(matches) < 5 {
		return nil
	}

	year := extractYear(matches[2])

	return &ParsedMovie{
		MovieName: cleanMovieName(matches[1]),
		Year:      year,
		Quality:   normalizeQuality(matches[4]),
		Is4K:      is4K(matches[4]),
	}
}

func parseMovieDotDelimitedFlexible(matches []string, raw string) *ParsedMovie {
	// matches: [full, movieName, year, quality]
	if len(matches) < 4 {
		return nil
	}

	year := extractYear(matches[2])

	return &ParsedMovie{
		MovieName: cleanMovieName(matches[1]),
		Year:      year,
		Quality:   normalizeQuality(matches[3]),
		Is4K:      is4K(matches[3]),
	}
}

func parseMovieQualityAnywhere(matches []string, raw string) *ParsedMovie {
	// matches: [full, movieName, year, quality]
	if len(matches) < 4 {
		return nil
	}

	year := extractYear(matches[2])

	return &ParsedMovie{
		MovieName: cleanMovieName(matches[1]),
		Year:      year,
		Quality:   normalizeQuality(matches[3]),
		Is4K:      is4K(matches[3]),
	}
}

// Movie helper functions

func cleanMovieName(name string) string {
	// Remove release groups in brackets/parentheses at the beginning
	releaseGroupRegex := regexp.MustCompile(`(?i)^[\[\(][^\]\)]+[\]\)]\s*`)
	cleaned := releaseGroupRegex.ReplaceAllString(name, "")

	// Replace dots and underscores with spaces
	cleaned = strings.ReplaceAll(cleaned, ".", " ")
	cleaned = strings.ReplaceAll(cleaned, "_", " ")

	// Handle special cases like "Part Two" "Part 2" etc
	cleaned = regexp.MustCompile(`\s+`).ReplaceAllString(cleaned, " ")

	return strings.TrimSpace(cleaned)
}

func extractYear(yearStr string) int {
	// Parse year string and validate range
	year, err := strconv.Atoi(yearStr)
	if err != nil {
		return 0
	}

	// Validate year is in reasonable range (1900-2099)
	if year < 1900 || year > 2099 {
		return 0
	}

	return year
}

func normalizeQuality(quality string) string {
	// Standardize quality strings
	quality = strings.ToLower(quality)

	// Convert 4k variants to 2160p
	if quality == "4k" {
		return "2160p"
	}

	return quality
}

func normalizeSource(source string) string {
	if source == "" {
		return ""
	}

	// Standardize source strings
	source = strings.ToLower(source)

	// Map common variants to standard names
	switch source {
	case "bluray", "blu-ray", "brrip", "bdrip":
		return "BluRay"
	case "web-dl", "webdl", "web":
		return "WEB-DL"
	case "webrip":
		return "WEBRip"
	case "hdrip":
		return "HDRip"
	case "hdtv":
		return "HDTV"
	default:
		// Capitalize first letter
		if len(source) > 0 {
			return strings.ToUpper(source[:1]) + source[1:]
		}
		return source
	}
}

func normalizeCodec(codec string) string {
	if codec == "" {
		return ""
	}

	// Standardize codec strings
	codec = strings.ToLower(codec)
	codec = strings.ReplaceAll(codec, ".", "")

	// Map variants to standard names
	switch codec {
	case "x265", "hevc", "h265":
		return "HEVC"
	case "x264", "h264":
		return "x264"
	case "xvid":
		return "XviD"
	default:
		return codec
	}
}

func normalizeEdition(edition string) string {
	if edition == "" {
		return ""
	}

	// Standardize edition strings
	edition = strings.ToLower(edition)
	edition = strings.ReplaceAll(edition, ".", " ")

	// Map variants to standard names
	switch edition {
	case "directors cut", "director's cut", "dc":
		return "Directors Cut"
	case "extended", "extended edition":
		return "Extended"
	case "unrated":
		return "Unrated"
	case "theatrical":
		return "Theatrical"
	case "remastered":
		return "Remastered"
	case "proper":
		return "PROPER"
	case "repack":
		return "REPACK"
	case "internal":
		return "iNTERNAL"
	default:
		// Capitalize first letter of each word
		words := strings.Fields(edition)
		for i, word := range words {
			if len(word) > 0 {
				words[i] = strings.ToUpper(word[:1]) + word[1:]
			}
		}
		return strings.Join(words, " ")
	}
}

func is4K(quality string) bool {
	// Check if quality indicates 4K/2160p/UHD
	quality = strings.ToLower(quality)
	return quality == "2160p" || quality == "4k" || strings.Contains(quality, "uhd")
}

func extractCodecFromString(str string) string {
	// Extract codec from a string that might contain additional info
	codecRegex := regexp.MustCompile(`(?i)(x264|x265|H\.?264|H\.?265|HEVC|XviD)`)
	if matches := codecRegex.FindStringSubmatch(str); len(matches) > 1 {
		return normalizeCodec(matches[1])
	}
	return ""
}

func extractAudioFromString(str string) string {
	// Extract audio format from a string
	audioRegex := regexp.MustCompile(`(?i)(AAC|AC3|DTS|DD5\.1|DDP5\.1|DDp5\.1|Atmos|DTS-HD|TrueHD|FLAC)`)
	if matches := audioRegex.FindStringSubmatch(str); len(matches) > 1 {
		return matches[1]
	}
	return ""
}

func extractSourceFromString(str string) string {
	// Extract source from a string
	sourceRegex := regexp.MustCompile(`(?i)(BluRay|Blu-ray|WEB-DL|WEBRip|HDRip|BrRip|HDTV|BDRip)`)
	if matches := sourceRegex.FindStringSubmatch(str); len(matches) > 1 {
		return normalizeSource(matches[1])
	}
	return ""
}

func extractLanguageFromString(str string) string {
	// Extract language tags from a string
	langRegex := regexp.MustCompile(`(?i)(MULTI|Dual[- ]Audio|VOSTFR|FRENCH|GERMAN|SPANISH|ITALIAN|JAPANESE)`)
	if matches := langRegex.FindStringSubmatch(str); len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// ParsedMovie utility methods

// GetQualityTier returns a numeric tier for quality comparison
func (pm *ParsedMovie) GetQualityTier() int {
	switch strings.ToLower(pm.Quality) {
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

// String returns a human-readable representation
func (pm *ParsedMovie) String() string {
	result := pm.MovieName
	if pm.Year > 0 {
		result += fmt.Sprintf(" (%d)", pm.Year)
	}
	if pm.Quality != "" {
		result += fmt.Sprintf(" [%s]", pm.Quality)
	}
	if pm.Edition != "" {
		result += fmt.Sprintf(" (%s)", pm.Edition)
	}
	return result
}

// Has4K returns true if this movie is 4K quality
func (pm *ParsedMovie) Has4K() bool {
	return pm.Is4K
}
