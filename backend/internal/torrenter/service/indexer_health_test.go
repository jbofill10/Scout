package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jbofill10/scout/backend/internal/torrenter/clients"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golift.io/starr"
	"golift.io/starr/prowlarr"
)

// fakeProwlarrAdmin serves the indexer, status, health and test endpoints.
type fakeProwlarrAdmin struct {
	mu       sync.Mutex
	disabled map[int64]time.Time // indexer id -> disabledTill
	health   []map[string]string
	tested   []int64
}

func (f *fakeProwlarrAdmin) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/indexer" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": 1, "name": "Nyaa.si", "enable": true},
				{"id": 2, "name": "1337x", "enable": true},
				{"id": 3, "name": "Old", "enable": false},
			})
		case r.URL.Path == "/api/v1/indexerstatus":
			out := []map[string]any{}
			for id, till := range f.disabled {
				out = append(out, map[string]any{"indexerId": id, "disabledTill": till.Format(time.RFC3339)})
			}
			_ = json.NewEncoder(w).Encode(out)
		case r.URL.Path == "/api/v1/health":
			_ = json.NewEncoder(w).Encode(f.health)
		case r.URL.Path == "/api/v1/indexer/2" || r.URL.Path == "/api/v1/indexer/1":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 2, "name": "1337x", "enable": true, "fields": []any{}})
		case r.URL.Path == "/api/v1/indexer/test" && r.Method == http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			var ix map[string]any
			require.NoError(t, json.Unmarshal(body, &ix))
			id := int64(ix["id"].(float64))
			f.tested = append(f.tested, id)
			delete(f.disabled, id) // a passing test re-enables it, as Prowlarr does
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}
}

type fakeAgent struct {
	configured bool
	rotations  []string
	refuse     error
}

func (a *fakeAgent) Configured() bool { return a.configured }
func (a *fakeAgent) Status(context.Context) (clients.RelayStatus, error) {
	return clients.RelayStatus{Connected: true, Relay: "ca-mtr-wg-004", IP: "188.241.176.194", City: "mtr", Country: "ca"}, nil
}
func (a *fakeAgent) Rotate(_ context.Context, exclude []string, reason string) (clients.RelayRotation, error) {
	if a.refuse != nil {
		return clients.RelayRotation{}, a.refuse
	}
	a.rotations = append(a.rotations, reason)
	return clients.RelayRotation{
		Previous: clients.RelayStatus{Relay: "ca-mtr-wg-004"},
		Relay:    clients.RelayPick{Hostname: "ca-tor-wg-101", IP: "178.249.214.1", City: "tor", Country: "ca"},
		Status:   clients.RelayStatus{Connected: true},
	}, nil
}

type fakeRotateSettings struct{ on bool }

func (f fakeRotateSettings) AutoRotateRelay() bool { return f.on }

func newHealthTest(t *testing.T, fake *fakeProwlarrAdmin, agent *fakeAgent, auto bool) *IndexerHealthMonitor {
	t.Helper()
	srv := httptest.NewServer(fake.handler(t))
	t.Cleanup(srv.Close)
	api := prowlarr.New(starr.New("k", srv.URL, 5*time.Second)).APIer
	m := NewIndexerHealthMonitor(api, agent, fakeRotateSettings{on: auto}, 30*time.Minute, time.Minute,
		slog.New(slog.NewTextHandler(new(bytes.Buffer), nil)))
	m.sleep = func(context.Context, time.Duration) {}
	return m
}

func TestIndexerHealth_HealthyNeedsNoRotation(t *testing.T) {
	agent := &fakeAgent{configured: true}
	m := newHealthTest(t, &fakeProwlarrAdmin{disabled: map[int64]time.Time{}}, agent, true)

	m.Check(context.Background())

	snap := m.Snapshot()
	require.Len(t, snap.Indexers, 3)
	assert.True(t, snap.Indexers[1].Healthy)
	assert.Empty(t, agent.rotations)
	assert.True(t, snap.Relay.Configured)
	assert.Equal(t, "ca-mtr-wg-004", snap.Relay.Relay)
	assert.Equal(t, "mtr, ca", snap.Relay.Location)
	assert.True(t, snap.AutoRotate)
}

// A disabled public indexer with no other named cause is treated as an IP
// block: the relay is rotated once and the indexer re-tested, which clears it.
func TestIndexerHealth_BlockedIndexerRotatesAndRetests(t *testing.T) {
	fake := &fakeProwlarrAdmin{disabled: map[int64]time.Time{2: time.Now().Add(time.Hour)},
		health: []map[string]string{{"source": "IndexerStatusCheck", "type": "warning", "message": "Indexers unavailable due to failures: 1337x (error code: 1006)"}}}
	agent := &fakeAgent{configured: true}
	m := newHealthTest(t, fake, agent, true)

	m.Check(context.Background())

	require.Equal(t, []string{"indexer blocked the exit IP: 1337x"}, agent.rotations)
	assert.Equal(t, []int64{2}, fake.tested, "only the disabled indexer is re-tested")
	snap := m.Snapshot()
	assert.True(t, snap.Indexers[1].Healthy, "the passing test re-enabled it")
	assert.Equal(t, "ca-tor-wg-101", snap.Relay.Relay)
	assert.NotNil(t, snap.Relay.LastRotationAt)
	require.NotEmpty(t, snap.Events)
	assert.Equal(t, "indexer_test", snap.Events[0].Kind)
	assert.Equal(t, "rotation", snap.Events[1].Kind)

	// A second check inside the cooldown must not rotate again even if the
	// indexer is disabled once more.
	fake.mu.Lock()
	fake.disabled[2] = time.Now().Add(time.Hour)
	fake.mu.Unlock()
	m.Check(context.Background())
	assert.Len(t, agent.rotations, 1, "cooldown holds")
}

func TestIndexerHealth_RespectsAutoRotateOffAndMissingAgent(t *testing.T) {
	fake := &fakeProwlarrAdmin{disabled: map[int64]time.Time{2: time.Now().Add(time.Hour)}}
	agent := &fakeAgent{configured: true}
	m := newHealthTest(t, fake, agent, false)
	m.Check(context.Background())
	assert.Empty(t, agent.rotations, "auto-rotate off")
	assert.False(t, m.Snapshot().Indexers[1].Healthy)
	assert.True(t, m.Snapshot().Indexers[1].IPBlocked)

	none := &fakeAgent{configured: false}
	m2 := newHealthTest(t, fake, none, true)
	m2.Check(context.Background())
	_, err := m2.RotateNow(context.Background(), "")
	assert.ErrorIs(t, err, ErrNoAgent)
	assert.False(t, m2.Snapshot().Relay.Configured)
}

func TestIndexerHealth_OtherCausesAreNotIPBlocks(t *testing.T) {
	fake := &fakeProwlarrAdmin{disabled: map[int64]time.Time{1: time.Now().Add(time.Hour)},
		health: []map[string]string{{"source": "IndexerStatusCheck", "type": "warning", "message": "Nyaa.si: invalid api key / unauthorized"}}}
	agent := &fakeAgent{configured: true}
	m := newHealthTest(t, fake, agent, true)
	m.Check(context.Background())
	assert.Empty(t, agent.rotations, "a credentials problem is not fixed by a new relay")
	assert.False(t, m.Snapshot().Indexers[0].IPBlocked)
}

func TestIndexerHealth_ManualRotationAndRefusal(t *testing.T) {
	fake := &fakeProwlarrAdmin{disabled: map[int64]time.Time{}}
	agent := &fakeAgent{configured: true}
	m := newHealthTest(t, fake, agent, true)
	m.Check(context.Background())

	ev, err := m.RotateNow(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, "rotation", ev.Kind)
	assert.True(t, ev.OK)
	assert.Equal(t, []string{"requested from the UI"}, agent.rotations)

	agent.refuse = errors.New("relay agent refused to rotate: rotated 5s ago")
	_, err = m.RotateNow(context.Background(), "again")
	require.Error(t, err)
	assert.Equal(t, "rotation", m.Snapshot().Events[0].Kind)
	assert.False(t, m.Snapshot().Events[0].OK)
}
