package service

import (
	"context"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	"github.com/jbofill10/scout/backend/pkg/dlstatus"
	"github.com/jbofill10/scout/backend/pkg/library"
	"github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/notifications"
)

// TorrentService handles torrent search and download operations
type TorrentService interface {
	HandleDownload(ctx context.Context, req *media.Media, done chan<- models.TorrentCompleteEvent) ([]dlstatus.EpisodeResult, error)
	ResumeMonitors(ctx context.Context, done chan<- models.TorrentCompleteEvent) (int, error)
	RemoveUUIDTag(ctx context.Context, hash string, uuid string) error
}

// MediaProcessor handles post-download processing
type MediaProcessor interface {
	ProcessDownloadedTorrent(ctx context.Context, media *models.TorrentCompleteEvent) error
	InvalidateCache(ctx context.Context)
	StartCacheCleanup(ctx context.Context)
}

// Repository defines database operations
type Repository interface {
	// Embed shared notifications.Repository interface (Get, Create, Update)
	notifications.Repository

	// Plex library methods
	UpsertLibraries(ctx context.Context, libs models.PlexLibrariesResponse)
	UpsertMovies(ctx context.Context, movies models.PlexMovieLibraryData)
	UpsertShows(ctx context.Context, shows *models.PlexShowLibraryData)
	PruneMissingShows(ctx context.Context, shows *models.PlexShowLibraryData) error
	PruneMissingMovies(ctx context.Context, movies models.PlexMovieLibraryData) error
	SetPreferredLibrary(ctx context.Context, id int, libType string) error
	GetPreferredLibrary(ctx context.Context, libType string) (models.PlexLibrary, error)
	GetLibraryByType(ctx context.Context, libType string) (int, error)
	GetShowBaseDirectory(ctx context.Context, tvdbId string) (string, error)
	GetMovieBaseDirectory(ctx context.Context, tvdbId string) (string, error)
	EpisodeExistsByTvdbId(ctx context.Context, tvdbId string, season, episode int) (bool, error)
	MovieExistsByTvdbId(ctx context.Context, tvdbId string) (bool, error)
	MediaExists(ctx context.Context, id string) (bool, error)
	GetShowSeasonEpisodes(ctx context.Context, tvdbId string) (map[int][]media.EpisodeInfo, error)
	InsertDownloadHistory(ctx context.Context, mediaTitle string, season, episode, absoluteEpisode int, torrentHash, status, reason string) error
	UpdateDownloadHistoryStatus(ctx context.Context, torrentHash, status, reason string) error
	GetPreferredUploaders(ctx context.Context, mediaType string, isAnime bool) ([]string, error)

	// In-flight torrent tracking, so completion monitors survive a restart
	InsertActiveTorrent(ctx context.Context, at *models.ActiveTorrent) error
	DeleteActiveTorrent(ctx context.Context, infoHash string) error
	GetActiveTorrents(ctx context.Context) ([]*models.ActiveTorrent, error)

	// Library browsing methods
	GetAllShows(ctx context.Context) ([]library.LibraryShow, error)
	GetAllMovies(ctx context.Context) ([]library.LibraryMovie, error)
	GetShowEpisodesWithStatus(ctx context.Context, tvdbId string) ([]library.EpisodeWithStatus, error)
	UpsertTvdbEpisodes(ctx context.Context, seriesTvdbId string, episodes []library.TvdbEpisode) error
	GetTvdbMetadataStatus(ctx context.Context, tvdbId string) (tvdbCount int, plexCount int, err error)
}

// FileSystem defines file system operations
type FileSystem interface {
	HardLink(sourcePath, destPath string) error
	MkDir(path string) error
	WalkFiles(root string) ([]models.FileEntry, error)
}

// PlexService handles Plex library operations
type PlexService interface {
	SyncPlexLibrary(ctx context.Context) error
}
