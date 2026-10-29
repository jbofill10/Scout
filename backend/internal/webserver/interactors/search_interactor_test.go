package interactors

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/search"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockSearchClient stands in for the tvdb-proxy client's Search.
type MockSearchClient struct {
	mock.Mock
}

func (m *MockSearchClient) Search(ctx context.Context, mediaType string, query search.Query) ([]tvdb.Media, error) {
	args := m.Called(ctx, mediaType, query)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]tvdb.Media), args.Error(1)
}

func setupSearchInteractor() (*SearchInteractor, *MockSearchClient) {
	client := new(MockSearchClient)
	interactor := newSearchInteractor(client, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	interactor.now = func() time.Time { return time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC) }
	return interactor, client
}

func TestSearchInteractor_ParsesQueryBeforeCallingProxy(t *testing.T) {
	interactor, client := setupSearchInteractor()
	want := []tvdb.Media{movie("154561")}
	client.On("Search", mock.Anything, "movie", mock.MatchedBy(func(q search.Query) bool {
		return q.Title == "X" && q.Year == "2022" && q.Director == "Ti West" && q.AltTitle == ""
	})).Return(want, nil)

	results, err := interactor.Search(context.Background(), "movie", "X (2022) director:Ti West")

	require.NoError(t, err)
	assert.Equal(t, want, results)
	client.AssertExpectations(t)
}

func TestSearchInteractor_FiltersAloneSkipTheProxy(t *testing.T) {
	interactor, client := setupSearchInteractor()

	results, err := interactor.Search(context.Background(), "movie", "year:2022")

	require.NoError(t, err)
	assert.Empty(t, results)
	assert.NotNil(t, results)
	client.AssertNotCalled(t, "Search", mock.Anything, mock.Anything, mock.Anything)
}

func TestSearchInteractor_CachesByRawQuery(t *testing.T) {
	interactor, client := setupSearchInteractor()
	client.On("Search", mock.Anything, "series", mock.Anything).Return([]tvdb.Media{series("1")}, nil).Once()

	first, err := interactor.Search(context.Background(), "series", "actor:Bryan Cranston")
	require.NoError(t, err)
	second, err := interactor.Search(context.Background(), "series", "Actor:Bryan Cranston ")
	require.NoError(t, err)

	assert.Equal(t, first, second)
	client.AssertNumberOfCalls(t, "Search", 1)
}

func TestSearchInteractor_ProxyErrorIsReturned(t *testing.T) {
	interactor, client := setupSearchInteractor()
	client.On("Search", mock.Anything, "series", mock.Anything).Return(nil, errors.New("proxy down"))

	_, err := interactor.Search(context.Background(), "series", "x")

	assert.Error(t, err)
}
