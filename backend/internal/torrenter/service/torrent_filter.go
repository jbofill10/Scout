package service

import (
	"context"
	"strconv"

	"github.com/jbofill10/scout/backend/pkg/dlstatus"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/notifications"
	"github.com/jbofill10/scout/backend/pkg/telemetry"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// filterExistingEpisodes checks which episodes already exist in Plex and returns only those that need downloading.
// For episodes that already exist, it updates their notifications to "completed" status and returns an
// EpisodeResult with Outcome "exists" so callers can report them in the structured download response.
func (q *QbittHandler) filterExistingEpisodes(ctx context.Context, req *tvdb.Media) ([]tvdb.Episode, []dlstatus.EpisodeResult, error) {
	ctx, span := torrentTracer.Start(ctx, "filterExistingEpisodes")
	defer span.End()

	span.SetAttributes(
		attribute.String("show_name", req.Name),
		attribute.String("tvdb_id", req.Id),
		attribute.Int("total_episodes", len(req.Metadata.Episodes)),
	)

	episodesToDownload := []tvdb.Episode{}
	existsResults := []dlstatus.EpisodeResult{}
	for _, episode := range req.Metadata.Episodes {
		exists, err := q.repo.EpisodeExistsByTvdbId(ctx, req.Id, episode.SeasonNumber, episode.Number)
		if err != nil {
			q.logger.WarnContext(ctx, "Failed to check episode existence by TVDB ID",
				telemetry.WithTraceContext(ctx, "tvdb_id", req.Id, "season", episode.SeasonNumber, "episode", episode.Number, "error", err.Error())...)
		}

		if exists {
			q.logger.InfoContext(ctx, "Episode already exists in Plex, skipping",
				telemetry.WithTraceContext(ctx, "name", req.Name, "season", episode.SeasonNumber, "episode", episode.Number)...)
			// Update notification to completed with "already exists" reason
			q.updateNotificationStatus(ctx, strconv.Itoa(episode.Id), "Already exists in Plex library", notifications.StatusCompleted)
			existsResults = append(existsResults, dlstatus.EpisodeResult{
				TvdbID:  strconv.Itoa(episode.Id),
				Season:  episode.SeasonNumber,
				Episode: episode.Number,
				Outcome: dlstatus.OutcomeExists,
			})
			continue
		}

		episodesToDownload = append(episodesToDownload, episode)
	}

	episodesFiltered := len(req.Metadata.Episodes) - len(episodesToDownload)
	span.SetAttributes(
		attribute.Int("episodes_filtered", episodesFiltered),
		attribute.Int("episodes_to_download", len(episodesToDownload)),
	)
	span.SetStatus(codes.Ok, "Episode filtering complete")

	return episodesToDownload, existsResults, nil
}

// inFlightTvdbIDs returns the notification ids (episode id for series, media id
// for movies) of every torrent still recorded as in flight. A lookup failure
// yields an empty set so the caller searches as it always did; the worst case
// is re-adding a torrent qBittorrent already has.
func (q *QbittHandler) inFlightTvdbIDs(ctx context.Context) map[string]bool {
	ids := map[string]bool{}
	active, err := q.repo.GetActiveTorrents(ctx)
	if err != nil {
		q.logger.WarnContext(ctx, "Failed to load in-flight torrents, searching without that check",
			telemetry.WithTraceContext(ctx, "error", err.Error())...)
		return ids
	}
	for _, at := range active {
		if at.Strategy != nil && at.Strategy.EpisodeTvdbID != "" {
			ids[at.Strategy.EpisodeTvdbID] = true
		}
	}
	return ids
}

// filterInFlightEpisodes drops episodes that already have a torrent in flight,
// which happens when a season is requested again or a retry row fires for an
// episode whose first pass did add a torrent but never got to report it. Those
// episodes are reported as downloading, and their notifications are put back
// on downloading in case the retry engine had moved them to "searching".
func (q *QbittHandler) filterInFlightEpisodes(ctx context.Context, req *tvdb.Media, episodes []tvdb.Episode) ([]tvdb.Episode, []dlstatus.EpisodeResult) {
	if len(episodes) == 0 {
		return episodes, nil
	}
	inFlight := q.inFlightTvdbIDs(ctx)
	if len(inFlight) == 0 {
		return episodes, nil
	}

	toDownload := make([]tvdb.Episode, 0, len(episodes))
	skipped := []dlstatus.EpisodeResult{}
	for _, episode := range episodes {
		id := strconv.Itoa(episode.Id)
		if !inFlight[id] {
			toDownload = append(toDownload, episode)
			continue
		}
		q.logger.InfoContext(ctx, "Episode already has a torrent in flight, skipping",
			telemetry.WithTraceContext(ctx, "name", req.Name, "season", episode.SeasonNumber, "episode", episode.Number)...)
		q.updateNotificationStatus(ctx, id, "", notifications.StatusDownloading)
		skipped = append(skipped, dlstatus.EpisodeResult{
			TvdbID:  id,
			Season:  episode.SeasonNumber,
			Episode: episode.Number,
			Outcome: dlstatus.OutcomeDownloading,
		})
	}
	return toDownload, skipped
}
