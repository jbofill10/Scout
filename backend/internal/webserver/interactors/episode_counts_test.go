package interactors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"
)

// A fixed "now" so the aired split does not depend on when the test runs.
var countsNow = time.Date(2026, time.November, 4, 15, 30, 0, 0, time.UTC)

func TestEpisodeHasAired(t *testing.T) {
	tests := []struct {
		name  string
		aired string
		want  bool
	}{
		{"last year", "2025-06-13", true},
		{"yesterday", "2026-11-03", true},
		{"today counts as aired", "2026-11-04", true},
		{"tomorrow", "2026-11-05", false},
		{"next season", "2027-02-01", false},
		{"blank date is unaired", "", false},
		{"garbage is unaired", "TBA", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, episodeHasAired(tt.aired, countsNow))
		})
	}
}

// Dexter: Resurrection as TVDB lists it mid-run: season 1 aired and in the
// library, season 2 announced with a date on one episode and none on the rest.
func TestCalculateEpisodeCountsFromMetadata_SplitsAiredFromUpcoming(t *testing.T) {
	episodes := []tvdb.Episode{
		{Id: 1, SeasonNumber: 0, Number: 1, Aired: "2025-07-01"}, // special, ignored
	}
	for n := 1; n <= 11; n++ {
		episodes = append(episodes, tvdb.Episode{Id: 100 + n, SeasonNumber: 1, Number: n, Aired: "2025-07-11"})
	}
	episodes = append(episodes, tvdb.Episode{Id: 201, SeasonNumber: 2, Number: 1, Aired: "2027-01-10"})
	for n := 2; n <= 9; n++ {
		episodes = append(episodes, tvdb.Episode{Id: 200 + n, SeasonNumber: 2, Number: n})
	}
	media := tvdb.Media{Id: "s1", Category: "series", Metadata: tvdb.TVDBSeriesMetadata{Episodes: episodes}}

	var have []tvdb.EpisodeStatus
	for n := 1; n <= 11; n++ {
		have = append(have, tvdb.EpisodeStatus{EpisodeNum: n, Downloaded: true})
	}
	status := tvdb.ShowStatus{TvdbId: "s1", Seasons: []tvdb.SeasonStatus{{SeasonNum: 1, Episodes: have}}}

	downloaded, aired, total := calculateEpisodeCountsFromMetadata(media, status, countsNow)

	assert.Equal(t, 11, downloaded)
	assert.Equal(t, 11, aired, "only the episodes that have aired count against the library")
	assert.Equal(t, 20, total, "total still reports everything TVDB lists")
}

func TestCalculateEpisodeCountsFromMetadata_AiredGapIsMissing(t *testing.T) {
	media := tvdb.Media{Id: "s1", Category: "series", Metadata: tvdb.TVDBSeriesMetadata{Episodes: []tvdb.Episode{
		{Id: 1, SeasonNumber: 1, Number: 1, Aired: "2026-10-21"},
		{Id: 2, SeasonNumber: 1, Number: 2, Aired: "2026-10-28"},
		{Id: 3, SeasonNumber: 1, Number: 3, Aired: "2026-11-04"},
		{Id: 4, SeasonNumber: 1, Number: 4, Aired: "2026-11-11"},
	}}}
	status := tvdb.ShowStatus{TvdbId: "s1", Seasons: []tvdb.SeasonStatus{
		{SeasonNum: 1, Episodes: []tvdb.EpisodeStatus{{EpisodeNum: 1, Downloaded: true, TvdbId: "1"}}},
	}}

	downloaded, aired, total := calculateEpisodeCountsFromMetadata(media, status, countsNow)

	assert.Equal(t, 1, downloaded)
	assert.Equal(t, 3, aired, "today's episode has aired")
	assert.Equal(t, 4, total)
}

func TestCalculateEpisodeCountsFromMetadata_NoMetadataCountsLibraryAsAired(t *testing.T) {
	media := tvdb.Media{Id: "s1", Category: "series"}
	status := tvdb.ShowStatus{TvdbId: "s1", Seasons: []tvdb.SeasonStatus{
		{SeasonNum: 1, Episodes: []tvdb.EpisodeStatus{{EpisodeNum: 1, Downloaded: true}, {EpisodeNum: 2, Downloaded: true}}},
	}}

	downloaded, aired, total := calculateEpisodeCountsFromMetadata(media, status, countsNow)

	assert.Equal(t, 2, downloaded)
	assert.Equal(t, 2, aired)
	assert.Equal(t, 2, total)
}

func TestCalculateEpisodeCountsFromMetadata_AnimeAbsoluteNumbers(t *testing.T) {
	media := tvdb.Media{Id: "jjk", Category: "series", Anime: true, Metadata: tvdb.TVDBSeriesMetadata{Episodes: []tvdb.Episode{
		{Id: 1, SeasonNumber: 1, Number: 1, AbsoluteNumber: 1, Aired: "2020-10-03"},
		{Id: 2, SeasonNumber: 1, Number: 24, AbsoluteNumber: 24, Aired: "2021-03-27"},
		{Id: 3, SeasonNumber: 2, Number: 1, AbsoluteNumber: 25, Aired: "2023-07-06"},
		{Id: 4, SeasonNumber: 2, Number: 2, AbsoluteNumber: 26, Aired: "2027-07-13"},
	}}}
	status := tvdb.ShowStatus{TvdbId: "jjk", Seasons: []tvdb.SeasonStatus{
		{SeasonNum: 1, Episodes: []tvdb.EpisodeStatus{{EpisodeNum: 1, Downloaded: true}, {EpisodeNum: 24, Downloaded: true}}},
		{SeasonNum: 2, Episodes: []tvdb.EpisodeStatus{{EpisodeNum: 25, Downloaded: true}}},
	}}

	downloaded, aired, total := calculateEpisodeCountsFromMetadata(media, status, countsNow)

	assert.Equal(t, 3, downloaded, "S02E01 matched through its absolute number")
	assert.Equal(t, 3, aired)
	assert.Equal(t, 4, total)
}

func TestCalculateEpisodeCountsFromMetadata_LibraryEpisodeCountsAsAired(t *testing.T) {
	// TVDB's date is wrong (or missing) but the file is on disk: 1/1, not 1/0.
	media := tvdb.Media{Id: "s1", Category: "series", Metadata: tvdb.TVDBSeriesMetadata{Episodes: []tvdb.Episode{
		{Id: 1, SeasonNumber: 1, Number: 1, Aired: ""},
		{Id: 2, SeasonNumber: 1, Number: 2, Aired: "2099-01-01"},
	}}}
	status := tvdb.ShowStatus{TvdbId: "s1", Seasons: []tvdb.SeasonStatus{
		{SeasonNum: 1, Episodes: []tvdb.EpisodeStatus{{EpisodeNum: 1, Downloaded: true, TvdbId: "1"}}},
	}}

	downloaded, aired, total := calculateEpisodeCountsFromMetadata(media, status, countsNow)

	assert.Equal(t, 1, downloaded)
	assert.Equal(t, 1, aired)
	assert.Equal(t, 2, total)
}
