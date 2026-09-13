package interactors

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/jbofill10/scout/backend/internal/webserver/clients"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

// Searcher returns base search results. In production this is the
// SearchInteractor, which caches tvdb-proxy responses per query.
type Searcher interface {
	Search(ctx context.Context, mediaType, query string) ([]tvdb.Media, error)
}

// ExtendedInfoClient fetches TVDB extended records. Enriched search uses it
// only to learn whether a show already in the library is anime.
type ExtendedInfoClient interface {
	GetExtendedBatch(ctx context.Context, requests []clients.ExtendedRequest) ([]tvdb.Media, error)
}

// animeFlagTTL bounds how long a show's anime classification is remembered.
// TVDB genres change rarely, so a day is plenty.
const animeFlagTTL = 24 * time.Hour

type animeFlag struct {
	anime   bool
	fetched time.Time
}

type EnrichedSearchInteractor struct {
	searcher        Searcher
	tvdbClient      ExtendedInfoClient
	torrenterClient TorrenterClient
	logger          *slog.Logger

	// Anime classification per series id, so a library show costs one extended
	// lookup rather than one per search. Only library shows are ever looked up,
	// which keeps this to the size of the library.
	animeMu    sync.Mutex
	animeFlags map[string]animeFlag
	now        func() time.Time
}

func NewEnrichedSearchInteractor(
	searcher Searcher,
	tvdbClient ExtendedInfoClient,
	torrenterClient TorrenterClient,
	logger *slog.Logger,
) *EnrichedSearchInteractor {
	return &EnrichedSearchInteractor{
		searcher:        searcher,
		tvdbClient:      tvdbClient,
		torrenterClient: torrenterClient,
		logger:          logger,
		animeFlags:      make(map[string]animeFlag),
		now:             time.Now,
	}
}

// EnrichedSearch returns search results with library status merged in.
//
// tvdb-proxy already returns every series with its episode list, plus aliases
// and (for movies) genres, so the only per-search work beyond the search itself
// is one status lookup against the torrenter. The TVDB extended record is
// fetched solely for series the library has episodes of: their anime flag
// decides whether Plex's absolute numbering is matched against TVDB's
// per-season numbering when counting what is downloaded. Everything else the
// extended record carries is either already in the search hit or fetched by
// the UI on demand when a card is opened.
func (i *EnrichedSearchInteractor) EnrichedSearch(
	ctx context.Context,
	mediaType, query string,
) ([]tvdb.EnrichedMedia, error) {
	i.logger.InfoContext(
		ctx,
		"Starting enriched search",
		telemetry.WithTraceContext(ctx, "query", query, "media_type", mediaType)...,
	)

	searchResults, err := i.searcher.Search(ctx, mediaType, query)
	if err != nil {
		i.logger.ErrorContext(
			ctx,
			"Search failed in enriched search",
			telemetry.WithTraceContext(ctx, "error", err)...,
		)
		return nil, err
	}

	if len(searchResults) == 0 {
		return []tvdb.EnrichedMedia{}, nil
	}

	// The searcher may hand back a cached slice shared with other requests;
	// work on a copy so the anime flag below never races another search.
	results := slices.Clone(searchResults)

	statusRequests := make([]tvdb.StatusRequest, len(results))
	for idx, media := range results {
		statusRequests[idx] = tvdb.StatusRequest{
			TvdbId:    media.Id,
			MediaType: statusMediaType(media.Category),
		}
	}

	statusResponse, err := i.torrenterClient.GetStatusBatch(ctx, statusRequests)
	if err != nil {
		// Graceful degradation: the user still gets search results, just
		// without library status.
		i.logger.WarnContext(
			ctx,
			"Status batch failed - continuing without status information",
			telemetry.WithTraceContext(ctx, "error", err)...,
		)
		return withoutStatus(results), nil
	}

	i.applyAnimeFlags(ctx, results, statusResponse)

	enrichedResults := i.mergeStatusWithMedia(results, statusResponse)

	i.logger.InfoContext(
		ctx,
		"Enriched search completed",
		telemetry.WithTraceContext(
			ctx,
			"result_count", len(enrichedResults),
			"show_status_count", len(statusResponse.Shows),
			"movie_status_count", len(statusResponse.Movies),
		)...,
	)

	return enrichedResults, nil
}

// applyAnimeFlags marks the series in results that TVDB classes as anime, but
// only for shows the library already holds episodes of. That is the one place
// the flag changes the response: anime shows in Plex use absolute numbering,
// which the episode count has to bridge. Search hits for series carry no
// genres, so this needs the extended record; keeping it to library shows and
// remembering the answer turns a per-result fan-out into a handful of lookups
// the first time a show is seen and none after that.
func (i *EnrichedSearchInteractor) applyAnimeFlags(
	ctx context.Context,
	results []tvdb.Media,
	status tvdb.StatusBatchResponse,
) {
	inLibrary := make(map[string]bool, len(status.Shows))
	for _, show := range status.Shows {
		if len(show.Seasons) > 0 {
			inLibrary[show.TvdbId] = true
		}
	}

	requests := make([]clients.ExtendedRequest, 0)
	for idx := range results {
		media := &results[idx]
		if media.Category != "series" || !inLibrary[media.Id] || media.Anime {
			continue
		}
		if anime, known := i.cachedAnimeFlag(media.Id); known {
			media.Anime = anime
			continue
		}
		requests = append(requests, clients.ExtendedRequest{Id: media.Id, MediaType: media.Category})
	}
	if len(requests) == 0 {
		return
	}

	i.logger.InfoContext(
		ctx,
		"Fetching extended info for library shows",
		telemetry.WithTraceContext(ctx, "request_count", len(requests))...,
	)

	extended, err := i.tvdbClient.GetExtendedBatch(ctx, requests)
	if err != nil {
		// Counts for anime shows may be off until the next search; the
		// results themselves are unaffected.
		i.logger.WarnContext(
			ctx,
			"Extended info batch failed - continuing without anime detection",
			telemetry.WithTraceContext(ctx, "error", err)...,
		)
		return
	}

	// Only ids that came back are remembered; a show the batch skipped is
	// retried on the next search.
	flags := make(map[string]bool, len(extended))
	for _, media := range extended {
		flags[media.Id] = media.Anime
	}
	i.rememberAnimeFlags(flags)

	for idx := range results {
		if flags[results[idx].Id] {
			results[idx].Anime = true
		}
	}
}

// cachedAnimeFlag reports a remembered classification and whether one exists.
func (i *EnrichedSearchInteractor) cachedAnimeFlag(id string) (anime bool, known bool) {
	i.animeMu.Lock()
	defer i.animeMu.Unlock()

	entry, ok := i.animeFlags[id]
	if !ok {
		return false, false
	}
	if i.now().Sub(entry.fetched) > animeFlagTTL {
		delete(i.animeFlags, id)
		return false, false
	}
	return entry.anime, true
}

func (i *EnrichedSearchInteractor) rememberAnimeFlags(flags map[string]bool) {
	i.animeMu.Lock()
	defer i.animeMu.Unlock()

	fetched := i.now()
	for id, anime := range flags {
		i.animeFlags[id] = animeFlag{anime: anime, fetched: fetched}
	}
}

// statusMediaType maps TVDB's category onto the torrenter's vocabulary, which
// says "show" where TVDB says "series".
func statusMediaType(category string) string {
	if category == "series" {
		return "show"
	}
	return category
}

// withoutStatus wraps results as EnrichedMedia carrying only the media type.
func withoutStatus(mediaList []tvdb.Media) []tvdb.EnrichedMedia {
	enrichedResults := make([]tvdb.EnrichedMedia, len(mediaList))
	for idx, media := range mediaList {
		enrichedResults[idx] = tvdb.EnrichedMedia{
			Media:  media,
			Status: tvdb.MediaStatusInfo{Type: media.Category},
		}
	}
	return enrichedResults
}

// mergeStatusWithMedia merges status information from StatusBatchResponse into
// Media objects, preserving the search order.
func (i *EnrichedSearchInteractor) mergeStatusWithMedia(
	mediaList []tvdb.Media,
	statusResponse tvdb.StatusBatchResponse,
) []tvdb.EnrichedMedia {
	// Build lookup maps for efficient merging
	showStatusMap := make(map[string]tvdb.ShowStatus)
	for _, showStatus := range statusResponse.Shows {
		showStatusMap[showStatus.TvdbId] = showStatus
	}

	movieStatusMap := make(map[string]tvdb.MovieStatus)
	for _, movieStatus := range statusResponse.Movies {
		movieStatusMap[movieStatus.TvdbId] = movieStatus
	}

	// Merge status into each media item
	enrichedResults := make([]tvdb.EnrichedMedia, len(mediaList))
	for idx, media := range mediaList {
		statusInfo := tvdb.MediaStatusInfo{
			Type: media.Category,
		}

		switch media.Category {
		case "series":
			// Merge show status
			if showStatus, found := showStatusMap[media.Id]; found {
				statusInfo.Seasons = showStatus.Seasons
				// Calculate downloaded/total using TVDB metadata for accurate counts
				downloaded, total := calculateEpisodeCountsFromMetadata(media, showStatus)
				statusInfo.Downloaded = downloaded
				statusInfo.Total = total
			}
		case "movie":
			// Merge movie status
			if movieStatus, found := movieStatusMap[media.Id]; found {
				statusInfo.InLibrary = movieStatus.InLibrary
			}
		}

		enrichedResults[idx] = tvdb.EnrichedMedia{
			Media:  media,
			Status: statusInfo,
		}
	}

	return enrichedResults
}
