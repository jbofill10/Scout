package service

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"sync"
	"time"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	"github.com/jbofill10/scout/backend/pkg/dlstatus"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/notifications"
	"github.com/jbofill10/scout/backend/pkg/telemetry"

	qbittorrent "github.com/autobrr/go-qbittorrent"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"golift.io/starr"
	"golift.io/starr/prowlarr"
)

var (
	baseSavePath  = "/data/Downloads"
	scoutTag      = "scout"
	torrentTracer = otel.Tracer("torrenter/torrent")

	// trailingYearRe matches a trailing parenthesized or bracketed year, e.g.
	// " (2019)" or " [2019]", optionally preceded by whitespace.
	trailingYearRe = regexp.MustCompile(`\s*[\(\[]\d{4}[\)\]]\s*$`)
)

type QbittHandler struct {
	c      *qbittorrent.Client
	p      *prowlarr.Prowlarr
	repo   Repository
	logger *slog.Logger
	parser *TorrentParser
	// minQualityTier is the lowest resolution tier pickBestTorrent will accept
	// (see QualityTier). Zero disables the floor.
	minQualityTier int
}

const (
	// qbittLoginTimeout bounds a single login attempt. The client's own default is 60s,
	// which is long enough that an unresponsive qBittorrent stalls startup badly.
	qbittLoginTimeout = 10 * time.Second
	// qbittLoginRetryInterval is how often the background reconnect loop retries after
	// an initial login failure.
	qbittLoginRetryInterval = 30 * time.Second
)

// NewQbittHandler builds the qBittorrent handler. A failed initial login is NOT fatal:
// the handler is returned anyway and a background goroutine keeps retrying. Torrent
// operations fail (and are reported as CodeTorrentClientError) until a login succeeds,
// but the rest of the service — the HTTP API and the Plex library sync — still starts.
// Blocking startup on qBittorrent previously meant an unresponsive torrent client took
// the whole torrenter down in a crash loop, which also starved the Plex sync.
func NewQbittHandler(qCfg *models.QbittCfg, pCfg *models.ProwlarrCfg, repo Repository, logger *slog.Logger) (*QbittHandler, error) {
	logger.Debug("QBittorrent config", "host", qCfg.Host, "user", qCfg.User)

	// Create new qBittorrent client with Config
	qb := qbittorrent.NewClient(qbittorrent.Config{
		Host:     qCfg.Host,
		Username: qCfg.User,
		Password: qCfg.Password,
		Timeout:  int(qbittLoginTimeout.Seconds()),
	})

	p := prowlarr.New(starr.New(pCfg.Key, pCfg.Host, 60*time.Hour))
	q := &QbittHandler{
		c:              qb,
		logger:         logger,
		p:              p,
		repo:           repo,
		parser:         NewTorrentParser(),
		minQualityTier: minQualityTier(),
	}
	logger.Info("Torrent quality floor", "min_quality_tier", q.minQualityTier)

	if err := loginWithTimeout(qb, qbittLoginTimeout); err != nil {
		logger.Warn("Failed to login to qBittorrent, continuing without it and retrying in background",
			"host", qCfg.Host, "error", err)
		go q.retryLogin()
		return q, nil
	}

	logger.Info("Connected to QBittorrent successfully")
	return q, nil
}

// loginWithTimeout performs a single login attempt bounded by timeout.
func loginWithTimeout(qb *qbittorrent.Client, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := qb.LoginCtx(ctx); err != nil {
		return fmt.Errorf("failed to login to qBittorrent: %w", err)
	}
	return nil
}

// retryLogin keeps attempting to authenticate until it succeeds. It runs for the life of
// the process; there is nothing useful to do with a permanently unreachable client except
// keep trying, and downloads report CodeTorrentClientError in the meantime.
func (q *QbittHandler) retryLogin() {
	for {
		time.Sleep(qbittLoginRetryInterval)

		if err := loginWithTimeout(q.c, qbittLoginTimeout); err != nil {
			q.logger.Warn("qBittorrent login retry failed", "error", err)
			continue
		}

		q.logger.Info("Connected to QBittorrent successfully after retry")
		return
	}
}

func (q *QbittHandler) HandleDownload(ctx context.Context, req *tvdb.Media, done chan<- models.TorrentCompleteEvent) ([]dlstatus.EpisodeResult, error) {
	results := []dlstatus.EpisodeResult{}

	// Handle movies: check if movie already exists in Plex
	if req.Category == "movie" {
		exists, err := q.repo.MovieExistsByTvdbId(ctx, req.Id)
		if err != nil {
			q.logger.WarnContext(ctx, "Failed to check movie existence by TVDB ID",
				telemetry.WithTraceContext(ctx, "tvdb_id", req.Id, "movie", req.Name, "error", err.Error())...)
		}
		if exists {
			q.logger.InfoContext(ctx, "Movie already exists in Plex, skipping",
				telemetry.WithTraceContext(ctx, "movie", req.Name, "tvdb_id", req.Id)...)
			// Update notification to completed with "already exists" reason
			q.updateNotificationStatus(ctx, req.Id, "Already exists in Plex library", notifications.StatusCompleted)
			results = append(results, dlstatus.EpisodeResult{
				TvdbID:  req.Id,
				Outcome: dlstatus.OutcomeExists,
			})
			close(done)
			return results, nil
		}

		if q.inFlightTvdbIDs(ctx)[req.Id] {
			q.logger.InfoContext(ctx, "Movie already has a torrent in flight, skipping",
				telemetry.WithTraceContext(ctx, "movie", req.Name, "tvdb_id", req.Id)...)
			q.updateNotificationStatus(ctx, req.Id, "", notifications.StatusDownloading)
			results = append(results, dlstatus.EpisodeResult{
				TvdbID:  req.Id,
				Outcome: dlstatus.OutcomeDownloading,
			})
			close(done)
			return results, nil
		}
	}

	// Handle series: Filter out episodes that already exist in Plex
	if req.Category == "series" {
		episodesToDownload, existsResults, err := q.filterExistingEpisodes(ctx, req)
		if err != nil {
			return results, err
		}
		results = append(results, existsResults...)

		episodesToDownload, inFlightResults := q.filterInFlightEpisodes(ctx, req, episodesToDownload)
		results = append(results, inFlightResults...)

		if len(episodesToDownload) == 0 {
			q.logger.InfoContext(ctx, "Every episode is already in Plex or downloading, nothing to search",
				telemetry.WithTraceContext(ctx, "show", req.Name)...)
			close(done)
			return results, nil
		}

		q.logger.InfoContext(ctx, "Episodes to download after filtering",
			telemetry.WithTraceContext(ctx, "show", req.Name,
				"total", len(req.Metadata.Episodes), "to_download", len(episodesToDownload),
				"show_aliases", req.Aliases)...)

		// Build search strategies for each episode
		searchStrategies := q.buildSearchStrategies(ctx, req, episodesToDownload)

		// Process all episodes concurrently
		var wg sync.WaitGroup
		ctx, searchSpan := torrentTracer.Start(ctx, "searchForEpisodes")
		searchSpan.SetAttributes(
			attribute.String("show_name", req.Name),
			attribute.String("tvdb_id", req.Id),
			attribute.Int("episode_count", len(searchStrategies)),
			attribute.Bool("is_anime", req.Anime),
		)
		defer searchSpan.End()

		for episodeIdx, strategies := range searchStrategies {
			result := q.processEpisodeDownload(ctx, episodeIdx, strategies, req, done, &wg)
			results = append(results, result)
		}

		// Close the channel after all monitoring goroutines complete
		go func() {
			wg.Wait()
			close(done)
			q.logger.InfoContext(ctx, "All torrents processed, channel closed")
		}()

		return results, nil
	}

	// Handle movies: create movie search strategy and process
	if req.Category == "movie" {
		movieStrategy := q.createMovieSearchStrategy(req)

		var wg sync.WaitGroup
		ctx, searchSpan := torrentTracer.Start(ctx, "searchForMovie")
		searchSpan.SetAttributes(
			attribute.String("movie_name", req.Name),
			attribute.String("tvdb_id", req.Id),
			attribute.Bool("is_anime", req.Anime),
		)
		defer searchSpan.End()

		// Process movie download using existing episode download logic
		result := q.processEpisodeDownload(ctx, 0, movieStrategy, req, done, &wg)
		results = append(results, result)

		// Close the channel after monitoring goroutine completes
		go func() {
			wg.Wait()
			close(done)
			q.logger.InfoContext(ctx, "Movie torrent processed, channel closed")
		}()

		return results, nil
	}

	// Unknown category — nothing to process; close the channel so the
	// completion handler goroutine can exit.
	close(done)
	return results, nil
}

// processEpisodeDownload handles the complete download workflow for a single episode (or movie)
// and returns a per-episode EpisodeResult describing the outcome. It never returns an error: every
// failure mode is mapped to a structured result so the webserver's retry engine can act on it.
func (q *QbittHandler) processEpisodeDownload(
	ctx context.Context,
	episodeIdx int,
	strategies []*models.SearchStrategy,
	req *tvdb.Media,
	done chan<- models.TorrentCompleteEvent,
	wg *sync.WaitGroup,
) dlstatus.EpisodeResult {
	// Create episode-level parent span
	episodeCtx, episodeSpan := torrentTracer.Start(ctx, "searchEpisode")

	// Overall per-episode search deadline so a sick indexer can't pin the request.
	episodeCtx, cancel := context.WithTimeout(episodeCtx, episodeSearchDeadline)
	defer cancel()

	if len(strategies) > 0 {
		episodeSpan.SetAttributes(
			attribute.Int("episode_index", episodeIdx),
			attribute.Int("season", strategies[0].Season),
			attribute.Int("episode", strategies[0].Episode),
			attribute.Int("strategy_count", len(strategies)),
			attribute.String("media_name", strategies[0].MediaName),
		)
	}

	var excluded *excludedTorrents
	if len(strategies) > 0 {
		excluded = q.loadExcludedTorrents(episodeCtx, req.Id, strategies[0].Season, strategies[0].Episode)
	}
	outcome := q.searchForMatch(episodeCtx, strategies, req, excluded)

	// If every strategy actually attempted (across all tried tiers) errored against
	// Prowlarr, the indexers are unreachable.
	if outcome.attemptedAny && outcome.erroredAll {
		q.logger.WarnContext(episodeCtx, "All indexer searches failed", "show", req.Name)
		ss := strategies[0]
		q.recordDownloadFailure(episodeCtx, req, ss, "indexer unreachable")
		q.applyFailureNotification(episodeCtx, ss.EpisodeTvdbID, dlstatus.CodeIndexerUnreachable)
		episodeSpan.SetStatus(codes.Error, "all indexer searches failed")
		episodeSpan.End()
		return q.failedResult(ss, dlstatus.CodeIndexerUnreachable)
	}

	if outcome.match == nil {
		q.logger.WarnContext(episodeCtx, "No suitable torrent found", "show", req.Name)
		ss := strategies[0]
		q.recordDownloadFailure(episodeCtx, req, ss, "no suitable torrent found")
		// Transient: a torrent may appear later, so keep searching (retry owned by Phase 2).
		q.applyFailureNotification(episodeCtx, ss.EpisodeTvdbID, dlstatus.CodeNoTorrentFound)
		episodeSpan.SetStatus(codes.Ok, "no suitable torrent found")
		episodeSpan.End()
		return q.failedResult(ss, dlstatus.CodeNoTorrentFound)
	}

	// Download the torrent and start monitoring
	episodeSpan.End()
	return q.initiateDownloadAndMonitor(outcome.winningCtx, outcome.match, searchRequestFor(req, strategies[0]), done, wg)
}

// initiateDownloadAndMonitor starts the download and begins monitoring for completion
func (q *QbittHandler) initiateDownloadAndMonitor(
	ctx context.Context,
	match *models.TorrentMatch,
	request *models.SearchRequest,
	done chan<- models.TorrentCompleteEvent,
	wg *sync.WaitGroup,
) dlstatus.EpisodeResult {
	ss := match.Strategy
	infoHash, trackingUUID, err := q.downloadTorrent(ctx, match.Torrent)
	if err != nil {
		q.logger.ErrorContext(ctx, "Failed to download torrent", "error", err)
		// Transient torrent-client error: keep the notification searching for the retry engine.
		q.applyFailureNotification(ctx, ss.EpisodeTvdbID, dlstatus.CodeTorrentClientError)
		return q.failedResult(ss, dlstatus.CodeTorrentClientError)
	}

	// Update notification: download has started successfully
	q.updateNotificationStatus(ctx, ss.EpisodeTvdbID, "", notifications.StatusDownloading)

	// Record the torrent as in-flight before monitoring starts, so a restart in
	// the next instant can still pick it back up. A failure here is not fatal —
	// the in-memory monitor below still works for as long as this process lives.
	startedAt := time.Now()
	active := &models.ActiveTorrent{
		InfoHash:     infoHash,
		TrackingUUID: trackingUUID,
		TorrentTitle: match.Torrent.Title,
		Strategy:     ss,
		StartedAt:    startedAt,
		Request:      request,
	}
	if err := q.repo.InsertActiveTorrent(ctx, active); err != nil {
		q.logger.ErrorContext(ctx, "Failed to persist in-flight torrent, monitor will not survive a restart",
			telemetry.WithTraceContext(ctx,
				"error", err.Error(),
				"torrent_title", match.Torrent.Title,
				"torrent_hash", infoHash)...)
	}

	// Start monitoring goroutine
	wg.Add(1)
	go q.monitorTorrentCompletion(ctx, match, request, infoHash, trackingUUID, startedAt, done, wg)

	return dlstatus.EpisodeResult{
		TvdbID:  ss.EpisodeTvdbID,
		Season:  ss.Season,
		Episode: ss.Episode,
		Outcome: dlstatus.OutcomeDownloading,
	}
}
