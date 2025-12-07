package repository

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/suite"
)

type SchedulerRepoTestSuite struct {
	suite.Suite
	repo   *SchedulerRepo
	db     *sql.DB
	mock   sqlmock.Sqlmock
	logger *slog.Logger
}

func TestSchedulerRepoSuite(t *testing.T) {
	suite.Run(t, new(SchedulerRepoTestSuite))
}

func (s *SchedulerRepoTestSuite) SetupTest() {
	var err error
	s.db, s.mock, err = sqlmock.New()
	s.Require().NoError(err)

	buf := new(bytes.Buffer)
	s.logger = slog.New(slog.NewTextHandler(buf, nil))

	s.repo = &SchedulerRepo{
		db:     s.db,
		logger: s.logger,
	}
}

func (s *SchedulerRepoTestSuite) TearDownTest() {
	s.db.Close()
}

func (s *SchedulerRepoTestSuite) TestSchedule_Success() {
	media := tvdb.Media{
		Id:   "123",
		Name: "Test Show",
		Metadata: tvdb.TVDBSeriesMetadata{
			Episodes: []tvdb.Episode{
				{SeasonNumber: 1, Number: 1},
			},
		},
	}

	releaseTime := time.Now().AddDate(0, 0, 7)

	mediaJSON, _ := json.Marshal(media)

	s.mock.ExpectQuery(`INSERT INTO ScheduledDownloads`).
		WithArgs(mediaJSON, releaseTime.Format(time.RFC3339), StatusPending, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	err := s.repo.Schedule(context.Background(), media, releaseTime, "test-trace-id", "test-span-id")

	s.NoError(err)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *SchedulerRepoTestSuite) TestSchedule_DuplicateContent() {
	media := tvdb.Media{
		Id:   "123",
		Name: "Test Show",
	}

	releaseTime := time.Now().AddDate(0, 0, 7)

	mediaJSON, _ := json.Marshal(media)

	// ON CONFLICT DO NOTHING returns no rows
	s.mock.ExpectQuery(`INSERT INTO ScheduledDownloads`).
		WithArgs(mediaJSON, releaseTime.Format(time.RFC3339), StatusPending, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnError(sql.ErrNoRows)

	err := s.repo.Schedule(context.Background(), media, releaseTime, "test-trace-id", "test-span-id")

	s.Equal(ErrDuplicateScheduled, err)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *SchedulerRepoTestSuite) TestSchedule_DatabaseError() {
	media := tvdb.Media{
		Id:   "123",
		Name: "Test Show",
	}

	releaseTime := time.Now().AddDate(0, 0, 7)

	mediaJSON, _ := json.Marshal(media)

	s.mock.ExpectQuery(`INSERT INTO ScheduledDownloads`).
		WithArgs(mediaJSON, releaseTime.Format(time.RFC3339), StatusPending, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnError(sql.ErrConnDone)

	err := s.repo.Schedule(context.Background(), media, releaseTime, "test-trace-id", "test-span-id")

	s.Error(err)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *SchedulerRepoTestSuite) TestGetDueMedia_Success() {
	media := tvdb.Media{
		Id:   "123",
		Name: "Test Show",
		Metadata: tvdb.TVDBSeriesMetadata{
			Episodes: []tvdb.Episode{
				{SeasonNumber: 1, Number: 1},
			},
		},
	}

	mediaJSON, _ := json.Marshal(media)
	windowEnd := time.Now().Add(24 * time.Hour)

	rows := sqlmock.NewRows([]string{"id", "media"}).
		AddRow(1, mediaJSON)

	s.mock.ExpectQuery(`SELECT id, media FROM ScheduledDownloads`).
		WithArgs(StatusPending, sqlmock.AnyArg()).
		WillReturnRows(rows)

	s.mock.ExpectExec(`UPDATE ScheduledDownloads SET schedule_status`).
		WithArgs(StatusQueued, 1).
		WillReturnResult(sqlmock.NewResult(0, 1))

	results, err := s.repo.GetDueMedia(context.Background(), windowEnd)

	s.NoError(err)
	s.Len(results, 1)
	s.Equal("Test Show", results[0].Name)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *SchedulerRepoTestSuite) TestGetDueMedia_EmptyResults() {
	windowEnd := time.Now().Add(24 * time.Hour)

	rows := sqlmock.NewRows([]string{"id", "media"})

	s.mock.ExpectQuery(`SELECT id, media FROM ScheduledDownloads`).
		WithArgs(StatusPending, sqlmock.AnyArg()).
		WillReturnRows(rows)

	results, err := s.repo.GetDueMedia(context.Background(), windowEnd)

	s.NoError(err)
	s.Empty(results)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *SchedulerRepoTestSuite) TestGetDueMedia_QueryError() {
	windowEnd := time.Now().Add(24 * time.Hour)

	s.mock.ExpectQuery(`SELECT id, media FROM ScheduledDownloads`).
		WithArgs(StatusPending, sqlmock.AnyArg()).
		WillReturnError(sql.ErrConnDone)

	results, err := s.repo.GetDueMedia(context.Background(), windowEnd)

	s.Error(err)
	s.Nil(results)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *SchedulerRepoTestSuite) TestGetDueMedia_InvalidJSON() {
	windowEnd := time.Now().Add(24 * time.Hour)

	rows := sqlmock.NewRows([]string{"id", "media"}).
		AddRow(1, []byte("invalid json"))

	s.mock.ExpectQuery(`SELECT id, media FROM ScheduledDownloads`).
		WithArgs(StatusPending, sqlmock.AnyArg()).
		WillReturnRows(rows)

	results, err := s.repo.GetDueMedia(context.Background(), windowEnd)

	s.NoError(err)
	s.Empty(results) // Invalid JSON should be skipped
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *SchedulerRepoTestSuite) TestGetDueMedia_MultipleResults() {
	media1 := tvdb.Media{Id: "123", Name: "Show 1"}
	media2 := tvdb.Media{Id: "456", Name: "Show 2"}

	mediaJSON1, _ := json.Marshal(media1)
	mediaJSON2, _ := json.Marshal(media2)
	windowEnd := time.Now().Add(24 * time.Hour)

	rows := sqlmock.NewRows([]string{"id", "media"}).
		AddRow(1, mediaJSON1).
		AddRow(2, mediaJSON2)

	s.mock.ExpectQuery(`SELECT id, media FROM ScheduledDownloads`).
		WithArgs(StatusPending, sqlmock.AnyArg()).
		WillReturnRows(rows)

	s.mock.ExpectExec(`UPDATE ScheduledDownloads SET schedule_status`).
		WithArgs(StatusQueued, 1).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.mock.ExpectExec(`UPDATE ScheduledDownloads SET schedule_status`).
		WithArgs(StatusQueued, 2).
		WillReturnResult(sqlmock.NewResult(0, 1))

	results, err := s.repo.GetDueMedia(context.Background(), windowEnd)

	s.NoError(err)
	s.Len(results, 2)
	s.Equal("Show 1", results[0].Name)
	s.Equal("Show 2", results[1].Name)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *SchedulerRepoTestSuite) TestInsertDownloadHistory_Success() {
	s.mock.ExpectExec(`INSERT INTO ShowDownloadHistory`).
		WithArgs("Test Show", 1, 2, 0, "searching", "", "test-trace-id", "test-span-id").
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := s.repo.InsertDownloadHistory("Test Show", 1, 2, 0, "searching", "", "test-trace-id", "test-span-id")

	s.NoError(err)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *SchedulerRepoTestSuite) TestInsertDownloadHistory_WithReason() {
	s.mock.ExpectExec(`INSERT INTO ShowDownloadHistory`).
		WithArgs("Test Show", 1, 2, 3, "failure", "Not found", "test-trace-id", "test-span-id").
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := s.repo.InsertDownloadHistory("Test Show", 1, 2, 3, "failure", "Not found", "test-trace-id", "test-span-id")

	s.NoError(err)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *SchedulerRepoTestSuite) TestInsertDownloadHistory_DatabaseError() {
	s.mock.ExpectExec(`INSERT INTO ShowDownloadHistory`).
		WithArgs("Test Show", 1, 2, 0, "searching", "", "test-trace-id", "test-span-id").
		WillReturnError(sql.ErrConnDone)

	err := s.repo.InsertDownloadHistory("Test Show", 1, 2, 0, "searching", "", "test-trace-id", "test-span-id")

	s.Error(err)
	s.NoError(s.mock.ExpectationsWereMet())
}
