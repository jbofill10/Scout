package service

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"strconv"
	"sync"
	"time"
	"github.com/jbofill10/scout/backend/internal/torrenter/models"
)

var (
	commonFileExtensions = []string{".mkv", ".mp4", ".avi", ".mov", ".webm"}
)

// BaseDirectoryCache provides thread-safe in-memory caching of tvdb_id -> base_directory mappings
type BaseDirectoryCache struct {
	mu    sync.RWMutex
	cache map[string]string
}

func NewBaseDirectoryCache() *BaseDirectoryCache {
	return &BaseDirectoryCache{
		cache: make(map[string]string),
	}
}

type MediaProcessSvc struct {
	Logger   *slog.Logger
	repo     Repository
	fs       FileSystem
	dirCache *BaseDirectoryCache
}

func NewMediaProcessSvc(logger *slog.Logger, repo Repository, fs FileSystem) MediaProcessor {
	return &MediaProcessSvc{
		Logger:   logger,
		repo:     repo,
		fs:       fs,
		dirCache: NewBaseDirectoryCache(),
	}
}

// GetFromCache retrieves a cached base directory for a given tvdb_id
func (mp *MediaProcessSvc) GetFromCache(tvdbId string) (string, bool) {
	mp.dirCache.mu.RLock()
	defer mp.dirCache.mu.RUnlock()
	baseDir, exists := mp.dirCache.cache[tvdbId]
	return baseDir, exists
}

// SetInCache stores a base directory for a given tvdb_id
func (mp *MediaProcessSvc) SetInCache(tvdbId, baseDir string) {
	mp.dirCache.mu.Lock()
	defer mp.dirCache.mu.Unlock()
	mp.dirCache.cache[tvdbId] = baseDir
	mp.Logger.Debug("Cached base directory", "tvdb_id", tvdbId, "base_dir", baseDir)
}

// InvalidateCache removes cache entries for tvdb_ids that now exist in the database
func (mp *MediaProcessSvc) InvalidateCache(ctx context.Context) {
	mp.dirCache.mu.Lock()
	defer mp.dirCache.mu.Unlock()

	for tvdbId := range mp.dirCache.cache {
		// Check if base_directory now exists in Shows or Movies
		_, showErr := mp.repo.GetShowBaseDirectory(ctx, tvdbId)
		_, movieErr := mp.repo.GetMovieBaseDirectory(ctx, tvdbId)

		// If either query succeeded, the base_directory is now in DB - remove from cache
		if showErr == nil || movieErr == nil {
			delete(mp.dirCache.cache, tvdbId)
			mp.Logger.Info("Invalidated cache entry", "tvdb_id", tvdbId)
		}
	}
}

// StartCacheCleanup runs a goroutine that periodically invalidates the cache every 30 minutes
func (mp *MediaProcessSvc) StartCacheCleanup(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Minute)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				mp.Logger.Info("Cache cleanup goroutine stopping")
				return
			case <-ticker.C:
				mp.Logger.Info("Running periodic cache cleanup")
				mp.InvalidateCache(ctx)
			}
		}
	}()
	mp.Logger.Info("Started cache cleanup goroutine (runs every 30 minutes)")
}

func (mp *MediaProcessSvc) ProcessDownloadedTorrent(ctx context.Context, media *models.TorrentCompleteEvent) error {
	mp.Logger.InfoContext(ctx, "Processing downloaded torrent", "media", media.Req.MediaName)

	// Create DownloadRequest from SearchStrategy
	req := &models.DownloadRequest{
		MediaName:   media.Req.MediaName,
		ReleaseYear: media.Req.ReleaseYear, // TODO: add to SearchStrategy
	}
	if media.Req.Episode == 0 {
		req.MediaType = "movie"
		req.Season = ""
		req.Episode = ""
	} else {
		req.MediaType = "show"
		// Always use seasonal format from EpisodeMeta (not SearchStrategy.Episode which could be absolute)
		if media.Req.EpisodeMeta != nil {
			req.Season = fmt.Sprint(media.Req.EpisodeMeta.SeasonNumber)
			req.Episode = fmt.Sprint(media.Req.EpisodeMeta.Number)
		} else {
			// Fallback to SearchStrategy values if EpisodeMeta is not available
			req.Season = fmt.Sprint(media.Req.Season)
			req.Episode = fmt.Sprint(media.Req.Episode)
		}
	}

	// Get original filename from torrent download
	originalFileName, err := mp.getFile(media.SavePath)
	if err != nil {
		return fmt.Errorf("failed to get file: %v", err)
	}

	// For shows, construct Plex-friendly filename (ShowName - S##E##.ext)
	// For movies, keep the original filename
	var fileName string
	if media.Req.Episode != 0 && media.Req.EpisodeMeta != nil {
		// Show: Always use seasonal format regardless of search strategy
		fileName = mp.constructPlexFilename(media.Req.MediaName, media.Req.EpisodeMeta, originalFileName)
		mp.Logger.InfoContext(ctx, "Constructed Plex filename",
			"original", originalFileName,
			"plex_format", fileName,
			"season", media.Req.EpisodeMeta.SeasonNumber,
			"episode", media.Req.EpisodeMeta.Number,
		)
	} else {
		// Movie: Keep original filename
		fileName = originalFileName
	}

	isShow := req.IsShow()

	var baseDir string
	var source string

	if media.Req.TvdbId != "" {
		// Check cache first
		cachedDir, found := mp.GetFromCache(media.Req.TvdbId)
		if found {
			baseDir = cachedDir
			source = "cache"
			mp.Logger.InfoContext(ctx, "Found base directory in cache", "tvdb_id", media.Req.TvdbId, "base_dir", baseDir)
		} else {
			// not in cache, query database
			var err error
			if isShow {
				baseDir, err = mp.repo.GetShowBaseDirectory(ctx, media.Req.TvdbId)
			} else {
				baseDir, err = mp.repo.GetMovieBaseDirectory(ctx, media.Req.TvdbId)
			}
			if err == nil && baseDir != "" {
				source = "database"
				mp.Logger.InfoContext(ctx, "Found base directory in database", "tvdb_id", media.Req.TvdbId, "base_dir", baseDir)
				// Cache the DB result for future use
				mp.SetInCache(media.Req.TvdbId, baseDir)
			}
		}
	}

	var fullSavePath string
	if baseDir != "" {
		// Use existing base directory (from cache or DB)
		if isShow {
			season, _ := strconv.Atoi(req.Season)
			seasonPath := fmt.Sprintf("Season %s", standardizeNumber(season))
			fullSavePath = filepath.Join(baseDir, seasonPath, fileName)
		} else {
			fullSavePath = filepath.Join(baseDir, fileName)
		}
		mp.Logger.InfoContext(ctx, "Using base directory", "source", source, "base_dir", baseDir)
	} else {
		// 3. Construct new path and cache it
		libraryPath, err := mp.getLibraryPath(ctx, isShow)
		if err != nil {
			return fmt.Errorf("failed to get library path: %v", err)
		}

		// Construct base directory (show/movie folder only, no season/episode)
		if isShow {
			baseDir = filepath.Join(libraryPath.Path, req.MediaName)
			season, _ := strconv.Atoi(req.Season)
			seasonPath := fmt.Sprintf("Season %s", standardizeNumber(season))
			fullSavePath = filepath.Join(baseDir, seasonPath, fileName)
		} else {
			baseDir = filepath.Join(libraryPath.Path, req.MediaName+" ("+req.ReleaseYear+")")
			fullSavePath = filepath.Join(baseDir, fileName)
		}

		// Cache the base directory for consistency
		if media.Req.TvdbId != "" {
			mp.SetInCache(media.Req.TvdbId, baseDir)
		}

		mp.Logger.InfoContext(ctx, "Constructed new base directory and cached it", "base_dir", baseDir, "tvdb_id", media.Req.TvdbId)
	}

	// Create source path (where qBittorrent saved the file)
	sourcePath := filepath.Join(media.SavePath, originalFileName)

	// Extract target directory from full save path
	targetDir := filepath.Dir(fullSavePath)

	mp.Logger.InfoContext(ctx, "Preparing to create symbolic link", "source", sourcePath, "destination", fullSavePath)
	mp.Logger.InfoContext(ctx, "Creating target directory", "path", targetDir)

	// Create directory structure (includes Season folder for shows)
	mp.Logger.InfoContext(ctx, "Creating target directory", "path", targetDir)
	if err := mp.fs.MkDir(targetDir); err != nil {
		return fmt.Errorf("failed to create target directory %s: %w", targetDir, err)
	}

	// Create symbolic link from qBittorrent download to Plex library
	mp.Logger.InfoContext(ctx, "Creating symbolic link", "source", sourcePath, "destination", fullSavePath)
	if err := mp.fs.HardLink(sourcePath, fullSavePath); err != nil {
		mp.Logger.ErrorContext(ctx, "Failed to create symbolic link", "source", sourcePath, "destination", fullSavePath, "error", err)

		// Log failure to download history
		absoluteEpisode := 0
		if media.Req.EpisodeMeta != nil {
			absoluteEpisode = media.Req.EpisodeMeta.AbsoluteNumber
		}
		reason := fmt.Sprintf("symbolic link failed: %v", err)
		if historyErr := mp.repo.InsertDownloadHistory(ctx, media.Req.MediaName, media.Req.Season, media.Req.Episode, absoluteEpisode, media.Hash, "failure", reason); historyErr != nil {
			mp.Logger.ErrorContext(ctx, "Failed to insert download history", "error", historyErr)
		}

		return fmt.Errorf("failed to create symbolic link from %s to %s: %w", sourcePath, fullSavePath, err)
	}

	mp.Logger.InfoContext(ctx, "Successfully created symbolic link", "path", fullSavePath)

	return nil
}

// Formats numbers less than 10 to be prefixed with a zero
func standardizeNumber(num int) string {
	if num < 10 {
		return "0" + fmt.Sprintf("%d", num)
	}
	return fmt.Sprintf("%d", num)
}

// constructPlexFilename creates a Plex-friendly filename in the format "ShowName - S##E##.ext"
// Always uses seasonal format (S##E##) regardless of which search strategy found the torrent
func (mp *MediaProcessSvc) constructPlexFilename(mediaName string, episodeMeta *tvdb.Episode, originalFileName string) string {
	// Extract file extension from original filename
	ext := filepath.Ext(originalFileName)

	// Always use seasonal formatting for consistency
	return fmt.Sprintf("%s - S%sE%s%s",
		mediaName,
		standardizeNumber(episodeMeta.SeasonNumber),
		standardizeNumber(episodeMeta.Number),
		ext,
	)
}

func (mp *MediaProcessSvc) getFile(dirPath string) (string, error) {
	files, err := mp.fs.ReadDir(dirPath)
	if err != nil {
		return "", fmt.Errorf("failed to read directory %s: %w", dirPath, err)
	}

	var videoFiles []string
	for _, file := range files {
		for _, ext := range commonFileExtensions {
			if len(file) >= len(ext) && file[len(file)-len(ext):] == ext {
				videoFiles = append(videoFiles, file)
				break
			}
		}
	}

	if len(videoFiles) == 0 {
		return "", fmt.Errorf("no video files found in directory %s", dirPath)
	}

	if len(videoFiles) > 1 {
		mp.Logger.Warn("Multiple video files found in directory, using first", "directory", dirPath, "files", videoFiles)
	}

	return videoFiles[0], nil
}

func (mp *MediaProcessSvc) getLibraryPath(ctx context.Context, isShow bool) (models.PlexLibrary, error) {
	var mediaType string
	if isShow {
		mediaType = "show"
	} else {
		mediaType = "movie"
	}

	lib, err := mp.repo.GetPreferredLibrary(ctx, mediaType)
	if err != nil {
		return models.PlexLibrary{}, fmt.Errorf("failed to get preferred library: %w", err)
	}

	return lib, nil
}
