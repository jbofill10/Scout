package interactors

import (
	"context"
	"strconv"
	"time"

	"github.com/jbofill10/scout/backend/internal/webserver/repository"
	"github.com/jbofill10/scout/backend/pkg/dlstatus"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/telemetry"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// dueSiblingWindow is how far ahead of a due row the other rows of the same
// show are pulled in. It covers rows re-armed in one reconciler pass (seconds
// apart) and same-day air dates.
const dueSiblingWindow = 15 * time.Minute

// processDueBatch dispatches a due row together with the show's other rows
// that are due within dueSiblingWindow, so a pack can be tried for them as a
// set. Movies, episode-less rows and rows with no siblings go through
// processDueItem unchanged.
func (i *DownloadInteractor) processDueBatch(item repository.DueItem) {
	if item.Media.Category != "series" || len(item.Media.Metadata.Episodes) == 0 {
		i.processDueItem(item)
		return
	}
	ctx := context.Background()
	siblings, err := i.repo.ClaimDueSiblings(ctx, item.Media.Id, item.ID, time.Now().Add(dueSiblingWindow))
	if err != nil {
		i.logger.ErrorContext(ctx, "Failed to claim sibling rows, dispatching row alone",
			"error", err, "media", item.Media.Name, "id", item.ID)
		i.processDueItem(item)
		return
	}
	for _, s := range siblings {
		if i.scheduler != nil {
			i.scheduler.CancelTimer(s.ID)
		}
	}
	if len(siblings) == 0 {
		i.processDueItem(item)
		return
	}
	i.processDueGroup(append([]repository.DueItem{item}, siblings...))
}

// processDueGroup dispatches the due rows of one show: a pack-only request for
// all their episodes first (when packs are on), then one request per episode
// the pack did not cover. Each row is settled from its own episode's answer.
func (i *DownloadInteractor) processDueGroup(items []repository.DueItem) {
	media := items[0].Media
	ctx, span := tracer.Start(context.Background(), "scheduled_download_group",
		trace.WithAttributes(
			attribute.String("media.name", media.Name),
			attribute.String("media.id", media.Id),
			attribute.Int("row.count", len(items)),
		),
	)
	defer span.End()
	traceID, spanID := telemetry.GetTraceSpanIDs(ctx)

	if items[0].Attempts == 0 && media.Source != "" && media.Source != "manual" {
		media.Source = "scheduled"
	}

	rowsByEpisode := make(map[string][]repository.DueItem, len(items))
	media.Metadata.Episodes = nil
	for _, row := range items {
		if len(row.Media.Metadata.Episodes) == 0 {
			continue
		}
		ep := row.Media.Metadata.Episodes[0]
		key := strconv.Itoa(ep.Id)
		if _, seen := rowsByEpisode[key]; !seen {
			media.Metadata.Episodes = append(media.Metadata.Episodes, ep)
		}
		rowsByEpisode[key] = append(rowsByEpisode[key], row)
	}

	resp, rest, attempted := i.packFirstRequest(ctx, media)
	packed := 0
	if attempted {
		for idx := range resp.Results {
			r := resp.Results[idx]
			if r.Code == dlstatus.CodeNoPack {
				continue
			}
			rows := rowsByEpisode[r.TvdbID]
			if len(rows) == 0 {
				continue
			}
			var failed *dlstatus.EpisodeResult
			if r.Outcome == dlstatus.OutcomeDownloading || r.Outcome == dlstatus.OutcomeExists {
				packed++
			} else {
				failed = &r
			}
			for _, row := range rows {
				i.settleDueRow(ctx, row, failed, traceID, spanID)
			}
		}
	}

	for _, ep := range rest {
		single := media
		single.PackOnly = false
		single.Metadata.Episodes = []tvdb.Episode{ep}
		eresp, err := i.torrenterClient.Download(ctx, single)
		if err != nil {
			i.logger.ErrorContext(ctx, "Download dispatch error", "error", err, "media", media.Name)
		}
		failed := failedResult(eresp.Results)
		for _, row := range rowsByEpisode[strconv.Itoa(ep.Id)] {
			i.settleDueRow(ctx, row, failed, traceID, spanID)
		}
	}

	i.logger.InfoContext(ctx, "Due rows dispatched together", "media", media.Name, "rows", len(items),
		"packed", packed, "per_episode", len(rest))
}

// airedAt is when the row's content aired: the newest episode's air date for a
// series, the first-aired date for a movie; zero when unknown or unparseable.
func airedAt(media tvdb.Media) time.Time {
	var latest time.Time
	for _, ep := range media.Metadata.Episodes {
		if t, err := time.Parse("2006-01-02", ep.Aired); err == nil && t.After(latest) {
			latest = t
		}
	}
	if len(media.Metadata.Episodes) > 0 {
		return latest
	}
	if t, err := time.Parse("2006-01-02", media.Metadata.FirstAired); err == nil {
		return t
	}
	return time.Time{}
}
