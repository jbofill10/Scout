package service

import (
	"bytes"
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jbofill10/scout/backend/internal/torrenter/service/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type fakeSyncSettings struct {
	delay    atomic.Int64
	interval atomic.Int64
}

func (f *fakeSyncSettings) PlexSyncDelay() time.Duration    { return time.Duration(f.delay.Load()) }
func (f *fakeSyncSettings) PlexSyncInterval() time.Duration { return time.Duration(f.interval.Load()) }

// The periodic sync follows the runtime setting: turning the interval on
// after start begins periodic syncs without a restart.
func TestPlexSyncScheduler_IntervalFollowsSettings(t *testing.T) {
	var syncs atomic.Int32
	plex := mocks.NewPlexService(t)
	plex.On("SyncPlexLibrary", mock.Anything).Run(func(mock.Arguments) { syncs.Add(1) }).Return(nil)

	s := NewPlexSyncScheduler(plex, time.Hour, 0, slog.New(slog.NewTextHandler(new(bytes.Buffer), nil)))
	settings := &fakeSyncSettings{}
	settings.delay.Store(int64(time.Hour))
	s.SetSettings(settings)
	s.settingsCheck = 10 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)

	require.Eventually(t, func() bool { return syncs.Load() == 1 }, time.Second, 5*time.Millisecond, "start-up sync")
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int32(1), syncs.Load(), "no periodic sync while the interval is 0")

	settings.interval.Store(int64(20 * time.Millisecond))
	require.Eventually(t, func() bool { return syncs.Load() >= 3 }, 2*time.Second, 5*time.Millisecond,
		"periodic syncs start once the setting turns the interval on")
}
