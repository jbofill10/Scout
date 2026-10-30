package interactors

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	"github.com/jbofill10/scout/backend/internal/torrenter/service"
	"github.com/jbofill10/scout/backend/pkg/dlstatus"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/notifications"
)

// ErrUpgradesOff is returned by Run when the pass is disabled.
var ErrUpgradesOff = errors.New("upgrades are turned off")

// ErrUpgradeRunning is returned when a pass is already in progress.
var ErrUpgradeRunning = errors.New("an upgrade pass is already running")

// UpgradeSettings is the slice of the runtime settings the pass reads.
type UpgradeSettings interface {
	UpgradesEnabled() bool
	UpgradeCutoff() string
	UpgradeBatch() int
}

type upgradeDownloader interface {
	DownloadUpgrade(ctx context.Context, media *tvdb.Media, episode *tvdb.Episode, info models.UpgradeInfo,
		minTier, maxTier int, done chan<- models.TorrentCompleteEvent) (dlstatus.EpisodeResult, error)
}

type completionRunner interface {
	RunCompletionHandler(ctx context.Context, dlComplete <-chan models.TorrentCompleteEvent)
}

const (
	upgradeRetryAfter  = 7 * 24 * time.Hour
	upgradeMaxAttempts = 3
	// upgradeRecentCompleted keeps a just-upgraded item out of the next pass
	// until the Plex mirror has caught up with the new file.
	upgradeRecentCompleted = 24 * time.Hour
)

// UpgradeInteractor finds library items below the quality cutoff and sends a
// batch of them through the search pipeline with a quality window.
type UpgradeInteractor struct {
	qbitt    upgradeDownloader
	repo     service.Repository
	settings UpgradeSettings
	runner   completionRunner
	interval time.Duration
	logger   *slog.Logger
	now      func() time.Time

	mu      sync.Mutex
	running bool
}

func NewUpgradeInteractor(qbitt upgradeDownloader, repo service.Repository, settings UpgradeSettings,
	runner completionRunner, interval time.Duration, logger *slog.Logger) *UpgradeInteractor {
	return &UpgradeInteractor{qbitt: qbitt, repo: repo, settings: settings, runner: runner, interval: interval, logger: logger, now: time.Now}
}

// PlexResolutionTier maps Plex's videoResolution to Scout's quality tier and
// label. Unknown resolutions map to tier 0.
func PlexResolutionTier(res string) (int, string) {
	switch strings.ToLower(strings.TrimSpace(res)) {
	case "sd", "480", "480p", "576", "576p":
		return service.QualityTier("480p"), "480p"
	case "720", "720p":
		return service.QualityTier("720p"), "720p"
	case "1080", "1080p":
		return service.QualityTier("1080p"), "1080p"
	case "4k", "2160", "2160p":
		return service.QualityTier("2160p"), "2160p"
	}
	return 0, ""
}

// Start runs a pass every interval while upgrades are enabled.
func (u *UpgradeInteractor) Start(ctx context.Context) {
	if u.interval <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(u.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if !u.settings.UpgradesEnabled() {
					continue
				}
				if _, err := u.Run(ctx); err != nil && !errors.Is(err, ErrUpgradeRunning) {
					u.logger.ErrorContext(ctx, "Upgrade pass failed", "error", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Candidates lists the items below the cutoff that a pass would consider now.
func (u *UpgradeInteractor) Candidates(ctx context.Context) (models.UpgradeCandidatesView, error) {
	cutoff := u.settings.UpgradeCutoff()
	view := models.UpgradeCandidatesView{Enabled: u.settings.UpgradesEnabled(), Cutoff: cutoff, Batch: u.settings.UpgradeBatch(), Candidates: []models.UpgradeCandidate{}}
	cutoffTier := service.QualityTier(cutoff)

	rows, err := u.repo.GetUpgradeMedia(ctx)
	if err != nil {
		return view, err
	}
	attempts, err := u.repo.GetUpgradeAttempts(ctx)
	if err != nil {
		return view, err
	}

	type item struct {
		cand     models.UpgradeCandidate
		bestTier int
	}
	items := map[string]*item{}
	order := []string{}
	for _, row := range rows {
		tier, label := PlexResolutionTier(row.Resolution)
		key, it := u.itemFor(row)
		cur, ok := items[key]
		if !ok {
			cur = &item{cand: it}
			items[key] = cur
			order = append(order, key)
		}
		if row.FilePath != "" {
			cur.cand.Files = append(cur.cand.Files, row.FilePath)
		}
		if tier > cur.bestTier {
			cur.bestTier = tier
			cur.cand.CurrentQuality = label
		}
	}

	now := u.now()
	for _, key := range order {
		it := items[key]
		if it.bestTier == 0 || it.bestTier >= cutoffTier {
			continue
		}
		it.cand.TargetQuality = cutoff
		if a, ok := attempts[key]; ok {
			last := a
			it.cand.LastAttempt = &last
			if deferred(a, now) {
				view.Deferred++
				continue
			}
		}
		view.Candidates = append(view.Candidates, it.cand)
	}
	// Worst copies first, then a stable name order so passes are predictable.
	sort.SliceStable(view.Candidates, func(i, j int) bool {
		ti, _ := PlexResolutionTier(view.Candidates[i].CurrentQuality)
		tj, _ := PlexResolutionTier(view.Candidates[j].CurrentQuality)
		if ti != tj {
			return ti < tj
		}
		return view.Candidates[i].Title < view.Candidates[j].Title
	})
	view.Total = len(view.Candidates)
	return view, nil
}

// deferred says whether an item's last attempt keeps it out of this pass.
func deferred(a models.UpgradeAttempt, now time.Time) bool {
	switch a.Status {
	case "searching", "downloading":
		return now.Sub(a.UpdatedAt) < 12*time.Hour // a stuck row frees itself eventually
	case "completed":
		return now.Sub(a.UpdatedAt) < upgradeRecentCompleted
	default: // no_release, failed
		if a.Attempts >= upgradeMaxAttempts {
			return true
		}
		return now.Sub(a.UpdatedAt) < upgradeRetryAfter
	}
}

func (u *UpgradeInteractor) itemFor(row models.UpgradeMediaRow) (string, models.UpgradeCandidate) {
	if row.Kind == "movie" {
		return row.MovieTvdbID, models.UpgradeCandidate{ItemKey: row.MovieTvdbID, Kind: "movie", Title: row.MovieTitle, Year: row.Year}
	}
	return row.EpisodeTvdbID, models.UpgradeCandidate{ItemKey: row.EpisodeTvdbID, Kind: "episode", Title: row.ShowTitle,
		ShowTvdbID: row.ShowTvdbID, Season: row.Season, Episode: row.Episode}
}

// Attempts lists recent attempts for the Settings page.
func (u *UpgradeInteractor) Attempts(ctx context.Context, limit int) ([]models.UpgradeAttempt, error) {
	return u.repo.ListUpgradeAttempts(ctx, limit)
}

// Run starts one pass: a batch of candidates is searched for in the
// background, each recorded as an attempt with a notification, and the call
// returns as soon as the batch is chosen.
func (u *UpgradeInteractor) Run(ctx context.Context) (models.UpgradeRunResult, error) {
	if !u.settings.UpgradesEnabled() {
		return models.UpgradeRunResult{}, ErrUpgradesOff
	}
	u.mu.Lock()
	if u.running {
		u.mu.Unlock()
		return models.UpgradeRunResult{}, ErrUpgradeRunning
	}
	u.running = true
	u.mu.Unlock()

	view, err := u.Candidates(ctx)
	if err != nil {
		u.clearRunning()
		return models.UpgradeRunResult{}, err
	}
	batch := view.Candidates
	if len(batch) > view.Batch {
		batch = batch[:view.Batch]
	}
	result := models.UpgradeRunResult{Started: len(batch), Items: []string{}}
	for _, c := range batch {
		result.Items = append(result.Items, describeCandidate(c))
	}
	if len(batch) == 0 {
		u.clearRunning()
		result.Message = "Nothing below the cutoff needs an upgrade right now"
		return result, nil
	}

	// The searches take minutes; run them off the request.
	bg := context.WithoutCancel(ctx)
	go func() {
		defer u.clearRunning()
		cutoffTier := service.QualityTier(view.Cutoff)
		for _, c := range batch {
			u.upgradeOne(bg, c, cutoffTier, view.Cutoff)
		}
		u.logger.InfoContext(bg, "Upgrade pass finished", "items", len(batch))
	}()
	return result, nil
}

func (u *UpgradeInteractor) clearRunning() {
	u.mu.Lock()
	u.running = false
	u.mu.Unlock()
}

func describeCandidate(c models.UpgradeCandidate) string {
	if c.Kind == "movie" {
		return fmt.Sprintf("%s (%s)", c.Title, c.CurrentQuality)
	}
	return fmt.Sprintf("%s S%02dE%02d (%s)", c.Title, c.Season, c.Episode, c.CurrentQuality)
}

// upgradeOne records the attempt, puts a notification on the Activity page
// and runs the search; the outcome of the search (not the download) settles
// the attempt here, the completion handler settles the download later.
func (u *UpgradeInteractor) upgradeOne(ctx context.Context, c models.UpgradeCandidate, cutoffTier int, cutoff string) {
	currentTier, _ := PlexResolutionTier(c.CurrentQuality)
	attempt := models.UpgradeAttempt{ItemKey: c.ItemKey, Kind: c.Kind, Title: c.Title, Season: c.Season, Episode: c.Episode,
		FromQuality: c.CurrentQuality, ToQuality: cutoff, Status: "searching"}
	if err := u.repo.RecordUpgradeAttempt(ctx, attempt); err != nil {
		u.logger.ErrorContext(ctx, "Failed to record upgrade attempt", "error", err, "item", c.ItemKey)
		return
	}

	media, episode := u.mediaFor(c)
	reason := fmt.Sprintf("Upgrading from %s", c.CurrentQuality)
	u.upsertNotification(ctx, media, episode, notifications.StatusSearching, reason)

	done := make(chan models.TorrentCompleteEvent, 1)
	go u.runner.RunCompletionHandler(ctx, done)
	info := models.UpgradeInfo{FromQuality: c.CurrentQuality, OldFiles: c.Files}
	result, err := u.qbitt.DownloadUpgrade(ctx, &media, episode, info, currentTier+1, cutoffTier, done)
	switch {
	case err != nil:
		u.settle(ctx, c.ItemKey, "failed", err.Error())
		u.upsertNotification(ctx, media, episode, notifications.StatusFailed, "Upgrade failed: "+err.Error())
	case result.Outcome == dlstatus.OutcomeDownloading:
		u.settle(ctx, c.ItemKey, "downloading", "")
	case result.Code == dlstatus.CodeNoTorrentFound:
		u.settle(ctx, c.ItemKey, "no_release", fmt.Sprintf("No release above %s found", c.CurrentQuality))
		u.upsertNotification(ctx, media, episode, notifications.StatusFailed,
			fmt.Sprintf("No release above %s found; will look again in a week", c.CurrentQuality))
	default:
		msg := result.Reason
		if msg == "" {
			msg = string(result.Code)
		}
		u.settle(ctx, c.ItemKey, "failed", msg)
		u.upsertNotification(ctx, media, episode, notifications.StatusFailed, "Upgrade failed: "+msg)
	}
}

func (u *UpgradeInteractor) settle(ctx context.Context, key, status, reason string) {
	if err := u.repo.SetUpgradeAttemptStatus(ctx, key, status, reason); err != nil {
		u.logger.ErrorContext(ctx, "Failed to update upgrade attempt", "error", err, "item", key)
	}
}

// mediaFor rebuilds the request payload from the mirror's row. Aliases and
// the anime flag are unknown here, so the plain seasonal strategies run.
func (u *UpgradeInteractor) mediaFor(c models.UpgradeCandidate) (tvdb.Media, *tvdb.Episode) {
	if c.Kind == "movie" {
		m := tvdb.Media{Id: c.ItemKey, Name: c.Title, Category: "movie", Source: "upgrade"}
		if c.Year > 0 {
			m.Year = strconv.Itoa(c.Year)
		}
		return m, nil
	}
	id, _ := strconv.Atoi(c.ItemKey)
	ep := &tvdb.Episode{Id: id, SeasonNumber: c.Season, Number: c.Episode}
	return tvdb.Media{Id: c.ShowTvdbID, Name: c.Title, Category: "series", Source: "upgrade", Metadata: tvdb.TVDBSeriesMetadata{Episodes: []tvdb.Episode{*ep}}}, ep
}

// upsertNotification reuses the item's notification when one exists (the
// episode was requested before) and creates one otherwise.
func (u *UpgradeInteractor) upsertNotification(ctx context.Context, media tvdb.Media, episode *tvdb.Episode, status notifications.NotificationStatus, reason string) {
	key := media.Id
	if episode != nil {
		key = strconv.Itoa(episode.Id)
	}
	existing, err := u.repo.GetNotification(ctx, key)
	if err == nil && existing != nil {
		existing.Status = status
		existing.Reason = reason
		existing.IsRead = false
		existing.AutoDismissed = false
		if err := u.repo.UpdateNotification(ctx, existing); err != nil {
			u.logger.ErrorContext(ctx, "Failed to update upgrade notification", "error", err, "item", key)
		}
		return
	}
	var n *notifications.Notification
	if episode != nil {
		n, err = notifications.NewFromSeries(media, *episode, "", "")
	} else {
		n, err = notifications.NewFromMovie(media, "", "")
	}
	if err != nil {
		u.logger.ErrorContext(ctx, "Failed to build upgrade notification", "error", err, "item", key)
		return
	}
	n.Status = status
	n.Reason = reason
	if err := u.repo.CreateNotification(ctx, n); err != nil {
		u.logger.ErrorContext(ctx, "Failed to create upgrade notification", "error", err, "item", key)
	}
}
