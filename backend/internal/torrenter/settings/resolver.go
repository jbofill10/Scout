package settings

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	api "github.com/jbofill10/scout/backend/pkg/settings"
)

// Defaults are the environment-provided values the overrides sit on top of.
type Defaults struct {
	MinQuality        string
	MinFreeSpaceGB    int
	SubtitleLanguages []string
	SubtitlePreferSDH bool
	PlexSyncDelay     time.Duration
	PlexSyncInterval  time.Duration
	// SubtitlesEnabled is reported to the UI; subtitle settings have no effect
	// without an OpenSubtitles key.
	SubtitlesEnabled bool
	// AutoRotateRelay default; RelayAgentConfigured is reported to the UI.
	AutoRotateRelay      bool
	RelayAgentConfigured bool
	// Upgrade pass defaults.
	UpgradesEnabled bool
	UpgradeCutoff   string
	UpgradeBatch    int
}

// qualityOptions are the accepted quality floor labels, lowest first.
var qualityOptions = []string{"480p", "720p", "1080p", "2160p"}

var languageRe = regexp.MustCompile(`^[a-z]{2,3}$`)

// ValidationError names the field a PUT got wrong. Handlers map it to 400.
type ValidationError struct {
	Field string
	Msg   string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Msg }

// cacheTTL bounds how stale a value read by a service can be: a change made
// on the Settings page applies within this long.
const cacheTTL = 15 * time.Second

// Resolver answers "what is the value right now?" for every setting.
type Resolver struct {
	store    *Store
	defaults Defaults
	logger   *slog.Logger
	now      func() time.Time
	// FreeSpace reports the downloads volume's free bytes for the view; nil
	// or an error shows as unknown.
	FreeSpace func() (uint64, error)

	mu        sync.Mutex
	loadedAt  time.Time
	overrides map[string]json.RawMessage
}

func NewResolver(store *Store, defaults Defaults, logger *slog.Logger) *Resolver {
	if defaults.MinQuality == "" {
		defaults.MinQuality = "1080p"
	}
	if defaults.UpgradeCutoff == "" {
		defaults.UpgradeCutoff = "1080p"
	}
	if defaults.UpgradeBatch <= 0 {
		defaults.UpgradeBatch = 3
	}
	return &Resolver{store: store, defaults: defaults, logger: logger, now: time.Now}
}

// View is GET /settings: effective values, defaults, which keys are
// overridden, and the options the page needs. It always reads the store.
func (r *Resolver) View(ctx context.Context) (api.View, error) {
	if err := r.reload(ctx); err != nil {
		return api.View{}, err
	}
	return r.view(), nil
}

// Apply validates and stores an update, then returns the new view. Nothing is
// written when any field fails validation.
func (r *Resolver) Apply(ctx context.Context, upd api.Update) (api.View, error) {
	writes := map[string]any{}

	if upd.MinQuality != nil {
		label, err := normalizeQuality(*upd.MinQuality)
		if err != nil {
			return api.View{}, err
		}
		writes[api.KeyMinQuality] = label
	}
	if upd.MinFreeSpaceGB != nil {
		if *upd.MinFreeSpaceGB < 0 || *upd.MinFreeSpaceGB > 100000 {
			return api.View{}, &ValidationError{Field: api.KeyMinFreeSpaceGB, Msg: "must be between 0 and 100000"}
		}
		writes[api.KeyMinFreeSpaceGB] = *upd.MinFreeSpaceGB
	}
	if upd.SubtitleLanguages != nil {
		langs, err := normalizeLanguages(*upd.SubtitleLanguages)
		if err != nil {
			return api.View{}, err
		}
		writes[api.KeySubtitleLanguages] = langs
	}
	if upd.SubtitlePreferSDH != nil {
		writes[api.KeySubtitlePreferSDH] = *upd.SubtitlePreferSDH
	}
	if upd.AutoRotateRelay != nil {
		writes[api.KeyAutoRotateRelay] = *upd.AutoRotateRelay
	}
	if upd.UpgradesEnabled != nil {
		writes[api.KeyUpgradesEnabled] = *upd.UpgradesEnabled
	}
	if upd.UpgradeCutoff != nil {
		label, err := normalizeQuality(*upd.UpgradeCutoff)
		if err != nil {
			return api.View{}, &ValidationError{Field: api.KeyUpgradeCutoff, Msg: err.(*ValidationError).Msg}
		}
		writes[api.KeyUpgradeCutoff] = label
	}
	if upd.UpgradeBatch != nil {
		if *upd.UpgradeBatch < 1 || *upd.UpgradeBatch > 50 {
			return api.View{}, &ValidationError{Field: api.KeyUpgradeBatch, Msg: "must be between 1 and 50"}
		}
		writes[api.KeyUpgradeBatch] = *upd.UpgradeBatch
	}
	if upd.PlexSyncDelay != nil {
		d, err := parseDuration(api.KeyPlexSyncDelay, *upd.PlexSyncDelay, 0, 24*time.Hour)
		if err != nil {
			return api.View{}, err
		}
		writes[api.KeyPlexSyncDelay] = d.String()
	}
	if upd.PlexSyncInterval != nil {
		d, err := parseDuration(api.KeyPlexSyncInterval, *upd.PlexSyncInterval, time.Minute, 7*24*time.Hour)
		if err != nil {
			return api.View{}, err
		}
		writes[api.KeyPlexSyncInterval] = d.String()
	}
	resets := map[string]bool{}
	for _, key := range upd.Reset {
		if !knownKey(key) {
			return api.View{}, &ValidationError{Field: "reset", Msg: "unknown setting " + key}
		}
		resets[key] = true
	}

	for key := range resets {
		if err := r.store.Delete(ctx, key); err != nil {
			return api.View{}, err
		}
	}
	for key, value := range writes {
		if resets[key] {
			continue // a reset and a value for the same key: the reset wins
		}
		if err := r.store.Set(ctx, key, value); err != nil {
			return api.View{}, err
		}
	}

	r.mu.Lock()
	r.loadedAt = time.Time{}
	r.mu.Unlock()
	r.logger.InfoContext(ctx, "Settings updated", "written", keysOf(writes), "reset", upd.Reset)
	return r.View(ctx)
}

// MinQuality is the effective quality floor label.
func (r *Resolver) MinQuality() string {
	if v, ok := r.override(api.KeyMinQuality); ok {
		var label string
		if json.Unmarshal(v, &label) == nil && label != "" {
			return label
		}
	}
	return r.defaults.MinQuality
}

// MinFreeSpaceGB is the effective free-space reserve in gigabytes.
func (r *Resolver) MinFreeSpaceGB() int {
	if v, ok := r.override(api.KeyMinFreeSpaceGB); ok {
		var n int
		if json.Unmarshal(v, &n) == nil && n >= 0 {
			return n
		}
	}
	return r.defaults.MinFreeSpaceGB
}

// MinFreeSpaceBytes is MinFreeSpaceGB in bytes (GiB).
func (r *Resolver) MinFreeSpaceBytes() uint64 {
	return uint64(r.MinFreeSpaceGB()) << 30
}

// SubtitleLanguages is the effective language list.
func (r *Resolver) SubtitleLanguages() []string {
	if v, ok := r.override(api.KeySubtitleLanguages); ok {
		var langs []string
		if json.Unmarshal(v, &langs) == nil {
			return langs
		}
	}
	return r.defaults.SubtitleLanguages
}

// SubtitlePreferSDH is the effective SDH preference.
func (r *Resolver) SubtitlePreferSDH() bool {
	if v, ok := r.override(api.KeySubtitlePreferSDH); ok {
		var b bool
		if json.Unmarshal(v, &b) == nil {
			return b
		}
	}
	return r.defaults.SubtitlePreferSDH
}

// AutoRotateRelay is whether the indexer health monitor may rotate the relay.
func (r *Resolver) AutoRotateRelay() bool {
	if v, ok := r.override(api.KeyAutoRotateRelay); ok {
		var b bool
		if json.Unmarshal(v, &b) == nil {
			return b
		}
	}
	return r.defaults.AutoRotateRelay
}

// UpgradesEnabled is whether the upgrade pass runs.
func (r *Resolver) UpgradesEnabled() bool {
	if v, ok := r.override(api.KeyUpgradesEnabled); ok {
		var b bool
		if json.Unmarshal(v, &b) == nil {
			return b
		}
	}
	return r.defaults.UpgradesEnabled
}

// UpgradeCutoff is the quality label upgrades stop at.
func (r *Resolver) UpgradeCutoff() string {
	if v, ok := r.override(api.KeyUpgradeCutoff); ok {
		var s string
		if json.Unmarshal(v, &s) == nil && s != "" {
			return s
		}
	}
	return r.defaults.UpgradeCutoff
}

// UpgradeBatch is how many items one pass searches for.
func (r *Resolver) UpgradeBatch() int {
	if v, ok := r.override(api.KeyUpgradeBatch); ok {
		var n int
		if json.Unmarshal(v, &n) == nil && n > 0 {
			return n
		}
	}
	return r.defaults.UpgradeBatch
}

// PlexSyncDelay is the effective quiet period before a post-download sync.
func (r *Resolver) PlexSyncDelay() time.Duration {
	return r.duration(api.KeyPlexSyncDelay, r.defaults.PlexSyncDelay)
}

// PlexSyncInterval is the effective periodic sync interval; zero disables it.
func (r *Resolver) PlexSyncInterval() time.Duration {
	return r.duration(api.KeyPlexSyncInterval, r.defaults.PlexSyncInterval)
}

func (r *Resolver) duration(key string, def time.Duration) time.Duration {
	if v, ok := r.override(key); ok {
		var s string
		if json.Unmarshal(v, &s) == nil {
			if d, err := time.ParseDuration(s); err == nil && d >= 0 {
				return d
			}
		}
	}
	return def
}

// override returns the stored value for key, refreshing the cache when it is
// older than cacheTTL. A failed refresh keeps the last known overrides (or
// none), so a database blip never changes behaviour abruptly.
func (r *Resolver) override(key string) (json.RawMessage, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.now().Sub(r.loadedAt) > cacheTTL {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		overrides, err := r.store.Load(ctx)
		cancel()
		if err != nil {
			r.logger.Warn("Failed to refresh settings, keeping the last known values", "error", err)
			r.loadedAt = r.now() // do not hammer a failing database
		} else {
			r.overrides = overrides
			r.loadedAt = r.now()
		}
	}
	v, ok := r.overrides[key]
	return v, ok
}

func (r *Resolver) reload(ctx context.Context) error {
	overrides, err := r.store.Load(ctx)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.overrides = overrides
	r.loadedAt = r.now()
	r.mu.Unlock()
	return nil
}

func (r *Resolver) view() api.View {
	r.mu.Lock()
	overridden := map[string]bool{}
	for _, key := range api.Keys {
		_, overridden[key] = r.overrides[key]
	}
	r.mu.Unlock()

	freeSpace := int64(-1)
	if r.FreeSpace != nil {
		if free, err := r.FreeSpace(); err == nil {
			freeSpace = int64(free)
		}
	}

	return api.View{
		Values: api.Values{
			MinQuality:        r.MinQuality(),
			MinFreeSpaceGB:    r.MinFreeSpaceGB(),
			SubtitleLanguages: nonNil(r.SubtitleLanguages()),
			SubtitlePreferSDH: r.SubtitlePreferSDH(),
			PlexSyncDelay:     r.PlexSyncDelay().String(),
			PlexSyncInterval:  r.PlexSyncInterval().String(),
			AutoRotateRelay:   r.AutoRotateRelay(),
			UpgradesEnabled:   r.UpgradesEnabled(),
			UpgradeCutoff:     r.UpgradeCutoff(),
			UpgradeBatch:      r.UpgradeBatch(),
		},
		Defaults: api.Values{
			MinQuality:        r.defaults.MinQuality,
			MinFreeSpaceGB:    r.defaults.MinFreeSpaceGB,
			SubtitleLanguages: nonNil(r.defaults.SubtitleLanguages),
			SubtitlePreferSDH: r.defaults.SubtitlePreferSDH,
			PlexSyncDelay:     r.defaults.PlexSyncDelay.String(),
			PlexSyncInterval:  r.defaults.PlexSyncInterval.String(),
			AutoRotateRelay:   r.defaults.AutoRotateRelay,
			UpgradesEnabled:   r.defaults.UpgradesEnabled,
			UpgradeCutoff:     r.defaults.UpgradeCutoff,
			UpgradeBatch:      r.defaults.UpgradeBatch,
		},
		Overridden: overridden,
		Options: api.Options{
			MinQuality:           append([]string{}, qualityOptions...),
			SubtitlesEnabled:     r.defaults.SubtitlesEnabled,
			FreeSpaceBytes:       freeSpace,
			RelayAgentConfigured: r.defaults.RelayAgentConfigured,
		},
	}
}

func normalizeQuality(label string) (string, error) {
	l := strings.ToLower(strings.TrimSpace(label))
	if l == "4k" {
		l = "2160p"
	}
	for _, opt := range qualityOptions {
		if l == opt {
			return opt, nil
		}
	}
	return "", &ValidationError{Field: api.KeyMinQuality,
		Msg: fmt.Sprintf("must be one of %s", strings.Join(qualityOptions, ", "))}
}

func normalizeLanguages(langs []string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, raw := range langs {
		l := strings.ToLower(strings.TrimSpace(raw))
		if l == "" {
			continue
		}
		if !languageRe.MatchString(l) {
			return nil, &ValidationError{Field: api.KeySubtitleLanguages,
				Msg: fmt.Sprintf("%q is not a two or three letter language code", raw)}
		}
		if !seen[l] {
			seen[l] = true
			out = append(out, l)
		}
	}
	if len(out) == 0 {
		return nil, &ValidationError{Field: api.KeySubtitleLanguages, Msg: "at least one language is required"}
	}
	return out, nil
}

func parseDuration(field, raw string, min, max time.Duration) (time.Duration, error) {
	d, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil {
		return 0, &ValidationError{Field: field, Msg: "must be a duration such as 2m or 1h30m"}
	}
	if d == 0 && field == api.KeyPlexSyncInterval {
		return 0, nil // 0 disables the periodic sync
	}
	if d < min || d > max {
		return 0, &ValidationError{Field: field, Msg: fmt.Sprintf("must be between %s and %s", min, max)}
	}
	return d, nil
}

func knownKey(key string) bool {
	for _, k := range api.Keys {
		if k == key {
			return true
		}
	}
	return false
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
