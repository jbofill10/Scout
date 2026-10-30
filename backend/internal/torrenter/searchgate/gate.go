// Package searchgate paces and orders the torrenter's indexer searches so a
// burst of automated work (a follow check, a missing-episode fill, an upgrade
// pass) cannot get the exit IP banned, and so a person's request is never
// stuck behind it.
//
// Every Prowlarr search passes through one Gate. Per indexer it keeps a token
// bucket (sustained searches per minute with a small burst) and lets one
// search run at a time; callers that cannot be served immediately wait in a
// queue ordered by Priority, then by arrival. An indexer the health monitor
// reports as disabled is paused: searches against it fail fast with
// ErrPaused so the caller can move on to its fallback indexers instead of
// piling onto a dead exit.
package searchgate

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Priority orders waiting searches. Lower values go first.
type Priority int

const (
	// Manual is work a person asked for right now: a request from the UI, a
	// retry-now, a release listing or a manual grab.
	Manual Priority = iota
	// Scheduled is an episode dispatched on its air date or an automatic retry.
	Scheduled
	// Follow is a follow check requesting new or back-catalogue episodes.
	Follow
	// Missing is a fill of a library show's missing episodes.
	Missing
	// Upgrade is the quality-upgrade pass.
	Upgrade
)

var priorityNames = map[Priority]string{Manual: "manual", Scheduled: "scheduled", Follow: "follow", Missing: "missing", Upgrade: "upgrade"}

func (p Priority) String() string {
	if s, ok := priorityNames[p]; ok {
		return s
	}
	return fmt.Sprintf("priority(%d)", int(p))
}

// PriorityFor maps a request source ("manual", "scheduled", "follow",
// "missing", "upgrade") to a Priority. Unknown and empty sources are Manual,
// which keeps requests from older clients at the front.
func PriorityFor(source string) Priority {
	for p, name := range priorityNames {
		if name == source {
			return p
		}
	}
	return Manual
}

// Limits is one indexer's allowance: PerMinute sustained searches with up to
// Burst spent at once.
type Limits struct {
	PerMinute float64 `json:"per_minute"`
	Burst     int     `json:"burst"`
}

// DefaultLimits is used when no limits function is configured.
var DefaultLimits = Limits{PerMinute: 6, Burst: 3}

func (l Limits) normalized() Limits {
	if l.PerMinute <= 0 {
		l.PerMinute = DefaultLimits.PerMinute
	}
	if l.Burst < 1 {
		l.Burst = 1
	}
	return l
}

// ErrPaused is returned when every requested indexer is paused.
var ErrPaused = errors.New("indexer paused")

// IndexerStats is one indexer's view of the gate, for the health endpoint.
type IndexerStats struct {
	ID                 int64          `json:"id"`
	Limits             Limits         `json:"limits"`
	Tokens             float64        `json:"tokens"`
	InFlight           string         `json:"in_flight,omitempty"`
	Queued             int            `json:"queued"`
	QueuedByPriority   map[string]int `json:"queued_by_priority,omitempty"`
	Waiting            []string       `json:"waiting,omitempty"`
	SearchesLastMinute int            `json:"searches_last_minute"`
	PausedUntil        *time.Time     `json:"paused_until,omitempty"`
	PauseReason        string         `json:"pause_reason,omitempty"`
}

// Gate is safe for concurrent use. The zero value is not usable; use New.
type Gate struct {
	mu       sync.Mutex
	now      func() time.Time
	limits   func(indexerID int64) Limits
	indexers map[int64]*indexer
	seq      uint64
}

// New returns a gate whose per-indexer limits come from limits, consulted on
// every refill so a settings change applies without a restart. A nil limits
// uses DefaultLimits for every indexer.
func New(limits func(indexerID int64) Limits) *Gate {
	if limits == nil {
		limits = func(int64) Limits { return DefaultLimits }
	}
	return &Gate{now: time.Now, limits: limits, indexers: map[int64]*indexer{}}
}

type indexer struct {
	id          int64
	tokens      float64
	lastRefill  time.Time
	inFlight    *waiter
	pausedUntil time.Time
	pauseReason string
	waiters     waiterHeap
	history     []time.Time
	wake        *time.Timer
}

type waiter struct {
	prio     Priority
	seq      uint64
	label    string
	ready    chan struct{}
	err      error
	admitted bool
	index    int
}

// Acquire waits for permission to run one Prowlarr search against ids and
// returns the indexers it may search (paused ones are left out) with a
// release function to call when the search has finished. It returns
// ErrPaused when every indexer is paused, or ctx's error when the wait is
// abandoned. Indexers are acquired in ascending id order so concurrent
// multi-indexer searches cannot deadlock.
func (g *Gate) Acquire(ctx context.Context, ids []int64, prio Priority, label string) ([]int64, func(), error) {
	sorted := append([]int64(nil), ids...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	granted := make([]int64, 0, len(sorted))
	release := func() {
		for _, id := range granted {
			g.release(id)
		}
	}
	for _, id := range sorted {
		err := g.acquireOne(ctx, id, prio, label)
		switch {
		case err == nil:
			granted = append(granted, id)
		case errors.Is(err, ErrPaused):
			continue
		default:
			release()
			return nil, func() {}, err
		}
	}
	if len(granted) == 0 {
		return nil, func() {}, fmt.Errorf("%w: %s", ErrPaused, g.pauseReasons(sorted))
	}
	return granted, release, nil
}

func (g *Gate) acquireOne(ctx context.Context, id int64, prio Priority, label string) error {
	g.mu.Lock()
	ix := g.indexerLocked(id)
	g.refillLocked(ix)
	if ix.pausedAt(g.now()) {
		g.mu.Unlock()
		return ErrPaused
	}
	w := &waiter{prio: prio, seq: g.seq, label: label, ready: make(chan struct{})}
	g.seq++
	if ix.waiters.Len() == 0 && ix.canServe() {
		g.takeLocked(ix, w)
		g.mu.Unlock()
		return nil
	}
	heap.Push(&ix.waiters, w)
	g.scheduleWakeLocked(ix)
	g.mu.Unlock()

	select {
	case <-w.ready:
		return w.err
	case <-ctx.Done():
		g.mu.Lock()
		if w.admitted {
			// Admitted in the same instant the caller gave up: hand the slot back.
			g.mu.Unlock()
			g.release(id)
			return ctx.Err()
		}
		if w.index >= 0 {
			heap.Remove(&ix.waiters, w.index)
		}
		g.mu.Unlock()
		return ctx.Err()
	}
}

func (g *Gate) release(id int64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	ix := g.indexerLocked(id)
	ix.inFlight = nil
	g.admitLocked(ix)
}

// Pause stops searches against an indexer until until, failing waiting and
// new searches fast with ErrPaused so callers fall through to other indexers.
func (g *Gate) Pause(id int64, until time.Time, reason string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	ix := g.indexerLocked(id)
	ix.pausedUntil = until
	ix.pauseReason = reason
	for ix.waiters.Len() > 0 {
		w := heap.Pop(&ix.waiters).(*waiter)
		w.err = ErrPaused
		close(w.ready)
	}
}

// Resume lifts a pause early, typically when the health monitor sees the
// indexer healthy again.
func (g *Gate) Resume(id int64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	ix := g.indexerLocked(id)
	ix.pausedUntil = time.Time{}
	ix.pauseReason = ""
	g.admitLocked(ix)
}

// Snapshot reports every indexer the gate has seen, lowest id first.
func (g *Gate) Snapshot() []IndexerStats {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	out := make([]IndexerStats, 0, len(g.indexers))
	for _, ix := range g.indexers {
		g.refillLocked(ix)
		st := IndexerStats{ID: ix.id, Limits: g.limits(ix.id).normalized(), Tokens: ix.tokens, Queued: ix.waiters.Len()}
		if ix.inFlight != nil {
			st.InFlight = ix.inFlight.label
			if st.InFlight == "" {
				st.InFlight = ix.inFlight.prio.String()
			}
		}
		if st.Queued > 0 {
			st.QueuedByPriority = map[string]int{}
			ordered := append([]*waiter(nil), ix.waiters...)
			sort.Slice(ordered, func(i, j int) bool {
				if ordered[i].prio != ordered[j].prio {
					return ordered[i].prio < ordered[j].prio
				}
				return ordered[i].seq < ordered[j].seq
			})
			for _, w := range ordered {
				st.QueuedByPriority[w.prio.String()]++
				if len(st.Waiting) < 10 && w.label != "" {
					st.Waiting = append(st.Waiting, w.label)
				}
			}
		}
		cutoff := now.Add(-time.Minute)
		for _, t := range ix.history {
			if t.After(cutoff) {
				st.SearchesLastMinute++
			}
		}
		if ix.pausedAt(now) {
			until := ix.pausedUntil
			st.PausedUntil = &until
			st.PauseReason = ix.pauseReason
		}
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// --- internals (all called with g.mu held) ---

func (g *Gate) indexerLocked(id int64) *indexer {
	ix, ok := g.indexers[id]
	if !ok {
		lim := g.limits(id).normalized()
		ix = &indexer{id: id, tokens: float64(lim.Burst), lastRefill: g.now()}
		g.indexers[id] = ix
	}
	return ix
}

func (g *Gate) refillLocked(ix *indexer) {
	lim := g.limits(ix.id).normalized()
	now := g.now()
	elapsed := now.Sub(ix.lastRefill)
	ix.lastRefill = now
	if elapsed > 0 {
		ix.tokens += elapsed.Minutes() * lim.PerMinute
	}
	if ix.tokens > float64(lim.Burst) {
		ix.tokens = float64(lim.Burst)
	}
	// Forget history older than a minute.
	cutoff := now.Add(-time.Minute)
	kept := ix.history[:0]
	for _, t := range ix.history {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	ix.history = kept
}

func (ix *indexer) canServe() bool { return ix.inFlight == nil && ix.tokens >= 1 }

func (ix *indexer) pausedAt(now time.Time) bool {
	return !ix.pausedUntil.IsZero() && now.Before(ix.pausedUntil)
}

func (g *Gate) takeLocked(ix *indexer, w *waiter) {
	ix.tokens--
	ix.inFlight = w
	ix.history = append(ix.history, g.now())
	w.admitted = true
}

func (g *Gate) admitLocked(ix *indexer) {
	g.refillLocked(ix)
	if ix.pausedAt(g.now()) {
		return
	}
	for ix.waiters.Len() > 0 && ix.canServe() {
		w := heap.Pop(&ix.waiters).(*waiter)
		g.takeLocked(ix, w)
		close(w.ready)
	}
	g.scheduleWakeLocked(ix)
}

// scheduleWakeLocked arms a timer for when the next token exists, if someone
// is waiting on tokens rather than on the in-flight slot.
func (g *Gate) scheduleWakeLocked(ix *indexer) {
	if ix.waiters.Len() == 0 || ix.tokens >= 1 || ix.wake != nil {
		return
	}
	lim := g.limits(ix.id).normalized()
	wait := time.Duration((1 - ix.tokens) / lim.PerMinute * float64(time.Minute))
	if wait < 10*time.Millisecond {
		wait = 10 * time.Millisecond
	}
	ix.wake = time.AfterFunc(wait, func() {
		g.mu.Lock()
		defer g.mu.Unlock()
		ix.wake = nil
		g.admitLocked(ix)
	})
}

func (g *Gate) pauseReasons(ids []int64) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	reasons := ""
	for _, id := range ids {
		if ix, ok := g.indexers[id]; ok && ix.pauseReason != "" {
			if reasons != "" {
				reasons += "; "
			}
			reasons += fmt.Sprintf("indexer %d: %s", id, ix.pauseReason)
		}
	}
	if reasons == "" {
		return "every requested indexer is paused"
	}
	return reasons
}

// --- priority queue ---

type waiterHeap []*waiter

func (h waiterHeap) Len() int { return len(h) }
func (h waiterHeap) Less(i, j int) bool {
	if h[i].prio != h[j].prio {
		return h[i].prio < h[j].prio
	}
	return h[i].seq < h[j].seq
}
func (h waiterHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index, h[j].index = i, j
}
func (h *waiterHeap) Push(x any) {
	w := x.(*waiter)
	w.index = len(*h)
	*h = append(*h, w)
}
func (h *waiterHeap) Pop() any {
	old := *h
	n := len(old)
	w := old[n-1]
	old[n-1] = nil
	w.index = -1
	*h = old[:n-1]
	return w
}

// --- request metadata carried on the context ---

type ctxKey struct{}

type request struct {
	prio  Priority
	label string
}

// WithRequest tags a context with the priority and a short label (what is
// being searched for) for every search made under it.
func WithRequest(ctx context.Context, prio Priority, label string) context.Context {
	return context.WithValue(ctx, ctxKey{}, request{prio: prio, label: label})
}

// FromContext returns the priority and label set by WithRequest, or Manual
// with no label.
func FromContext(ctx context.Context) (Priority, string) {
	if r, ok := ctx.Value(ctxKey{}).(request); ok {
		return r.prio, r.label
	}
	return Manual, ""
}
