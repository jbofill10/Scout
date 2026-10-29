package interactors

import (
	"context"
	"log/slog"
	"time"

	"github.com/jbofill10/scout/backend/internal/webserver/cache"
	"github.com/jbofill10/scout/backend/internal/webserver/clients"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/search"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

// SearchClient is the part of the tvdb-proxy client search needs.
type SearchClient interface {
	Search(ctx context.Context, mediaType string, query search.Query) ([]tvdb.Media, error)
}

type SearchInteractor struct {
	tvdbClient  SearchClient
	searchCache *cache.SearchCache
	logger      *slog.Logger
	now         func() time.Time
}

func NewSearchInteractor(tvdbClient *clients.TVDBProxyClient, logger *slog.Logger) *SearchInteractor {
	return newSearchInteractor(tvdbClient, logger)
}

func newSearchInteractor(tvdbClient SearchClient, logger *slog.Logger) *SearchInteractor {
	// Initialize cache with 5-minute TTL as specified in requirements
	cacheTTL := 5 * time.Minute

	return &SearchInteractor{
		tvdbClient:  tvdbClient,
		searchCache: cache.NewSearchCache(cacheTTL),
		logger:      logger,
		now:         time.Now,
	}
}

// Search parses the text typed into the search box (see pkg/search for the
// syntax) and asks tvdb-proxy for matches. Results are cached per raw query
// and media type, so retyping a term is free.
func (i *SearchInteractor) Search(ctx context.Context, mediaType, query string) ([]tvdb.Media, error) {
	// Check cache first
	if cachedResults, found := i.searchCache.Get(query, mediaType); found {
		i.logger.InfoContext(
			ctx,
			"Search cache hit",
			telemetry.WithTraceContext(ctx, "query", query, "media_type", mediaType, "result_count", len(cachedResults))...,
		)
		return cachedResults, nil
	}

	parsed := search.Parse(query, i.now())
	if parsed.IsEmpty() {
		// Filters with nothing to search for (a bare year, say): the UI
		// explains what to add, there is nothing to ask TVDB.
		return []tvdb.Media{}, nil
	}

	// Cache miss - log and fetch from tvdb-proxy
	i.logger.InfoContext(
		ctx,
		"Search cache miss - fetching from tvdb-proxy",
		telemetry.WithTraceContext(ctx, "query", query, "media_type", mediaType,
			"title", parsed.Title, "filters", len(parsed.Filters))...,
	)

	results, err := i.tvdbClient.Search(ctx, mediaType, parsed)
	if err != nil {
		return nil, err
	}

	// Cache the results before returning
	i.searchCache.Set(query, mediaType, results)

	i.logger.InfoContext(
		ctx,
		"Search results cached",
		telemetry.WithTraceContext(ctx, "query", query, "media_type", mediaType, "result_count", len(results))...,
	)

	return results, nil
}
