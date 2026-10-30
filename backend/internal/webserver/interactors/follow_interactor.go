package interactors

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"time"

	"github.com/jbofill10/scout/backend/internal/webserver/repository"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
)

// ErrNotSeries is returned when something other than a show is followed.
var ErrNotSeries = errors.New("only series can be followed")

// showRequester is the existing show request path; the follow check feeds
// it the episodes that are new, so searching, scheduling and notifications
// behave exactly as for a manual request.
type showRequester interface {
	DownloadShow(ctx context.Context, req tvdb.Media) (DownloadSummary, error)
}

type followTVDB interface {
	GetExtendedInfo(ctx context.Context, mediaId, mediaType string) (tvdb.TVDBSeriesExtendedResponse, error)
}

type followTorrenter interface {
	GetStatusBatch(ctx context.Context, requests []tvdb.StatusRequest) (tvdb.StatusBatchResponse, error)
}

type followNotifications interface {
	ExistingTvdbIDs(ctx context.Context, tvdbIDs []string) (map[string]bool, error)
}

type followScheduler interface {
	TrackedItemKeys(ctx context.Context) (map[string]bool, error)
}

// CheckResult is what one follow check did for one show.
type CheckResult struct {
	TvdbID    string    `json:"tvdb_id"`
	Title     string    `json:"title"`
	Listed    int       `json:"listed"`             // episodes TVDB lists
	New       int       `json:"new"`                // episodes handed to the request path
	Deferred  int       `json:"deferred,omitempty"` // wanted episodes left for a later check by the per-run budget
	QueuedNow int       `json:"queued_now"`         // of those, searched for right away
	Scheduled int       `json:"scheduled"`          // of those, scheduled for release
	Error     string    `json:"error,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
}

// CheckReport covers one pass over every followed show.
type CheckReport struct {
	Checked int           `json:"checked"`
	Added   int           `json:"added"`
	Results []CheckResult `json:"results"`
}

// FollowResponse is what POST /follows returns: the stored follow and what
// the first check did.
type FollowResponse struct {
	Show  repository.FollowedShow `json:"show"`
	Check CheckResult             `json:"check"`
}

// FollowInteractor keeps followed shows current: on a timer and on demand it
// asks TVDB for each show's episodes and requests the ones Scout has not seen.
type FollowInteractor struct {
	repo      repository.FollowRepository
	sched     followScheduler
	notifs    followNotifications
	tvdb      followTVDB
	torrenter followTorrenter
	requester showRequester
	interval  time.Duration
	logger    *slog.Logger
	now       func() time.Time
	// bulkBatch is how many episodes one scheduled check may request across
	// all shows (0 = no limit); it follows the bulk_request_batch setting.
	bulkBatch func(ctx context.Context) int
	// firstRunDelay lets the other services come up before the start-up check.
	firstRunDelay time.Duration
}

func NewFollowInteractor(
	repo repository.FollowRepository,
	sched followScheduler,
	notifs followNotifications,
	tvdbClient followTVDB,
	torrenter followTorrenter,
	requester showRequester,
	interval time.Duration,
	logger *slog.Logger,
) *FollowInteractor {
	return &FollowInteractor{
		repo:          repo,
		sched:         sched,
		notifs:        notifs,
		tvdb:          tvdbClient,
		torrenter:     torrenter,
		requester:     requester,
		interval:      interval,
		logger:        logger,
		now:           time.Now,
		firstRunDelay: 2 * time.Minute,
		bulkBatch:     func(context.Context) int { return 0 },
	}
}

// Follow stores the show and runs its first check right away.
func (f *FollowInteractor) Follow(ctx context.Context, media tvdb.Media, includeMissing bool) (FollowResponse, error) {
	if media.Category != "" && media.Category != "series" {
		return FollowResponse{}, ErrNotSeries
	}
	if media.Id == "" || media.Name == "" {
		return FollowResponse{}, errors.New("media needs an id and a name")
	}
	media.Category = "series"
	media.Metadata.Episodes = nil

	show := repository.FollowedShow{
		TvdbID:         media.Id,
		Title:          media.Name,
		PosterUrl:      media.ImageUrl,
		Media:          media,
		IncludeMissing: includeMissing,
		FollowedAt:     f.now(),
	}
	if err := f.repo.Upsert(ctx, show); err != nil {
		return FollowResponse{}, err
	}
	// Re-read so followed_at reflects what was stored (a re-follow keeps the
	// original date, which is what the pre-follow filter should use).
	if stored, err := f.repo.Get(ctx, media.Id); err == nil && stored != nil {
		show = *stored
	}

	result := f.checkShow(ctx, show, -1)
	if stored, err := f.repo.Get(ctx, media.Id); err == nil && stored != nil {
		show = *stored
	}
	f.logger.InfoContext(ctx, "Followed show", "tvdb_id", media.Id, "title", media.Name,
		"include_missing", includeMissing, "new_episodes", result.New)
	return FollowResponse{Show: show, Check: result}, nil
}

// Unfollow stops checking a show; nothing already requested is touched.
func (f *FollowInteractor) Unfollow(ctx context.Context, tvdbID string) (bool, error) {
	return f.repo.Delete(ctx, tvdbID)
}

func (f *FollowInteractor) List(ctx context.Context) ([]repository.FollowedShow, error) {
	return f.repo.List(ctx)
}

// CheckOne re-checks a single followed show now.
func (f *FollowInteractor) CheckOne(ctx context.Context, tvdbID string) (CheckResult, error) {
	show, err := f.repo.Get(ctx, tvdbID)
	if err != nil {
		return CheckResult{}, err
	}
	if show == nil {
		return CheckResult{}, ErrNothingToControl
	}
	return f.checkShow(ctx, *show, -1), nil
}

// CheckAll re-checks every followed show now.
func (f *FollowInteractor) CheckAll(ctx context.Context) (CheckReport, error) {
	shows, err := f.repo.List(ctx)
	if err != nil {
		return CheckReport{}, err
	}
	// Least recently checked first, so a budget that runs out leaves the
	// shows that waited longest at the front next time.
	sort.SliceStable(shows, func(i, j int) bool {
		a, b := shows[i].LastCheckedAt, shows[j].LastCheckedAt
		switch {
		case a == nil:
			return b != nil
		case b == nil:
			return false
		default:
			return a.Before(*b)
		}
	})
	budget := f.bulkBatch(ctx)
	report := CheckReport{Results: []CheckResult{}}
	for _, show := range shows {
		if ctx.Err() != nil {
			break
		}
		limit := -1
		if budget > 0 {
			limit = max(budget-report.Added, 0)
		}
		result := f.checkShow(ctx, show, limit)
		report.Checked++
		report.Added += result.New
		report.Results = append(report.Results, result)
	}
	f.logger.InfoContext(ctx, "Follow check complete", "shows", report.Checked, "episodes_added", report.Added)
	return report, nil
}

// Start runs CheckAll shortly after boot and then every interval, until ctx ends.
func (f *FollowInteractor) Start(ctx context.Context) {
	go func() {
		timer := time.NewTimer(f.firstRunDelay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return
		}
		if _, err := f.CheckAll(ctx); err != nil {
			f.logger.ErrorContext(ctx, "Follow check failed", "error", err)
		}

		ticker := time.NewTicker(f.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if _, err := f.CheckAll(ctx); err != nil {
					f.logger.ErrorContext(ctx, "Follow check failed", "error", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

// checkShow fetches the show's episodes, drops everything Scout already
// handles, and requests the rest through the normal show request path.
// SetBulkBatch sets where the per-check episode budget comes from.
func (f *FollowInteractor) SetBulkBatch(fn func(ctx context.Context) int) {
	if fn != nil {
		f.bulkBatch = fn
	}
}

// checkShow requests what the show is missing. limit caps how many episodes
// this check may hand over (-1 for no cap); the rest wait for a later check.
func (f *FollowInteractor) checkShow(ctx context.Context, show repository.FollowedShow, limit int) CheckResult {
	result := CheckResult{TvdbID: show.TvdbID, Title: show.Title, CheckedAt: f.now()}
	fail := func(err error) CheckResult {
		result.Error = err.Error()
		f.logger.ErrorContext(ctx, "Follow check failed for show", "tvdb_id", show.TvdbID, "title", show.Title, "error", err)
		if recErr := f.repo.RecordCheck(ctx, show.TvdbID, 0, result.Error); recErr != nil {
			f.logger.ErrorContext(ctx, "Failed to record follow check", "error", recErr)
		}
		return result
	}

	ext, err := f.tvdb.GetExtendedInfo(ctx, show.TvdbID, "series")
	if err != nil {
		return fail(fmt.Errorf("tvdb lookup: %w", err))
	}
	episodes := ext.Data.Episodes
	result.Listed = len(episodes)

	inPlex, err := f.plexEpisodes(ctx, show.TvdbID)
	if err != nil {
		return fail(fmt.Errorf("library lookup: %w", err))
	}
	tracked, err := f.sched.TrackedItemKeys(ctx)
	if err != nil {
		return fail(fmt.Errorf("scheduled rows: %w", err))
	}
	ids := make([]string, 0, len(episodes))
	for _, ep := range episodes {
		ids = append(ids, strconv.Itoa(ep.Id))
	}
	notified, err := f.notifs.ExistingTvdbIDs(ctx, ids)
	if err != nil {
		return fail(fmt.Errorf("notifications: %w", err))
	}

	followedDay := show.FollowedAt.Truncate(24 * time.Hour)
	var fresh []tvdb.Episode
	for _, ep := range episodes {
		if ep.SeasonNumber == 0 {
			continue // specials are never followed
		}
		aired, err := time.Parse("2006-01-02", ep.Aired)
		if err != nil {
			continue // no air date yet: it will be picked up once TVDB has one
		}
		if !show.IncludeMissing && aired.Before(followedDay) {
			continue // the back catalogue was not asked for
		}
		key := strconv.Itoa(ep.Id)
		if tracked[key] || notified[key] {
			continue
		}
		if inPlex[seasonEpisodeKey(ep.SeasonNumber, ep.Number)] || inPlex[key] {
			continue
		}
		fresh = append(fresh, ep)
	}
	if limit >= 0 && len(fresh) > limit {
		result.Deferred = len(fresh) - limit
		fresh = fresh[:limit] // TVDB order: the oldest wanted episodes go first
	}
	result.New = len(fresh)

	if len(fresh) > 0 {
		media := show.Media
		media.Id = show.TvdbID
		media.Name = show.Title
		media.Category = "series"
		media.Source = "follow"
		media.Metadata.Episodes = fresh
		summary, err := f.requester.DownloadShow(ctx, media)
		if err != nil {
			return fail(fmt.Errorf("request: %w", err))
		}
		result.QueuedNow = summary.QueuedNow
		result.Scheduled = summary.Scheduled
		f.logger.InfoContext(ctx, "Follow check requested new episodes", "tvdb_id", show.TvdbID, "title", show.Title,
			"new", len(fresh), "queued_now", summary.QueuedNow, "scheduled", summary.Scheduled)
	}

	if err := f.repo.RecordCheck(ctx, show.TvdbID, result.QueuedNow+result.Scheduled, ""); err != nil {
		f.logger.ErrorContext(ctx, "Failed to record follow check", "error", err)
	}
	return result
}

// plexEpisodes returns the episodes the torrenter reports as in the library,
// keyed both by "S:E" and by episode tvdb id when it knows one.
func (f *FollowInteractor) plexEpisodes(ctx context.Context, showID string) (map[string]bool, error) {
	resp, err := f.torrenter.GetStatusBatch(ctx, []tvdb.StatusRequest{{TvdbId: showID, MediaType: "show"}})
	if err != nil {
		return nil, err
	}
	have := map[string]bool{}
	for _, show := range resp.Shows {
		if show.TvdbId != showID {
			continue
		}
		for _, season := range show.Seasons {
			for _, ep := range season.Episodes {
				if !ep.Downloaded {
					continue
				}
				have[seasonEpisodeKey(season.SeasonNum, ep.EpisodeNum)] = true
				if ep.TvdbId != "" {
					have[ep.TvdbId] = true
				}
			}
		}
	}
	return have, nil
}

func seasonEpisodeKey(season, episode int) string {
	return "s" + strconv.Itoa(season) + "e" + strconv.Itoa(episode)
}
