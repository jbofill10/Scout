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
	dlComplete := make(chan models.TorrentCompleteEvent, 1)

	err := i.qbitt.HandleDownload(ctx, req, dlComplete)
	if err != nil {
		return err
	}

	// Spawn goroutine to handle download completion
	go i.handleDownloadCompletion(dlComplete)

	return nil
}

// handleDownloadCompletion processes the completed torrent
func (i *DownloadInteractor) handleDownloadCompletion(dlComplete <-chan models.TorrentCompleteEvent) {
	event := <-dlComplete

	if err := i.mp.ProcessDownloadedTorrent(&event); err != nil {
		i.logger.Error("Error processing downloaded torrent", "error", err)
		if err2 := i.repo.UpdateDownloadHistoryStatus(event.Hash, "failure", err.Error()); err2 != nil {
			i.logger.Error("Failed to update download history", "error", err2)
		}
		return
	}

	if err := i.repo.UpdateDownloadHistoryStatus(event.Hash, "success", ""); err != nil {
		i.logger.Error("Failed to update download history", "error", err)
	}
}
