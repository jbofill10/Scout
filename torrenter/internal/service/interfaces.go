package service

import (
	"context"
	tvdb "shared/media"
	"torrenter/internal/models"
)

// TorrentService handles torrent search and download operations
type TorrentService interface {
	HandleDownload(ctx context.Context, req *tvdb.Media, done chan<- models.TorrentCompleteEvent) error
}

// MediaProcessor handles post-download processing
type MediaProcessor interface {
	ProcessDownloadedTorrent(ctx context.Context, media *models.TorrentCompleteEvent) error
}

// Repository defines database operations
type Repository interface {
	UpsertLibraries(ctx context.Context, libs models.PlexLibrariesResponse)
	UpsertMovies(ctx context.Context, movies models.PlexMovieLibraryData)
	UpsertShows(ctx context.Context, shows *models.PlexShowLibraryData)
	SetPreferredLibrary(ctx context.Context, id int, libType string) error
	GetPreferredLibrary(ctx context.Context, libType string) (models.PlexLibrary, error)
	GetLibraryByType(ctx context.Context, libType string) (int, error)
	EpisodeExistsByTvdbId(ctx context.Context, tvdbId string, season, episode int) (bool, error)
	MediaExists(ctx context.Context, id string) (bool, error)
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
	SyncPlexLibrary(ctx context.Context)
}
