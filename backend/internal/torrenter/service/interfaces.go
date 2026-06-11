package service

import (
	"context"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	"github.com/jbofill10/scout/backend/pkg/dlstatus"
	"github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/notifications"
)

// TorrentService handles torrent search and download operations
type TorrentService interface {
	HandleDownload(ctx context.Context, req *media.Media, done chan<- models.TorrentCompleteEvent) ([]dlstatus.EpisodeResult, error)
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
}

// FileSystem defines file system operations
type FileSystem interface {
	HardLink(sourcePath, destPath string) error
	MkDir(path string) error
	ReadDir(path string) ([]string, error)
}

// PlexService handles Plex library operations
type PlexService interface {
	SyncPlexLibrary(ctx context.Context) error
}
