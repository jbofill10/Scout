package interactors

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jbofill10/scout/backend/internal/webserver/clients"
	"github.com/jbofill10/scout/backend/internal/webserver/repository"
	"github.com/jbofill10/scout/backend/pkg/controls"
	"github.com/jbofill10/scout/backend/pkg/dlstatus"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/notifications"
)

var (
	// ErrNothingToControl means no row, torrent or notification exists for the item.
	ErrNothingToControl = errors.New("nothing found for item")
	// ErrDispatchInProgress means the item is being handed to the torrenter right now.
	ErrDispatchInProgress = errors.New("a dispatch for the item is in progress")
)

// controlRepository is what the controls need from the scheduler repository.
type controlRepository interface {
	FindRowsByItem(ctx context.Context, itemKey string) ([]repository.ScheduledRow, error)
	CancelRow(ctx context.Context, id int) error
	RearmRow(ctx context.Context, id int, resetAttempts bool) error
	MarkCompleted(ctx context.Context, id int) error
	GetUpcoming(ctx context.Context) ([]repository.UpcomingItem, error)
}

// timerControl is what the controls need from the scheduler.
type timerControl interface {
	CancelTimer(id int) bool
	Kick()
}

// controlTorrenter is what the controls need from the torrenter client.
type controlTorrenter interface {
	Cancel(ctx context.Context, itemID string) (controls.CancelResponse, error)
	ListReleases(ctx context.Context, itemID string, media *tvdb.Media) (controls.ReleasesResponse, error)
	ManualDownload(ctx context.Context, itemID string, media *tvdb.Media, sel controls.ReleaseSelection) (dlstatus.EpisodeResult, error)
}

// CancelOutcome reports what a cancel found to stop, across both services.
type CancelOutcome struct {
	ItemKey             string `json:"tvdb_id"`
	RowsCancelled       int    `json:"rows_cancelled"`
	TimersCancelled     int    `json:"timers_cancelled"`
	SearchCancelled     bool   `json:"search_cancelled"`
	TorrentsRemoved     int    `json:"torrents_removed"`
	NotificationUpdated bool   `json:"notification_updated"`
}

// RetryOutcome reports how many rows were re-armed for an immediate attempt.
type RetryOutcome struct {
	ItemKey string `json:"tvdb_id"`
	Rearmed int    `json:"rearmed"`
}

// DownloadControlInteractor lets a user act on a requested item after the
// fact: cancel it, retry it now, see the releases the indexers have for it,
// download one by hand, and list what is scheduled.
type DownloadControlInteractor struct {
	repo          controlRepository
	notifications repository.NotificationRepository
	timers        timerControl
	torrenter     controlTorrenter
	logger        *slog.Logger
}

func NewDownloadControlInteractor(
	repo controlRepository,
	notifs repository.NotificationRepository,
	timers timerControl,
	torrenter controlTorrenter,
	logger *slog.Logger,
) *DownloadControlInteractor {
	return &DownloadControlInteractor{repo: repo, notifications: notifs, timers: timers, torrenter: torrenter, logger: logger}
}

// Cancel stops everything for an item: armed timers and scheduled rows here,
// searches and torrents in the torrenter, and marks the notification
// cancelled. ErrNothingToControl when none of the three had anything.
func (c *DownloadControlInteractor) Cancel(ctx context.Context, itemKey string) (CancelOutcome, error) {
	out := CancelOutcome{ItemKey: itemKey}

	rows, err := c.repo.FindRowsByItem(ctx, itemKey)
	if err != nil {
		return out, fmt.Errorf("find rows: %w", err)
	}
	for _, row := range rows {
		if c.timers.CancelTimer(row.ID) {
			out.TimersCancelled++
		}
		if err := c.repo.CancelRow(ctx, row.ID); err != nil {
			c.logger.ErrorContext(ctx, "Failed to cancel scheduled row", "error", err, "id", row.ID, "tvdb_id", itemKey)
			continue
		}
		out.RowsCancelled++
	}

	// The torrenter may be mid-search or mid-download; a failure to reach it
	// is logged rather than fatal so the row and notification still settle.
	tResp, err := c.torrenter.Cancel(ctx, itemKey)
	if err != nil {
		c.logger.ErrorContext(ctx, "Torrenter cancel failed", "error", err, "tvdb_id", itemKey)
	}
	out.SearchCancelled = tResp.SearchCancelled
	out.TorrentsRemoved = tResp.TorrentsRemoved
	out.NotificationUpdated = tResp.NotificationUpdated

	if !out.NotificationUpdated {
		out.NotificationUpdated = c.setNotification(ctx, itemKey, notifications.StatusCancelled, "Cancelled")
	}

	if out.RowsCancelled == 0 && !tResp.Found() && !out.NotificationUpdated {
		return out, ErrNothingToControl
	}
	c.logger.InfoContext(ctx, "Cancelled item", "tvdb_id", itemKey, "rows", out.RowsCancelled,
		"timers", out.TimersCancelled, "search_cancelled", out.SearchCancelled, "torrents_removed", out.TorrentsRemoved)
	return out, nil
}

// RetryNow re-arms the item's rows so the scheduler dispatches them at once.
// ErrNothingToControl when the item has no row; ErrDispatchInProgress when a
// dispatch is already under way.
func (c *DownloadControlInteractor) RetryNow(ctx context.Context, itemKey string) (RetryOutcome, error) {
	out := RetryOutcome{ItemKey: itemKey}

	rows, err := c.repo.FindRowsByItem(ctx, itemKey)
	if err != nil {
		return out, fmt.Errorf("find rows: %w", err)
	}
	if len(rows) == 0 {
		return out, ErrNothingToControl
	}
	for _, row := range rows {
		if row.ScheduleStatus == repository.StatusQueued {
			return out, ErrDispatchInProgress
		}
	}

	for _, row := range rows {
		c.timers.CancelTimer(row.ID)
		if err := c.repo.RearmRow(ctx, row.ID, row.ScheduleStatus == repository.StatusFailed); err != nil {
			c.logger.ErrorContext(ctx, "Failed to re-arm scheduled row", "error", err, "id", row.ID, "tvdb_id", itemKey)
			continue
		}
		out.Rearmed++
	}
	if out.Rearmed == 0 {
		return out, errors.New("no row could be re-armed")
	}

	c.timers.Kick()
	c.setNotification(ctx, itemKey, notifications.StatusSearching, "Retrying now")
	c.logger.InfoContext(ctx, "Re-armed item for an immediate attempt", "tvdb_id", itemKey, "rows", out.Rearmed)
	return out, nil
}

// ListReleases returns the torrenter's candidate list for an item, using the
// media from its scheduled row when there is one.
func (c *DownloadControlInteractor) ListReleases(ctx context.Context, itemKey string) (controls.ReleasesResponse, error) {
	media, _, err := c.mediaFor(ctx, itemKey)
	if err != nil {
		return controls.ReleasesResponse{}, err
	}
	resp, err := c.torrenter.ListReleases(ctx, itemKey, media)
	if errors.Is(err, clients.ErrItemUnknown) {
		return resp, ErrNothingToControl
	}
	return resp, err
}

// Grab downloads the chosen release through the torrenter and, when that
// starts a download, settles the item's rows the way an automatic dispatch
// would: timers released, rows completed.
func (c *DownloadControlInteractor) Grab(ctx context.Context, itemKey string, sel controls.ReleaseSelection) (dlstatus.EpisodeResult, error) {
	media, rows, err := c.mediaFor(ctx, itemKey)
	if err != nil {
		return dlstatus.EpisodeResult{}, err
	}

	result, err := c.torrenter.ManualDownload(ctx, itemKey, media, sel)
	if errors.Is(err, clients.ErrItemUnknown) {
		return result, ErrNothingToControl
	}
	if err != nil {
		return result, err
	}
	if result.Outcome != dlstatus.OutcomeDownloading {
		return result, nil
	}

	for _, row := range rows {
		c.timers.CancelTimer(row.ID)
		if err := c.repo.MarkCompleted(ctx, row.ID); err != nil {
			c.logger.ErrorContext(ctx, "Failed to complete row after manual grab", "error", err, "id", row.ID)
		}
	}
	c.logger.InfoContext(ctx, "Manual grab started", "tvdb_id", itemKey, "torrent_title", sel.Title, "rows_completed", len(rows))
	return result, nil
}

// Upcoming lists every pending scheduled row, soonest first.
func (c *DownloadControlInteractor) Upcoming(ctx context.Context) ([]repository.UpcomingItem, error) {
	return c.repo.GetUpcoming(ctx)
}

// mediaFor returns the item's media from its first controllable row, or nil
// when it has none (the torrenter may still know it from an active torrent).
func (c *DownloadControlInteractor) mediaFor(ctx context.Context, itemKey string) (*tvdb.Media, []repository.ScheduledRow, error) {
	rows, err := c.repo.FindRowsByItem(ctx, itemKey)
	if err != nil {
		return nil, nil, fmt.Errorf("find rows: %w", err)
	}
	if len(rows) == 0 {
		return nil, rows, nil
	}
	media := rows[0].Media
	return &media, rows, nil
}

// setNotification moves the item's notification to status, reporting whether
// one existed. Missing notifications are not an error.
func (c *DownloadControlInteractor) setNotification(ctx context.Context, itemKey string, status notifications.NotificationStatus, reason string) bool {
	n, err := c.notifications.GetNotification(ctx, itemKey)
	if err != nil {
		c.logger.ErrorContext(ctx, "Failed to load notification", "error", err, "tvdb_id", itemKey)
		return false
	}
	if n == nil {
		return false
	}
	n.Status = status
	n.Reason = reason
	if err := c.notifications.UpdateNotification(ctx, n); err != nil {
		c.logger.ErrorContext(ctx, "Failed to update notification", "error", err, "tvdb_id", itemKey, "status", string(status))
		return false
	}
	return true
}
