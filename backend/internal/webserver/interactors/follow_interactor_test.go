package interactors

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jbofill10/scout/backend/internal/webserver/repository"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeFollowRepo struct {
	shows   map[string]repository.FollowedShow
	checks  []string
	added   int
	lastErr string
}

func newFakeFollowRepo() *fakeFollowRepo {
	return &fakeFollowRepo{shows: map[string]repository.FollowedShow{}}
}

func (f *fakeFollowRepo) Upsert(_ context.Context, show repository.FollowedShow) error {
	if existing, ok := f.shows[show.TvdbID]; ok {
		show.FollowedAt = existing.FollowedAt // a re-follow keeps its original date
	}
	f.shows[show.TvdbID] = show
	return nil
}
func (f *fakeFollowRepo) Delete(_ context.Context, id string) (bool, error) {
	_, ok := f.shows[id]
	delete(f.shows, id)
	return ok, nil
}
func (f *fakeFollowRepo) List(context.Context) ([]repository.FollowedShow, error) {
	out := []repository.FollowedShow{}
	for _, s := range f.shows {
		out = append(out, s)
	}
	return out, nil
}
func (f *fakeFollowRepo) Get(_ context.Context, id string) (*repository.FollowedShow, error) {
	s, ok := f.shows[id]
	if !ok {
		return nil, nil
	}
	return &s, nil
}
func (f *fakeFollowRepo) RecordCheck(_ context.Context, id string, added int, checkErr string) error {
	f.checks = append(f.checks, id)
	f.added += added
	f.lastErr = checkErr
	return nil
}

type fakeFollowDeps struct {
	episodes  []tvdb.Episode
	tvdbErr   error
	plex      tvdb.StatusBatchResponse
	tracked   map[string]bool
	notified  map[string]bool
	requested []tvdb.Media
	summary   DownloadSummary
}

func (d *fakeFollowDeps) GetExtendedInfo(context.Context, string, string) (tvdb.TVDBSeriesExtendedResponse, error) {
	var ext tvdb.TVDBSeriesExtendedResponse
	ext.Data.Episodes = d.episodes
	return ext, d.tvdbErr
}
func (d *fakeFollowDeps) GetStatusBatch(context.Context, []tvdb.StatusRequest) (tvdb.StatusBatchResponse, error) {
	return d.plex, nil
}
func (d *fakeFollowDeps) ExistingTvdbIDs(_ context.Context, ids []string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, id := range ids {
		if d.notified[id] {
			out[id] = true
		}
	}
	return out, nil
}
func (d *fakeFollowDeps) TrackedItemKeys(context.Context) (map[string]bool, error) {
	return d.tracked, nil
}
func (d *fakeFollowDeps) DownloadShow(_ context.Context, media tvdb.Media) (DownloadSummary, error) {
	d.requested = append(d.requested, media)
	return d.summary, nil
}

func ep(id, season, number int, aired string) tvdb.Episode {
	return tvdb.Episode{Id: id, SeasonNumber: season, Number: number, Aired: aired}
}

func newFollowTest(t *testing.T, deps *fakeFollowDeps) (*FollowInteractor, *fakeFollowRepo) {
	t.Helper()
	repo := newFakeFollowRepo()
	f := NewFollowInteractor(repo, deps, deps, deps, deps, deps, time.Hour, testLogger())
	f.now = func() time.Time { return time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC) }
	return f, repo
}

// Episode ids: 1 special, 2 in Plex, 3 tracked by a row, 4 has a notification,
// 5 aired before the follow, 6 aired after, 7 is in the future, 8 has no date.
func followFixture() *fakeFollowDeps {
	return &fakeFollowDeps{
		episodes: []tvdb.Episode{
			ep(1, 0, 1, "2020-01-01"),
			ep(2, 1, 1, "2020-01-08"),
			ep(3, 1, 2, "2020-01-15"),
			ep(4, 1, 3, "2020-01-22"),
			ep(5, 1, 4, "2026-06-14"),
			ep(6, 2, 1, "2026-06-15"),
			ep(7, 2, 2, "2026-07-01"),
			ep(8, 2, 3, ""),
		},
		plex: tvdb.StatusBatchResponse{Shows: []tvdb.ShowStatus{{TvdbId: "900", Seasons: []tvdb.SeasonStatus{
			{SeasonNum: 1, Episodes: []tvdb.EpisodeStatus{{EpisodeNum: 1, Downloaded: true}, {EpisodeNum: 2, Downloaded: false}}},
		}}}},
		tracked:  map[string]bool{"3": true},
		notified: map[string]bool{"4": true},
		summary:  DownloadSummary{QueuedNow: 1, Scheduled: 1},
	}
}

func TestFollow_NewEpisodesOnly(t *testing.T) {
	deps := followFixture()
	f, repo := newFollowTest(t, deps)

	resp, err := f.Follow(context.Background(), tvdb.Media{Id: "900", Name: "Some Show", ImageUrl: "poster",
		Metadata: tvdb.TVDBSeriesMetadata{Episodes: []tvdb.Episode{ep(99, 1, 1, "2020-01-01")}}}, false)

	require.NoError(t, err)
	require.Len(t, deps.requested, 1)
	got := []int{}
	for _, e := range deps.requested[0].Metadata.Episodes {
		got = append(got, e.Id)
	}
	assert.Equal(t, []int{6, 7}, got, "only episodes airing on or after the follow day that Scout has not seen")
	assert.Equal(t, "series", deps.requested[0].Category)
	assert.Equal(t, 8, resp.Check.Listed)
	assert.Equal(t, 2, resp.Check.New)
	assert.Equal(t, 1, resp.Check.QueuedNow)
	assert.Equal(t, 1, resp.Check.Scheduled)
	assert.Empty(t, repo.shows["900"].Media.Metadata.Episodes, "the snapshot carries no episodes")
	assert.Equal(t, "poster", repo.shows["900"].PosterUrl)
	assert.Equal(t, []string{"900"}, repo.checks)
	assert.Equal(t, 2, repo.added)
}

func TestFollow_IncludeMissingAddsTheBackCatalogue(t *testing.T) {
	deps := followFixture()
	f, _ := newFollowTest(t, deps)

	_, err := f.Follow(context.Background(), tvdb.Media{Id: "900", Name: "Some Show"}, true)

	require.NoError(t, err)
	require.Len(t, deps.requested, 1)
	got := []int{}
	for _, e := range deps.requested[0].Metadata.Episodes {
		got = append(got, e.Id)
	}
	assert.Equal(t, []int{5, 6, 7}, got, "pre-follow episodes join in; specials, Plex, tracked and notified ones still do not")
}

func TestFollow_RejectsMovies(t *testing.T) {
	f, _ := newFollowTest(t, followFixture())
	_, err := f.Follow(context.Background(), tvdb.Media{Id: "1", Name: "A Movie", Category: "movie"}, false)
	assert.ErrorIs(t, err, ErrNotSeries)
}

func TestCheckAll_NothingNewMakesNoRequest(t *testing.T) {
	deps := followFixture()
	deps.episodes = deps.episodes[:5] // everything is handled or pre-follow
	f, repo := newFollowTest(t, deps)
	repo.shows["900"] = repository.FollowedShow{TvdbID: "900", Title: "Some Show", FollowedAt: f.now(), Media: tvdb.Media{Id: "900", Name: "Some Show"}}

	report, err := f.CheckAll(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 1, report.Checked)
	assert.Equal(t, 0, report.Added)
	assert.Empty(t, deps.requested, "no request, so no duplicate notifications")
	assert.Equal(t, []string{"900"}, repo.checks, "the check is still recorded")
}

func TestCheckOne_RecordsTvdbFailure(t *testing.T) {
	deps := followFixture()
	deps.tvdbErr = errors.New("tvdb down")
	f, repo := newFollowTest(t, deps)
	repo.shows["900"] = repository.FollowedShow{TvdbID: "900", Title: "Some Show", FollowedAt: f.now()}

	result, err := f.CheckOne(context.Background(), "900")

	require.NoError(t, err)
	assert.Contains(t, result.Error, "tvdb down")
	assert.Contains(t, repo.lastErr, "tvdb down")
	assert.Empty(t, deps.requested)

	_, err = f.CheckOne(context.Background(), "nope")
	assert.ErrorIs(t, err, ErrNothingToControl)
}

func TestUnfollow(t *testing.T) {
	f, repo := newFollowTest(t, followFixture())
	repo.shows["900"] = repository.FollowedShow{TvdbID: "900"}
	ok, err := f.Unfollow(context.Background(), "900")
	require.NoError(t, err)
	assert.True(t, ok)
	ok, _ = f.Unfollow(context.Background(), "900")
	assert.False(t, ok)
}
