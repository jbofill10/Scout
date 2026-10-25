package service

import (
	"context"
	"testing"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	repomocks "github.com/jbofill10/scout/backend/internal/torrenter/repository/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golift.io/starr/prowlarr"
)

func TestMinQualityTier(t *testing.T) {
	t.Setenv("MIN_QUALITY", "")
	assert.Equal(t, 4, minQualityTier(), "default floor is 1080p")
	t.Setenv("MIN_QUALITY", "720p")
	assert.Equal(t, 3, minQualityTier())
	t.Setenv("MIN_QUALITY", "4K")
	assert.Equal(t, 5, minQualityTier())
	t.Setenv("MIN_QUALITY", "potato")
	assert.Equal(t, 4, minQualityTier(), "garbage falls back to the default, not to no floor")
}

// With the floor at 1080p a 720p listing is never chosen, however well seeded,
// and an unparseable resolution counts as below the floor. When nothing clears
// the floor there is no match, so the search falls through to the next tier.
func TestPickBestTorrent_MinimumQualityFloor(t *testing.T) {
	const hd = "Some.Show.S02E13.1080p.WEB.x264-GRP"
	const sd = "Some Show S02E13 720p WEB x264 GRP [eztv]"
	const unknown = "Some.Show.S02E13.WEB.x264-GRP"

	t.Run("720p rejected even with far more seeders", func(t *testing.T) {
		h := newSeederTestHandler(t)
		h.minQualityTier = QualityTier("1080p")
		best := h.pickBestTorrent(context.Background(),
			[]*models.TorrentMatch{seederTestMatch(hd, 3), seederTestMatch(sd, 200), seederTestMatch(unknown, 300)}, "series", false)
		require.NotNil(t, best)
		assert.Equal(t, hd, best.Torrent.Title)
	})

	t.Run("nothing at or above the floor means no match", func(t *testing.T) {
		h := newSeederTestHandler(t)
		h.minQualityTier = QualityTier("1080p")
		best := h.pickBestTorrent(context.Background(),
			[]*models.TorrentMatch{seederTestMatch(sd, 200), seederTestMatch(unknown, 300)}, "series", false)
		assert.Nil(t, best)
	})

	t.Run("zero disables the floor", func(t *testing.T) {
		h := newSeederTestHandler(t)
		h.minQualityTier = 0
		best := h.pickBestTorrent(context.Background(),
			[]*models.TorrentMatch{seederTestMatch(sd, 200)}, "series", false)
		require.NotNil(t, best)
		assert.Equal(t, sd, best.Torrent.Title)
	})
}

func TestSeederBoost(t *testing.T) {
	tests := []struct {
		seeders int
		want    float64
	}{
		{-1, -1.0},
		{0, -1.0},
		{1, -0.32},
		{3, -0.16},
		{5, 0.0},
		{6, 0.28},
		{7, 0.3},
		{15, 0.4},
		{31, 0.5},
		{63, 0.6},
		{127, 0.7},
		{1000, 0.7}, // capped
	}
	for _, tt := range tests {
		assert.InDelta(t, tt.want, seederBoost(tt.seeders), 0.001, "seeders=%d", tt.seeders)
	}
}

func seederTestMatch(title string, seeders int) *models.TorrentMatch {
	return &models.TorrentMatch{
		Strategy: &models.SearchStrategy{MediaName: "Some Show", Season: 2, Episode: 13},
		Torrent:  &prowlarr.Search{Title: title, Seeders: seeders},
	}
}

func newSeederTestHandler(t *testing.T) *QbittHandler {
	t.Helper()
	repo := repomocks.NewRepository(t)
	repo.On("GetPreferredUploaders", mock.Anything, "series", false).Return([]string{}, nil)
	h := newRelaxTestHandler(t)
	h.repo = repo
	return h
}

// Seeders can outweigh one resolution tier, but not a healthy swarm at the
// higher resolution.
func TestPickBestTorrent_SeedersOutweighOneQualityTier(t *testing.T) {
	const hd = "Some.Show.S02E13.1080p.WEB.x264-GRP"
	const sd = "Some Show S02E13 720p WEB x264 GRP [eztv]"

	t.Run("well-seeded 720p beats barely-seeded 1080p", func(t *testing.T) {
		h := newSeederTestHandler(t)
		best := h.pickBestTorrent(context.Background(),
			[]*models.TorrentMatch{seederTestMatch(hd, 4), seederTestMatch(sd, 120)}, "series", false)
		require.NotNil(t, best)
		assert.Equal(t, sd, best.Torrent.Title)
	})

	// The case seen on The Punisher S01E08: a 1080p listing showing 3 seeders
	// (in practice none) must not outrank a 720p one with 17.
	t.Run("suspect 1080p loses to a modestly seeded 720p", func(t *testing.T) {
		h := newSeederTestHandler(t)
		best := h.pickBestTorrent(context.Background(),
			[]*models.TorrentMatch{seederTestMatch(hd, 3), seederTestMatch(sd, 17)}, "series", false)
		require.NotNil(t, best)
		assert.Equal(t, sd, best.Torrent.Title)
	})

	t.Run("healthy 1080p still beats well-seeded 720p", func(t *testing.T) {
		h := newSeederTestHandler(t)
		best := h.pickBestTorrent(context.Background(),
			[]*models.TorrentMatch{seederTestMatch(hd, 30), seederTestMatch(sd, 120)}, "series", false)
		require.NotNil(t, best)
		assert.Equal(t, hd, best.Torrent.Title)
	})

	t.Run("zero-seeder listing loses to anything alive", func(t *testing.T) {
		h := newSeederTestHandler(t)
		best := h.pickBestTorrent(context.Background(),
			[]*models.TorrentMatch{seederTestMatch(hd, 0), seederTestMatch(sd, 2)}, "series", false)
		require.NotNil(t, best)
		assert.Equal(t, sd, best.Torrent.Title)
	})

	t.Run("zero-seeder listing is still chosen when it is the only option", func(t *testing.T) {
		h := newSeederTestHandler(t)
		best := h.pickBestTorrent(context.Background(),
			[]*models.TorrentMatch{seederTestMatch(hd, 0)}, "series", false)
		require.NotNil(t, best)
		assert.Equal(t, hd, best.Torrent.Title)
	})
}
