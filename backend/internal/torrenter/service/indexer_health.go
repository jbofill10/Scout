package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"sync"
	"time"

	"golift.io/starr"

	"github.com/jbofill10/scout/backend/internal/torrenter/clients"
)

// IndexerState is one Prowlarr indexer as the health panel shows it.
type IndexerState struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	Enabled      bool       `json:"enabled"`
	Healthy      bool       `json:"healthy"`
	DisabledTill *time.Time `json:"disabled_till,omitempty"`
	Failure      string     `json:"failure,omitempty"`
	// IPBlocked means the failure reads like the exit IP being refused
	// (Cloudflare 1006, 403), which a relay change can fix.
	IPBlocked bool `json:"ip_blocked"`
}

// RelayInfo is the host tunnel as last seen through the relay agent.
type RelayInfo struct {
	Configured         bool       `json:"configured"`
	Connected          bool       `json:"connected"`
	Relay              string     `json:"relay,omitempty"`
	IP                 string     `json:"ip,omitempty"`
	Location           string     `json:"location,omitempty"`
	LastRotationAt     *time.Time `json:"last_rotation_at,omitempty"`
	LastRotationReason string     `json:"last_rotation_reason,omitempty"`
	Error              string     `json:"error,omitempty"`
}

// HealthEvent is something the monitor did or saw, newest first in the snapshot.
type HealthEvent struct {
	At      time.Time `json:"at"`
	Kind    string    `json:"kind"` // rotation, indexer_test, check_error
	Message string    `json:"message"`
	Indexer string    `json:"indexer,omitempty"`
	OK      bool      `json:"ok"`
}

// IndexerHealthSnapshot is GET /indexers/health.
type IndexerHealthSnapshot struct {
	Indexers   []IndexerState `json:"indexers"`
	Relay      RelayInfo      `json:"relay"`
	Events     []HealthEvent  `json:"events"`
	AutoRotate bool           `json:"auto_rotate"`
	CheckedAt  *time.Time     `json:"checked_at,omitempty"`
	CheckError string         `json:"check_error,omitempty"`
}

// relayAgent is the slice of the relay agent client the monitor uses.
type relayAgent interface {
	Configured() bool
	Status(ctx context.Context) (clients.RelayStatus, error)
	Rotate(ctx context.Context, exclude []string, reason string) (clients.RelayRotation, error)
}

// RotateSettings is the runtime setting the monitor consults.
type RotateSettings interface {
	AutoRotateRelay() bool
}

// ErrNoAgent is returned by RotateNow when no relay agent is configured.
var ErrNoAgent = errors.New("no relay agent configured")

// ipBlockRe matches Prowlarr failure messages that mean the indexer refused
// the exit IP rather than the query.
var ipBlockRe = regexp.MustCompile(`(?i)1006|cloudflare|forbidden|\b403\b|access denied|captcha|just a moment|blocked`)

const (
	defaultHealthInterval = time.Minute
	maxHealthEvents       = 50
	rotateSettleDelay     = 5 * time.Second
)

// IndexerHealthMonitor polls Prowlarr for disabled indexers and, when one is
// disabled by an IP block and a relay agent is available, rotates the host's
// relay and re-tests the indexer so Prowlarr re-enables it.
type IndexerHealthMonitor struct {
	api      starr.APIer
	agent    relayAgent
	settings RotateSettings
	cooldown time.Duration
	interval time.Duration
	logger   *slog.Logger
	now      func() time.Time
	sleep    func(context.Context, time.Duration)

	mu           sync.Mutex
	indexers     []IndexerState
	relay        RelayInfo
	events       []HealthEvent
	checkedAt    *time.Time
	checkErr     string
	lastRotation time.Time
	recentRelays []string
	rotating     bool
}

func NewIndexerHealthMonitor(api starr.APIer, agent relayAgent, settings RotateSettings, cooldown, interval time.Duration, logger *slog.Logger) *IndexerHealthMonitor {
	if interval <= 0 {
		interval = defaultHealthInterval
	}
	return &IndexerHealthMonitor{
		api: api, agent: agent, settings: settings, cooldown: cooldown, interval: interval, logger: logger,
		now:   time.Now,
		sleep: func(ctx context.Context, d time.Duration) { sleepCtx(ctx, d) },
		relay: RelayInfo{Configured: agent != nil && agent.Configured()},
	}
}

// Start checks shortly after boot and then every interval until ctx ends.
func (m *IndexerHealthMonitor) Start(ctx context.Context) {
	go func() {
		if !sleepCtx(ctx, 15*time.Second) {
			return
		}
		m.Check(ctx)
		ticker := time.NewTicker(m.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				m.Check(ctx)
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Snapshot is the current view for the UI.
func (m *IndexerHealthMonitor) Snapshot() IndexerHealthSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	snap := IndexerHealthSnapshot{
		Indexers:   append([]IndexerState{}, m.indexers...),
		Relay:      m.relay,
		Events:     append([]HealthEvent{}, m.events...),
		AutoRotate: m.autoRotate(),
		CheckedAt:  m.checkedAt,
		CheckError: m.checkErr,
	}
	return snap
}

func (m *IndexerHealthMonitor) autoRotate() bool {
	if m.settings == nil {
		return true
	}
	return m.settings.AutoRotateRelay()
}

type prowlarrIndexer struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Enable bool   `json:"enable"`
}

type prowlarrIndexerStatus struct {
	IndexerID         int64      `json:"indexerId"`
	DisabledTill      *time.Time `json:"disabledTill"`
	MostRecentFailure *time.Time `json:"mostRecentFailure"`
	InitialFailure    *time.Time `json:"initialFailure"`
	// Prowlarr's status rows carry no message; the failure text comes from
	// the health resource when present.
}

type prowlarrHealth struct {
	Source  string `json:"source"`
	Type    string `json:"type"`
	Message string `json:"message"`
}

// Check refreshes the indexer and relay state once, rotating when warranted.
func (m *IndexerHealthMonitor) Check(ctx context.Context) {
	states, err := m.fetchIndexers(ctx)
	now := m.now()
	m.mu.Lock()
	m.checkedAt = &now
	if err != nil {
		m.checkErr = err.Error()
		m.mu.Unlock()
		m.logger.WarnContext(ctx, "Indexer health check failed", "error", err)
		return
	}
	m.checkErr = ""
	m.indexers = states
	m.mu.Unlock()

	m.refreshRelay(ctx)

	blocked := []IndexerState{}
	for _, s := range states {
		if s.Enabled && !s.Healthy && s.IPBlocked {
			blocked = append(blocked, s)
		}
	}
	if len(blocked) == 0 || m.agent == nil || !m.agent.Configured() || !m.autoRotate() {
		return
	}
	m.mu.Lock()
	tooSoon := !m.lastRotation.IsZero() && m.now().Sub(m.lastRotation) < m.cooldown
	m.mu.Unlock()
	if tooSoon {
		return
	}
	names := make([]string, 0, len(blocked))
	for _, b := range blocked {
		names = append(names, b.Name)
	}
	if _, err := m.rotate(ctx, "indexer blocked the exit IP: "+joinNames(names)); err != nil {
		m.logger.WarnContext(ctx, "Automatic relay rotation failed", "error", err)
	}
}

// RotateNow rotates on request (the UI button) and re-tests disabled indexers.
func (m *IndexerHealthMonitor) RotateNow(ctx context.Context, reason string) (HealthEvent, error) {
	if m.agent == nil || !m.agent.Configured() {
		return HealthEvent{}, ErrNoAgent
	}
	if reason == "" {
		reason = "requested from the UI"
	}
	return m.rotate(ctx, reason)
}

// rotate asks the agent for a new relay, then re-tests every disabled indexer.
func (m *IndexerHealthMonitor) rotate(ctx context.Context, reason string) (HealthEvent, error) {
	m.mu.Lock()
	if m.rotating {
		m.mu.Unlock()
		return HealthEvent{}, errors.New("a rotation is already in progress")
	}
	m.rotating = true
	exclude := append([]string{}, m.recentRelays...)
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.rotating = false
		m.mu.Unlock()
	}()

	rot, err := m.agent.Rotate(ctx, exclude, reason)
	if err != nil {
		ev := m.record(HealthEvent{Kind: "rotation", Message: "Relay rotation failed: " + err.Error(), OK: false})
		return ev, err
	}
	at := m.now()
	m.mu.Lock()
	m.lastRotation = at
	m.recentRelays = append([]string{rot.Relay.Hostname}, m.recentRelays...)
	if len(m.recentRelays) > 5 {
		m.recentRelays = m.recentRelays[:5]
	}
	m.relay = RelayInfo{Configured: true, Connected: rot.Status.Connected, Relay: rot.Relay.Hostname, IP: rot.Relay.IP,
		Location: location(rot.Relay.City, rot.Relay.Country), LastRotationAt: &at, LastRotationReason: reason}
	m.mu.Unlock()
	msg := fmt.Sprintf("Relay changed from %s to %s (%s)", orUnknown(rot.Previous.Relay), rot.Relay.Hostname, rot.Relay.IP)
	ev := m.record(HealthEvent{Kind: "rotation", Message: msg + " because " + reason, OK: rot.Status.Connected})
	m.logger.InfoContext(ctx, "Relay rotated", "from", rot.Previous.Relay, "to", rot.Relay.Hostname, "ip", rot.Relay.IP, "reason", reason)

	// Give the tunnel a moment, then ask Prowlarr to re-test what it disabled.
	m.sleep(ctx, rotateSettleDelay)
	m.mu.Lock()
	disabled := []IndexerState{}
	for _, s := range m.indexers {
		if s.Enabled && !s.Healthy {
			disabled = append(disabled, s)
		}
	}
	m.mu.Unlock()
	for _, s := range disabled {
		if err := m.TestIndexer(ctx, s.ID); err != nil {
			m.logger.WarnContext(ctx, "Indexer re-test failed after rotation", "indexer", s.Name, "error", err)
		}
	}
	if states, err := m.fetchIndexers(ctx); err == nil {
		m.mu.Lock()
		m.indexers = states
		m.mu.Unlock()
	}
	return ev, nil
}

// TestIndexer runs Prowlarr's indexer test, which clears the disabled state
// when it passes.
func (m *IndexerHealthMonitor) TestIndexer(ctx context.Context, id int64) error {
	var indexer map[string]any
	if err := m.api.GetInto(ctx, starr.Request{URI: "v1/indexer/" + strconv.FormatInt(id, 10)}, &indexer); err != nil {
		return fmt.Errorf("load indexer %d: %w", id, err)
	}
	name, _ := indexer["name"].(string)
	raw, err := json.Marshal(indexer)
	if err != nil {
		return err
	}
	// Raw Post takes the full path (GetInto prepends /api/ itself).
	resp, err := m.api.Post(ctx, starr.Request{URI: "/api/v1/indexer/test", Body: bytes.NewReader(raw)})
	if err != nil {
		m.record(HealthEvent{Kind: "indexer_test", Indexer: name, Message: "Test failed: " + err.Error(), OK: false})
		return err
	}
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		err := fmt.Errorf("prowlarr answered %d", resp.StatusCode)
		m.record(HealthEvent{Kind: "indexer_test", Indexer: name, Message: "Test failed: " + err.Error(), OK: false})
		return err
	}
	m.record(HealthEvent{Kind: "indexer_test", Indexer: name, Message: "Test passed; Prowlarr re-enabled it", OK: true})
	return nil
}

// fetchIndexers joins Prowlarr's indexer list with its status and health rows.
func (m *IndexerHealthMonitor) fetchIndexers(ctx context.Context) ([]IndexerState, error) {
	var list []prowlarrIndexer
	if err := m.api.GetInto(ctx, starr.Request{URI: "v1/indexer"}, &list); err != nil {
		return nil, fmt.Errorf("prowlarr indexers: %w", err)
	}
	var statuses []prowlarrIndexerStatus
	if err := m.api.GetInto(ctx, starr.Request{URI: "v1/indexerstatus"}, &statuses); err != nil {
		return nil, fmt.Errorf("prowlarr indexer status: %w", err)
	}
	var health []prowlarrHealth
	_ = m.api.GetInto(ctx, starr.Request{URI: "v1/health"}, &health) // best effort: only used for failure text

	byID := map[int64]prowlarrIndexerStatus{}
	for _, s := range statuses {
		byID[s.IndexerID] = s
	}
	healthText := ""
	for _, h := range health {
		if h.Source == "IndexerStatusCheck" || h.Source == "IndexerLongTermStatusCheck" {
			healthText += " " + h.Message
		}
	}

	now := m.now()
	states := make([]IndexerState, 0, len(list))
	for _, ix := range list {
		st := IndexerState{ID: ix.ID, Name: ix.Name, Enabled: ix.Enable, Healthy: true}
		if s, ok := byID[ix.ID]; ok && s.DisabledTill != nil && s.DisabledTill.After(now) {
			st.Healthy = false
			till := *s.DisabledTill
			st.DisabledTill = &till
			st.Failure = fmt.Sprintf("Disabled by Prowlarr until %s after repeated failures", till.Local().Format("15:04"))
			// Prowlarr keeps the reason in its logs, not the status row; the
			// health message mentions the indexer when it is a long outage.
			// An IP block is the common cause for a public indexer, so treat
			// a disabled public indexer as one unless the text says otherwise.
			st.IPBlocked = ipBlockRe.MatchString(healthText) || !mentionsOtherCause(healthText)
		}
		states = append(states, st)
	}
	return states, nil
}

// mentionsOtherCause is true when Prowlarr's health text names a cause that a
// relay change will not fix (bad credentials, parsing errors).
func mentionsOtherCause(text string) bool {
	return regexp.MustCompile(`(?i)unauthorized|invalid api key|parse|credentials|login`).MatchString(text)
}

// refreshRelay updates the relay view from the agent, when one is configured.
func (m *IndexerHealthMonitor) refreshRelay(ctx context.Context) {
	if m.agent == nil || !m.agent.Configured() {
		return
	}
	st, err := m.agent.Status(ctx)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.relay.Configured = true
	if err != nil {
		m.relay.Error = err.Error()
		return
	}
	m.relay.Error = ""
	m.relay.Connected = st.Connected
	m.relay.Relay = st.Relay
	m.relay.IP = st.IP
	m.relay.Location = location(st.City, st.Country)
}

func (m *IndexerHealthMonitor) record(ev HealthEvent) HealthEvent {
	ev.At = m.now()
	m.mu.Lock()
	m.events = append([]HealthEvent{ev}, m.events...)
	if len(m.events) > maxHealthEvents {
		m.events = m.events[:maxHealthEvents]
	}
	m.mu.Unlock()
	return ev
}

func location(city, country string) string {
	switch {
	case city != "" && country != "":
		return city + ", " + country
	case country != "":
		return country
	}
	return ""
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}

func joinNames(names []string) string {
	out := ""
	for i, n := range names {
		if i > 0 {
			out += ", "
		}
		out += n
	}
	return out
}
