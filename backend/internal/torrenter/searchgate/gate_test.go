package searchgate

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fixedLimits(perMinute float64, burst int) func(int64) Limits {
	return func(int64) Limits { return Limits{PerMinute: perMinute, Burst: burst} }
}

func TestAcquire_BurstThenWaitsForRefill(t *testing.T) {
	g := New(fixedLimits(6000, 2)) // 100 tokens a second: refills fast enough to test
	ctx := context.Background()

	_, rel1, err := g.Acquire(ctx, []int64{2}, Manual, "a")
	require.NoError(t, err)
	rel1()
	_, rel2, err := g.Acquire(ctx, []int64{2}, Manual, "b")
	require.NoError(t, err)
	rel2()

	// Both burst tokens spent: the third waits on the refill timer.
	start := time.Now()
	_, rel3, err := g.Acquire(ctx, []int64{2}, Manual, "c")
	require.NoError(t, err)
	rel3()
	assert.Greater(t, time.Since(start), 5*time.Millisecond, "the third search had to wait for a token")

	st := g.Snapshot()
	require.Len(t, st, 1)
	assert.Equal(t, 3, st[0].SearchesLastMinute)
}

func TestAcquire_OneInFlightPerIndexer(t *testing.T) {
	g := New(fixedLimits(6000, 10))
	ctx := context.Background()
	_, release, err := g.Acquire(ctx, []int64{2}, Manual, "first")
	require.NoError(t, err)

	second := make(chan struct{})
	go func() {
		_, rel, err := g.Acquire(ctx, []int64{2}, Manual, "second")
		assert.NoError(t, err)
		rel()
		close(second)
	}()
	select {
	case <-second:
		t.Fatal("second search ran while the first held the indexer")
	case <-time.After(50 * time.Millisecond):
	}
	st := g.Snapshot()
	assert.Equal(t, "first", st[0].InFlight)
	assert.Equal(t, 1, st[0].Queued)

	release()
	select {
	case <-second:
	case <-time.After(time.Second):
		t.Fatal("second search never ran after the first released")
	}
}

// A person's request goes ahead of queued automation regardless of arrival.
func TestAcquire_PriorityOrder(t *testing.T) {
	g := New(fixedLimits(6000, 10))
	ctx := context.Background()
	_, release, err := g.Acquire(ctx, []int64{2}, Manual, "holder")
	require.NoError(t, err)

	var mu sync.Mutex
	order := []string{}
	var wg sync.WaitGroup
	start := func(p Priority, label string) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, rel, err := g.Acquire(ctx, []int64{2}, p, label)
			require.NoError(t, err)
			mu.Lock()
			order = append(order, label)
			mu.Unlock()
			time.Sleep(2 * time.Millisecond)
			rel()
		}()
		// Let the goroutine enqueue before the next one.
		require.Eventually(t, func() bool { return g.Snapshot()[0].Queued >= 1 }, time.Second, time.Millisecond)
	}
	start(Upgrade, "upgrade")
	require.Eventually(t, func() bool { return g.Snapshot()[0].Queued == 1 }, time.Second, time.Millisecond)
	start(Follow, "follow")
	require.Eventually(t, func() bool { return g.Snapshot()[0].Queued == 2 }, time.Second, time.Millisecond)
	start(Manual, "manual")
	require.Eventually(t, func() bool { return g.Snapshot()[0].Queued == 3 }, time.Second, time.Millisecond)

	st := g.Snapshot()
	assert.Equal(t, map[string]int{"upgrade": 1, "follow": 1, "manual": 1}, st[0].QueuedByPriority)
	assert.Equal(t, []string{"manual", "follow", "upgrade"}, st[0].Waiting, "the queue is reported best-first")

	release()
	wg.Wait()
	assert.Equal(t, []string{"manual", "follow", "upgrade"}, order)
}

func TestAcquire_PausedIndexerIsSkipped(t *testing.T) {
	g := New(nil)
	ctx := context.Background()
	g.Pause(2, time.Now().Add(time.Hour), "Cloudflare 1006")

	granted, release, err := g.Acquire(ctx, []int64{1, 2}, Manual, "anime movie")
	require.NoError(t, err)
	release()
	assert.Equal(t, []int64{1}, granted, "the paused indexer is left out, the other is searched")

	_, _, err = g.Acquire(ctx, []int64{2}, Manual, "show")
	require.ErrorIs(t, err, ErrPaused)
	assert.Contains(t, err.Error(), "Cloudflare 1006")

	g.Resume(2)
	granted, release, err = g.Acquire(ctx, []int64{2}, Manual, "show")
	require.NoError(t, err)
	release()
	assert.Equal(t, []int64{2}, granted)
}

// Pausing an indexer wakes everything queued on it with ErrPaused instead of
// leaving searches hanging for the length of the ban.
func TestPause_ReleasesWaiters(t *testing.T) {
	g := New(fixedLimits(6000, 10))
	ctx := context.Background()
	_, release, err := g.Acquire(ctx, []int64{2}, Manual, "holder")
	require.NoError(t, err)
	defer release()

	errs := make(chan error, 1)
	go func() {
		_, _, err := g.Acquire(ctx, []int64{2}, Follow, "queued")
		errs <- err
	}()
	require.Eventually(t, func() bool { return g.Snapshot()[0].Queued == 1 }, time.Second, time.Millisecond)

	g.Pause(2, time.Now().Add(time.Hour), "disabled by Prowlarr")
	select {
	case err := <-errs:
		assert.True(t, errors.Is(err, ErrPaused))
	case <-time.After(time.Second):
		t.Fatal("the queued search was not released by the pause")
	}
}

func TestAcquire_CancelledWaiterLeavesQueue(t *testing.T) {
	g := New(fixedLimits(6000, 10))
	_, release, err := g.Acquire(context.Background(), []int64{2}, Manual, "holder")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	errs := make(chan error, 1)
	go func() {
		_, _, err := g.Acquire(ctx, []int64{2}, Scheduled, "waiting")
		errs <- err
	}()
	require.Eventually(t, func() bool { return g.Snapshot()[0].Queued == 1 }, time.Second, time.Millisecond)
	cancel()
	assert.ErrorIs(t, <-errs, context.Canceled)
	assert.Equal(t, 0, g.Snapshot()[0].Queued)
	release()

	// The slot is free again for the next caller.
	_, rel, err := g.Acquire(context.Background(), []int64{2}, Manual, "next")
	require.NoError(t, err)
	rel()
}

func TestPriorityFor(t *testing.T) {
	assert.Equal(t, Manual, PriorityFor(""))
	assert.Equal(t, Manual, PriorityFor("manual"))
	assert.Equal(t, Scheduled, PriorityFor("scheduled"))
	assert.Equal(t, Follow, PriorityFor("follow"))
	assert.Equal(t, Missing, PriorityFor("missing"))
	assert.Equal(t, Upgrade, PriorityFor("upgrade"))
	assert.Equal(t, Manual, PriorityFor("something new"))
	assert.Equal(t, "follow", Follow.String())
}

func TestRequestContext(t *testing.T) {
	p, label := FromContext(context.Background())
	assert.Equal(t, Manual, p)
	assert.Empty(t, label)
	ctx := WithRequest(context.Background(), Missing, "Dan Da Dan S02E03")
	p, label = FromContext(ctx)
	assert.Equal(t, Missing, p)
	assert.Equal(t, "Dan Da Dan S02E03", label)
}
