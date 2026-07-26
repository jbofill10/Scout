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
	release := time.Now().Add(-1 * time.Hour)

	rows := sqlmock.NewRows([]string{"id", "media", "release_time", "attempts", "due_at"}).
		AddRow(1, mediaJSON, release, 2, release)

	s.mock.ExpectQuery(`SELECT id, media, release_time, attempts, COALESCE`).
		WithArgs(StatusPending, sqlmock.AnyArg()).
		WillReturnRows(rows)

	results, err := s.repo.GetDueMedia(context.Background(), windowEnd)

	s.NoError(err)
	s.Len(results, 1)
	s.Equal("Test Show", results[0].Media.Name)
	s.Equal(1, results[0].ID)
	s.Equal(2, results[0].Attempts)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *SchedulerRepoTestSuite) TestGetDueMedia_EmptyResults() {
	windowEnd := time.Now().Add(24 * time.Hour)

	rows := sqlmock.NewRows([]string{"id", "media", "release_time", "attempts", "due_at"})

	s.mock.ExpectQuery(`SELECT id, media, release_time, attempts, COALESCE`).
		WithArgs(StatusPending, sqlmock.AnyArg()).
		WillReturnRows(rows)

	results, err := s.repo.GetDueMedia(context.Background(), windowEnd)

	s.NoError(err)
	s.Empty(results)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *SchedulerRepoTestSuite) TestGetDueMedia_QueryError() {
	windowEnd := time.Now().Add(24 * time.Hour)

	s.mock.ExpectQuery(`SELECT id, media, release_time, attempts, COALESCE`).
		WithArgs(StatusPending, sqlmock.AnyArg()).
		WillReturnError(sql.ErrConnDone)

	results, err := s.repo.GetDueMedia(context.Background(), windowEnd)

	s.Error(err)
	s.Nil(results)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *SchedulerRepoTestSuite) TestGetDueMedia_InvalidJSON() {
	windowEnd := time.Now().Add(24 * time.Hour)
	release := time.Now()

	rows := sqlmock.NewRows([]string{"id", "media", "release_time", "attempts", "due_at"}).
		AddRow(1, []byte("invalid json"), release, 0, release)

	s.mock.ExpectQuery(`SELECT id, media, release_time, attempts, COALESCE`).
		WithArgs(StatusPending, sqlmock.AnyArg()).
		WillReturnRows(rows)

	results, err := s.repo.GetDueMedia(context.Background(), windowEnd)

	s.NoError(err)
	s.Empty(results) // Invalid JSON should be skipped
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *SchedulerRepoTestSuite) TestMarkQueued() {
	s.mock.ExpectExec(`UPDATE ScheduledDownloads SET schedule_status = \$1, queued_at = now\(\) WHERE id = \$2`).
		WithArgs(StatusQueued, 7).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := s.repo.MarkQueued(context.Background(), 7)
	s.NoError(err)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *SchedulerRepoTestSuite) TestMarkCompleted() {
	s.mock.ExpectExec(`UPDATE ScheduledDownloads SET schedule_status = \$1 WHERE id = \$2`).
		WithArgs(StatusCompleted, 7).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := s.repo.MarkCompleted(context.Background(), 7)
	s.NoError(err)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *SchedulerRepoTestSuite) TestRecordFailure() {
	next := time.Now().Add(time.Hour)
	s.mock.ExpectExec(`UPDATE ScheduledDownloads`).
		WithArgs(StatusPending, next.Format(time.RFC3339), "no_torrent_found", "no torrent", 7).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := s.repo.RecordFailure(context.Background(), 7, "no_torrent_found", "no torrent", next)
	s.NoError(err)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *SchedulerRepoTestSuite) TestMarkPermanentlyFailed() {
	s.mock.ExpectExec(`UPDATE ScheduledDownloads`).
		WithArgs(StatusFailed, "invalid_media", "bad", 7).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := s.repo.MarkPermanentlyFailed(context.Background(), 7, "invalid_media", "bad")
	s.NoError(err)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *SchedulerRepoTestSuite) TestResetStaleQueued() {
	s.mock.ExpectExec(`UPDATE ScheduledDownloads SET schedule_status = \$1\s+WHERE schedule_status = \$2 AND queued_at < \$3`).
		WithArgs(StatusPending, StatusQueued, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 3))

	count, err := s.repo.ResetStaleQueued(context.Background(), 30*time.Minute)
	s.NoError(err)
	s.Equal(int64(3), count)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *SchedulerRepoTestSuite) TestScheduleRetry_Success() {
	media := tvdb.Media{
		Id:       "123",
		Name:     "Test Show",
		Category: "series",
		Metadata: tvdb.TVDBSeriesMetadata{
			Episodes: []tvdb.Episode{{SeasonNumber: 1, Number: 1, Aired: "2026-01-01"}},
		},
	}
	mediaJSON, _ := json.Marshal(media)
	first := time.Now().Add(time.Hour)
	hashTime, _ := time.Parse("2006-01-02", "2026-01-01")

	s.mock.ExpectQuery(`INSERT INTO ScheduledDownloads`).
		WithArgs(mediaJSON, hashTime.Format(time.RFC3339), StatusPending, sqlmock.AnyArg(),
			"trace", "span", 1, first.Format(time.RFC3339), "no_torrent_found", "nope").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(5))

	err := s.repo.ScheduleRetry(context.Background(), media, first, "no_torrent_found", "nope", "trace", "span")
	s.NoError(err)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *SchedulerRepoTestSuite) TestScheduleRetry_AlreadyTracked() {
	media := tvdb.Media{Id: "123", Name: "Test Show", Category: "movie", Metadata: tvdb.TVDBSeriesMetadata{FirstAired: "2026-01-01"}}
	first := time.Now().Add(time.Hour)

	s.mock.ExpectQuery(`INSERT INTO ScheduledDownloads`).
		WillReturnError(sql.ErrNoRows)

	// ON CONFLICT DO NOTHING (no rows) is not an error for retries.
	err := s.repo.ScheduleRetry(context.Background(), media, first, "no_torrent_found", "nope", "trace", "span")
	s.NoError(err)
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

// TestComputeContentHash verifies that different shows with the same episode
// numbers produce different hashes, and that the same show produces the same hash.
func (s *SchedulerRepoTestSuite) TestComputeContentHash_ShowsDontCollide() {
	releaseTime := time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC)

	show1 := tvdb.Media{
		Id:       "111",
		Category: "series",
		Metadata: tvdb.TVDBSeriesMetadata{
			Episodes: []tvdb.Episode{
				{SeasonNumber: 1, Number: 5, AbsoluteNumber: 5},
			},
		},
	}
	show2 := tvdb.Media{
		Id:       "222",
		Category: "series",
		Metadata: tvdb.TVDBSeriesMetadata{
			Episodes: []tvdb.Episode{
				{SeasonNumber: 1, Number: 5, AbsoluteNumber: 5},
			},
		},
	}

	hash1 := ComputeContentHash(show1, releaseTime)
	hash2 := ComputeContentHash(show2, releaseTime)

	s.NotEqual(hash1, hash2, "Different shows with same episode numbers must not collide")
	// Same show same episode → same hash
	s.Equal(hash1, ComputeContentHash(show1, releaseTime))
}

func (s *SchedulerRepoTestSuite) TestComputeContentHash_MovieUsesDate() {
	movie := tvdb.Media{
		Id:       "999",
		Category: "movie",
	}
	t1 := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)

	hash1 := ComputeContentHash(movie, t1)
	hash2 := ComputeContentHash(movie, t2)

	s.NotEqual(hash1, hash2, "Same movie with different release dates must produce different hashes")
	s.Equal(hash1, ComputeContentHash(movie, t1))
}

func (s *SchedulerRepoTestSuite) TestGetPendingScheduleMeta_KeysByNotificationID() {
	show := tvdb.Media{
		Id:       "111",
		Name:     "Test Show",
		Category: "series",
		Metadata: tvdb.TVDBSeriesMetadata{
			Episodes: []tvdb.Episode{{Id: 4242, SeasonNumber: 1, Number: 5}},
		},
	}
	movie := tvdb.Media{Id: "999", Name: "Test Movie", Category: "movie"}

	showJSON, err := json.Marshal(show)
	s.Require().NoError(err)
	movieJSON, err := json.Marshal(movie)
	s.Require().NoError(err)

	releaseTime := time.Now().Add(-2 * time.Hour)
	nextAttempt := time.Now().Add(15 * time.Minute)

	rows := sqlmock.NewRows([]string{
		"media", "release_time", "schedule_status", "attempts",
		"next_attempt_at", "last_failure_code", "last_failure_reason",
	}).
		AddRow(showJSON, releaseTime, StatusPending, 2, nextAttempt, "no_torrent_found", "No torrent found yet").
		AddRow(movieJSON, releaseTime, StatusQueued, 0, nil, nil, nil)

	s.mock.ExpectQuery("SELECT media, release_time, schedule_status, attempts").
		WithArgs(StatusPending, StatusQueued).
		WillReturnRows(rows)

	meta, err := s.repo.GetPendingScheduleMeta(context.Background())
	s.Require().NoError(err)
	s.Len(meta, 2)

	// Shows are keyed by episode id, movies by media id — matching notifications.
	episodeMeta, ok := meta["4242"]
	s.Require().True(ok, "show should be keyed by its episode id")
	s.Equal(2, episodeMeta.Attempts)
	s.Equal("no_torrent_found", episodeMeta.LastFailureCode)
	s.Require().NotNil(episodeMeta.NextAttemptAt)
	s.WithinDuration(nextAttempt, *episodeMeta.NextAttemptAt, time.Second)

	movieMeta, ok := meta["999"]
	s.Require().True(ok, "movie should be keyed by its media id")
	s.Equal(StatusQueued, movieMeta.ScheduleStatus)
	s.Nil(movieMeta.NextAttemptAt)
	s.Empty(movieMeta.LastFailureCode)

	s.Require().NoError(s.mock.ExpectationsWereMet())
}
