package interactors

import (
	"context"
	"log/slog"
	"time"

	"github.com/jbofill10/scout/backend/internal/webserver/cache"
	"github.com/jbofill10/scout/backend/internal/webserver/clients"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

type SearchInteractor struct {
	tvdbClient  *clients.TVDBProxyClient
	searchCache *cache.SearchCache
	logger      *slog.Logger
}

func NewSearchInteractor(tvdbClient *clients.TVDBProxyClient, logger *slog.Logger) *SearchInteractor {
	// Initialize cache with 5-minute TTL as specified in requirements
	cacheTTL := 5 * time.Minute

	return &SearchInteractor{
		tvdbClient:  tvdbClient,
		searchCache: cache.NewSearchCache(cacheTTL),
		logger:      logger,
	}
}

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

	// Cache miss - log and fetch from tvdb-proxy
	i.logger.InfoContext(
		ctx,
		"Search cache miss - fetching from tvdb-proxy",
		telemetry.WithTraceContext(ctx, "query", query, "media_type", mediaType)...,
	)

	results, err := i.tvdbClient.Search(ctx, mediaType, query)
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
