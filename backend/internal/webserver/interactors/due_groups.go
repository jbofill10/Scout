package interactors

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/jbofill10/scout/backend/internal/webserver/clients"
	"github.com/jbofill10/scout/backend/internal/webserver/repository"
	"github.com/jbofill10/scout/backend/pkg/dlstatus"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/notifications"
	"github.com/jbofill10/scout/backend/pkg/telemetry"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// dueSiblingWindow is how far ahead of a due row the other rows of the same
// show are pulled in. It covers rows re-armed in one reconciler pass (seconds
// apart) and same-day air dates.
const dueSiblingWindow = 15 * time.Minute

// torrenterRetryDelay is how long a row waits when the torrenter could not be
// reached. The service is restarting or down; the request was never tried, so
// the wait is short and costs no attempt.
const torrenterRetryDelay = 2 * time.Minute

// processDueBatch dispatches a due row together with the show's other rows
// that are due within dueSiblingWindow, so a pack can be tried for them as a
// set. Movies, episode-less rows and rows with no siblings go through
// processDueItem unchanged.
func (i *DownloadInteractor) processDueBatch(item repository.DueItem) {
	ctx := context.Background()
	promoteDueSource(&item)
	if item.Media.Category != "series" || len(item.Media.Metadata.Episodes) == 0 {
		i.processDueItem(ctx, item)
		return
	}
	siblings, err := i.repo.ClaimDueSiblings(ctx, item.Media.Id, item.ID, time.Now().Add(dueSiblingWindow))
	if err != nil {
		i.logger.ErrorContext(ctx, "Failed to claim sibling rows, dispatching row alone",
			"error", err, "media", item.Media.Name, "id", item.ID)
		i.processDueItem(ctx, item)
		return
	}
	for idx := range siblings {
		if i.scheduler != nil {
			i.scheduler.CancelTimer(siblings[idx].ID)
		}
		promoteDueSource(&siblings[idx])
	}
	if len(siblings) == 0 {
		i.processDueItem(ctx, item)
		return
	}
	i.processDueGroup(ctx, append([]repository.DueItem{item}, siblings...))
}

// promoteDueSource gives a row the scheduler dispatches on its first attempt
// the "scheduled" search priority: an episode sent on its air date is wanted
// now whoever set it up. Retries keep the priority of the request they came
// from, and a person's request stays ahead of everything.
func promoteDueSource(item *repository.DueItem) {
	if item.Attempts == 0 && item.Media.Source != "" && item.Media.Source != "manual" {
		item.Media.Source = "scheduled"
	}
}

// processDueGroup dispatches the rows of one show: a pack-only request for all
// their episodes first (when packs are on), then one request per episode the
// pack did not cover. Each row is settled from its own episode's answer. Rows
// still waiting their turn are kept fresh for ResetStaleQueued, and when the
// torrenter cannot be reached the rest of the group is deferred at once rather
// than asked one by one.
func (i *DownloadInteractor) processDueGroup(ctx context.Context, items []repository.DueItem) {
	media := items[0].Media
	ctx, span := tracer.Start(ctx, "scheduled_download_group",
		trace.WithAttributes(
			attribute.String("media.name", media.Name),
			attribute.String("media.id", media.Id),
			attribute.Int("row.count", len(items)),
		),
	)
	defer span.End()
	traceID, spanID := telemetry.GetTraceSpanIDs(ctx)

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

	for idx, ep := range rest {
		single := media
		single.PackOnly = false
		single.Metadata.Episodes = []tvdb.Episode{ep}
		eresp, err := i.torrenterClient.Download(ctx, single)
		if torrenterDown(err) {
			// Nothing else will get through either: hold this episode's rows
			// and every row still waiting for the torrenter's return.
			i.deferRows(ctx, rowsFor(rowsByEpisode, rest[idx:]), err)
			break
		}
		if err != nil {
			i.logger.ErrorContext(ctx, "Download dispatch error", "error", err, "media", media.Name)
		}
		failed := failedResult(eresp.Results)
		for _, row := range rowsByEpisode[strconv.Itoa(ep.Id)] {
			i.settleDueRow(ctx, row, failed, traceID, spanID)
		}
		i.touchWaiting(ctx, rowsFor(rowsByEpisode, rest[idx+1:]))
	}

	i.logger.InfoContext(ctx, "Due rows dispatched together", "media", media.Name, "rows", len(items),
		"packed", packed, "per_episode", len(rest))
}

// rowsFor is the rows of the given episodes, in episode order.
func rowsFor(rowsByEpisode map[string][]repository.DueItem, episodes []tvdb.Episode) []repository.DueItem {
	var rows []repository.DueItem
	for _, ep := range episodes {
		rows = append(rows, rowsByEpisode[strconv.Itoa(ep.Id)]...)
	}
	return rows
}

// touchWaiting refreshes queued_at on the rows still waiting their turn, so a
// group that works through its episodes for longer than the stale threshold is
// not taken for a crashed dispatch and sent a second time.
func (i *DownloadInteractor) touchWaiting(ctx context.Context, rows []repository.DueItem) {
	if len(rows) == 0 {
		return
	}
	ids := make([]int, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	if err := i.repo.TouchQueued(ctx, ids); err != nil {
		i.logger.WarnContext(ctx, "Failed to refresh waiting rows", "error", err, "rows", len(ids))
	}
}

// torrenterDown reports whether a dispatch error means the torrenter could not
// be reached at all (connection refused or reset, no such host): the service is
// restarting or down, and the request will go through once it is back. An
// answer with an error status, a timeout and a cancelled context are not that;
// the torrenter was there, and the usual retry ladder applies.
func torrenterDown(err error) bool {
	if err == nil {
		return false
	}
	var statusErr *clients.StatusError
	if errors.As(err, &statusErr) {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return false
	}
	var urlErr *url.Error
	return errors.As(err, &urlErr)
}

// deferRows holds rows for torrenterRetryDelay because the torrenter could not
// be reached: pending again, due shortly, no attempt counted, the notification
// saying so. The scheduler is woken so the timers are armed right away.
func (i *DownloadInteractor) deferRows(ctx context.Context, rows []repository.DueItem, cause error) {
	if len(rows) == 0 {
		return
	}
	next := time.Now().Add(torrenterRetryDelay)
	code := dlstatus.CodeTorrenterUnreachable
	reason := fmt.Sprintf("%s (next try %s)", code.HumanReason(), next.Format("15:04"))
	deferred := 0
	for _, row := range rows {
		if err := i.repo.DeferRow(ctx, row.ID, string(code), reason, next); err != nil {
			i.logger.ErrorContext(ctx, "Failed to defer row for the torrenter's return", "error", err, "id", row.ID)
			continue
		}
		deferred++
		i.updateNotificationStatus(ctx, repository.NotificationKey(row.Media), notifications.StatusSearching, reason)
	}
	if i.scheduler != nil {
		i.scheduler.Kick()
	}
	i.logger.WarnContext(ctx, "Torrenter unreachable, rows deferred without counting an attempt",
		"error", cause, "media", rows[0].Media.Name, "rows", deferred, "next_attempt", next.Format(time.RFC3339))
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
