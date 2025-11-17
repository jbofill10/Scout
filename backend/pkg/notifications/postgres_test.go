package notifications

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/suite"
)

type PostgresRepositoryTestSuite struct {
	suite.Suite
	db     *sql.DB
	mock   sqlmock.Sqlmock
	repo   *PostgresRepository
	logger *slog.Logger
}

func (s *PostgresRepositoryTestSuite) SetupTest() {
	var err error
	s.db, s.mock, err = sqlmock.New()
	s.Require().NoError(err)

	s.logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
	s.repo = NewPostgresRepository(s.db, s.logger)
}

func (s *PostgresRepositoryTestSuite) TearDownTest() {
	s.db.Close()
}

func TestPostgresRepositoryTestSuite(t *testing.T) {
	suite.Run(t, new(PostgresRepositoryTestSuite))
}

// TestCreateNotification_Series tests creating a notification for a TV series episode
func (s *PostgresRepositoryTestSuite) TestCreateNotification_Series() {
	ctx := context.Background()
	season := 1
	episode := 5
	absEpisode := 5

	notification := &Notification{
		TvdbID:          "987654", // Episode ID
		MediaTitle:      "Test Show",
		Category:        "series",
		Season:          &season,
		Episode:         &episode,
		AbsoluteEpisode: &absEpisode,
		PosterURL:       "https://example.com/poster.jpg",
		IsAnime:         false,
		Status:          StatusSearching,
		TraceID:         "trace123",
		SpanID:          "span456",
	}

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
			sqlmock.AnyArg(), // created_at
			sqlmock.AnyArg(), // updated_at
		).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
			AddRow(1, time.Now(), time.Now()))

	err := s.repo.CreateNotification(ctx, notification)

	s.NoError(err)
	s.Equal(1, notification.ID)
	s.NoError(s.mock.ExpectationsWereMet())
}

// TestCreateNotification_Movie tests creating a notification for a movie
func (s *PostgresRepositoryTestSuite) TestCreateNotification_Movie() {
	ctx := context.Background()

	notification := &Notification{
		TvdbID:     "123456", // Movie ID
		MediaTitle: "Test Movie",
		Category:   "movie",
		PosterURL:  "https://example.com/movie.jpg",
		IsAnime:    true,
		Status:     StatusSearching,
		TraceID:    "trace789",
		SpanID:     "span012",
	}

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
			AddRow(2, time.Now(), time.Now()))

	err := s.repo.CreateNotification(ctx, notification)

	s.NoError(err)
	s.Equal(2, notification.ID)
	s.NoError(s.mock.ExpectationsWereMet())
}

// TestCreateNotification_DatabaseError tests error handling during creation
func (s *PostgresRepositoryTestSuite) TestCreateNotification_DatabaseError() {
	ctx := context.Background()

	notification := &Notification{
		TvdbID:     "111111",
		MediaTitle: "Error Test",
		Category:   "series",
		Status:     StatusSearching,
	}

	s.mock.ExpectQuery(`INSERT INTO Notifications`).
		WillReturnError(sql.ErrConnDone)

	err := s.repo.CreateNotification(ctx, notification)

	s.Error(err)
	s.Contains(err.Error(), "failed to insert notification")
	s.NoError(s.mock.ExpectationsWereMet())
}

// TestGetNotification_Found tests retrieving a notification by tvdb_id
func (s *PostgresRepositoryTestSuite) TestGetNotification_Found() {
	ctx := context.Background()
	tvdbID := "987654"
	season := 2
	episode := 10

	rows := sqlmock.NewRows([]string{
		"id", "tvdb_id", "media_title", "category", "season", "episode", "absolute_episode",
		"poster_url", "is_anime", "status", "reason",
		"is_read", "auto_dismissed", "trace_id", "span_id", "created_at", "updated_at",
	}).AddRow(
		5, tvdbID, "Found Show", "series", season, episode, nil,
		"poster.jpg", false, StatusDownloading, "",
		false, false, "trace", "span", time.Now(), time.Now(),
	)

	s.mock.ExpectQuery(`SELECT .+ FROM Notifications WHERE tvdb_id = \$1 AND auto_dismissed = false`).
		WithArgs(tvdbID).
		WillReturnRows(rows)

	notification, err := s.repo.GetNotification(ctx, tvdbID)

	s.NoError(err)
	s.NotNil(notification)
	s.Equal(5, notification.ID)
	s.Equal(tvdbID, notification.TvdbID)
	s.Equal("Found Show", notification.MediaTitle)
	s.Equal(StatusDownloading, notification.Status)
	s.NoError(s.mock.ExpectationsWereMet())
}

// TestGetNotification_NotFound tests not found case
func (s *PostgresRepositoryTestSuite) TestGetNotification_NotFound() {
	ctx := context.Background()
	tvdbID := "nonexistent"

	s.mock.ExpectQuery(`SELECT .+ FROM Notifications WHERE tvdb_id = \$1`).
		WithArgs(tvdbID).
		WillReturnError(sql.ErrNoRows)

	notification, err := s.repo.GetNotification(ctx, tvdbID)

	s.NoError(err) // Not found should return nil without error
	s.Nil(notification)
	s.NoError(s.mock.ExpectationsWereMet())
}

// TestGetNotification_DatabaseError tests error handling during retrieval
func (s *PostgresRepositoryTestSuite) TestGetNotification_DatabaseError() {
	ctx := context.Background()
	tvdbID := "error-id"

	s.mock.ExpectQuery(`SELECT .+ FROM Notifications WHERE tvdb_id = \$1`).
		WithArgs(tvdbID).
		WillReturnError(sql.ErrConnDone)

	notification, err := s.repo.GetNotification(ctx, tvdbID)

	s.Error(err)
	s.Nil(notification)
	s.Contains(err.Error(), "failed to query notification")
	s.NoError(s.mock.ExpectationsWereMet())
}

// TestUpdateNotification_Success tests updating a notification
func (s *PostgresRepositoryTestSuite) TestUpdateNotification_Success() {
	ctx := context.Background()

	notification := &Notification{
		ID:     10,
		Status: StatusCompleted,
		Reason: "",
		IsRead: false,
	}

	s.mock.ExpectExec(`UPDATE Notifications SET`).
		WithArgs(
			notification.Status,
			notification.Reason,
			notification.IsRead,
			notification.AutoDismissed,
			sqlmock.AnyArg(), // updated_at
			notification.ID,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := s.repo.UpdateNotification(ctx, notification)

	s.NoError(err)
	s.NoError(s.mock.ExpectationsWereMet())
}

// TestUpdateNotification_NotFound tests updating non-existent notification
func (s *PostgresRepositoryTestSuite) TestUpdateNotification_NotFound() {
	ctx := context.Background()

	notification := &Notification{
		ID:     999,
		Status: StatusCompleted,
	}

	s.mock.ExpectExec(`UPDATE Notifications SET`).
		WithArgs(
			notification.Status,
			notification.Reason,
			notification.IsRead,
			notification.AutoDismissed,
			sqlmock.AnyArg(),
			notification.ID,
		).
		WillReturnResult(sqlmock.NewResult(0, 0)) // 0 rows affected

	err := s.repo.UpdateNotification(ctx, notification)

	s.Error(err)
	s.Contains(err.Error(), "notification not found")
	s.NoError(s.mock.ExpectationsWereMet())
}

// TestUpdateNotification_DatabaseError tests error handling during update
func (s *PostgresRepositoryTestSuite) TestUpdateNotification_DatabaseError() {
	ctx := context.Background()

	notification := &Notification{
		ID:     20,
		Status: StatusFailed,
		Reason: "Download error",
	}

	s.mock.ExpectExec(`UPDATE Notifications SET`).
		WillReturnError(sql.ErrConnDone)

	err := s.repo.UpdateNotification(ctx, notification)

	s.Error(err)
	s.Contains(err.Error(), "failed to update notification")
	s.NoError(s.mock.ExpectationsWereMet())
}
