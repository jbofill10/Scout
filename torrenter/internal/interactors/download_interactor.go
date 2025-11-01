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

	go i.handleDownloadCompletion(ctx, dlComplete)

	return nil
}

// handleDownloadCompletion processes the completed torrent
func (i *DownloadInteractor) handleDownloadCompletion(ctx context.Context, dlComplete <-chan models.TorrentCompleteEvent) {
	event := <-dlComplete

	// Create a new background context since the original HTTP request context may be canceled
	// by the time the torrent completes downloading
	bgCtx := context.Background()

	if err := i.mp.ProcessDownloadedTorrent(bgCtx, &event); err != nil {
		i.logger.ErrorContext(bgCtx, "Error processing downloaded torrent", "error", err)
		if err2 := i.repo.UpdateDownloadHistoryStatus(bgCtx, event.Hash, "failure", err.Error()); err2 != nil {
			i.logger.ErrorContext(bgCtx, "Failed to update download history", "error", err2)
		}
		return
	}

	if err := i.repo.UpdateDownloadHistoryStatus(bgCtx, event.Hash, "success", ""); err != nil {
		i.logger.ErrorContext(bgCtx, "Failed to update download history", "error", err)
	}
}
