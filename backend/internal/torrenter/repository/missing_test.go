package repository

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetMissingEpisodes_GroupsByShow(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := &Repo{db: db, logger: slog.New(slog.NewTextHandler(new(bytes.Buffer), nil))}

	cols := []string{"show_id", "title", "thumb", "ep_id", "season", "episode", "absolute", "name", "aired"}
	mock.ExpectQuery(`FROM Shows sh`).WillReturnRows(sqlmock.NewRows(cols).
		AddRow("900", "Some Show", "/thumb", "55", 1, 5, 0, "Five", "2026-01-01").
		AddRow("900", "Some Show", "/thumb", "56", 1, 6, 0, "Six", "2026-01-08").
		AddRow("901", "Other Show", "", "77", 2, 1, 14, "One", "2025-12-01"))

	shows, err := repo.GetMissingEpisodes(context.Background())

	require.NoError(t, err)
	require.Len(t, shows, 2)
	assert.Equal(t, "900", shows[0].TvdbId)
	assert.Equal(t, "Some Show", shows[0].Title)
	assert.Len(t, shows[0].Episodes, 2)
	assert.Equal(t, "56", shows[0].Episodes[1].TvdbId)
	assert.Equal(t, 6, shows[0].Episodes[1].EpisodeNumber)
	assert.False(t, shows[0].Episodes[1].Requested, "the torrenter does not know about requests")
	assert.Equal(t, 14, shows[1].Episodes[0].AbsoluteNumber)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetMissingEpisodes_EmptyIsNotNil(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := &Repo{db: db, logger: slog.New(slog.NewTextHandler(new(bytes.Buffer), nil))}
	mock.ExpectQuery(`FROM Shows sh`).WillReturnRows(sqlmock.NewRows([]string{"a"}))

	shows, err := repo.GetMissingEpisodes(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, shows)
	assert.Empty(t, shows)
}
