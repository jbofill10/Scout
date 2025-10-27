package interactors

import (
	"context"
	tvdb "shared/media"
	"webserver/internal/clients"
)

type SearchInteractor struct {
	tvdbClient *clients.TVDBProxyClient
}

func NewSearchInteractor(tvdbClient *clients.TVDBProxyClient) *SearchInteractor {
	return &SearchInteractor{
		tvdbClient: tvdbClient,
	}
}

func (i *SearchInteractor) Search(ctx context.Context, mediaType, query string) ([]tvdb.Media, error) {
	return i.tvdbClient.Search(ctx, mediaType, query)
}
