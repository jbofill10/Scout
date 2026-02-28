package scheduler

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"
)

// SchedulerRepository defines the interface for scheduler data operations
type SchedulerRepository interface {
	GetDueMedia(ctx context.Context, windowEnd time.Time) ([]tvdb.Media, error)
}

// Scheduler receives a channel and checks the DB for due shows, sending them to the channel when ready
type Scheduler struct {
	repo     SchedulerRepository
	logger   *slog.Logger
	stopChan chan struct{} // signals shutdown
	tracer   trace.Tracer
}

func NewScheduler(repo SchedulerRepository, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		repo:     repo,
		logger:   logger,
		stopChan: make(chan struct{}),
		tracer:   otel.Tracer("webserver"),
	}
}

// Stop gracefully shuts down the scheduler by closing stopChan
func (s *Scheduler) Stop() {
	close(s.stopChan)
	s.logger.Info("Scheduler stopped")
}

// Start begins polling for scheduled media and sends them to the queue when due
func (s *Scheduler) Start(queue chan<- tvdb.Media) {
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		// Immediate poll on startup
		s.pollAndSchedule(queue)

		// Poll every minute
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

// pollAndSchedule queries for due media and enqueues them immediately.
// The database is the source of truth for due-time and retry scheduling.
func (s *Scheduler) pollAndSchedule(queue chan<- tvdb.Media) {
	// Create a new trace for this polling cycle
	ctx, span := s.tracer.Start(context.Background(), "scheduler.poll")
	defer span.End()

	now := time.Now().UTC()
	s.logger.InfoContext(ctx, "Polling for scheduled media", "due_before", now.Format(time.RFC3339))

	mediaList, err := s.repo.GetDueMedia(ctx, now)
	if err != nil {
		s.logger.ErrorContext(ctx, "Scheduler poll error", "error", err)
		span.RecordError(err)
		return
	}

	span.SetAttributes(attribute.Int("media.count", len(mediaList)))

	if len(mediaList) == 0 {
		return // No new items, no log spam
	}

	s.logger.InfoContext(ctx, "Found scheduled media", "count", len(mediaList))

	for _, media := range mediaList {
		s.logger.InfoContext(ctx, "Queueing due media", "media", media.Name)
		queue <- media
	}
}
