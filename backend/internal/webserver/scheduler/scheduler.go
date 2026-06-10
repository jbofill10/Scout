package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/jbofill10/scout/backend/internal/webserver/repository"
)

// SchedulerRepository defines the interface for scheduler data operations
type SchedulerRepository interface {
	GetDueMedia(ctx context.Context, windowEnd time.Time) ([]repository.DueItem, error)
	MarkQueued(ctx context.Context, id int) error
	ResetStaleQueued(ctx context.Context, olderThan time.Duration) (int64, error)
}

// Scheduler receives a channel and checks the DB for due shows, sending them to the channel when ready
type Scheduler struct {
	repo         SchedulerRepository
	logger       *slog.Logger
	scheduledMap map[string]bool // content_hash -> scheduled
	mu           sync.Mutex      // protects scheduledMap
	stopChan     chan struct{}    // signals shutdown
	stopOnce     sync.Once       // ensures Stop() is idempotent
	tracer       trace.Tracer
}

func NewScheduler(repo SchedulerRepository, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		repo:         repo,
		logger:       logger,
		scheduledMap: make(map[string]bool),
		stopChan:     make(chan struct{}),
		tracer:       otel.Tracer("webserver"),
	}
}

// Stop gracefully shuts down the scheduler by closing stopChan. Safe to call multiple times.
func (s *Scheduler) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopChan)
		s.logger.Info("Scheduler stopped")
	})
}

// Start begins polling for scheduled media and sends them to the queue when due
func (s *Scheduler) Start(queue chan<- repository.DueItem) {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		// Immediate poll on startup
		s.pollAndSchedule(queue)

		// Poll every 5 minutes
		for {
			select {
			case <-ticker.C:
				s.pollAndSchedule(queue)
			case <-s.stopChan:
				s.logger.Info("Scheduler polling stopped")
				return
			}
		}
	}()
}

// pollAndSchedule queries the database for scheduled media and creates timers for future items
func (s *Scheduler) pollAndSchedule(queue chan<- repository.DueItem) {
	// Create a new trace for this polling cycle
	ctx, span := s.tracer.Start(context.Background(), "scheduler.poll")
	defer span.End()

	now := time.Now()
	windowEnd := now.Add(24 * time.Hour)

	// Recover rows stranded in 'queued' by a crashed/failed dispatch before
	// dispatching new work. Don't abort the poll on error.
	if reset, err := s.repo.ResetStaleQueued(ctx, 30*time.Minute); err != nil {
		s.logger.ErrorContext(ctx, "Failed to reset stale queued downloads", "error", err)
	} else if reset > 0 {
		s.logger.InfoContext(ctx, "Reset stale queued downloads", "count", reset)
	}

	s.logger.InfoContext(ctx, "Polling for scheduled media", "window_end", windowEnd.Format(time.RFC3339))

	dueItems, err := s.repo.GetDueMedia(ctx, windowEnd)
	if err != nil {
		s.logger.ErrorContext(ctx, "Scheduler poll error", "error", err)
		span.RecordError(err)
		return
	}

	span.SetAttributes(attribute.Int("media.count", len(dueItems)))

	if len(dueItems) == 0 {
		return // No new items, no log spam
	}

	s.logger.InfoContext(ctx, "Found scheduled media", "count", len(dueItems))

	for _, item := range dueItems {
		// The effective due time was selected by SQL as DueAt
		// (COALESCE(next_attempt_at, release_time)). Use it for timer math and
		// hashing so retries (which carry a next_attempt_at) dedup consistently
		// with their canonical scheduled row.
		hashTime := item.ReleaseTime
		if len(item.Media.Metadata.Episodes) == 0 && item.Media.Category != "movie" {
			s.logger.WarnContext(ctx, "Media has no episodes or FirstAired date", "media", item.Media.Name)
			continue
		}
		contentHash := repository.ComputeContentHash(item.Media, hashTime)

		// Check and mark as scheduled atomically to prevent duplicate timers.
		s.mu.Lock()
		alreadyScheduled := s.scheduledMap[contentHash]
		if !alreadyScheduled {
			s.scheduledMap[contentHash] = true
		}
		s.mu.Unlock()

		if alreadyScheduled {
			continue // Skip already-scheduled items
		}

		if !item.DueAt.After(now) {
			// Already due, send immediately
			s.logger.InfoContext(ctx, "Queueing media immediately (already due)", "media", item.Media.Name)
			s.markQueued(ctx, item.ID)
			select {
			case queue <- item:
			case <-s.stopChan:
				return
			}
			// Immediate dispatches don't go through scheduleTimer, so release the
			// dedup slot now to allow future polls to reconsider the row.
			s.mu.Lock()
			delete(s.scheduledMap, contentHash)
			s.mu.Unlock()
		} else {
			// Schedule for the future
			duration := time.Until(item.DueAt)
			s.logger.InfoContext(ctx, "Scheduling timer for media",
				"media", item.Media.Name,
				"due_at", item.DueAt.Format(time.RFC3339),
				"wait_duration", duration.String())

			go s.scheduleTimer(item, duration, queue, contentHash)
		}
	}
}

// markQueued transitions a row to queued just before dispatch. Failures are
// logged but non-fatal: we still dispatch so the download isn't lost.
func (s *Scheduler) markQueued(ctx context.Context, id int) {
	if err := s.repo.MarkQueued(ctx, id); err != nil {
		s.logger.ErrorContext(ctx, "Failed to mark row queued, dispatching anyway", "id", id, "error", err)
	}
}

// scheduleTimer creates a timer that sends the due item to the queue when it fires
func (s *Scheduler) scheduleTimer(item repository.DueItem, duration time.Duration, queue chan<- repository.DueItem, contentHash string) {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-timer.C:
		s.logger.Info("Timer fired, queueing media", "media", item.Media.Name)
		// Mark queued at dispatch time (not poll time).
		s.markQueued(context.Background(), item.ID)
		select {
		case queue <- item:
		case <-s.stopChan:
			s.logger.Info("Timer fired but shutdown in progress, dropping media", "media", item.Media.Name)
		}

		// Remove from scheduled map (cleanup)
		s.mu.Lock()
		delete(s.scheduledMap, contentHash)
		s.mu.Unlock()

	case <-s.stopChan:
		// Scheduler stopped before timer fired
		s.logger.Info("Timer cancelled due to shutdown", "media", item.Media.Name)

		// Remove from scheduled map
		s.mu.Lock()
		delete(s.scheduledMap, contentHash)
		s.mu.Unlock()
	}
}
