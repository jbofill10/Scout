package interactors

import (
	"context"
	"log/slog"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	"github.com/jbofill10/scout/backend/internal/torrenter/service"

	"go.opentelemetry.io/otel/trace"
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
	// Process all events until the channel is closed
	for event := range dlComplete {
		// Create a new background context with the span context from the event
		// This allows us to continue the trace from the search strategy that initiated the download
		eventCtx := trace.ContextWithSpanContext(context.Background(), event.SpanContext)

		i.logger.InfoContext(eventCtx, "Processing completed torrent", "hash", event.Hash)

		if err := i.mp.ProcessDownloadedTorrent(eventCtx, &event); err != nil {
			i.logger.ErrorContext(eventCtx, "Error processing downloaded torrent", "error", err, "hash", event.Hash)
			if err2 := i.repo.UpdateDownloadHistoryStatus(eventCtx, event.Hash, "failure", err.Error()); err2 != nil {
				i.logger.ErrorContext(eventCtx, "Failed to update download history", "error", err2)
			}
			continue
		}

		if err := i.repo.UpdateDownloadHistoryStatus(eventCtx, event.Hash, "success", ""); err != nil {
			i.logger.ErrorContext(eventCtx, "Failed to update download history", "error", err, "hash", event.Hash)
		}

		// Remove UUID tracking tag to prevent tag bloat
		if err := i.qbitt.RemoveUUIDTag(eventCtx, event.Hash, event.UUID); err != nil {
			i.logger.WarnContext(eventCtx, "Failed to remove UUID tag (non-fatal)", "error", err, "hash", event.Hash, "uuid", event.UUID)
		}
	}

	i.logger.InfoContext(ctx, "Download completion handler exiting")
}
