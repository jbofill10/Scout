package repository

import (
	"bytes"
	"context"
	"database/sql"
	"log/slog"
	"testing"
	"time"

	"github.com/jbofill10/scout/backend/pkg/notifications"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/suite"
)

type NotificationRepoTestSuite struct {
	suite.Suite
	repo   *NotificationRepo
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

	s.repo = &NotificationRepo{
		PostgresRepository: notifications.NewPostgresRepository(s.db, s.logger),
		db:                 s.db,
		logger:             s.logger,
	}
}

func (s *NotificationRepoTestSuite) TearDownTest() {
	s.db.Close()
}

func (s *NotificationRepoTestSuite) TestCreateNotification_Success() {
	season := 1
	episode := 2
	notification := &notifications.Notification{
		TvdbID:     "123456",
		MediaTitle: "Test Show",
		Category:   "series",
		Season:     &season,
		Episode:    &episode,
		PosterURL:  "https://example.com/poster.jpg",
		IsAnime:    false,
		Status:     notifications.StatusSearching,
		TraceID:    "test-trace-id",
		SpanID:     "test-span-id",
	}

	now := time.Now()
	s.mock.ExpectQuery(`INSERT INTO Notifications`).
		WithArgs(
			notification.TvdbID,
			notification.MediaTitle,
			notification.Category,
			notification.Season,
			notification.Episode,
			notification.AbsoluteEpisode,
			notification.PosterURL,
			notification.IsAnime,
			notification.Status,
			notification.Reason,
			notification.IsRead,
			notification.AutoDismissed,
			notification.TraceID,
			notification.SpanID,
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
		).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
			AddRow(1, now, now))

	err := s.repo.CreateNotification(context.Background(), notification)

	s.NoError(err)
	s.Equal(1, notification.ID)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *NotificationRepoTestSuite) TestCreateNotification_Movie() {
	notification := &notifications.Notification{
		TvdbID:     "789012",
		MediaTitle: "Test Movie",
		Category:   "movie",
		PosterURL:  "https://example.com/movie-poster.jpg",
		IsAnime:    true,
		Status:     notifications.StatusSearching,
		TraceID:    "test-trace-id",
		SpanID:     "test-span-id",
	}

	now := time.Now()
	s.mock.ExpectQuery(`INSERT INTO Notifications`).
		WithArgs(
			notification.TvdbID,
			notification.MediaTitle,
			notification.Category,
			notification.Season,
			notification.Episode,
			notification.AbsoluteEpisode,
			notification.PosterURL,
			notification.IsAnime,
			notification.Status,
			notification.Reason,
			notification.IsRead,
			notification.AutoDismissed,
			notification.TraceID,
			notification.SpanID,
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
		).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
			AddRow(2, now, now))

	err := s.repo.CreateNotification(context.Background(), notification)

	s.NoError(err)
	s.Equal(2, notification.ID)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *NotificationRepoTestSuite) TestCreateNotification_DatabaseError() {
	notification := &notifications.Notification{
		TvdbID:     "123456",
		MediaTitle: "Test Show",
		Category:   "series",
		Status:     notifications.StatusSearching,
	}

	s.mock.ExpectQuery(`INSERT INTO Notifications`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnError(sql.ErrConnDone)

	err := s.repo.CreateNotification(context.Background(), notification)

	s.Error(err)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *NotificationRepoTestSuite) TestUpdateNotification_Success() {
	notification := &notifications.Notification{
		ID:     1,
		Status: notifications.StatusDownloading,
		Reason: "Found torrent",
	}

	s.mock.ExpectExec(`UPDATE Notifications`).
		WithArgs(
			notification.Status,
			notification.Reason,
			notification.IsRead,
			notification.AutoDismissed,
			sqlmock.AnyArg(),
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
	}

	s.mock.ExpectExec(`UPDATE Notifications`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), notification.ID).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := s.repo.UpdateNotification(context.Background(), notification)

	s.Error(err)
	s.Contains(err.Error(), "notification not found")
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *NotificationRepoTestSuite) TestGetNotifications_NoFilters() {
	now := time.Now()
	season := 1
	episode := 2

	rows := sqlmock.NewRows([]string{
		"id", "tvdb_id", "media_title", "category", "season", "episode", "absolute_episode",
		"poster_url", "is_anime", "status", "reason",
		"is_read", "auto_dismissed", "trace_id", "span_id", "created_at", "updated_at",
	}).AddRow(
		1, "123456", "Test Show", "series", &season, &episode, nil,
		"https://example.com/poster.jpg", false, "completed", "",
		false, false, "trace-1", "span-1", now, now,
	)

	s.mock.ExpectQuery(`SELECT (.+) FROM Notifications WHERE auto_dismissed = false ORDER BY created_at DESC LIMIT`).
		WithArgs(100).
		WillReturnRows(rows)

	notifications, err := s.repo.GetNotifications(context.Background(), 100, "", "")

	s.NoError(err)
	s.Len(notifications, 1)
	s.Equal("123456", notifications[0].TvdbID)
	s.Equal("Test Show", notifications[0].MediaTitle)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *NotificationRepoTestSuite) TestGetNotifications_WithFilters() {
	now := time.Now()

	rows := sqlmock.NewRows([]string{
		"id", "tvdb_id", "media_title", "category", "season", "episode", "absolute_episode",
		"poster_url", "is_anime", "status", "reason",
		"is_read", "auto_dismissed", "trace_id", "span_id", "created_at", "updated_at",
	}).AddRow(
		2, "789012", "Test Movie", "movie", nil, nil, nil,
		"https://example.com/movie.jpg", true, "scheduled", "",
		false, false, "trace-2", "span-2", now, now,
	)

	s.mock.ExpectQuery(`SELECT (.+) FROM Notifications WHERE auto_dismissed = false AND category`).
		WithArgs("movie", "scheduled", 50).
		WillReturnRows(rows)

	notifs, err := s.repo.GetNotifications(context.Background(), 50, "movie", "scheduled")

	s.NoError(err)
	s.Len(notifs, 1)
	s.Equal("movie", notifs[0].Category)
	s.Equal(notifications.StatusScheduled, notifs[0].Status)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *NotificationRepoTestSuite) TestGetNotifications_Empty() {
	rows := sqlmock.NewRows([]string{
		"id", "tvdb_id", "media_title", "category", "season", "episode", "absolute_episode",
		"poster_url", "is_anime", "status", "reason",
		"is_read", "auto_dismissed", "trace_id", "span_id", "created_at", "updated_at",
	})

	s.mock.ExpectQuery(`SELECT (.+) FROM Notifications WHERE auto_dismissed = false ORDER BY created_at DESC LIMIT`).
		WithArgs(100).
		WillReturnRows(rows)

	notifications, err := s.repo.GetNotifications(context.Background(), 100, "", "")

	s.NoError(err)
	s.Empty(notifications)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *NotificationRepoTestSuite) TestGetGrouped_Success() {
	now := time.Now()
	season := 1
	episode := 2

	// Mock media query
	mediaRows := sqlmock.NewRows([]string{
		"tvdb_id", "media_title", "category", "poster_url", "is_anime", "latest_timestamp",
	}).AddRow("123456", "Test Show", "series", "https://example.com/poster.jpg", false, now)

	s.mock.ExpectQuery(`SELECT DISTINCT ON \(tvdb_id\) tvdb_id`).
		WillReturnRows(mediaRows)

	// Mock notifications query for this media
	notifRows := sqlmock.NewRows([]string{
		"id", "tvdb_id", "media_title", "category", "season", "episode", "absolute_episode",
		"poster_url", "is_anime", "status", "reason",
		"is_read", "auto_dismissed", "trace_id", "span_id", "created_at", "updated_at",
	}).AddRow(
		1, "123456", "Test Show", "series", &season, &episode, nil,
		"https://example.com/poster.jpg", false, "completed", "",
		false, false, "trace-1", "span-1", now, now,
	)

	s.mock.ExpectQuery(`SELECT (.+) FROM Notifications WHERE tvdb_id`).
		WithArgs("123456").
		WillReturnRows(notifRows)

	grouped, err := s.repo.GetGrouped(context.Background())

	s.NoError(err)
	s.Len(grouped, 1)
	s.Equal("123456", grouped[0].TvdbID)
	s.Equal("Test Show", grouped[0].MediaTitle)
	s.Len(grouped[0].Notifications, 1)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *NotificationRepoTestSuite) TestMarkAsRead_Success() {
	s.mock.ExpectExec(`UPDATE Notifications SET is_read = true`).
		WithArgs(sqlmock.AnyArg(), 1).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := s.repo.MarkAsRead(context.Background(), 1)

	s.NoError(err)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *NotificationRepoTestSuite) TestMarkAsRead_NotFound() {
	s.mock.ExpectExec(`UPDATE Notifications SET is_read = true`).
		WithArgs(sqlmock.AnyArg(), 999).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := s.repo.MarkAsRead(context.Background(), 999)

	s.Error(err)
	s.Contains(err.Error(), "notification not found")
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *NotificationRepoTestSuite) TestDismiss_Success() {
	s.mock.ExpectExec(`UPDATE Notifications SET auto_dismissed = true`).
		WithArgs(sqlmock.AnyArg(), 1).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := s.repo.Dismiss(context.Background(), 1)

	s.NoError(err)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *NotificationRepoTestSuite) TestDismiss_NotFound() {
	s.mock.ExpectExec(`UPDATE Notifications SET auto_dismissed = true`).
		WithArgs(sqlmock.AnyArg(), 999).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := s.repo.Dismiss(context.Background(), 999)

	s.Error(err)
	s.Contains(err.Error(), "notification not found")
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *NotificationRepoTestSuite) TestGetUnreadCount_Success() {
	s.mock.ExpectQuery(`SELECT COUNT\(\*\) FROM Notifications WHERE is_read = false AND auto_dismissed = false`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))

	count, err := s.repo.GetUnreadCount(context.Background())

	s.NoError(err)
	s.Equal(5, count)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *NotificationRepoTestSuite) TestGetUnreadCount_Zero() {
	s.mock.ExpectQuery(`SELECT COUNT\(\*\) FROM Notifications WHERE is_read = false AND auto_dismissed = false`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	count, err := s.repo.GetUnreadCount(context.Background())

	s.NoError(err)
	s.Equal(0, count)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *NotificationRepoTestSuite) TestCleanupOld_Success() {
	// Expect auto-dismiss update
	s.mock.ExpectExec(`UPDATE Notifications SET auto_dismissed = true`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 3))

	// Expect delete old notifications
	s.mock.ExpectExec(`DELETE FROM Notifications WHERE created_at`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 10))

	err := s.repo.CleanupOld(context.Background(), 30)

	s.NoError(err)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *NotificationRepoTestSuite) TestCleanupOld_NoRecordsToClean() {
	// Expect auto-dismiss update
	s.mock.ExpectExec(`UPDATE Notifications SET auto_dismissed = true`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 0))

	// Expect delete old notifications
	s.mock.ExpectExec(`DELETE FROM Notifications WHERE created_at`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := s.repo.CleanupOld(context.Background(), 30)

	s.NoError(err)
	s.NoError(s.mock.ExpectationsWereMet())
}
