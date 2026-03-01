package repository

import (
	"bytes"
	"context"
	"database/sql"
	"log/slog"
	"testing"
	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	"github.com/jbofill10/scout/backend/pkg/media"

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
	rows := sqlmock.NewRows([]string{"id", "type", "path", "preferred", "section"}).
		AddRow(1, "show", "/data/shows", 1, 1)

	s.mock.ExpectQuery(`SELECT id, type, path, preferred, section FROM Libraries`).
		WithArgs("show").
		WillReturnRows(rows)

	lib, err := s.repo.GetPreferredLibrary(context.Background(), "show")

	s.NoError(err)
	s.Equal("/data/shows", lib.Path)
	s.Equal("show", lib.Type)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *RepoTestSuite) TestGetPreferredLibrary_NotFound() {
	s.mock.ExpectQuery(`SELECT id, type, path, preferred, section FROM Libraries`).
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
				Id:            "1",
				Title:         "Test Movie",
				Year:          2023,
				Thumb:         "/thumb.jpg",
				Art:           "/art.jpg",
				TvdbId:        "123456",
				BaseDirectory: "/data/movies",
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

	// Expect movie insert with base_directory - uses ExecContext (not Query)
	s.mock.ExpectExec(`INSERT INTO Movies`).
		WithArgs("1", "Test Movie", 2023, "/thumb.jpg", "/art.jpg", "123456", "/data/movies").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Expect movie media insert - uses movie's Plex ID ("1") as parentId
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
				Id:            "1",
				Title:         "Test Show",
				ShowMeta:      "/shows/1",
				Thumb:         "/thumb.jpg",
				TvdbId:        "123",
				BaseDirectory: "/data/shows",
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

	// Expect show insert with base_directory - uses ExecContext (not Query)
	s.mock.ExpectExec(`INSERT INTO Shows`).
		WithArgs("1", "Test Show", "/shows/1", "/thumb.jpg", "123", "/data/shows").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Expect season insert - uses show's Plex ID ("1") as parentId
	s.mock.ExpectExec(`INSERT INTO Seasons`).
		WithArgs("s1", "1", "/shows/1/season/1", 1, "season123").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Expect episode insert - uses season's Plex ID ("s1") as parentId
	s.mock.ExpectExec(`INSERT INTO Episodes`).
		WithArgs("e1", "s1", "/shows/1/season/1/episode/1", 1, "ep123").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Expect episode media insert - uses episode's Plex ID ("e1") as parentId
	s.mock.ExpectExec(`INSERT INTO EpisodeMedia`).
		WithArgs("e1", "1080p", "/data/shows/test.mkv").
		WillReturnResult(sqlmock.NewResult(1, 1))

	s.repo.UpsertShows(context.Background(), lib)

	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *RepoTestSuite) TestUpsertShows_MultipleEpisodes() {
	lib := &models.PlexShowLibraryData{
		Shows: []models.PlexShowData{
			{
				Id:            "1",
				Title:         "Test Show",
				ShowMeta:      "/shows/1",
				Thumb:         "/thumb.jpg",
				TvdbId:        "123",
				BaseDirectory: "/data/shows",
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
								Media:         []models.PlexMediaData{},
							},
							{
								Id:            "e2",
								EpisodeMeta:   "/shows/1/season/1/episode/2",
								EpisodeNumber: 2,
								TvdbId:        "ep456",
								Media:         []models.PlexMediaData{},
							},
						},
					},
				},
			},
		},
	}

	// Expect show insert with base_directory
	s.mock.ExpectExec(`INSERT INTO Shows`).
		WithArgs("1", "Test Show", "/shows/1", "/thumb.jpg", "123", "/data/shows").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Expect season insert
	s.mock.ExpectExec(`INSERT INTO Seasons`).
		WithArgs("s1", "1", "/shows/1/season/1", 1, "season123").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Expect first episode insert
	s.mock.ExpectExec(`INSERT INTO Episodes`).
		WithArgs("e1", "s1", "/shows/1/season/1/episode/1", 1, "ep123").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Expect second episode insert
	s.mock.ExpectExec(`INSERT INTO Episodes`).
		WithArgs("e2", "s1", "/shows/1/season/1/episode/2", 2, "ep456").
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.repo.UpsertShows(context.Background(), lib)

	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *RepoTestSuite) TestGetShowSeasonEpisodes_MultipleSeasons() {
	rows := sqlmock.NewRows([]string{"season_number", "episode_number", "tvdb_id"}).
		AddRow(1, 1, "ep100").
		AddRow(1, 2, "ep101").
		AddRow(1, 3, "ep102").
		AddRow(2, 1, "ep200").
		AddRow(2, 2, "ep201")

	s.mock.ExpectQuery(`SELECT s.season_number, e.episode_number`).
		WithArgs("123").
		WillReturnRows(rows)

	result, err := s.repo.GetShowSeasonEpisodes(context.Background(), "123")

	s.NoError(err)
	s.Len(result, 2)
	s.Equal([]media.EpisodeInfo{
		{EpisodeNum: 1, TvdbId: "ep100"},
		{EpisodeNum: 2, TvdbId: "ep101"},
		{EpisodeNum: 3, TvdbId: "ep102"},
	}, result[1])
	s.Equal([]media.EpisodeInfo{
		{EpisodeNum: 1, TvdbId: "ep200"},
		{EpisodeNum: 2, TvdbId: "ep201"},
	}, result[2])
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *RepoTestSuite) TestGetShowSeasonEpisodes_NoShow() {
	rows := sqlmock.NewRows([]string{"season_number", "episode_number", "tvdb_id"})

	s.mock.ExpectQuery(`SELECT s.season_number, e.episode_number`).
		WithArgs("nonexistent").
		WillReturnRows(rows)

	result, err := s.repo.GetShowSeasonEpisodes(context.Background(), "nonexistent")

	s.NoError(err)
	s.NotNil(result)
	s.Empty(result)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *RepoTestSuite) TestGetShowSeasonEpisodes_EmptyTvdbId() {
	result, err := s.repo.GetShowSeasonEpisodes(context.Background(), "")

	s.NoError(err)
	s.NotNil(result)
	s.Empty(result)
}

func (s *RepoTestSuite) TestGetShowSeasonEpisodes_QueryError() {
	s.mock.ExpectQuery(`SELECT s.season_number, e.episode_number`).
		WithArgs("123").
		WillReturnError(sql.ErrConnDone)

	result, err := s.repo.GetShowSeasonEpisodes(context.Background(), "123")

	s.Error(err)
	s.Nil(result)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *RepoTestSuite) TestMovieExistsByTvdbId_True() {
	rows := sqlmock.NewRows([]string{"exists"}).AddRow(true)

	s.mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs("movie123").
		WillReturnRows(rows)

	exists, err := s.repo.MovieExistsByTvdbId(context.Background(), "movie123")

	s.NoError(err)
	s.True(exists)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *RepoTestSuite) TestMovieExistsByTvdbId_False() {
	rows := sqlmock.NewRows([]string{"exists"}).AddRow(false)

	s.mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs("movie456").
		WillReturnRows(rows)

	exists, err := s.repo.MovieExistsByTvdbId(context.Background(), "movie456")

	s.NoError(err)
	s.False(exists)
	s.NoError(s.mock.ExpectationsWereMet())
}

func (s *RepoTestSuite) TestMovieExistsByTvdbId_EmptyTvdbId() {
	exists, err := s.repo.MovieExistsByTvdbId(context.Background(), "")

	s.NoError(err)
	s.False(exists)
}
