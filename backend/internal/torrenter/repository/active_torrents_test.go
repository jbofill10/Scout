package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"

	"github.com/DATA-DOG/go-sqlmock"
)

func activeTorrentFixture() *models.ActiveTorrent {
	return &models.ActiveTorrent{
		InfoHash:     "abc123",
		TrackingUUID: "uuid-1",
		TorrentTitle: "Some.Show.S01E02-GRP",
		StartedAt:    time.Date(2026, 7, 26, 20, 0, 0, 0, time.UTC),
		Strategy: &models.SearchStrategy{
			MediaName:     "Some Show",
			Season:        1,
			Episode:       2,
			TvdbId:        "12345",
			EpisodeTvdbID: "999",
			EpisodeMeta:   &tvdb.Episode{SeasonNumber: 1, Number: 2, AbsoluteNumber: 14},
		},
	}
}

func (s *RepoTestSuite) TestInsertActiveTorrent_Success() {
	at := activeTorrentFixture()

	strategy, err := json.Marshal(at.Strategy)
	s.Require().NoError(err)

	s.mock.ExpectExec(`INSERT INTO ActiveTorrents`).
		WithArgs(at.InfoHash, at.TrackingUUID, at.TorrentTitle, strategy, at.StartedAt, "", "").
		WillReturnResult(sqlmock.NewResult(1, 1))

	s.NoError(s.repo.InsertActiveTorrent(context.Background(), at))
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *RepoTestSuite) TestInsertActiveTorrent_Error() {
	s.mock.ExpectExec(`INSERT INTO ActiveTorrents`).WillReturnError(errors.New("db down"))

	err := s.repo.InsertActiveTorrent(context.Background(), activeTorrentFixture())

	s.Error(err)
	s.Contains(err.Error(), "failed to insert active torrent")
}

func (s *RepoTestSuite) TestDeleteActiveTorrent_Success() {
	s.mock.ExpectExec(`DELETE FROM ActiveTorrents`).
		WithArgs("abc123").
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.NoError(s.repo.DeleteActiveTorrent(context.Background(), "abc123"))
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *RepoTestSuite) TestDeleteActiveTorrent_Error() {
	s.mock.ExpectExec(`DELETE FROM ActiveTorrents`).WillReturnError(errors.New("db down"))

	err := s.repo.DeleteActiveTorrent(context.Background(), "abc123")

	s.Error(err)
	s.Contains(err.Error(), "failed to delete active torrent")
}

// The strategy has to survive the JSONB round trip intact — it is the only copy
// of what the download was for, and it cannot be re-derived after a restart.
func (s *RepoTestSuite) TestGetActiveTorrents_RoundTripsStrategy() {
	at := activeTorrentFixture()
	strategy, err := json.Marshal(at.Strategy)
	s.Require().NoError(err)

	rows := sqlmock.NewRows([]string{"info_hash", "tracking_uuid", "torrent_title", "strategy", "started_at"}).
		AddRow(at.InfoHash, at.TrackingUUID, at.TorrentTitle, strategy, at.StartedAt)
	s.mock.ExpectQuery(`SELECT info_hash, tracking_uuid, torrent_title, strategy, started_at`).WillReturnRows(rows)

	active, err := s.repo.GetActiveTorrents(context.Background())

	s.NoError(err)
	s.Require().Len(active, 1)
	s.Equal(at.InfoHash, active[0].InfoHash)
	s.Equal(at.TrackingUUID, active[0].TrackingUUID)
	s.Equal(at.TorrentTitle, active[0].TorrentTitle)
	s.True(at.StartedAt.Equal(active[0].StartedAt))
	s.Require().NotNil(active[0].Strategy)
	s.Equal("Some Show", active[0].Strategy.MediaName)
	s.Equal("999", active[0].Strategy.EpisodeTvdbID)
	s.Require().NotNil(active[0].Strategy.EpisodeMeta)
	s.Equal(14, active[0].Strategy.EpisodeMeta.AbsoluteNumber)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *RepoTestSuite) TestGetActiveTorrents_Empty() {
	rows := sqlmock.NewRows([]string{"info_hash", "tracking_uuid", "torrent_title", "strategy", "started_at"})
	s.mock.ExpectQuery(`SELECT info_hash, tracking_uuid, torrent_title, strategy, started_at`).WillReturnRows(rows)

	active, err := s.repo.GetActiveTorrents(context.Background())

	s.NoError(err)
	s.Empty(active)
}

// A row whose strategy cannot be parsed can never be resumed, so it is dropped
// rather than left to wedge the startup sweep on every boot.
func (s *RepoTestSuite) TestGetActiveTorrents_DiscardsUnreadableStrategy() {
	rows := sqlmock.NewRows([]string{"info_hash", "tracking_uuid", "torrent_title", "strategy", "started_at"}).
		AddRow("bad-hash", "uuid-1", "Broken", []byte("{not json"), time.Now())
	s.mock.ExpectQuery(`SELECT info_hash, tracking_uuid, torrent_title, strategy, started_at`).WillReturnRows(rows)
	s.mock.ExpectExec(`DELETE FROM ActiveTorrents`).
		WithArgs("bad-hash").
		WillReturnResult(sqlmock.NewResult(0, 1))

	active, err := s.repo.GetActiveTorrents(context.Background())

	s.NoError(err)
	s.Empty(active)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *RepoTestSuite) TestGetActiveTorrents_QueryError() {
	s.mock.ExpectQuery(`SELECT info_hash, tracking_uuid, torrent_title, strategy, started_at`).
		WillReturnError(errors.New("db down"))

	active, err := s.repo.GetActiveTorrents(context.Background())

	s.Error(err)
	s.Nil(active)
}
