package service

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"torrenter/internal/models"
)

var (
	commonFileExtensions = []string{".mkv", ".mp4", ".avi", ".mov", ".webm"}
)

type MediaProcessSvc struct {
	Logger *slog.Logger
	repo   Repository
	fs     FileSystem
}

func NewMediaProcessSvc(logger *slog.Logger, repo Repository, fs FileSystem) MediaProcessor {
	return &MediaProcessSvc{Logger: logger, repo: repo, fs: fs}
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
		req.Season = fmt.Sprint(media.Req.Season)
		req.Episode = fmt.Sprint(media.Req.Episode)
	}

	// Prepare the media path based on the request
	mediaPath := mp.prepMediaPath(req)

	// TODO: implement logic for movies
	isShow := req.IsShow()
	libraryPath, err := mp.getLibraryPath(ctx, isShow)
	if err != nil {
		return fmt.Errorf("failed to get library path: %v", err)
	}

	fileExt, err := mp.getFileExtension(media.SavePath)
	if err != nil {
		return fmt.Errorf("failed to get file extension: %v", err)
	}

	fullSavePath := fmt.Sprintf("%s/%s.%s", libraryPath.Path, mediaPath, fileExt)

	// TODO: Add hard-linking target file

	mp.Logger.InfoContext(ctx, "Full save path", "path", fullSavePath)
	mp.Logger.InfoContext(ctx, "Torrent processing completed", "media", media.Req.MediaName)

	return nil
}

func (mp *MediaProcessSvc) prepMediaPath(req *models.DownloadRequest) string {
	if req.IsShow() {
		season, _ := strconv.Atoi(req.Season)
		episode, _ := strconv.Atoi(req.Episode)
		return fmt.Sprintf("%s/Season %s/%s - S%sE%s", req.MediaName, standardizeNumber(season),
			req.MediaName, standardizeNumber(season), standardizeNumber(episode))
	} else {
		return req.MediaName + " (" + req.ReleaseYear + ")"
	}
}

// Formats numbers less than 10 to be prefixed with a zero
func standardizeNumber(num int) string {
	if num < 10 {
		return "0" + fmt.Sprintf("%d", num)
	}
	return fmt.Sprintf("%d", num)
}

func (mp *MediaProcessSvc) getFileExtension(fileName string) (string, error) {
	for _, ext := range commonFileExtensions {
		if len(fileName) >= len(ext) && fileName[len(fileName)-len(ext):] == ext {
			return ext, nil
		}
	}
	return "", fmt.Errorf("no valid file extension found for %s", fileName)
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
