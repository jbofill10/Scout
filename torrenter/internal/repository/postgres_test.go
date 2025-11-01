package repository

import (
	"bytes"
	"context"
	"database/sql"
	"log/slog"
	"testing"
	"torrenter/internal/models"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/suite"
)

type RepoTestSuite struct {
	suite.Suite
	repo   *Repo
	db     *sql.DB
	mock   sqlmock.Sqlmock
	logger *slog.Logger
}

func TestRepoSuite(t *testing.T) {
	suite.Run(t, new(RepoTestSuite))
}

func (s *RepoTestSuite) SetupTest() {
	var err error
	s.db, s.mock, err = sqlmock.New()
	s.Require().NoError(err)

	buf := new(bytes.Buffer)
	s.logger = slog.New(slog.NewTextHandler(buf, nil))

	s.repo = &Repo{
		db:     s.db,
		logger: s.logger,
	}
}

func (s *RepoTestSuite) TearDownTest() {
	s.db.Close()
}

func (s *RepoTestSuite) TestUpsertLibraries_Success() {
	libs := models.PlexLibrariesResponse{
		Directories: []models.Directory{
			{
				Key:  "1",
				Type: "show",
				Locations: []models.Location{
					{
						ID:   1,
						Path: "/data/shows",
					},
				},
			},
		},
	}

	s.mock.ExpectExec(`INSERT INTO Libraries`).
		WithArgs(1, "show", "/data/shows", "1").
		WillReturnResult(sqlmock.NewResult(1, 1))

	s.repo.UpsertLibraries(context.Background(), libs)

	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *RepoTestSuite) TestUpsertLibraries_MultipleLocations() {
	libs := models.PlexLibrariesResponse{
		Directories: []models.Directory{
			{
				Key:  "1",
				Type: "show",
				Locations: []models.Location{
					{ID: 1, Path: "/data/shows1"},
					{ID: 2, Path: "/data/shows2"},
				},
			},
		},
	}

	s.mock.ExpectExec(`INSERT INTO Libraries`).
		WithArgs(1, "show", "/data/shows1", "1").
		WillReturnResult(sqlmock.NewResult(1, 1))

	s.mock.ExpectExec(`INSERT INTO Libraries`).
		WithArgs(2, "show", "/data/shows2", "1").
		WillReturnResult(sqlmock.NewResult(2, 1))

	s.repo.UpsertLibraries(context.Background(), libs)

	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *RepoTestSuite) TestSetPreferredLibrary_Success() {
	s.mock.ExpectBegin()
	s.mock.ExpectExec(`UPDATE Libraries SET preferred = 0`).
		WithArgs("show").
		WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectExec(`UPDATE Libraries SET preferred = 1`).
		WithArgs(1).
		WillReturnResult(sqlmock.NewResult(0, 1))
	s.mock.ExpectCommit()

	err := s.repo.SetPreferredLibrary(context.Background(), 1, "show")

	s.NoError(err)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *RepoTestSuite) TestSetPreferredLibrary_TransactionError() {
	s.mock.ExpectBegin().WillReturnError(sql.ErrConnDone)

	err := s.repo.SetPreferredLibrary(context.Background(), 1, "show")

	s.Error(err)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *RepoTestSuite) TestSetPreferredLibrary_UnsetError() {
	s.mock.ExpectBegin()
	s.mock.ExpectExec(`UPDATE Libraries SET preferred = 0`).
		WithArgs("show").
		WillReturnError(sql.ErrConnDone)
	s.mock.ExpectRollback()

	err := s.repo.SetPreferredLibrary(context.Background(), 1, "show")

	s.Error(err)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *RepoTestSuite) TestGetPreferredLibrary_Success() {
	rows := sqlmock.NewRows([]string{"id", "type", "path", "preferred"}).
		AddRow(1, "show", "/data/shows", 1)

	s.mock.ExpectQuery(`SELECT \* FROM Libraries`).
		WithArgs("show").
		WillReturnRows(rows)

	lib, err := s.repo.GetPreferredLibrary(context.Background(), "show")

	s.NoError(err)
	s.Equal("/data/shows", lib.Path)
	s.Equal("show", lib.Type)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *RepoTestSuite) TestGetPreferredLibrary_NotFound() {
	s.mock.ExpectQuery(`SELECT \* FROM Libraries`).
		WithArgs("show").
		WillReturnError(sql.ErrNoRows)

	lib, err := s.repo.GetPreferredLibrary(context.Background(), "show")

	s.Error(err)
	s.Contains(err.Error(), "no preferred library found")
	s.Equal(models.PlexLibrary{}, lib)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *RepoTestSuite) TestMediaExists_True() {
	rows := sqlmock.NewRows([]string{"exists"}).AddRow(true)

	s.mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs("abc123").
		WillReturnRows(rows)

	exists, err := s.repo.MediaExists(context.Background(), "abc123")

	s.NoError(err)
	s.True(exists)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *RepoTestSuite) TestMediaExists_False() {
	rows := sqlmock.NewRows([]string{"exists"}).AddRow(false)

	s.mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs("nonexistent").
		WillReturnRows(rows)

	exists, err := s.repo.MediaExists(context.Background(), "nonexistent")

	s.NoError(err)
	s.False(exists)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *RepoTestSuite) TestEpisodeExistsByTvdbId_True() {
	rows := sqlmock.NewRows([]string{"exists"}).AddRow(true)

	s.mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs("123", 1, 2).
		WillReturnRows(rows)

	exists, err := s.repo.EpisodeExistsByTvdbId(context.Background(), "123", 1, 2)

	s.NoError(err)
	s.True(exists)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *RepoTestSuite) TestEpisodeExistsByTvdbId_False() {
	rows := sqlmock.NewRows([]string{"exists"}).AddRow(false)

	s.mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs("123", 1, 2).
		WillReturnRows(rows)

	exists, err := s.repo.EpisodeExistsByTvdbId(context.Background(), "123", 1, 2)

	s.NoError(err)
	s.False(exists)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *RepoTestSuite) TestEpisodeExistsByTvdbId_Error() {
	s.mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs("123", 1, 2).
		WillReturnError(sql.ErrConnDone)

	exists, err := s.repo.EpisodeExistsByTvdbId(context.Background(), "123", 1, 2)

	s.Error(err)
	s.False(exists)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *RepoTestSuite) TestGetAllLibrarySections_Success() {
	rows := sqlmock.NewRows([]string{"id", "section", "type", "path", "preferred"}).
		AddRow(1, 1, "show", "/data/shows", 1).
		AddRow(2, 2, "movie", "/data/movies", 0)

	s.mock.ExpectQuery(`SELECT id, section, type, path, preferred FROM Libraries`).
		WillReturnRows(rows)

	libs := s.repo.GetAllLibrarySections()

	s.Len(libs, 2)
	s.Equal("/data/shows", libs[0].Path)
	s.Equal("/data/movies", libs[1].Path)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *RepoTestSuite) TestGetAllLibrarySections_Empty() {
	rows := sqlmock.NewRows([]string{"id", "section", "type", "path", "preferred"})

	s.mock.ExpectQuery(`SELECT id, section, type, path, preferred FROM Libraries`).
		WillReturnRows(rows)

	libs := s.repo.GetAllLibrarySections()

	s.Empty(libs)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *RepoTestSuite) TestGetLibraryByType_Success() {
	rows := sqlmock.NewRows([]string{"section"}).AddRow(1)

	s.mock.ExpectQuery(`SELECT section FROM Libraries`).
		WithArgs("show").
		WillReturnRows(rows)

	section, err := s.repo.GetLibraryByType(context.Background(), "show")

	s.NoError(err)
	s.Equal(1, section)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *RepoTestSuite) TestGetLibraryByType_NotFound() {
	s.mock.ExpectQuery(`SELECT section FROM Libraries`).
		WithArgs("unknown").
		WillReturnError(sql.ErrNoRows)

	section, err := s.repo.GetLibraryByType(context.Background(), "unknown")

	s.Error(err)
	s.Equal(0, section)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *RepoTestSuite) TestUpsertMovies_Success() {
	movies := models.PlexMovieLibraryData{
		Movies: []models.Movie{
			{
				Id:     "1",
				Title:  "Test Movie",
				Year:   2023,
				Thumb:  "/thumb.jpg",
				Art:    "/art.jpg",
				TvdbId: "123456",
				MovieMeta: []models.MediaMeta{
					{
						VideoResolution: "1080p",
						Part: []models.Part{
							{File: "/data/movies/test.mkv"},
						},
					},
				},
			},
		},
	}

	s.mock.ExpectExec(`INSERT INTO Movies`).
		WithArgs("1", "Test Movie", 2023, "/thumb.jpg", "/art.jpg", "123456").
		WillReturnResult(sqlmock.NewResult(1, 1))

	s.mock.ExpectExec(`INSERT INTO MovieMedia`).
		WithArgs("1", "1080p", "/data/movies/test.mkv").
		WillReturnResult(sqlmock.NewResult(1, 1))

	s.repo.UpsertMovies(context.Background(), movies)

	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *RepoTestSuite) TestUpsertShows_Success() {
	lib := &models.PlexShowLibraryData{
		Shows: []models.PlexShowData{
			{
				Id:       "1",
				Title:    "Test Show",
				ShowMeta: "/shows/1",
				Thumb:    "/thumb.jpg",
				TvdbId:   "123",
				Seasons: []models.PlexSeasonData{
					{
						Id:           "s1",
						SeasonMeta:   "/shows/1/season/1",
						SeasonNumber: 1,
						TvdbId:       "season123",
						Episodes: []models.PlexEpisodeData{
							{
								Id:            "e1",
								EpisodeMeta:   "/shows/1/season/1/episode/1",
								EpisodeNumber: 1,
								TvdbId:        "ep123",
								Media: []models.PlexMediaData{
									{
										Id:              "1",
										VideoResolution: "1080p",
										File:            "/data/shows/test.mkv",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Expect show insert
	s.mock.ExpectExec(`INSERT INTO Shows`).
		WithArgs("1", "Test Show", "/shows/1", "/thumb.jpg", "123").
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Expect season insert
	s.mock.ExpectExec(`INSERT INTO Seasons`).
		WithArgs("s1", "1", "/shows/1/season/1", 1, "season123").
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Expect episode insert
	s.mock.ExpectExec(`INSERT INTO Episodes`).
		WithArgs("e1", "s1", "/shows/1/season/1/episode/1", 1, "ep123").
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Expect episode media insert
	s.mock.ExpectExec(`INSERT INTO EpisodeMedia`).
		WithArgs("1", "e1", "1080p", "/data/shows/test.mkv").
		WillReturnResult(sqlmock.NewResult(1, 1))

	s.repo.UpsertShows(context.Background(), lib)

	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *RepoTestSuite) TestUpsertShows_MultipleEpisodes() {
	lib := &models.PlexShowLibraryData{
		Shows: []models.PlexShowData{
			{
				Id:       "1",
				Title:    "Test Show",
				ShowMeta: "/shows/1",
				Thumb:    "/thumb.jpg",
				TvdbId:   "123",
				Seasons: []models.PlexSeasonData{
					{
						Id:           "s1",
						SeasonMeta:   "/shows/1/season/1",
						SeasonNumber: 1,
						Episodes: []models.PlexEpisodeData{
							{
								Id:            "e1",
								EpisodeMeta:   "/shows/1/season/1/episode/1",
								EpisodeNumber: 1,
								Media:         []models.PlexMediaData{},
							},
							{
								Id:            "e2",
								EpisodeMeta:   "/shows/1/season/1/episode/2",
								EpisodeNumber: 2,
								Media:         []models.PlexMediaData{},
							},
						},
					},
				},
			},
		},
	}

	s.mock.ExpectExec(`INSERT INTO Shows`).WillReturnResult(sqlmock.NewResult(1, 1))
	s.mock.ExpectExec(`INSERT INTO Seasons`).WillReturnResult(sqlmock.NewResult(1, 1))
	s.mock.ExpectExec(`INSERT INTO Episodes`).WillReturnResult(sqlmock.NewResult(1, 1))
	s.mock.ExpectExec(`INSERT INTO Episodes`).WillReturnResult(sqlmock.NewResult(1, 1))

	s.repo.UpsertShows(context.Background(), lib)

	s.NoError(s.mock.ExpectationsWereMet())
}
