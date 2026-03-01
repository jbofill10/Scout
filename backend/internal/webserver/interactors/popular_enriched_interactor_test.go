package interactors

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"

	"github.com/jbofill10/scout/backend/internal/webserver/clients"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockTVDBProxyClient is a mock implementation of TVDBProxyClient
type MockTVDBProxyClient struct {
	mock.Mock
}

func (m *MockTVDBProxyClient) GetPopularShows(
	ctx context.Context, genre string, limit int,
) ([]tvdb.Media, error) {
	args := m.Called(ctx, genre, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]tvdb.Media), args.Error(1)
}

func (m *MockTVDBProxyClient) GetPopularMovies(
	ctx context.Context, genre string, limit int,
) ([]tvdb.Media, error) {
	args := m.Called(ctx, genre, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]tvdb.Media), args.Error(1)
}

func (m *MockTVDBProxyClient) GetEpisodesBatch(
	ctx context.Context, requests []clients.EpisodesRequest,
) ([]clients.EpisodesResponse, error) {
	args := m.Called(ctx, requests)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]clients.EpisodesResponse), args.Error(1)
}

func (m *MockTVDBProxyClient) GetExtendedBatch(
	ctx context.Context, requests []clients.ExtendedRequest,
) ([]tvdb.Media, error) {
	args := m.Called(ctx, requests)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]tvdb.Media), args.Error(1)
}

// MockTorrenterClient is a mock implementation of TorrenterClient
type MockTorrenterClient struct {
	mock.Mock
}

func (m *MockTorrenterClient) GetStatusBatch(
	ctx context.Context, requests []tvdb.StatusRequest,
) (tvdb.StatusBatchResponse, error) {
	args := m.Called(ctx, requests)
	return args.Get(0).(tvdb.StatusBatchResponse), args.Error(1)
}

func setupPopularEnrichedInteractor() (*PopularEnrichedInteractor, *MockTVDBProxyClient, *MockTorrenterClient) {
	mockTVDB := new(MockTVDBProxyClient)
	mockTorrenter := new(MockTorrenterClient)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	interactor := NewPopularEnrichedInteractor(mockTVDB, mockTorrenter, logger)
	return interactor, mockTVDB, mockTorrenter
}

func TestEnrichedPopularShows_HappyPath_EpisodeMetadataPreserved(t *testing.T) {
	interactor, mockTVDB, mockTorrenter := setupPopularEnrichedInteractor()
	ctx := context.Background()

	// Step 1: Mock popular shows response (basic info, no episodes)
	popularShows := []tvdb.Media{
		{
			Id:       "12345",
			Name:     "Test Show",
			Category: "series",
		},
	}
	mockTVDB.On("GetPopularShows", ctx, "Action", 1).Return(popularShows, nil)

	// Step 2: Mock episodes batch response
	episodesResp := []clients.EpisodesResponse{
		{
			Request: &clients.EpisodesRequest{SeriesId: "12345"},
			Data: &tvdb.TVDBSeriesMetadata{
				Episodes: []tvdb.Episode{
					{Id: 1, Name: "Episode 1", SeasonNumber: 1, Number: 1},
					{Id: 2, Name: "Episode 2", SeasonNumber: 1, Number: 2},
				},
			},
		},
	}
	mockTVDB.On("GetEpisodesBatch", ctx, mock.Anything).Return(episodesResp, nil)

	// Step 3: Mock extended batch response (no episodes, has overview)
	enrichedShows := []tvdb.Media{
		{
			Id:       "12345",
			Name:     "Test Show",
			Category: "series",
			Overview: "A test show overview",
			Metadata: tvdb.TVDBSeriesMetadata{Episodes: []tvdb.Episode{}}, // Empty
		},
	}
	mockTVDB.On("GetExtendedBatch", ctx, mock.Anything).Return(enrichedShows, nil)

	// Step 4: Mock status batch response
	statusResp := tvdb.StatusBatchResponse{
		Shows: []tvdb.ShowStatus{
			{
				TvdbId:  "12345",
				Seasons: []tvdb.SeasonStatus{},
			},
		},
	}
	mockTorrenter.On("GetStatusBatch", ctx, mock.Anything).Return(statusResp, nil)

	// Execute
	result, err := interactor.EnrichedPopularShows(ctx, "Action", 1)

	// Assert
	assert.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, "12345", result[0].Media.Id)
	assert.Equal(t, "Test Show", result[0].Media.Name)
	assert.Equal(t, "A test show overview", result[0].Media.Overview)

	// CRITICAL: Verify episode metadata from Step 2 is preserved after Step 3
	assert.Len(t, result[0].Media.Metadata.Episodes, 2, "Episode metadata should be preserved")
	assert.Equal(t, "Episode 1", result[0].Media.Metadata.Episodes[0].Name)
	assert.Equal(t, "Episode 2", result[0].Media.Metadata.Episodes[1].Name)

	// Verify status is merged
	assert.Equal(t, "series", result[0].Status.Type)
	assert.Len(t, result[0].Status.Seasons, 0)

	mockTVDB.AssertExpectations(t)
	mockTorrenter.AssertExpectations(t)
}

func TestEnrichedPopularShows_EpisodeBatchFailure_ContinuesWithoutEpisodes(t *testing.T) {
	interactor, mockTVDB, mockTorrenter := setupPopularEnrichedInteractor()
	ctx := context.Background()

	// Step 1: Mock popular shows
	popularShows := []tvdb.Media{
		{Id: "12345", Name: "Test Show", Category: "series"},
	}
	mockTVDB.On("GetPopularShows", ctx, "Action", 1).Return(popularShows, nil)

	// Step 2: Episode batch fails
	mockTVDB.On("GetEpisodesBatch", ctx, mock.Anything).
		Return(nil, errors.New("episodes batch failed"))

	// Step 3: Extended batch succeeds (no episodes)
	enrichedShows := []tvdb.Media{
		{
			Id:       "12345",
			Name:     "Test Show",
			Category: "series",
			Overview: "Overview",
			Metadata: tvdb.TVDBSeriesMetadata{Episodes: []tvdb.Episode{}},
		},
	}
	mockTVDB.On("GetExtendedBatch", ctx, mock.Anything).Return(enrichedShows, nil)

	// Step 4: Status succeeds
	statusResp := tvdb.StatusBatchResponse{Shows: []tvdb.ShowStatus{}}
	mockTorrenter.On("GetStatusBatch", ctx, mock.Anything).Return(statusResp, nil)

	// Execute
	result, err := interactor.EnrichedPopularShows(ctx, "Action", 1)

	// Assert - should continue without episodes
	assert.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Len(t, result[0].Media.Metadata.Episodes, 0, "Should have no episodes due to batch failure")

	mockTVDB.AssertExpectations(t)
	mockTorrenter.AssertExpectations(t)
}

func TestEnrichedPopularShows_ExtendedBatchFailure_FallsBackToPopularShows(t *testing.T) {
	interactor, mockTVDB, mockTorrenter := setupPopularEnrichedInteractor()
	ctx := context.Background()

	// Step 1: Mock popular shows
	popularShows := []tvdb.Media{
		{Id: "12345", Name: "Test Show", Category: "series"},
	}
	mockTVDB.On("GetPopularShows", ctx, "Action", 1).Return(popularShows, nil)

	// Step 2: Episodes succeed
	episodesResp := []clients.EpisodesResponse{
		{
			Request: &clients.EpisodesRequest{SeriesId: "12345"},
			Data: &tvdb.TVDBSeriesMetadata{
				Episodes: []tvdb.Episode{
					{Id: 1, Name: "Episode 1"},
				},
			},
		},
	}
	mockTVDB.On("GetEpisodesBatch", ctx, mock.Anything).Return(episodesResp, nil)

	// Step 3: Extended batch FAILS
	mockTVDB.On("GetExtendedBatch", ctx, mock.Anything).
		Return(nil, errors.New("extended batch failed"))

	// Step 4: Status succeeds
	statusResp := tvdb.StatusBatchResponse{Shows: []tvdb.ShowStatus{}}
	mockTorrenter.On("GetStatusBatch", ctx, mock.Anything).Return(statusResp, nil)

	// Execute
	result, err := interactor.EnrichedPopularShows(ctx, "Action", 1)

	// Assert - should fallback to popularShows with episode metadata intact
	assert.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Len(t, result[0].Media.Metadata.Episodes, 1, "Should preserve episodes from Step 2")
	assert.Equal(t, "Episode 1", result[0].Media.Metadata.Episodes[0].Name)

	mockTVDB.AssertExpectations(t)
	mockTorrenter.AssertExpectations(t)
}

func TestEnrichedPopularShows_StatusBatchFailure_ReturnsMinimalStatus(t *testing.T) {
	interactor, mockTVDB, mockTorrenter := setupPopularEnrichedInteractor()
	ctx := context.Background()

	// Mock all steps succeeding except status
	popularShows := []tvdb.Media{
		{Id: "12345", Name: "Test Show", Category: "series"},
	}
	mockTVDB.On("GetPopularShows", ctx, "Action", 1).Return(popularShows, nil)

	episodesResp := []clients.EpisodesResponse{
		{Request: &clients.EpisodesRequest{SeriesId: "12345"}, Data: &tvdb.TVDBSeriesMetadata{}},
	}
	mockTVDB.On("GetEpisodesBatch", ctx, mock.Anything).Return(episodesResp, nil)

	enrichedShows := []tvdb.Media{{Id: "12345", Name: "Test Show", Category: "series"}}
	mockTVDB.On("GetExtendedBatch", ctx, mock.Anything).Return(enrichedShows, nil)

	// Status FAILS
	mockTorrenter.On("GetStatusBatch", ctx, mock.Anything).
		Return(tvdb.StatusBatchResponse{}, errors.New("status batch failed"))

	// Execute
	result, err := interactor.EnrichedPopularShows(ctx, "Action", 1)

	// Assert - should return with minimal status
	assert.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, "series", result[0].Status.Type)
	assert.Nil(t, result[0].Status.Seasons)

	mockTVDB.AssertExpectations(t)
	mockTorrenter.AssertExpectations(t)
}

func TestEnrichedPopularShows_EmptyResults(t *testing.T) {
	interactor, mockTVDB, _ := setupPopularEnrichedInteractor()
	ctx := context.Background()

	// Mock empty popular shows
	mockTVDB.On("GetPopularShows", ctx, "", 10).Return([]tvdb.Media{}, nil)

	// Execute
	result, err := interactor.EnrichedPopularShows(ctx, "", 10)

	// Assert
	assert.NoError(t, err)
	assert.Empty(t, result)

	mockTVDB.AssertExpectations(t)
}

func TestEnrichedPopularShows_PopularShowsFailure_ReturnsError(t *testing.T) {
	interactor, mockTVDB, _ := setupPopularEnrichedInteractor()
	ctx := context.Background()

	// Step 1 fails
	mockTVDB.On("GetPopularShows", ctx, "Action", 1).
		Return(nil, errors.New("popular shows failed"))

	// Execute
	result, err := interactor.EnrichedPopularShows(ctx, "Action", 1)

	// Assert - should fail immediately
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "popular shows failed")

	mockTVDB.AssertExpectations(t)
}

func TestEnrichedPopularMovies_HappyPath(t *testing.T) {
	interactor, mockTVDB, mockTorrenter := setupPopularEnrichedInteractor()
	ctx := context.Background()

	// Step 1: Mock popular movies
	popularMovies := []tvdb.Media{
		{Id: "67890", Name: "Test Movie", Category: "movie"},
	}
	mockTVDB.On("GetPopularMovies", ctx, "Action", 1).Return(popularMovies, nil)

	// Step 2: Extended batch (movies don't need episode batch)
	enrichedMovies := []tvdb.Media{
		{
			Id:       "67890",
			Name:     "Test Movie",
			Category: "movie",
			Overview: "Movie overview",
		},
	}
	mockTVDB.On("GetExtendedBatch", ctx, mock.Anything).Return(enrichedMovies, nil)

	// Step 3: Status batch
	statusResp := tvdb.StatusBatchResponse{
		Movies: []tvdb.MovieStatus{
			{TvdbId: "67890", InLibrary: true},
		},
	}
	mockTorrenter.On("GetStatusBatch", ctx, mock.Anything).Return(statusResp, nil)

	// Execute
	result, err := interactor.EnrichedPopularMovies(ctx, "Action", 1)

	// Assert
	assert.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, "67890", result[0].Media.Id)
	assert.Equal(t, "Test Movie", result[0].Media.Name)
	assert.Equal(t, "Movie overview", result[0].Media.Overview)
	assert.Equal(t, "movie", result[0].Status.Type)
	assert.True(t, result[0].Status.InLibrary)

	mockTVDB.AssertExpectations(t)
	mockTorrenter.AssertExpectations(t)
}

func TestMergeStatusWithMedia_CalculatesEpisodeCounts_NoMetadata(t *testing.T) {
	interactor, _, _ := setupPopularEnrichedInteractor()
	ctx := context.Background()

	// No TVDB metadata - falls back to DB-only counting
	mediaList := []tvdb.Media{
		{Id: "12345", Category: "series"},
	}

	statusResponse := tvdb.StatusBatchResponse{
		Shows: []tvdb.ShowStatus{
			{
				TvdbId: "12345",
				Seasons: []tvdb.SeasonStatus{
					{
						SeasonNum: 1,
						Episodes: []tvdb.EpisodeStatus{
							{EpisodeNum: 1, Downloaded: true},
							{EpisodeNum: 2, Downloaded: false},
							{EpisodeNum: 3, Downloaded: true},
						},
					},
				},
			},
		},
	}

	// Execute
	result := interactor.mergeStatusWithMedia(ctx, mediaList, statusResponse)

	// Assert - fallback to DB counting
	assert.Len(t, result, 1)
	assert.Equal(t, 2, result[0].Status.Downloaded, "Should count 2 downloaded episodes")
	assert.Equal(t, 3, result[0].Status.Total, "Should count 3 total episodes")
	assert.Len(t, result[0].Status.Seasons, 1)
}

func TestMergeStatusWithMedia_CalculatesEpisodeCounts_AnimeAbsoluteNumbering(t *testing.T) {
	interactor, _, _ := setupPopularEnrichedInteractor()
	ctx := context.Background()

	// Anime show: TVDB has standard seasonal numbering with AbsoluteNumber set
	// Plex stores everything under Season 1 with absolute episode numbers
	mediaList := []tvdb.Media{
		{
			Id:       "99999",
			Category: "series",
			Anime:    true,
			Metadata: tvdb.TVDBSeriesMetadata{
				Episodes: []tvdb.Episode{
					{Id: 500, Name: "Ep 1", SeasonNumber: 1, Number: 1, AbsoluteNumber: 1},
					{Id: 501, Name: "Ep 2", SeasonNumber: 1, Number: 2, AbsoluteNumber: 2},
					{Id: 600, Name: "S2 Ep 1", SeasonNumber: 2, Number: 1, AbsoluteNumber: 25},
					{Id: 601, Name: "S2 Ep 2", SeasonNumber: 2, Number: 2, AbsoluteNumber: 26},
				},
			},
		},
	}

	// Plex status: all episodes under Season 1 with absolute numbering
	statusResponse := tvdb.StatusBatchResponse{
		Shows: []tvdb.ShowStatus{
			{
				TvdbId: "99999",
				Seasons: []tvdb.SeasonStatus{
					{
						SeasonNum: 1,
						Episodes: []tvdb.EpisodeStatus{
							{EpisodeNum: 1, Downloaded: true},
							{EpisodeNum: 2, Downloaded: true},
							{EpisodeNum: 25, Downloaded: true},  // S02E01 in TVDB
							{EpisodeNum: 26, Downloaded: false}, // S02E02 not downloaded
						},
					},
				},
			},
		},
	}

	result := interactor.mergeStatusWithMedia(ctx, mediaList, statusResponse)

	// S1E1 matches by key "1-1", S1E2 matches by key "1-2",
	// S2E1 matches by absolute number 25, S2E2 not downloaded
	assert.Len(t, result, 1)
	assert.Equal(t, 3, result[0].Status.Downloaded, "Should match 3 episodes (2 by key + 1 by absolute number)")
	assert.Equal(t, 4, result[0].Status.Total, "Should count 4 total episodes from TVDB metadata")
}

func TestMergeStatusWithMedia_CalculatesEpisodeCounts_NonAnimeSkipsAbsolute(t *testing.T) {
	interactor, _, _ := setupPopularEnrichedInteractor()
	ctx := context.Background()

	// Non-anime show: even with AbsoluteNumber set and Season 1 only status,
	// absolute matching should NOT activate
	mediaList := []tvdb.Media{
		{
			Id:       "88888",
			Category: "series",
			Anime:    false,
			Metadata: tvdb.TVDBSeriesMetadata{
				Episodes: []tvdb.Episode{
					{Id: 700, Name: "Ep 1", SeasonNumber: 1, Number: 1, AbsoluteNumber: 1},
					{Id: 800, Name: "S2 Ep 1", SeasonNumber: 2, Number: 1, AbsoluteNumber: 13},
				},
			},
		},
	}

	// Status has Season 1 only with episode 13
	statusResponse := tvdb.StatusBatchResponse{
		Shows: []tvdb.ShowStatus{
			{
				TvdbId: "88888",
				Seasons: []tvdb.SeasonStatus{
					{
						SeasonNum: 1,
						Episodes: []tvdb.EpisodeStatus{
							{EpisodeNum: 1, Downloaded: true},
							{EpisodeNum: 13, Downloaded: true},
						},
					},
				},
			},
		},
	}

	result := interactor.mergeStatusWithMedia(ctx, mediaList, statusResponse)

	// Only S1E1 matches by key "1-1". S2E1 should NOT match via absolute (non-anime).
	assert.Len(t, result, 1)
	assert.Equal(t, 1, result[0].Status.Downloaded, "Non-anime should not use absolute matching")
	assert.Equal(t, 2, result[0].Status.Total)
}

func TestMergeStatusWithMedia_CalculatesEpisodeCounts_WithMetadata(t *testing.T) {
	interactor, _, _ := setupPopularEnrichedInteractor()
	ctx := context.Background()

	// With TVDB metadata - uses metadata for total, matches by TVDB ID
	mediaList := []tvdb.Media{
		{
			Id:       "12345",
			Category: "series",
			Metadata: tvdb.TVDBSeriesMetadata{
				Episodes: []tvdb.Episode{
					{Id: 100, Name: "Ep 1", SeasonNumber: 1, Number: 1},
					{Id: 101, Name: "Ep 2", SeasonNumber: 1, Number: 2},
					{Id: 102, Name: "Ep 3", SeasonNumber: 1, Number: 3},
					{Id: 200, Name: "Ep 1 S2", SeasonNumber: 2, Number: 1},
				},
			},
		},
	}

	statusResponse := tvdb.StatusBatchResponse{
		Shows: []tvdb.ShowStatus{
			{
				TvdbId: "12345",
				Seasons: []tvdb.SeasonStatus{
					{
						SeasonNum: 1,
						Episodes: []tvdb.EpisodeStatus{
							{EpisodeNum: 1, Downloaded: true, TvdbId: "100"},
							{EpisodeNum: 2, Downloaded: true, TvdbId: "101"},
						},
					},
				},
			},
		},
	}

	// Execute
	result := interactor.mergeStatusWithMedia(ctx, mediaList, statusResponse)

	// Assert - total from TVDB metadata (4 eps), downloaded matched by TVDB ID (2 eps)
	assert.Len(t, result, 1)
	assert.Equal(t, 2, result[0].Status.Downloaded, "Should count 2 downloaded episodes matched by TVDB ID")
	assert.Equal(t, 4, result[0].Status.Total, "Should count 4 total episodes from TVDB metadata")
}
