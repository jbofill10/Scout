package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Indexers that mirror eztv listings print scene releases with spaces instead
// of dots. Those used to fall through to the fallback pattern, which records no
// resolution, so the best-seeded listings never earned a quality boost.
func TestTorrentParser_Parse_SpaceSeparatedScene(t *testing.T) {
	tp := NewTorrentParser()

	tests := []struct {
		title      string
		show       string
		season     int
		episode    int
		quality    string
		group      string
		confidence float64
	}{
		{
			title:      "Marvels The Punisher S02E13 REPACK 720p WEB X264 AMCON [eztv]",
			show:       "Marvels The Punisher",
			season:     2,
			episode:    13,
			quality:    "720p",
			confidence: 0.85,
		},
		{
			title:      "Marvels The Punisher S02E13 REPACK 480p x264 mSD [eztv]",
			show:       "Marvels The Punisher",
			season:     2,
			episode:    13,
			quality:    "480p",
			confidence: 0.85,
		},
		{
			title:      "Marvel's The Punisher S01E13 (1080p NF WEBRip x265 HEVC 10bit AC3 5.1 Qman) [UTR]",
			show:       "Marvel's The Punisher",
			season:     1,
			episode:    13,
			quality:    "1080p",
			confidence: 0.85,
		},
		{
			title:      "Some Show S01E05 Gray Matter 1080p BluRay x264-DEMAND",
			show:       "Some Show",
			season:     1,
			episode:    5,
			quality:    "1080p",
			group:      "DEMAND",
			confidence: 0.90,
		},
		// Dotted releases keep parsing exactly as before.
		{
			title:      "Marvels.The.Punisher.S02E13.REPACK.720p.WEB.X264-AMCON[rartv]",
			show:       "Marvels The Punisher",
			season:     2,
			episode:    13,
			quality:    "720p",
			group:      "AMCON[rartv]",
			confidence: 0.90,
		},
	}

	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			parsed := tp.Parse(tt.title)
			require.NotNil(t, parsed)
			assert.Equal(t, tt.show, parsed.ShowName)
			assert.Equal(t, tt.season, parsed.Season)
			assert.Equal(t, tt.episode, parsed.Episode)
			assert.Equal(t, tt.quality, parsed.Quality)
			assert.Equal(t, tt.group, parsed.ReleaseGroup)
			assert.InDelta(t, tt.confidence, parsed.MatchConfidence, 0.001)
			assert.True(t, parsed.IsSeasonal)
		})
	}
}

// A spaced title with no resolution token still parses through the fallback,
// just without a quality.
func TestTorrentParser_Parse_SpaceSeparatedWithoutResolution(t *testing.T) {
	tp := NewTorrentParser()

	parsed := tp.Parse("Marvels The Punisher S02E13 WEB x264 STRiFE [eztv]")
	require.NotNil(t, parsed)
	assert.Equal(t, "Marvels The Punisher", parsed.ShowName)
	assert.Equal(t, 2, parsed.Season)
	assert.Equal(t, 13, parsed.Episode)
	assert.Equal(t, "", parsed.Quality)
	assert.InDelta(t, 0.60, parsed.MatchConfidence, 0.001)
}
