package scheduler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"
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
	repo         SchedulerRepository
	logger       *slog.Logger
	scheduledMap map[string]bool // content_hash -> scheduled
	mu           sync.RWMutex    // protects scheduledMap
	stopChan     chan struct{}   // signals shutdown
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

// computeContentHash computes SHA256 hash for media deduplication
// Uses same logic as repository.Schedule() for consistency
func (s *Scheduler) computeContentHash(media tvdb.Media) string {
	var scheduleHash string
	if media.Category == "movie" || len(media.Metadata.Episodes) == 0 {
		// For movies or media without episodes, use media ID + FirstAired
		scheduleHash = fmt.Sprintf("%s-%s", media.Id, media.Metadata.FirstAired)
	} else {
		// For TV shows, use episode identifiers
		scheduleHash = fmt.Sprintf("%d-%d-%d",
			media.Metadata.Episodes[0].SeasonNumber,
			media.Metadata.Episodes[0].Number,
			media.Metadata.Episodes[0].AbsoluteNumber,
		)
	}
	h := sha256.Sum256([]byte(scheduleHash))
	return hex.EncodeToString(h[:])
}

// Stop gracefully shuts down the scheduler by closing stopChan
func (s *Scheduler) Stop() {
	close(s.stopChan)
	s.logger.Info("Scheduler stopped")
}

// Start begins polling for scheduled media and sends them to the queue when due
func (s *Scheduler) Start(queue chan<- tvdb.Media) {
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
func (s *Scheduler) pollAndSchedule(queue chan<- tvdb.Media) {
	// Create a new trace for this polling cycle
	ctx, span := s.tracer.Start(context.Background(), "scheduler.poll")
	defer span.End()

	now := time.Now()
	windowEnd := now.Add(24 * time.Hour)

	s.logger.InfoContext(ctx, "Polling for scheduled media", "window_end", windowEnd.Format(time.RFC3339))

	mediaList, err := s.repo.GetDueMedia(ctx, windowEnd)
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
		contentHash := s.computeContentHash(media)

		// Check if already scheduled
		s.mu.RLock()
		alreadyScheduled := s.scheduledMap[contentHash]
		s.mu.RUnlock()

		if alreadyScheduled {
			continue // Skip already-scheduled items
		}

		// Mark as scheduled in memory
		s.mu.Lock()
		s.scheduledMap[contentHash] = true
		s.mu.Unlock()

		// Extract release time from first episode (for shows) or FirstAired (for movies)
		var releaseTime time.Time
		var parseErr error

		if len(media.Metadata.Episodes) > 0 {
			// TV show episode
			releaseTime, parseErr = time.Parse("2006-01-02", media.Metadata.Episodes[0].Aired)
		} else if media.Category == "movie" {
			// Movie
			releaseTime, parseErr = time.Parse("2006-01-02", media.Metadata.FirstAired)
		} else {
			s.logger.WarnContext(ctx, "Media has no episodes or FirstAired date", "media", media.Name)
			continue
		}

		if parseErr != nil {
			s.logger.ErrorContext(ctx, "Failed to parse release time", "error", parseErr, "media", media.Name)
			continue
		}

		if releaseTime.Before(now) || releaseTime.Equal(now) {
			// Already due, send immediately
			s.logger.InfoContext(ctx, "Queueing media immediately (already due)", "media", media.Name)
			queue <- media
		} else {
			// Schedule for the future
			duration := time.Until(releaseTime)
			s.logger.InfoContext(ctx, "Scheduling timer for media",
				"media", media.Name,
				"release_time", releaseTime.Format(time.RFC3339),
				"wait_duration", duration.String())

			go s.scheduleTimer(media, duration, queue, contentHash)
		}
	}
}

// scheduleTimer creates a timer that sends media to the queue when it fires
func (s *Scheduler) scheduleTimer(media tvdb.Media, duration time.Duration, queue chan<- tvdb.Media, contentHash string) {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-timer.C:
		s.logger.Info("Timer fired, queueing media", "media", media.Name)
		queue <- media

		// Remove from scheduled map (cleanup)
		s.mu.Lock()
		delete(s.scheduledMap, contentHash)
		s.mu.Unlock()

	case <-s.stopChan:
		// Scheduler stopped before timer fired
		s.logger.Info("Timer cancelled due to shutdown", "media", media.Name)

		// Remove from scheduled map
		s.mu.Lock()
		delete(s.scheduledMap, contentHash)
		s.mu.Unlock()
	}
}
