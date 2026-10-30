package interactors

import (
	"context"
	"testing"

	"github.com/jbofill10/scout/backend/pkg/library"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeMissingDeps struct {
	fakeFollowDeps
	missing []library.MissingShow
}

func (d *fakeMissingDeps) GetMissingEpisodes(context.Context) ([]library.MissingShow, error) {
	return d.missing, nil
}

func missingEp(id string, season, number int) library.MissingEpisode {
	return library.MissingEpisode{EpisodeWithStatus: library.EpisodeWithStatus{TvdbId: id, SeasonNumber: season, EpisodeNumber: number, Aired: "2026-01-01"}}
}

func newMissingTest() (*MissingInteractor, *fakeMissingDeps) {
	deps := &fakeMissingDeps{
		fakeFollowDeps: fakeFollowDeps{tracked: map[string]bool{"2": true}, notified: map[string]bool{"3": true},
			summary: DownloadSummary{MediaTitle: "Some Show", Category: "series", QueuedNow: 2}},
		missing: []library.MissingShow{
			{TvdbId: "900", Title: "Some Show", Episodes: []library.MissingEpisode{missingEp("1", 1, 1), missingEp("2", 1, 2), missingEp("3", 1, 3), missingEp("4", 1, 4)}},
			{TvdbId: "901", Title: "Done Show", Episodes: []library.MissingEpisode{missingEp("5", 1, 1)}},
		},
	}
	deps.tracked["5"] = true
	return NewMissingInteractor(deps, deps, deps, deps, testLogger()), deps
}

func TestMissingList_MarksRequested(t *testing.T) {
	m, _ := newMissingTest()
	shows, err := m.List(context.Background())
	require.NoError(t, err)
	require.Len(t, shows, 2)
	flags := []bool{}
	for _, ep := range shows[0].Episodes {
		flags = append(flags, ep.Requested)
	}
	assert.Equal(t, []bool{false, true, true, false}, flags, "a scheduled row or a notification marks an episode requested")
}

func TestFillMissing_RequestsOnlyUnrequestedEpisodes(t *testing.T) {
	m, deps := newMissingTest()
	summary, err := m.FillMissing(context.Background(), "900")
	require.NoError(t, err)
	require.Len(t, deps.requested, 1)
	req := deps.requested[0]
	assert.Equal(t, "900", req.Id)
	assert.Equal(t, "Some Show", req.Name)
	assert.Equal(t, "series", req.Category)
	ids := []int{}
	for _, ep := range req.Metadata.Episodes {
		ids = append(ids, ep.Id)
	}
	assert.Equal(t, []int{1, 4}, ids)
	assert.Equal(t, 2, summary.QueuedNow)
	assert.Equal(t, 2, summary.Skipped, "the two already-requested episodes count as skipped")
}

func TestFillMissing_NothingLeftMakesNoRequest(t *testing.T) {
	m, deps := newMissingTest()
	summary, err := m.FillMissing(context.Background(), "901")
	require.NoError(t, err)
	assert.Empty(t, deps.requested)
	assert.Equal(t, 1, summary.Skipped)
	assert.Equal(t, 0, summary.QueuedNow)

	_, err = m.FillMissing(context.Background(), "nope")
	assert.ErrorIs(t, err, ErrNothingToControl)
	_ = tvdb.Media{}
}
