package interactors

import (
	"log"
	tvdb "shared/media"
	"torrenter/internal/models"
	"torrenter/internal/service"
)

type DownloadInteractor struct {
	qbitt  service.TorrentService
	mp     service.MediaProcessor
	repo   service.Repository
	logger *log.Logger
}

func NewDownloadInteractor(
	qbitt service.TorrentService,
	mp service.MediaProcessor,
	repo service.Repository,
	logger *log.Logger,
) *DownloadInteractor {
	return &DownloadInteractor{
		qbitt:  qbitt,
		mp:     mp,
		repo:   repo,
		logger: logger,
	}
}

// InitiateDownload orchestrates the torrent download process
func (i *DownloadInteractor) InitiateDownload(req *tvdb.Media) error {
	dlComplete := make(chan models.TorrentCompleteEvent, 1)

	err := i.qbitt.HandleDownload(req, dlComplete)
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
		i.logger.Printf("Error processing downloaded torrent: %v", err)
		if err2 := i.repo.UpdateDownloadHistoryStatus(event.Hash, "failure", err.Error()); err2 != nil {
			i.logger.Printf("Failed to update download history: %v", err2)
		}
		return
	}

	if err := i.repo.UpdateDownloadHistoryStatus(event.Hash, "success", ""); err != nil {
		i.logger.Printf("Failed to update download history: %v", err)
	}
}
