package interactors

import (
	"context"
	"log/slog"
	tvdb "shared/media"
	"torrenter/internal/models"
	"torrenter/internal/service"
)

type DownloadInteractor struct {
	qbitt  service.TorrentService
	mp     service.MediaProcessor
	repo   service.Repository
	logger *slog.Logger
}

func NewDownloadInteractor(
	qbitt service.TorrentService,
	mp service.MediaProcessor,
	repo service.Repository,
	logger *slog.Logger,
) *DownloadInteractor {
	return &DownloadInteractor{
		qbitt:  qbitt,
		mp:     mp,
		repo:   repo,
		logger: logger,
	}
}

// InitiateDownload orchestrates the torrent download process
func (i *DownloadInteractor) InitiateDownload(ctx context.Context, req *tvdb.Media) error {
	// Use a buffered channel sized for potential multiple episodes
	// TV shows can have multiple episodes, each spawning a monitoring goroutine
	dlComplete := make(chan models.TorrentCompleteEvent, 10)

	// Start the completion handler before initiating downloads
	// so it's ready to receive events
	go i.handleDownloadCompletion(ctx, dlComplete)

	err := i.qbitt.HandleDownload(ctx, req, dlComplete)
	if err != nil {
		// Close the channel to signal the goroutine to exit
		close(dlComplete)
		return err
	}

	// Note: We don't close the channel here because HandleDownload spawns
	// background goroutines that will send to it later. The channel should
	// be closed by HandleDownload or have a timeout mechanism.

	return nil
}

// handleDownloadCompletion processes the completed torrents
// This runs in a goroutine and handles multiple completion events until the channel is closed
func (i *DownloadInteractor) handleDownloadCompletion(ctx context.Context, dlComplete <-chan models.TorrentCompleteEvent) {
	// Create a new background context since the original HTTP request context may be canceled
	// by the time the torrent completes downloading
	bgCtx := context.Background()

	// Process all events until the channel is closed
	for event := range dlComplete {
		i.logger.InfoContext(bgCtx, "Processing completed torrent", "hash", event.Hash)

		if err := i.mp.ProcessDownloadedTorrent(bgCtx, &event); err != nil {
			i.logger.ErrorContext(bgCtx, "Error processing downloaded torrent", "error", err, "hash", event.Hash)
			if err2 := i.repo.UpdateDownloadHistoryStatus(bgCtx, event.Hash, "failure", err.Error()); err2 != nil {
				i.logger.ErrorContext(bgCtx, "Failed to update download history", "error", err2)
			}
			continue
		}

		if err := i.repo.UpdateDownloadHistoryStatus(bgCtx, event.Hash, "success", ""); err != nil {
			i.logger.ErrorContext(bgCtx, "Failed to update download history", "error", err, "hash", event.Hash)
		}
	}

	i.logger.InfoContext(bgCtx, "Download completion handler exiting")
}
