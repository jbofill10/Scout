package interactors

import (
	"context"
	"log/slog"
	"sort"
	"time"

	"github.com/jbofill10/scout/backend/internal/webserver/repository"
	"github.com/jbofill10/scout/backend/pkg/notifications"
)

// activeStatuses are the notification statuses that mean work is still in
// flight. Everything else is terminal and belongs in the recent list.
var activeStatuses = map[notifications.NotificationStatus]bool{
	notifications.StatusScheduled:   true,
	notifications.StatusSearching:   true,
	notifications.StatusDownloading: true,
}

// ActivityItem is a download notification enriched with the scheduling state of
// its row, so the activity view can explain why an item is waiting and when it
// will next be attempted.
type ActivityItem struct {
	notifications.Notification
	Attempts          int        `json:"attempts,omitempty"`
	NextAttemptAt     *time.Time `json:"next_attempt_at,omitempty"`
	ReleaseTime       *time.Time `json:"release_time,omitempty"`
	ScheduleStatus    string     `json:"schedule_status,omitempty"`
	LastFailureCode   string     `json:"last_failure_code,omitempty"`
	LastFailureReason string     `json:"last_failure_reason,omitempty"`
}

// ActivitySnapshot splits recent download work into what is still in flight and
// what has finished, with a count per stage for at-a-glance summaries.
type ActivitySnapshot struct {
	Active []ActivityItem `json:"active"`
	Recent []ActivityItem `json:"recent"`
	Counts map[string]int `json:"counts"`
}

// ActivityInteractor assembles the activity view from notifications (which
// carry the per-item stage) and the scheduled downloads table (which carries
// retry counts and next-attempt times).
type ActivityInteractor struct {
	notificationRepo repository.NotificationRepository
	schedulerRepo    repository.SchedulerRepository
	logger           *slog.Logger
}

func NewActivityInteractor(
	notificationRepo repository.NotificationRepository,
	schedulerRepo repository.SchedulerRepository,
	logger *slog.Logger,
) *ActivityInteractor {
	return &ActivityInteractor{
		notificationRepo: notificationRepo,
		schedulerRepo:    schedulerRepo,
		logger:           logger,
	}
}

// GetActivity returns up to limit recent notifications, split into in-flight
// and finished work. Scheduling metadata is best-effort: if that lookup fails
// the snapshot is still returned, just without retry details.
func (a *ActivityInteractor) GetActivity(ctx context.Context, limit int) (ActivitySnapshot, error) {
	notifs, err := a.notificationRepo.GetNotifications(ctx, limit, "", "")
	if err != nil {
		return ActivitySnapshot{}, err
	}

	meta, err := a.schedulerRepo.GetPendingScheduleMeta(ctx)
	if err != nil {
		a.logger.ErrorContext(ctx, "Failed to load schedule metadata for activity", "error", err)
		meta = map[string]repository.ScheduleMeta{}
	}

	snapshot := ActivitySnapshot{
		Active: []ActivityItem{},
		Recent: []ActivityItem{},
		Counts: map[string]int{},
	}

	for _, n := range notifs {
		item := ActivityItem{Notification: n}
		if m, ok := meta[n.TvdbID]; ok {
			item.Attempts = m.Attempts
			item.ScheduleStatus = m.ScheduleStatus
			item.LastFailureCode = m.LastFailureCode
			item.LastFailureReason = m.LastFailureReason
			releaseTime := m.ReleaseTime
			item.ReleaseTime = &releaseTime
			item.NextAttemptAt = m.NextAttemptAt
		}

		snapshot.Counts[string(n.Status)]++
		if activeStatuses[n.Status] {
			snapshot.Active = append(snapshot.Active, item)
		} else {
			snapshot.Recent = append(snapshot.Recent, item)
		}
	}

	// Most recently touched first in both lists — that is the order a user
	// scanning for "what is happening right now" expects.
	sortByUpdatedDesc(snapshot.Active)
	sortByUpdatedDesc(snapshot.Recent)

	return snapshot, nil
}

func sortByUpdatedDesc(items []ActivityItem) {
	sort.SliceStable(items, func(x, y int) bool {
		return items[x].UpdatedAt.After(items[y].UpdatedAt)
	})
}
