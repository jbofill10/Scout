package service

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(new(bytes.Buffer), nil))
}

// fakePlexSync records sync calls. Each call sends on calls before returning,
// then waits on block when one is set, so a test can hold a sync "in flight".
type fakePlexSync struct {
	mu    sync.Mutex
	errs  []error // returned in order, nil once exhausted
	calls chan struct{}
	block chan struct{}
}

func newFakePlexSync(errs ...error) *fakePlexSync {
	return &fakePlexSync{errs: errs, calls: make(chan struct{}, 16)}
}

func (f *fakePlexSync) SyncPlexLibrary(context.Context) error {
	f.calls <- struct{}{}
	if f.block != nil {
		<-f.block
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.errs) == 0 {
		return nil
	}
	err := f.errs[0]
	f.errs = f.errs[1:]
	return err
}

func startScheduler(t *testing.T, plex PlexService, delay, interval time.Duration) *PlexSyncScheduler {
	t.Helper()
	s := NewPlexSyncScheduler(plex, delay, interval, quietLogger())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.Run(ctx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("scheduler did not stop on context cancel")
		}
	})
	return s
}

func expectSync(t *testing.T, calls <-chan struct{}, within time.Duration) {
	t.Helper()
	select {
	case <-calls:
	case <-time.After(within):
		t.Fatalf("no sync within %s", within)
	}
}

func expectNoSync(t *testing.T, calls <-chan struct{}, within time.Duration) {
	t.Helper()
	select {
	case <-calls:
		t.Fatal("unexpected sync")
	case <-time.After(within):
	}
}

func TestPlexSyncScheduler_SyncsOnceAtStart(t *testing.T) {
	plex := newFakePlexSync()
	startScheduler(t, plex, time.Hour, 0)

	expectSync(t, plex.calls, time.Second)
	expectNoSync(t, plex.calls, 150*time.Millisecond)
}

// A burst of downloads is one sync, not one per file.
func TestPlexSyncScheduler_CoalescesRequests(t *testing.T) {
	plex := newFakePlexSync()
	s := startScheduler(t, plex, 50*time.Millisecond, 0)
	expectSync(t, plex.calls, time.Second) // start-up

	s.RequestSync()
	s.RequestSync()
	s.RequestSync()

	expectSync(t, plex.calls, time.Second)
	expectNoSync(t, plex.calls, 200*time.Millisecond)
}

// The quiet period is measured from the last request, so the newest file has
// had the full delay for Plex to scan it.
func TestPlexSyncScheduler_DelayRestartsOnEachRequest(t *testing.T) {
	plex := newFakePlexSync()
	s := startScheduler(t, plex, 300*time.Millisecond, 0)
	expectSync(t, plex.calls, time.Second) // start-up

	s.RequestSync()
	time.Sleep(150 * time.Millisecond)
	s.RequestSync()

	// Measured from the first request the sync would land at 300ms; from the
	// second it lands at 450ms.
	expectNoSync(t, plex.calls, 200*time.Millisecond)
	expectSync(t, plex.calls, 2*time.Second)
}

// A download that lands mid-sync must not be lost: the running sync may have
// read Plex before the file was there.
func TestPlexSyncScheduler_RequestDuringSyncRunsAnother(t *testing.T) {
	plex := newFakePlexSync()
	plex.block = make(chan struct{})
	s := startScheduler(t, plex, 20*time.Millisecond, 0)

	expectSync(t, plex.calls, time.Second) // start-up, now held in flight
	s.RequestSync()
	plex.block <- struct{}{} // release it

	// The release unblocks the first sync; the second needs a release too.
	go func() { plex.block <- struct{}{} }()
	expectSync(t, plex.calls, time.Second)
}

func TestPlexSyncScheduler_PeriodicSync(t *testing.T) {
	plex := newFakePlexSync()
	startScheduler(t, plex, time.Hour, 40*time.Millisecond)

	for range 3 {
		expectSync(t, plex.calls, time.Second)
	}
}

// Plex is usually still booting when the torrenter starts alongside it; the
// first sync fills an empty mirror, so it gets one quick retry.
func TestPlexSyncScheduler_RetriesFailedStartupSync(t *testing.T) {
	plex := newFakePlexSync(errors.New("plex not ready"))
	s := NewPlexSyncScheduler(plex, time.Hour, 0, quietLogger())
	s.retry = 30 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)

	expectSync(t, plex.calls, time.Second)
	expectSync(t, plex.calls, time.Second)
	expectNoSync(t, plex.calls, 150*time.Millisecond)
}

// A later failure is logged and left to the next request or tick.
func TestPlexSyncScheduler_FailedRequestedSyncDoesNotRetry(t *testing.T) {
	plex := newFakePlexSync(nil, errors.New("plex down"))
	s := startScheduler(t, plex, 20*time.Millisecond, 0)
	expectSync(t, plex.calls, time.Second) // start-up

	s.RequestSync()
	expectSync(t, plex.calls, time.Second)
	expectNoSync(t, plex.calls, 200*time.Millisecond)
}

// The download path calls this; it must never wait on the scheduler.
func TestPlexSyncScheduler_RequestSyncNeverBlocks(t *testing.T) {
	s := NewPlexSyncScheduler(newFakePlexSync(), time.Hour, 0, quietLogger())

	done := make(chan struct{})
	go func() {
		for range 100 {
			s.RequestSync()
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("RequestSync blocked without a running scheduler")
	}
}
