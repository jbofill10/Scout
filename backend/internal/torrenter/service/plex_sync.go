package service

import (
	"context"
	"log/slog"
	"time"
)

// startupRetryDelay is how long the first sync waits before its one retry.
// Plex is often still coming up when the torrenter boots alongside it.
const startupRetryDelay = time.Minute

// PlexSyncScheduler keeps the database mirror of the Plex library current.
// It syncs once at start, again shortly after downloads land (so a new movie
// or episode reads as in the library without a restart), and on a timer as a
// backstop for anything that reaches Plex by other means.
//
// Requests coalesce: the sync runs once no download has landed for `delay`,
// however many landed in the meantime. A request raised while a sync is
// running is kept, so the file it was raised for is picked up by the next one.
type PlexSyncScheduler struct {
	plex     PlexService
	delay    time.Duration // quiet period between the last request and the sync
	interval time.Duration // periodic sync; zero disables the timer
	retry    time.Duration // wait before the start-up sync's one retry
	requests chan struct{}
	logger   *slog.Logger
	// settings, when set, overrides delay and interval at runtime. The
	// interval is re-read every settingsCheck so a change takes effect
	// without a restart.
	settings      SyncSettings
	settingsCheck time.Duration
}

// SyncSettings is the slice of the runtime settings the sync scheduler needs.
type SyncSettings interface {
	PlexSyncDelay() time.Duration
	PlexSyncInterval() time.Duration
}

// SetSettings makes the delay and interval follow the runtime settings.
func (s *PlexSyncScheduler) SetSettings(settings SyncSettings) {
	s.settings = settings
}

func (s *PlexSyncScheduler) currentDelay() time.Duration {
	if s.settings != nil {
		return s.settings.PlexSyncDelay()
	}
	return s.delay
}

func (s *PlexSyncScheduler) currentInterval() time.Duration {
	if s.settings != nil {
		return s.settings.PlexSyncInterval()
	}
	return s.interval
}

func NewPlexSyncScheduler(plex PlexService, delay, interval time.Duration, logger *slog.Logger) *PlexSyncScheduler {
	return &PlexSyncScheduler{
		plex:          plex,
		delay:         delay,
		interval:      interval,
		retry:         startupRetryDelay,
		requests:      make(chan struct{}, 1),
		logger:        logger,
		settingsCheck: time.Minute,
	}
}

// RequestSync asks for the library to be re-read once downloads go quiet.
// It never blocks: the download path calls it and must not wait on Plex.
func (s *PlexSyncScheduler) RequestSync() {
	select {
	case s.requests <- struct{}{}:
	default:
		// One is already queued; it covers this request too.
	}
}

// Run blocks until ctx is done, so it is started as a goroutine.
func (s *PlexSyncScheduler) Run(ctx context.Context) {
	// The periodic ticker follows the effective interval; it is rebuilt when
	// the setting changes.
	var tick <-chan time.Time
	var ticker *time.Ticker
	interval := s.currentInterval()
	if interval > 0 {
		ticker = time.NewTicker(interval)
		tick = ticker.C
	}
	defer func() {
		if ticker != nil {
			ticker.Stop()
		}
	}()
	settingsTicker := time.NewTicker(s.settingsCheck)
	defer settingsTicker.Stop()

	// Armed by every request, so the sync lands `delay` after the last file did
	// and Plex has had time to scan it. The ticker bounds how long a steady
	// stream of downloads can push it back.
	var pending <-chan time.Time

	// The first sync is what fills an empty mirror, so a failure (Plex not
	// up yet, usually) gets one quick retry rather than waiting for the timer.
	if err := s.sync(ctx); err != nil {
		s.logger.ErrorContext(ctx, "Plex library sync failed, retrying", "retry_in", s.retry.String(), "error", err)
		pending = time.After(s.retry)
	}

	for {
		select {
		case <-ctx.Done():
			s.logger.InfoContext(ctx, "Plex sync scheduler stopped")
			return
		case <-s.requests:
			pending = time.After(s.currentDelay())
		case <-pending:
			pending = nil
			if err := s.sync(ctx); err != nil {
				s.logger.ErrorContext(ctx, "Plex library sync failed", "error", err)
			}
		case <-tick:
			if err := s.sync(ctx); err != nil {
				s.logger.ErrorContext(ctx, "Periodic Plex library sync failed", "error", err)
			}
		case <-settingsTicker.C:
			if next := s.currentInterval(); next != interval {
				s.logger.InfoContext(ctx, "Plex sync interval changed", "from", interval.String(), "to", next.String())
				if ticker != nil {
					ticker.Stop()
					ticker, tick = nil, nil
				}
				interval = next
				if interval > 0 {
					ticker = time.NewTicker(interval)
					tick = ticker.C
				}
			}
		}
	}
}

func (s *PlexSyncScheduler) sync(ctx context.Context) error {
	return s.plex.SyncPlexLibrary(ctx)
}
