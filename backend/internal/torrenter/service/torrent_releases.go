package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"golift.io/starr/prowlarr"

	"github.com/jbofill10/scout/backend/internal/torrenter/models"
	"github.com/jbofill10/scout/backend/pkg/controls"
	"github.com/jbofill10/scout/backend/pkg/dlstatus"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

// ErrUnknownItem is returned when no media is available for an item: none was
// posted and the torrenter has no in-flight torrent carrying its request.
var ErrUnknownItem = errors.New("no media known for item")

const (
	// releaseRelaxLimit caps how far queries are relaxed when listing releases
	// for a user: strict and the first relaxed tier. Broad queries mostly add
	// noise, and every query counts against the indexer's rate limit.
	releaseRelaxLimit = 1
	// maxReleaseCandidates bounds the list handed to the UI.
	maxReleaseCandidates = 60
)

// ResolveItemMedia returns the single-item media a release listing or a manual
// grab works from: the posted media when there is one, otherwise the request
// stored with the item's in-flight torrent.
func (q *QbittHandler) ResolveItemMedia(ctx context.Context, itemID string, media *tvdb.Media) (*tvdb.Media, error) {
	if media != nil {
		if media.Category == "series" && len(media.Metadata.Episodes) != 1 {
			return nil, fmt.Errorf("series media must carry exactly one episode, got %d", len(media.Metadata.Episodes))
		}
		return media, nil
	}
	active, err := q.repo.GetActiveTorrents(ctx)
	if err != nil {
		return nil, fmt.Errorf("list in-flight torrents: %w", err)
	}
	for _, at := range active {
		if at.Strategy == nil || at.Strategy.EpisodeTvdbID != itemID || at.Request == nil {
			continue
		}
		m := at.Request.Media
		if at.Request.Episode != nil {
			m.Metadata.Episodes = []tvdb.Episode{*at.Request.Episode}
		}
		return &m, nil
	}
	return nil, ErrUnknownItem
}

// itemStrategies builds the search strategies for a single-item media, the
// same way the automatic path and the dead-swarm replacement do.
func (q *QbittHandler) itemStrategies(media *tvdb.Media) []*models.SearchStrategy {
	if media.Category == "movie" {
		return q.createMovieSearchStrategy(media)
	}
	if len(media.Metadata.Episodes) == 0 {
		return nil
	}
	return q.createSearchStrategy(media, &media.Metadata.Episodes[0])
}

// ListReleases runs the item's searches against every indexer tier, without
// stopping at the first usable hit, and reports each distinct torrent seen with
// the verdict the automatic picker would reach on it.
func (q *QbittHandler) ListReleases(ctx context.Context, itemID string, media *tvdb.Media) (controls.ReleasesResponse, error) {
	resp := controls.ReleasesResponse{ItemID: itemID, MediaTitle: media.Name, Candidates: []controls.ReleaseCandidate{}}

	strategies := q.itemStrategies(media)
	if len(strategies) == 0 {
		return resp, errors.New("no search strategy for item")
	}
	first := strategies[0]
	if first.EpisodeTvdbID != itemID {
		return resp, fmt.Errorf("media does not match item %s (strategy is for %s)", itemID, first.EpisodeTvdbID)
	}

	ctx, span := torrentTracer.Start(ctx, "listReleases")
	defer span.End()
	span.SetAttributes(
		attribute.String("media_name", media.Name),
		attribute.String("tvdb_id", itemID),
		attribute.Bool("is_anime", media.Anime),
	)

	kept := make([]*models.SearchStrategy, 0, len(strategies))
	for _, ss := range strategies {
		if ss.RelaxLevel <= releaseRelaxLimit {
			kept = append(kept, ss)
		}
	}

	excluded := q.loadExcludedTorrents(ctx, media.Id, first.Season, first.Episode)
	preferred := q.preferredUploaders(ctx, media.Category, media.Anime)

	seen := map[string]bool{}
	for _, indexerIDs := range q.indexerTiers(media.Category, media.Anime) {
		for _, ss := range kept {
			if ctx.Err() != nil {
				span.SetStatus(codes.Error, "cancelled")
				return resp, ctx.Err()
			}
			results, err := q.searchProwlarr(ctx, ss.Query, media.Category, indexerIDs)
			resp.Queries++
			if err != nil {
				q.logger.WarnContext(ctx, "Release listing: Prowlarr search failed, continuing",
					telemetry.WithTraceContext(ctx, "query", ss.Query, "indexer_ids", indexerIDs, "error", err.Error())...)
				continue
			}
			for _, t := range results {
				key := candidateKey(t)
				if seen[key] {
					continue
				}
				seen[key] = true
				resp.Candidates = append(resp.Candidates, q.describeCandidate(ctx, t, ss, excluded, preferred))
			}
		}
	}

	sortCandidates(resp.Candidates)
	if len(resp.Candidates) > maxReleaseCandidates {
		resp.Candidates = resp.Candidates[:maxReleaseCandidates]
	}

	accepted := 0
	for _, c := range resp.Candidates {
		if c.Accepted {
			accepted++
		}
	}
	span.SetAttributes(attribute.Int("queries", resp.Queries),
		attribute.Int("candidates", len(resp.Candidates)), attribute.Int("accepted", accepted))
	q.logger.InfoContext(ctx, "Listed releases",
		telemetry.WithTraceContext(ctx, "media", media.Name, "tvdb_id", itemID,
			"queries", resp.Queries, "candidates", len(resp.Candidates), "accepted", accepted)...)
	return resp, nil
}

// candidateKey collapses the same release seen through several queries: by
// info hash when the indexer gives one, else by guid, else by title.
func candidateKey(t *prowlarr.Search) string {
	if t.InfoHash != "" {
		return "h:" + strings.ToLower(t.InfoHash)
	}
	if t.GUID != "" {
		return "g:" + t.GUID
	}
	return "t:" + normalizeTitle(t.Title)
}

// describeCandidate records what the picker thinks of one search hit.
func (q *QbittHandler) describeCandidate(
	ctx context.Context,
	t *prowlarr.Search,
	ss *models.SearchStrategy,
	excluded *excludedTorrents,
	preferred []string,
) controls.ReleaseCandidate {
	c := controls.ReleaseCandidate{
		Title:            t.Title,
		Indexer:          t.Indexer,
		IndexerID:        t.IndexerID,
		GUID:             t.GUID,
		DownloadURL:      t.DownloadURL,
		InfoHash:         t.InfoHash,
		Size:             t.Size,
		Seeders:          t.Seeders,
		Leechers:         t.Leechers,
		PublishDate:      t.PublishDate,
		Query:            ss.Query,
		PreviouslyFailed: excluded.contains(t),
	}

	vr := q.validateTorrent(t, ss)
	if vr.ParsedMovie != nil {
		c.Quality = vr.ParsedMovie.Quality
		c.Source = vr.ParsedMovie.Source
		c.Codec = vr.ParsedMovie.Codec
		c.ReleaseGroup = vr.ParsedMovie.ReleaseGroup
	} else if vr.Parsed != nil {
		// Show titles are parsed for quality and group only.
		c.Quality = vr.Parsed.Quality
		c.ReleaseGroup = vr.Parsed.ReleaseGroup
	}
	if !vr.IsValid {
		c.Reason = vr.Reason
		return c
	}

	verdict := q.scoreMatch(ctx, &models.TorrentMatch{Strategy: ss, Torrent: t}, vr, preferred)
	if verdict.rejection != "" {
		c.Reason = verdict.rejection
		return c
	}
	c.Accepted = true
	c.Score = verdict.score
	return c
}

// sortCandidates puts accepted releases first by score, then rejected ones by
// seeders, so the top of the list is what Scout itself would have taken.
func sortCandidates(cs []controls.ReleaseCandidate) {
	sort.SliceStable(cs, func(i, j int) bool {
		a, b := cs[i], cs[j]
		if a.Accepted != b.Accepted {
			return a.Accepted
		}
		if a.Accepted && a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.Seeders != b.Seeders {
			return a.Seeders > b.Seeders
		}
		return a.Title < b.Title
	})
}

// DownloadRelease grabs one chosen release for an item, replacing anything
// already in flight for it, and monitors it exactly like an automatic pick, so
// completion, Plex linking and subtitles behave the same. It owns closing done.
func (q *QbittHandler) DownloadRelease(
	ctx context.Context,
	itemID string,
	media *tvdb.Media,
	sel controls.ReleaseSelection,
	done chan<- models.TorrentCompleteEvent,
) (dlstatus.EpisodeResult, error) {
	strategies := q.itemStrategies(media)
	if len(strategies) == 0 {
		close(done)
		return dlstatus.EpisodeResult{}, errors.New("no search strategy for item")
	}
	ss := strategies[0]
	if ss.EpisodeTvdbID != itemID {
		close(done)
		return dlstatus.EpisodeResult{}, fmt.Errorf("media does not match item %s (strategy is for %s)", itemID, ss.EpisodeTvdbID)
	}

	// The user's choice supersedes whatever Scout was doing for the item.
	var superseded controls.CancelResponse
	q.cancelInflight(ctx, itemID, &superseded)

	grabCtx, span := torrentTracer.Start(ctx, "manualGrab")
	span.SetAttributes(
		attribute.String("media_name", media.Name),
		attribute.String("tvdb_id", itemID),
		attribute.String("torrent_title", sel.Title),
		attribute.String("indexer", sel.Indexer),
		attribute.Int("seeders", sel.Seeders),
		attribute.Bool("replaced_in_flight", superseded.SearchCancelled || superseded.TorrentsRemoved > 0),
	)
	q.logger.InfoContext(grabCtx, "Downloading release chosen by user",
		telemetry.WithTraceContext(grabCtx, "media", media.Name, "tvdb_id", itemID, "torrent_title", sel.Title,
			"indexer", sel.Indexer, "search_cancelled", superseded.SearchCancelled, "torrents_removed", superseded.TorrentsRemoved)...)

	match := &models.TorrentMatch{
		Strategy: ss,
		Torrent: &prowlarr.Search{
			GUID:        sel.GUID,
			DownloadURL: sel.DownloadURL,
			InfoHash:    sel.InfoHash,
			Title:       sel.Title,
			Indexer:     sel.Indexer,
			IndexerID:   sel.IndexerID,
			Size:        sel.Size,
			Seeders:     sel.Seeders,
			Leechers:    sel.Leechers,
		},
	}

	var wg sync.WaitGroup
	// On success the monitor ends the span once it takes over (it treats the
	// context's span as the finished search); on failure nothing else will.
	result := q.initiateDownloadAndMonitor(grabCtx, match, searchRequestFor(media, ss), done, &wg)
	if result.Outcome != dlstatus.OutcomeDownloading {
		span.SetStatus(codes.Error, "torrent add failed")
		span.End()
	}
	go func() {
		wg.Wait()
		close(done)
	}()
	return result, nil
}
