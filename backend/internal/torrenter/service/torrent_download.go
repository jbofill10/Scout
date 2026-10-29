package service

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jbofill10/scout/backend/pkg/telemetry"

	qbittorrent "github.com/autobrr/go-qbittorrent"
	"github.com/google/uuid"
	"golift.io/starr/prowlarr"
)

// clearActiveTorrent drops the in-flight record for a torrent that will never be
// processed. Best-effort: a stale row costs one wasted resume attempt on the next
// boot, which is cheaper than failing the monitor over it.
func (q *QbittHandler) clearActiveTorrent(ctx context.Context, infoHash string) {
	if err := q.repo.DeleteActiveTorrent(ctx, infoHash); err != nil {
		q.logger.ErrorContext(ctx, "Failed to clear in-flight torrent record",
			telemetry.WithTraceContext(ctx, "error", err.Error(), "torrent_hash", infoHash)...)
	}
}

func (q *QbittHandler) downloadTorrent(ctx context.Context, torrent *prowlarr.Search) (string, string, error) {
	// Generate UUID for torrent tracking
	trackingUUID := uuid.New().String()

	q.logger.InfoContext(ctx, "Downloading torrent",
		"filename",
		torrent.FileName,
		"hash", torrent.InfoHash,
		"guid", torrent.GUID,
		"tracking_uuid", trackingUUID,
		"torrent_meta", torrent)

	torrentSavePath := baseSavePath + "/" + torrent.Title
	q.logger.InfoContext(ctx, "Torrent save path", "path", torrentSavePath)

	if err := os.Mkdir(torrentSavePath, 0777); err != nil && !os.IsExist(err) {
		q.logger.ErrorContext(ctx, "Error creating directory", "value", torrentSavePath, "error", err)
		return "", "", err
	}

	// Prepare torrent add options using new library
	addOpts := qbittorrent.TorrentAddOptions{
		SavePath: torrentSavePath,
		Category: scoutTag,
		Tags:     trackingUUID,
	}
	options := addOpts.Prepare()

	var url string // If downloadURL is empty, that means we have a magnet
	if torrent.DownloadURL == "" {
		url = torrent.GUID
	} else {
		url = torrent.DownloadURL
	}

	if err := q.c.AddTorrentFromUrlCtx(ctx, url, options); err != nil {
		q.logger.ErrorContext(ctx, "Error downloading torrent", "error", err)
		return "", "", err
	}

	// If this was a .torrent file (not a magnet), resolve the InfoHash from qBittorrent
	if torrent.InfoHash == "" {
		q.logger.InfoContext(ctx, "Torrent file detected, resolving InfoHash from qBittorrent",
			"torrent_title", torrent.Title,
			"download_url", torrent.DownloadURL,
			"prowlarr_hash", torrent.InfoHash,
			"tracking_uuid", trackingUUID)

		resolvedHash, err := q.resolveInfoHashFromQbittorrent(ctx, trackingUUID)
		if err != nil {
			q.logger.ErrorContext(ctx, "Failed to resolve InfoHash from qBittorrent via UUID tag",
				"error", err,
				"torrent_title", torrent.Title,
				"tracking_uuid", trackingUUID)
			return "", "", fmt.Errorf("failed to resolve InfoHash via UUID tag for torrent '%s': %w", torrent.Title, err)
		}

		q.logger.InfoContext(ctx, "Successfully resolved InfoHash for .torrent file",
			"torrent_title", torrent.Title,
			"resolved_hash", resolvedHash,
			"tracking_uuid", trackingUUID)
		return resolvedHash, trackingUUID, nil
	}

	// Magnet link - InfoHash already available from Prowlarr
	q.logger.InfoContext(ctx, "Magnet link detected, using InfoHash from GUID",
		"torrent_title", torrent.Title,
		"infohash", torrent.InfoHash)
	return torrent.InfoHash, trackingUUID, nil
}

func (q *QbittHandler) queryQbittorrentWithRetries(
	ctx context.Context,
	filter qbittorrent.TorrentFilterOptions,
	maxRetries int,
	retryDelay time.Duration) ([]qbittorrent.Torrent, error) {
	var torrents []qbittorrent.Torrent
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		q.logger.InfoContext(ctx, "Querying qBittorrent",
			telemetry.WithTraceContext(ctx,
				"attempt", attempt,
				"max_retries", maxRetries,
				"filter_category", filter.Category,
				"filter_hashes", filter.Hashes)...)

		torrents, lastErr = q.c.GetTorrentsCtx(ctx, filter)
		if lastErr == nil {
			if len(torrents) > 0 {
				q.logger.InfoContext(ctx, "Successfully queried qBittorrent",
					telemetry.WithTraceContext(ctx,
						"attempt", attempt,
						"torrents", torrents,
						"torrents_returned", len(torrents))...)
				return torrents, nil
			}

			// API succeeded but no torrents found - retry
			q.logger.InfoContext(ctx, "qBittorrent query returned empty results",
				telemetry.WithTraceContext(ctx,
					"attempt", attempt,
					"max_retries", maxRetries,
					"filter_category", filter.Category,
					"filter_tag", filter.Tag)...)
		} else {
			// API error - retry
			q.logger.WarnContext(ctx, "Failed to query qBittorrent",
				telemetry.WithTraceContext(ctx,
					"attempt", attempt,
					"max_retries", maxRetries,
					"error", lastErr.Error())...)
		}

		if attempt < maxRetries {
			q.logger.InfoContext(ctx, "Retrying qBittorrent query",
				telemetry.WithTraceContext(ctx,
					"retry_delay", retryDelay.String(),
					"next_attempt", attempt+1)...)
			time.Sleep(retryDelay)
		}
	}

	// All retries exhausted
	if lastErr != nil {
		q.logger.ErrorContext(ctx, "Max retries exhausted querying qBittorrent",
			telemetry.WithTraceContext(ctx,
				"max_retries", maxRetries,
				"last_error", lastErr.Error())...)
		return nil, fmt.Errorf("failed to query qBittorrent after %d attempts: %w", maxRetries, lastErr)
	}

	// API succeeded but no torrents found after all retries
	q.logger.ErrorContext(ctx, "Max retries exhausted querying qBittorrent",
		telemetry.WithTraceContext(ctx,
			"max_retries", maxRetries,
			"reason", "empty results")...)
	return nil, fmt.Errorf("no torrents found in qBittorrent after %d attempts with filter: category=%s, tag=%s",
		maxRetries, filter.Category, filter.Tag)
}

func (q *QbittHandler) resolveInfoHashFromQbittorrent(ctx context.Context, trackingUUID string) (string, error) {
	q.logger.InfoContext(ctx, "Resolving InfoHash from qBittorrent", "tracking_uuid", trackingUUID)

	// Query torrent by UUID tag with retries
	filter := qbittorrent.TorrentFilterOptions{
		Category: scoutTag,
		Tag:      trackingUUID,
	}

	torrents, err := q.queryQbittorrentWithRetries(ctx, filter, 10, 5*time.Second)
	if err != nil {
		return "", fmt.Errorf("failed to resolve hash from qBittorrent: %w", err)
	}

	if len(torrents) == 0 {
		q.logger.WarnContext(ctx, "Could not find torrent in qBittorrent with UUID tag",
			"tracking_uuid", trackingUUID)
		return "", fmt.Errorf("torrent not found in qBittorrent with UUID: %s", trackingUUID)
	}

	// Should only be one torrent with this unique UUID tag
	torrent := torrents[0]
	q.logger.InfoContext(ctx, "Successfully resolved InfoHash from qBittorrent",
		"tracking_uuid", trackingUUID,
		"qbittorrent_name", torrent.Name,
		"resolved_hash", torrent.Hash)

	return torrent.Hash, nil
}

// RemoveUUIDTag removes the UUID tracking tag from a torrent after processing is complete
func (q *QbittHandler) RemoveUUIDTag(ctx context.Context, hash string, uuid string) error {
	q.logger.InfoContext(ctx, "Removing UUID tag from torrent",
		"hash", hash,
		"uuid", uuid)

	err := q.c.RemoveTagsCtx(ctx, []string{hash}, uuid)
	if err != nil {
		q.logger.WarnContext(ctx, "Failed to remove UUID tag from torrent",
			"hash", hash,
			"uuid", uuid,
			"error", err)
		return err
	}

	q.logger.InfoContext(ctx, "Successfully removed UUID tag from torrent",
		"hash", hash,
		"uuid", uuid)
	return nil
}
