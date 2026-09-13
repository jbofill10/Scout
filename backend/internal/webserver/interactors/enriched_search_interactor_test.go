package interactors

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jbofill10/scout/backend/internal/webserver/clients"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockSearcher stands in for the SearchInteractor.
type MockSearcher struct {
	mock.Mock
}

func (m *MockSearcher) Search(ctx context.Context, mediaType, query string) ([]tvdb.Media, error) {
	args := m.Called(ctx, mediaType, query)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]tvdb.Media), args.Error(1)
}

func setupEnrichedSearchInteractor() (
	*EnrichedSearchInteractor, *MockSearcher, *MockTVDBProxyClient, *MockTorrenterClient,
) {
	searcher := new(MockSearcher)
	mockTVDB := new(MockTVDBProxyClient)
	mockTorrenter := new(MockTorrenterClient)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	interactor := NewEnrichedSearchInteractor(searcher, mockTVDB, mockTorrenter, logger)
	return interactor, searcher, mockTVDB, mockTorrenter
}

func episode(id, season, number, absolute int) tvdb.Episode {
	return tvdb.Episode{Id: id, SeasonNumber: season, Number: number, AbsoluteNumber: absolute}
}

func series(id string, episodes ...tvdb.Episode) tvdb.Media {
	return tvdb.Media{
		Id:       id,
		Name:     "Show " + id,
		Category: "series",
		Metadata: tvdb.TVDBSeriesMetadata{Episodes: episodes},
	}
}

func movie(id string) tvdb.Media {
	return tvdb.Media{Id: id, Name: "Movie " + id, Category: "movie"}
}

// extendedRequestsFor matches a GetExtendedBatch call by the exact set of ids.
func extendedRequestsFor(ids ...string) interface{} {
	return mock.MatchedBy(func(requests []clients.ExtendedRequest) bool {
		if len(requests) != len(ids) {
			return false
		}
		for idx, request := range requests {
			if request.Id != ids[idx] || request.MediaType != "series" {
				return false
			}
		}
		return true
	})
}

func TestEnrichedSearch_MoviesMergeLibraryStatusWithoutExtendedInfo(t *testing.T) {
	interactor, searcher, mockTVDB, mockTorrenter := setupEnrichedSearchInteractor()

	searcher.On("Search", mock.Anything, "movie", "dune").
		Return([]tvdb.Media{movie("1"), movie("2")}, nil)
	mockTorrenter.On("GetStatusBatch", mock.Anything, []tvdb.StatusRequest{
		{TvdbId: "1", MediaType: "movie"},
		{TvdbId: "2", MediaType: "movie"},
	}).Return(tvdb.StatusBatchResponse{
		Movies: []tvdb.MovieStatus{{TvdbId: "2", InLibrary: true}},
	}, nil)

	results, err := interactor.EnrichedSearch(context.Background(), "movie", "dune")

	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "movie", results[0].Status.Type)
	assert.False(t, results[0].Status.InLibrary)
	assert.True(t, results[1].Status.InLibrary)
	mockTVDB.AssertNotCalled(t, "GetExtendedBatch", mock.Anything, mock.Anything)
}

func TestEnrichedSearch_SeriesCountsUseSearchEpisodes(t *testing.T) {
	interactor, searcher, mockTVDB, mockTorrenter := setupEnrichedSearchInteractor()

	show := series("10", episode(101, 1, 1, 1), episode(102, 1, 2, 2), episode(201, 2, 1, 3))
	searcher.On("Search", mock.Anything, "series", "office").Return([]tvdb.Media{show}, nil)
	mockTorrenter.On("GetStatusBatch", mock.Anything, mock.Anything).Return(tvdb.StatusBatchResponse{
		Shows: []tvdb.ShowStatus{{
			TvdbId: "10",
			Seasons: []tvdb.SeasonStatus{
				{SeasonNum: 1, Episodes: []tvdb.EpisodeStatus{{EpisodeNum: 1, Downloaded: true, TvdbId: "101"}}},
			},
		}},
	}, nil)
	// The show is in the library, so its anime flag is looked up once
	mockTVDB.On("GetExtendedBatch", mock.Anything, extendedRequestsFor("10")).
		Return([]tvdb.Media{{Id: "10", Anime: false}}, nil)

	results, err := interactor.EnrichedSearch(context.Background(), "series", "office")

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, 1, results[0].Status.Downloaded)
	assert.Equal(t, 3, results[0].Status.Total, "total comes from TVDB episodes, not the library")
	assert.Len(t, results[0].Status.Seasons, 1)
	assert.Len(t, results[0].Media.Metadata.Episodes, 3, "episodes from the search hit are kept")
	mockTVDB.AssertExpectations(t)
}

// Plex stores anime with absolute numbering (S02E03 for the third episode
// overall); without the anime flag that episode would not match TVDB's S02E01.
func TestEnrichedSearch_AnimeFlagBridgesAbsoluteNumbering(t *testing.T) {
	interactor, searcher, mockTVDB, mockTorrenter := setupEnrichedSearchInteractor()

	show := series("20", episode(1, 1, 1, 1), episode(2, 1, 2, 2), episode(3, 2, 1, 3))
	searcher.On("Search", mock.Anything, "series", "jjk").Return([]tvdb.Media{show}, nil)
	mockTorrenter.On("GetStatusBatch", mock.Anything, mock.Anything).Return(tvdb.StatusBatchResponse{
		Shows: []tvdb.ShowStatus{{
			TvdbId: "20",
			Seasons: []tvdb.SeasonStatus{
				{SeasonNum: 1, Episodes: []tvdb.EpisodeStatus{
					{EpisodeNum: 1, Downloaded: true},
					{EpisodeNum: 2, Downloaded: true},
				}},
				{SeasonNum: 2, Episodes: []tvdb.EpisodeStatus{{EpisodeNum: 3, Downloaded: true}}},
			},
		}},
	}, nil)
	mockTVDB.On("GetExtendedBatch", mock.Anything, extendedRequestsFor("20")).
		Return([]tvdb.Media{{Id: "20", Anime: true}}, nil)

	results, err := interactor.EnrichedSearch(context.Background(), "series", "jjk")

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].Media.Anime)
	assert.Equal(t, 3, results[0].Status.Downloaded)
	assert.Equal(t, 3, results[0].Status.Total)
}

func TestEnrichedSearch_OnlyLooksUpExtendedInfoForLibraryShows(t *testing.T) {
	interactor, searcher, mockTVDB, mockTorrenter := setupEnrichedSearchInteractor()

	searcher.On("Search", mock.Anything, "series", "naruto").Return([]tvdb.Media{
		series("1", episode(11, 1, 1, 1)),
		series("2", episode(21, 1, 1, 1)),
		series("3", episode(31, 1, 1, 1)),
	}, nil)
	mockTorrenter.On("GetStatusBatch", mock.Anything, mock.Anything).Return(tvdb.StatusBatchResponse{
		Shows: []tvdb.ShowStatus{
			{TvdbId: "2", Seasons: []tvdb.SeasonStatus{
				{SeasonNum: 1, Episodes: []tvdb.EpisodeStatus{{EpisodeNum: 1, Downloaded: true}}},
			}},
			// Known to the torrenter but with nothing downloaded: no lookup needed
			{TvdbId: "3", Seasons: []tvdb.SeasonStatus{}},
		},
	}, nil)
	mockTVDB.On("GetExtendedBatch", mock.Anything, extendedRequestsFor("2")).
		Return([]tvdb.Media{{Id: "2", Anime: true}}, nil)

	results, err := interactor.EnrichedSearch(context.Background(), "series", "naruto")

	require.NoError(t, err)
	require.Len(t, results, 3)
	assert.False(t, results[0].Media.Anime)
	assert.True(t, results[1].Media.Anime)
	assert.False(t, results[2].Media.Anime)
	mockTVDB.AssertExpectations(t)
}

func TestEnrichedSearch_PreservesSearchOrder(t *testing.T) {
	interactor, searcher, _, mockTorrenter := setupEnrichedSearchInteractor()

	searcher.On("Search", mock.Anything, "movie", "alien").
		Return([]tvdb.Media{movie("c"), movie("a"), movie("b")}, nil)
	mockTorrenter.On("GetStatusBatch", mock.Anything, mock.Anything).Return(tvdb.StatusBatchResponse{
		Movies: []tvdb.MovieStatus{{TvdbId: "a", InLibrary: true}, {TvdbId: "c", InLibrary: true}},
	}, nil)

	results, err := interactor.EnrichedSearch(context.Background(), "movie", "alien")

	require.NoError(t, err)
	ids := []string{results[0].Media.Id, results[1].Media.Id, results[2].Media.Id}
	assert.Equal(t, []string{"c", "a", "b"}, ids)
}

func TestEnrichedSearch_ExtendedInfoFailureStillReturnsResults(t *testing.T) {
	interactor, searcher, mockTVDB, mockTorrenter := setupEnrichedSearchInteractor()

	searcher.On("Search", mock.Anything, "series", "x").
		Return([]tvdb.Media{series("5", episode(51, 1, 1, 1))}, nil)
	mockTorrenter.On("GetStatusBatch", mock.Anything, mock.Anything).Return(tvdb.StatusBatchResponse{
		Shows: []tvdb.ShowStatus{{TvdbId: "5", Seasons: []tvdb.SeasonStatus{
			{SeasonNum: 1, Episodes: []tvdb.EpisodeStatus{{EpisodeNum: 1, Downloaded: true}}},
		}}},
	}, nil)
	mockTVDB.On("GetExtendedBatch", mock.Anything, mock.Anything).
		Return(nil, errors.New("tvdb-proxy unavailable"))

	results, err := interactor.EnrichedSearch(context.Background(), "series", "x")

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.False(t, results[0].Media.Anime)
	assert.Equal(t, 1, results[0].Status.Downloaded)
}

func TestEnrichedSearch_StatusFailureReturnsPlainResults(t *testing.T) {
	interactor, searcher, mockTVDB, mockTorrenter := setupEnrichedSearchInteractor()

	searcher.On("Search", mock.Anything, "series", "x").
		Return([]tvdb.Media{series("5", episode(51, 1, 1, 1))}, nil)
	mockTorrenter.On("GetStatusBatch", mock.Anything, mock.Anything).
		Return(tvdb.StatusBatchResponse{}, errors.New("torrenter down"))

	results, err := interactor.EnrichedSearch(context.Background(), "series", "x")

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "series", results[0].Status.Type)
	assert.Nil(t, results[0].Status.Seasons)
	mockTVDB.AssertNotCalled(t, "GetExtendedBatch", mock.Anything, mock.Anything)
}

func TestEnrichedSearch_EmptySearchReturnsEmptySlice(t *testing.T) {
	interactor, searcher, _, mockTorrenter := setupEnrichedSearchInteractor()

	searcher.On("Search", mock.Anything, "series", "zzz").Return([]tvdb.Media{}, nil)

	results, err := interactor.EnrichedSearch(context.Background(), "series", "zzz")

	require.NoError(t, err)
	assert.NotNil(t, results)
	assert.Empty(t, results)
	mockTorrenter.AssertNotCalled(t, "GetStatusBatch", mock.Anything, mock.Anything)
}

func TestEnrichedSearch_SearchErrorIsReturned(t *testing.T) {
	interactor, searcher, _, _ := setupEnrichedSearchInteractor()

	searcher.On("Search", mock.Anything, "series", "x").Return(nil, errors.New("tvdb down"))

	_, err := interactor.EnrichedSearch(context.Background(), "series", "x")

	assert.Error(t, err)
}

// The searcher caches its results and hands the same slice to every caller.
func TestEnrichedSearch_DoesNotMutateSearcherResults(t *testing.T) {
	interactor, searcher, mockTVDB, mockTorrenter := setupEnrichedSearchInteractor()

	cached := []tvdb.Media{series("7", episode(71, 1, 1, 1))}
	searcher.On("Search", mock.Anything, "series", "x").Return(cached, nil)
	mockTorrenter.On("GetStatusBatch", mock.Anything, mock.Anything).Return(tvdb.StatusBatchResponse{
		Shows: []tvdb.ShowStatus{{TvdbId: "7", Seasons: []tvdb.SeasonStatus{
			{SeasonNum: 1, Episodes: []tvdb.EpisodeStatus{{EpisodeNum: 1, Downloaded: true}}},
		}}},
	}, nil)
	mockTVDB.On("GetExtendedBatch", mock.Anything, mock.Anything).
		Return([]tvdb.Media{{Id: "7", Anime: true}}, nil)

	results, err := interactor.EnrichedSearch(context.Background(), "series", "x")

	require.NoError(t, err)
	assert.True(t, results[0].Media.Anime)
	assert.False(t, cached[0].Anime, "the cached slice must be left untouched")
}

func libraryShowStatus(id string) tvdb.StatusBatchResponse {
	return tvdb.StatusBatchResponse{
		Shows: []tvdb.ShowStatus{{TvdbId: id, Seasons: []tvdb.SeasonStatus{
			{SeasonNum: 1, Episodes: []tvdb.EpisodeStatus{{EpisodeNum: 1, Downloaded: true}}},
		}}},
	}
}

// Searching for the same library show twice should cost one extended lookup;
// TVDB's genre for a show does not change between keystrokes.
func TestEnrichedSearch_RemembersAnimeFlagAcrossSearches(t *testing.T) {
	interactor, searcher, mockTVDB, mockTorrenter := setupEnrichedSearchInteractor()

	searcher.On("Search", mock.Anything, "series", mock.Anything).
		Return([]tvdb.Media{series("9", episode(91, 1, 1, 1))}, nil)
	mockTorrenter.On("GetStatusBatch", mock.Anything, mock.Anything).Return(libraryShowStatus("9"), nil)
	mockTVDB.On("GetExtendedBatch", mock.Anything, extendedRequestsFor("9")).
		Return([]tvdb.Media{{Id: "9", Anime: true}}, nil).Once()

	first, err := interactor.EnrichedSearch(context.Background(), "series", "one")
	require.NoError(t, err)
	second, err := interactor.EnrichedSearch(context.Background(), "series", "one p")
	require.NoError(t, err)

	assert.True(t, first[0].Media.Anime)
	assert.True(t, second[0].Media.Anime)
	mockTVDB.AssertNumberOfCalls(t, "GetExtendedBatch", 1)
}

func TestEnrichedSearch_NotAnimeIsRememberedToo(t *testing.T) {
	interactor, searcher, mockTVDB, mockTorrenter := setupEnrichedSearchInteractor()

	searcher.On("Search", mock.Anything, "series", mock.Anything).
		Return([]tvdb.Media{series("9", episode(91, 1, 1, 1))}, nil)
	mockTorrenter.On("GetStatusBatch", mock.Anything, mock.Anything).Return(libraryShowStatus("9"), nil)
	mockTVDB.On("GetExtendedBatch", mock.Anything, extendedRequestsFor("9")).
		Return([]tvdb.Media{{Id: "9", Anime: false}}, nil).Once()

	_, err := interactor.EnrichedSearch(context.Background(), "series", "one")
	require.NoError(t, err)
	second, err := interactor.EnrichedSearch(context.Background(), "series", "one p")
	require.NoError(t, err)

	assert.False(t, second[0].Media.Anime)
	mockTVDB.AssertNumberOfCalls(t, "GetExtendedBatch", 1)
}

func TestEnrichedSearch_AnimeFlagExpires(t *testing.T) {
	interactor, searcher, mockTVDB, mockTorrenter := setupEnrichedSearchInteractor()
	clock := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	interactor.now = func() time.Time { return clock }

	searcher.On("Search", mock.Anything, "series", mock.Anything).
		Return([]tvdb.Media{series("9", episode(91, 1, 1, 1))}, nil)
	mockTorrenter.On("GetStatusBatch", mock.Anything, mock.Anything).Return(libraryShowStatus("9"), nil)
	mockTVDB.On("GetExtendedBatch", mock.Anything, extendedRequestsFor("9")).
		Return([]tvdb.Media{{Id: "9", Anime: true}}, nil)

	_, err := interactor.EnrichedSearch(context.Background(), "series", "one")
	require.NoError(t, err)

	clock = clock.Add(animeFlagTTL + time.Minute)
	_, err = interactor.EnrichedSearch(context.Background(), "series", "one")
	require.NoError(t, err)

	mockTVDB.AssertNumberOfCalls(t, "GetExtendedBatch", 2)
}

// A show the extended batch dropped (e.g. a transient TVDB error for that id)
// must not be remembered as "not anime".
func TestEnrichedSearch_MissingExtendedRecordIsRetriedNextSearch(t *testing.T) {
	interactor, searcher, mockTVDB, mockTorrenter := setupEnrichedSearchInteractor()

	searcher.On("Search", mock.Anything, "series", mock.Anything).
		Return([]tvdb.Media{series("9", episode(91, 1, 1, 1))}, nil)
	mockTorrenter.On("GetStatusBatch", mock.Anything, mock.Anything).Return(libraryShowStatus("9"), nil)
	mockTVDB.On("GetExtendedBatch", mock.Anything, extendedRequestsFor("9")).
		Return([]tvdb.Media{}, nil).Once()
	mockTVDB.On("GetExtendedBatch", mock.Anything, extendedRequestsFor("9")).
		Return([]tvdb.Media{{Id: "9", Anime: true}}, nil).Once()

	first, err := interactor.EnrichedSearch(context.Background(), "series", "one")
	require.NoError(t, err)
	second, err := interactor.EnrichedSearch(context.Background(), "series", "one")
	require.NoError(t, err)

	assert.False(t, first[0].Media.Anime)
	assert.True(t, second[0].Media.Anime)
	mockTVDB.AssertNumberOfCalls(t, "GetExtendedBatch", 2)
}

// Movies arrive from tvdb-proxy with the anime flag already set from the
// search hit's genres; it must survive the merge without any extended lookup.
func TestEnrichedSearch_MovieAnimeFlagPassesThrough(t *testing.T) {
	interactor, searcher, mockTVDB, mockTorrenter := setupEnrichedSearchInteractor()

	ghibli := movie("276")
	ghibli.Anime = true
	searcher.On("Search", mock.Anything, "movie", "spirited").Return([]tvdb.Media{ghibli}, nil)
	mockTorrenter.On("GetStatusBatch", mock.Anything, mock.Anything).Return(tvdb.StatusBatchResponse{
		Movies: []tvdb.MovieStatus{{TvdbId: "276", InLibrary: true}},
	}, nil)

	results, err := interactor.EnrichedSearch(context.Background(), "movie", "spirited")

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].Media.Anime)
	assert.True(t, results[0].Status.InLibrary)
	mockTVDB.AssertNotCalled(t, "GetExtendedBatch", mock.Anything, mock.Anything)
}
