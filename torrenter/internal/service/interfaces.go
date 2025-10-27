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
	ProcessDownloadedTorrent(media *models.TorrentCompleteEvent) error
}

// Repository defines database operations
type Repository interface {
	UpsertLibraries(libs models.PlexLibrariesResponse)
	UpsertMovies(movies models.PlexMovieLibraryData)
	UpsertShows(shows *models.PlexShowLibraryData)
	SetPreferredLibrary(id int, libType string) error
	GetPreferredLibrary(libType string) (models.PlexLibrary, error)
	GetLibraryByType(libType string) (int, error)
	EpisodeExistsByTvdbId(tvdbId string, season, episode int) (bool, error)
	EpisodeExists(showTitle string, season, episode int) (bool, error)
	MediaExists(id string) (bool, error)
	InsertDownloadHistory(mediaTitle string, season, episode, absoluteEpisode int, torrentHash, status, reason string) error
	UpdateDownloadHistoryStatus(torrentHash, status, reason string) error
	GetPreferredUploaders(mediaType string, isAnime bool) ([]string, error)
}

// FileSystem defines file system operations
type FileSystem interface {
	HardLink(sourcePath, destPath string) error
	MkDir(path string) error
}

// PlexService handles Plex library operations
type PlexService interface {
	SyncPlexLibrary()
}
