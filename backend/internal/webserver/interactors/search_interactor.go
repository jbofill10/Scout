package interactors

import (
	"context"
	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/internal/webserver/clients"
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
