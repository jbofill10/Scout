package repository

import (
	"bytes"
	"context"
	"database/sql"
	"log/slog"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jbofill10/scout/backend/pkg/notifications"
	"github.com/stretchr/testify/suite"
)

type NotificationRepoTestSuite struct {
	suite.Suite
	repo   *Repo
	db     *sql.DB
	mock   sqlmock.Sqlmock
	logger *slog.Logger
}

func TestNotificationRepoSuite(t *testing.T) {
	suite.Run(t, new(NotificationRepoTestSuite))
}

func (s *NotificationRepoTestSuite) SetupTest() {
	var err error
	s.db, s.mock, err = sqlmock.New()
	s.Require().NoError(err)

	buf := new(bytes.Buffer)
	s.logger = slog.New(slog.NewTextHandler(buf, nil))

	s.repo = &Repo{
		PostgresRepository: notifications.NewPostgresRepository(s.db, s.logger),
		db:                 s.db,
		logger:             s.logger,
	}
}

func (s *NotificationRepoTestSuite) TearDownTest() {
	s.db.Close()
}

func (s *NotificationRepoTestSuite) TestGetNotification_Series_Success() {
	now := time.Now()
	season := 1
	episode := 2
	absoluteEpisode := 5

	rows := sqlmock.NewRows([]string{
		"id", "tvdb_id", "media_title", "category", "season", "episode", "absolute_episode",
		"poster_url", "is_anime", "status", "reason",
		"is_read", "auto_dismissed", "trace_id", "span_id", "created_at", "updated_at",
	}).AddRow(
		1, "123456", "Test Show", "series", &season, &episode, &absoluteEpisode,
		"https://example.com/poster.jpg", false, "searching", "",
		false, false, "trace-1", "span-1", now, now,
	)

	s.mock.ExpectQuery(`SELECT (.+) FROM Notifications WHERE tvdb_id = (.+)`).
		WithArgs("123456").
		WillReturnRows(rows)

	notification, err := s.repo.GetNotification(context.Background(), "123456")

	s.NoError(err)
	s.NotNil(notification)
	s.Equal(1, notification.ID)
	s.Equal("123456", notification.TvdbID)
	s.Equal("Test Show", notification.MediaTitle)
	s.Equal("series", notification.Category)
	s.Equal(1, *notification.Season)
	s.Equal(2, *notification.Episode)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *NotificationRepoTestSuite) TestGetNotification_Series_NotFound() {
	s.mock.ExpectQuery(`SELECT (.+) FROM Notifications WHERE tvdb_id = (.+)`).
		WithArgs("123456").
		WillReturnError(sql.ErrNoRows)

	notification, err := s.repo.GetNotification(context.Background(), "123456")

	s.NoError(err) // Should not return error for not found
	s.Nil(notification)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *NotificationRepoTestSuite) TestGetNotification_Series_DatabaseError() {
	s.mock.ExpectQuery(`SELECT (.+) FROM Notifications WHERE tvdb_id = (.+)`).
		WithArgs("123456").
		WillReturnError(sql.ErrConnDone)

	notification, err := s.repo.GetNotification(context.Background(), "123456")

	s.Error(err)
	s.Nil(notification)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *NotificationRepoTestSuite) TestGetNotification_Movie_Success() {
	now := time.Now()

	rows := sqlmock.NewRows([]string{
		"id", "tvdb_id", "media_title", "category", "season", "episode", "absolute_episode",
		"poster_url", "is_anime", "status", "reason",
		"is_read", "auto_dismissed", "trace_id", "span_id", "created_at", "updated_at",
	}).AddRow(
		2, "789012", "Test Movie", "movie", nil, nil, nil,
		"https://example.com/movie.jpg", true, "searching", "",
		false, false, "trace-2", "span-2", now, now,
	)

	s.mock.ExpectQuery(`SELECT (.+) FROM Notifications WHERE tvdb_id = (.+) AND auto_dismissed = false`).
		WithArgs("789012").
		WillReturnRows(rows)

	notification, err := s.repo.GetNotification(context.Background(), "789012")

	s.NoError(err)
	s.NotNil(notification)
	s.Equal(2, notification.ID)
	s.Equal("789012", notification.TvdbID)
	s.Equal("Test Movie", notification.MediaTitle)
	s.Equal("movie", notification.Category)
	s.Nil(notification.Season)
	s.Nil(notification.Episode)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *NotificationRepoTestSuite) TestGetNotification_Movie_NotFound() {
	s.mock.ExpectQuery(`SELECT (.+) FROM Notifications WHERE tvdb_id = (.+) AND auto_dismissed = false`).
		WithArgs("789012").
		WillReturnError(sql.ErrNoRows)

	notification, err := s.repo.GetNotification(context.Background(), "789012")

	s.NoError(err) // Should not return error for not found
	s.Nil(notification)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *NotificationRepoTestSuite) TestUpdateNotification_Success() {
	season := 1
	episode := 2
	notification := &notifications.Notification{
		ID:         1,
		TvdbID:     "123456",
		MediaTitle: "Test Show",
		Category:   "series",
		Season:     &season,
		Episode:    &episode,
		Status:     notifications.StatusDownloading,
		Reason:     "",
	}

	s.mock.ExpectExec(`UPDATE Notifications SET status = (.+), reason = (.+), is_read = (.+), auto_dismissed = (.+), updated_at = (.+) WHERE id = (.+)`).
		WithArgs(
			notification.Status,
			notification.Reason,
			notification.IsRead,
			notification.AutoDismissed,
			sqlmock.AnyArg(), // updated_at
			notification.ID,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := s.repo.UpdateNotification(context.Background(), notification)

	s.NoError(err)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *NotificationRepoTestSuite) TestUpdateNotification_NotFound() {
	notification := &notifications.Notification{
		ID:     999,
		Status: notifications.StatusFailed,
		Reason: "Test error",
	}

	s.mock.ExpectExec(`UPDATE Notifications SET status = (.+), reason = (.+), is_read = (.+), auto_dismissed = (.+), updated_at = (.+) WHERE id = (.+)`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), notification.ID).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := s.repo.UpdateNotification(context.Background(), notification)

	s.Error(err)
	s.Contains(err.Error(), "notification not found")
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *NotificationRepoTestSuite) TestUpdateNotification_DatabaseError() {
	notification := &notifications.Notification{
		ID:     1,
		Status: notifications.StatusCompleted,
	}

	s.mock.ExpectExec(`UPDATE Notifications SET status = (.+), reason = (.+), is_read = (.+), auto_dismissed = (.+), updated_at = (.+) WHERE id = (.+)`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), notification.ID).
		WillReturnError(sql.ErrConnDone)

	err := s.repo.UpdateNotification(context.Background(), notification)

	s.Error(err)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *NotificationRepoTestSuite) TestUpdateNotification_FullWorkflow() {
	// Test full workflow: searching -> downloading -> completed
	season := 1
	episode := 3
	notification := &notifications.Notification{
		ID:         1,
		TvdbID:     "123456",
		MediaTitle: "Test Show",
		Category:   "series",
		Season:     &season,
		Episode:    &episode,
	}

	// Stage 1: Update to downloading
	notification.Status = notifications.StatusDownloading

	s.mock.ExpectExec(`UPDATE Notifications SET status = (.+), reason = (.+), is_read = (.+), auto_dismissed = (.+), updated_at = (.+) WHERE id = (.+)`).
		WithArgs("downloading", "", false, false, sqlmock.AnyArg(), 1).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := s.repo.UpdateNotification(context.Background(), notification)
	s.NoError(err)

	// Stage 2: Update to completed
	notification.Status = notifications.StatusCompleted
	notification.Reason = ""

	s.mock.ExpectExec(`UPDATE Notifications SET status = (.+), reason = (.+), is_read = (.+), auto_dismissed = (.+), updated_at = (.+) WHERE id = (.+)`).
		WithArgs("completed", "", false, false, sqlmock.AnyArg(), 1).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = s.repo.UpdateNotification(context.Background(), notification)
	s.NoError(err)

	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *NotificationRepoTestSuite) TestUpdateNotification_FailureWorkflow() {
	// Test failure workflow: searching -> failed
	notification := &notifications.Notification{
		ID:       1,
		TvdbID:   "123456",
		Category: "movie",
	}

	// Update to failed
	notification.Status = notifications.StatusFailed
	notification.Reason = "No torrents found matching criteria"

	s.mock.ExpectExec(`UPDATE Notifications SET status = (.+), reason = (.+), is_read = (.+), auto_dismissed = (.+), updated_at = (.+) WHERE id = (.+)`).
		WithArgs("failed", "No torrents found matching criteria", false, false, sqlmock.AnyArg(), 1).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := s.repo.UpdateNotification(context.Background(), notification)
	s.NoError(err)

	s.NoError(s.mock.ExpectationsWereMet())
}
