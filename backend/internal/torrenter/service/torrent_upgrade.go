package service

import (
	"context"
	"errors"
	"sync"

	"go.opentelemetry.io/otel/attribute"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	"github.com/jbofill10/scout/backend/internal/torrenter/searchgate"
	"github.com/jbofill10/scout/backend/pkg/dlstatus"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

// DownloadUpgrade searches for a better copy of an item that is already in
// the library and downloads it through the normal pipeline. The quality
// window is above the current copy (minTier) and at most the cutoff
// (maxTier); the strategies carry the upgrade info so the completion path
// removes the old files. Unlike HandleDownload it does not skip items that
// exist in Plex, which is the whole point. It owns closing done.
func (q *QbittHandler) DownloadUpgrade(
	ctx context.Context,
	media *tvdb.Media,
	episode *tvdb.Episode,
	info models.UpgradeInfo,
	minTier, maxTier int,
	done chan<- models.TorrentCompleteEvent,
) (dlstatus.EpisodeResult, error) {
	var strategies []*models.SearchStrategy
	if episode != nil {
		strategies = q.createSearchStrategy(media, episode)
	} else {
		strategies = q.createMovieSearchStrategy(media)
	}
	if len(strategies) == 0 {
		close(done)
		return dlstatus.EpisodeResult{}, errors.New("no search strategy for item")
	}
	for _, ss := range strategies {
		ss.MinTier, ss.MaxTier = minTier, maxTier
		u := info
		ss.Upgrade = &u
	}
	itemID := strategies[0].EpisodeTvdbID

	if q.inFlightTvdbIDs(ctx)[itemID] {
		close(done)
		q.logger.InfoContext(ctx, "Upgrade skipped: a torrent for the item is already in flight",
			telemetry.WithTraceContext(ctx, "media", media.Name, "tvdb_id", itemID)...)
		return dlstatus.EpisodeResult{TvdbID: itemID, Outcome: dlstatus.OutcomeDownloading}, nil
	}

	ctx = searchgate.WithRequest(ctx, searchgate.Upgrade, searchLabel(media, strategies[0]))
	ctx, span := torrentTracer.Start(ctx, "upgradeItem")
	defer span.End()
	span.SetAttributes(
		attribute.String("media_name", media.Name),
		attribute.String("tvdb_id", itemID),
		attribute.String("from_quality", info.FromQuality),
		attribute.Int("min_tier", minTier),
		attribute.Int("max_tier", maxTier),
	)

	var wg sync.WaitGroup
	result := q.processEpisodeDownload(ctx, 0, strategies, media, done, &wg)
	go func() {
		wg.Wait()
		close(done)
	}()
	return result, nil
}
