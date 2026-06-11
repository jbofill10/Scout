package service

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	repomocks "github.com/jbofill10/scout/backend/internal/torrenter/repository/mocks"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golift.io/starr/prowlarr"
)

func newRelaxTestHandler(t *testing.T) *QbittHandler {
	t.Helper()
	return &QbittHandler{
		logger: slog.New(slog.NewTextHandler(new(bytes.Buffer), nil)),
		parser: NewTorrentParser(),
	}
}

func queriesAtLevel(strategies []*models.SearchStrategy, level int) []string {
	out := []string{}
	for _, ss := range strategies {
		if ss.RelaxLevel == level {
			out = append(out, ss.Query)
		}
	}
	return out
}

func TestStripNamePunctuation(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Show", "Show"},
		{"Re:Zero", "ReZero"},
		{"It's Always Sunny", "Its Always Sunny"},
		{"Don’t Look Up", "Dont Look Up"},
		{"Brooklyn Nine-Nine", "Brooklyn NineNine"},
		{"Show, The", "Show The"},
		{"Show (2019)", "Show"},
		{"Show [2019]", "Show"},
		{"Foo: Bar - Baz (2021)", "Foo Bar Baz"},
		{"  Multiple   Spaces  ", "Multiple Spaces"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, stripNamePunctuation(c.in), "stripNamePunctuation(%q)", c.in)
	}
}

func TestCreateSearchStrategy_TiersAndRelaxLevels(t *testing.T) {
	q := newRelaxTestHandler(t)
	req := &tvdb.Media{Id: "42", Name: "Re:Zero", Anime: false}
	ep := &tvdb.Episode{SeasonNumber: 1, Number: 5, AbsoluteNumber: 0}

	ss := q.createSearchStrategy(req, ep)

	tier0 := queriesAtLevel(ss, 0)
	assert.Contains(t, tier0, "Re:Zero S01E05")
	assert.Contains(t, tier0, "Re:Zero Season 01 Episode 05")

	tier1 := queriesAtLevel(ss, 1)
	assert.Contains(t, tier1, "Re:Zero 1x05", "alternate season-x-episode format")
	assert.Contains(t, tier1, "ReZero S01E05", "punctuation-stripped name on S##E## format")

	tier2 := queriesAtLevel(ss, 2)
	assert.Contains(t, tier2, "Re:Zero E05", "loose episode token")

	// Non-anime should not emit name-only at tier 2.
	assert.NotContains(t, tier2, "Re:Zero")
}

func TestCreateSearchStrategy_AnimeAbsoluteTiers(t *testing.T) {
	q := newRelaxTestHandler(t)
	req := &tvdb.Media{Id: "7", Name: "My Anime", Anime: true}
	ep := &tvdb.Episode{SeasonNumber: 2, Number: 3, AbsoluteNumber: 28}

	ss := q.createSearchStrategy(req, ep)

	// Tier 0: padded absolute, with Exclude preserved.
	var absStrict *models.SearchStrategy
	for _, s := range ss {
		if s.RelaxLevel == 0 && s.Query == "My Anime 28" {
			absStrict = s
		}
	}
	require.NotNil(t, absStrict, "expected padded absolute tier-0 strategy")
	assert.Equal(t, []string{"season", "episode"}, absStrict.Exclude)
	assert.Equal(t, 28, absStrict.Episode, "absolute strategy uses absolute number as Episode")

	// Tier 1: unpadded absolute (same value here since 28 has no padding, but exercises the path)
	// and punctuation-stripped name (none for "My Anime"). Verify the 1x format too.
	tier1 := queriesAtLevel(ss, 1)
	assert.Contains(t, tier1, "My Anime 2x03")

	// Tier 2: anime name-only present.
	tier2 := queriesAtLevel(ss, 2)
	assert.Contains(t, tier2, "My Anime")

	// The tier-2 anime name-only strategy must keep absolute Exclude behavior.
	for _, s := range ss {
		if s.RelaxLevel == 2 && s.Query == "My Anime" {
			assert.Equal(t, []string{"season", "episode"}, s.Exclude)
			assert.Equal(t, 28, s.Episode)
		}
	}
}

func TestCreateSearchStrategy_NoAbsoluteWhenZero(t *testing.T) {
	q := newRelaxTestHandler(t)
	req := &tvdb.Media{Id: "7", Name: "My Anime", Anime: true}
	ep := &tvdb.Episode{SeasonNumber: 1, Number: 4, AbsoluteNumber: 0}

	ss := q.createSearchStrategy(req, ep)
	for _, s := range ss {
		// No absolute-style strategy (Exclude set) should exist when AbsoluteNumber == 0,
		// except the tier-2 name-only which falls back to a seasonal strategy.
		if len(s.Exclude) > 0 {
			t.Fatalf("unexpected absolute strategy when AbsoluteNumber==0: %q", s.Query)
		}
	}
	// Tier-2 anime name-only falls back to seasonal name-only.
	assert.Contains(t, queriesAtLevel(ss, 2), "My Anime")
}

func TestCreateMovieSearchStrategy_Tiers(t *testing.T) {
	q := newRelaxTestHandler(t)
	req := &tvdb.Media{Id: "9", Name: "The Movie: Part II", Year: "2021"}

	ss := q.createMovieSearchStrategy(req)

	assert.Equal(t, []string{"The Movie: Part II 2021"}, queriesAtLevel(ss, 0))
	assert.Equal(t, []string{"The Movie: Part II"}, queriesAtLevel(ss, 1))
	assert.Equal(t, []string{"The Movie Part II"}, queriesAtLevel(ss, 2))
	for _, s := range ss {
		assert.True(t, s.IsMovie)
	}
}

func TestGroupStrategiesByRelaxLevel(t *testing.T) {
	in := []*models.SearchStrategy{
		{Query: "a", RelaxLevel: 0},
		{Query: "b", RelaxLevel: 2},
		{Query: "c", RelaxLevel: 1},
		{Query: "d", RelaxLevel: 0},
		{Query: "e", RelaxLevel: 2},
	}
	tiers := groupStrategiesByRelaxLevel(in)
	require.Len(t, tiers, 3)
	assert.Equal(t, []string{"a", "d"}, []string{tiers[0][0].Query, tiers[0][1].Query})
	assert.Equal(t, "c", tiers[1][0].Query)
	assert.Equal(t, []string{"b", "e"}, []string{tiers[2][0].Query, tiers[2][1].Query})

	assert.Nil(t, groupStrategiesByRelaxLevel(nil))
}

func TestRelaxConfidenceCutoff(t *testing.T) {
	assert.Equal(t, baseConfidenceCutoff, relaxConfidenceCutoff(&models.SearchStrategy{RelaxLevel: 0}))
	assert.Equal(t, baseConfidenceCutoff, relaxConfidenceCutoff(&models.SearchStrategy{RelaxLevel: 1}))
	assert.Equal(t, baseConfidenceCutoff+relaxConfidenceBump, relaxConfidenceCutoff(&models.SearchStrategy{RelaxLevel: 2}))
	assert.Equal(t, baseConfidenceCutoff, relaxConfidenceCutoff(nil))
}

func TestNextPollInterval(t *testing.T) {
	assert.Equal(t, monitorFastInterval, nextPollInterval(0))
	assert.Equal(t, monitorFastInterval, nextPollInterval(9*time.Minute))
	assert.Equal(t, monitorSlowInterval, nextPollInterval(10*time.Minute))
	assert.Equal(t, monitorSlowInterval, nextPollInterval(2*time.Hour))
}

func TestMonitorTimeout(t *testing.T) {
	t.Setenv("MONITOR_TIMEOUT", "")
	assert.Equal(t, defaultMonitorTimeout, monitorTimeout())

	t.Setenv("MONITOR_TIMEOUT", "2h30m")
	assert.Equal(t, 2*time.Hour+30*time.Minute, monitorTimeout())

	t.Setenv("MONITOR_TIMEOUT", "not-a-duration")
	assert.Equal(t, defaultMonitorTimeout, monitorTimeout())
}

// TestPickBestTorrent_RelaxGuardrail verifies that a broad (RelaxLevel>=2) match is
// rejected when its confidence is below the bumped cutoff, while a strict (RelaxLevel<=1)
// match at the same confidence is accepted.
func TestPickBestTorrent_RelaxGuardrail(t *testing.T) {
	// Title parses cleanly to S01E05 of "Some Show" → high confidence (well above
	// base cutoff, below or above bump depending — pick a title that lands between).
	mkMatch := func(relax int) *models.TorrentMatch {
		return &models.TorrentMatch{
			Strategy: &models.SearchStrategy{
				MediaName:  "Some Show",
				Season:     1,
				Episode:    5,
				RelaxLevel: relax,
			},
			Torrent: &prowlarr.Search{
				// Marginal valid match (~0.69 confidence): clears base cutoff but
				// falls below the bumped cutoff applied to broad queries.
				Title:   "Some Show Extra S01E05",
				Seeders: 100,
			},
		}
	}

	// Sanity: confirm the validation confidence sits in the guardrail band so the
	// test is meaningful (strict accepted, broad rejected).
	q := newRelaxTestHandler(t)
	vr := q.validateShowTorrent(mkMatch(0).Torrent, mkMatch(0).Strategy)
	require.True(t, vr.IsValid)
	require.GreaterOrEqual(t, vr.Confidence, baseConfidenceCutoff,
		"fixture confidence must clear base cutoff (strict accepted)")
	require.Less(t, vr.Confidence, baseConfidenceCutoff+relaxConfidenceBump,
		"fixture confidence must fall below bumped cutoff so the guardrail bites for broad")

	t.Run("strict match accepted", func(t *testing.T) {
		repo := repomocks.NewRepository(t)
		repo.On("GetPreferredUploaders", mock.Anything, "series", false).Return([]string{}, nil)
		h := newRelaxTestHandler(t)
		h.repo = repo

		best := h.pickBestTorrent(context.Background(), []*models.TorrentMatch{mkMatch(0)}, "series", false)
		require.NotNil(t, best)
		assert.Equal(t, 0, best.Strategy.RelaxLevel)
	})

	t.Run("broad match rejected by guardrail", func(t *testing.T) {
		repo := repomocks.NewRepository(t)
		repo.On("GetPreferredUploaders", mock.Anything, "series", false).Return([]string{}, nil)
		h := newRelaxTestHandler(t)
		h.repo = repo

		best := h.pickBestTorrent(context.Background(), []*models.TorrentMatch{mkMatch(2)}, "series", false)
		assert.Nil(t, best, "broad match below bumped cutoff should be rejected")
	})
}
