package interactors

import (
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

func (i *SearchInteractor) Search(mediaType, query string) ([]tvdb.Media, error) {
	return i.tvdbClient.Search(mediaType, query)
}
