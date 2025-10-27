package scheduler

import (
	"log/slog"
	"time"

	tvdb "shared/media"
)

// SchedulerRepository defines the interface for scheduler data operations
type SchedulerRepository interface {
	GetDueMedia() ([]tvdb.Media, error)
}

// Scheduler receives a channel and checks the DB for due shows, sending them to the channel when ready
type Scheduler struct {
	repo   SchedulerRepository
	logger *slog.Logger
}

func NewScheduler(repo SchedulerRepository, logger *slog.Logger) *Scheduler {
	return &Scheduler{repo: repo, logger: logger}
}

// Start schedules all pending media to be sent at their release time
func (s *Scheduler) Start(queue chan<- tvdb.Media) {
	go func() {
		mediaList, err := s.repo.GetDueMedia()
		if err != nil {
			s.logger.Error("Scheduler error", "error", err)
			return
		}
		now := time.Now()
		for _, media := range mediaList {
			// Extract release time from first episode
			if len(media.Metadata.Episodes) > 0 {
				// Parse the release time from episode aired date
				releaseTime, err := time.Parse("2006-01-02", media.Metadata.Episodes[0].Aired)
				if err != nil {
					s.logger.Error("Failed to parse aired date", "error", err)
					continue
				}

				if releaseTime.Before(now) {
					// Already due, send immediately
					queue <- media
					continue
				}
				// Schedule for the future
				go func(m tvdb.Media, rt time.Time) {
					timer := time.NewTimer(time.Until(rt))
					<-timer.C
					s.logger.Info("Queueing media", "name", m.Name)
					queue <- m
				}(media, releaseTime)
			}
		}
	}()
}
