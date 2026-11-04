package interactors

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	servicemocks "github.com/jbofill10/scout/backend/internal/torrenter/service/mocks"
	"github.com/jbofill10/scout/backend/pkg/library"
)

func newLibraryInteractorForTest(t *testing.T) (*LibraryInteractor, *servicemocks.Repository) {
	t.Helper()
	repo := servicemocks.NewRepository(t)
	return NewLibraryInteractor(repo, slog.New(slog.NewTextHandler(new(bytes.Buffer), nil))), repo
}

func TestGetShowMetadataStatus_CountsAiredAndUpcomingSeparately(t *testing.T) {
	i, repo := newLibraryInteractorForTest(t)
	// Dexter: Resurrection mid-run: 11 aired and in Plex, 9 announced
	repo.On("GetTvdbMetadataStatus", mock.Anything, "900").Return(11, 11, 9, nil)

	status, err := i.GetShowMetadataStatus(context.Background(), "900")

	require.NoError(t, err)
	assert.Equal(t, library.MetadataStatus{
		HasTvdbData:      true,
		TvdbEpisodeCount: 11,
		PlexEpisodeCount: 11,
		MissingCount:     0,
		UpcomingCount:    9,
	}, status, "upcoming episodes are neither missing nor part of the total")
}

func TestGetShowMetadataStatus_AnnouncedOnlyStillHasTvdbData(t *testing.T) {
	i, repo := newLibraryInteractorForTest(t)
	repo.On("GetTvdbMetadataStatus", mock.Anything, "901").Return(0, 0, 10, nil)

	status, err := i.GetShowMetadataStatus(context.Background(), "901")

	require.NoError(t, err)
	assert.True(t, status.HasTvdbData, "TVDB knows the show even though nothing has aired")
	assert.Equal(t, 0, status.MissingCount)
	assert.Equal(t, 10, status.UpcomingCount)
}

func TestGetShowMetadataStatus_RepositoryError(t *testing.T) {
	i, repo := newLibraryInteractorForTest(t)
	repo.On("GetTvdbMetadataStatus", mock.Anything, "902").Return(0, 0, 0, errors.New("db down"))

	_, err := i.GetShowMetadataStatus(context.Background(), "902")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "db down")
}
