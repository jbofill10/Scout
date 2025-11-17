package notifications

import (
	"testing"

	"github.com/jbofill10/scout/backend/pkg/media"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
)

func TestNewFromSeries_Success(t *testing.T) {
	m := media.Media{
		Id:       "12345", // Show ID
		Name:     "Test Show",
		Category: "series",
		ImageUrl: "https://example.com/poster.jpg",
		Anime:    false,
	}

	episode := media.Episode{
		Id:             98765, // Episode ID - THIS is what should be used as tvdb_id
		SeasonNumber:   2,
		Number:         5,
		AbsoluteNumber: 0,
	}

	traceID := "trace123"
	spanID := "span456"

	notification, err := NewFromSeries(m, episode, traceID, spanID)

	require.NoError(t, err)
	assert.NotNil(t, notification)

	// CRITICAL: tvdb_id should be episode ID, NOT show ID
	assert.Equal(t, "98765", notification.TvdbID, "tvdb_id must be episode ID")
	assert.Equal(t, "Test Show", notification.MediaTitle)
	assert.Equal(t, "series", notification.Category)
	assert.Equal(t, 2, *notification.Season)
	assert.Equal(t, 5, *notification.Episode)
	assert.Nil(t, notification.AbsoluteEpisode) // Not anime, so should be nil
	assert.Equal(t, "https://example.com/poster.jpg", notification.PosterURL)
	assert.False(t, notification.IsAnime)
	assert.Equal(t, StatusSearching, notification.Status)
	assert.Equal(t, traceID, notification.TraceID)
	assert.Equal(t, spanID, notification.SpanID)
}

func TestNewFromSeries_AnimeWithAbsoluteNumber(t *testing.T) {
	m := media.Media{
		Id:       "anime-show-123",
		Name:     "One Piece",
		Category: "series",
		ImageUrl: "https://example.com/anime.jpg",
		Anime:    true,
	}

	episode := media.Episode{
		Id:             555555,
		SeasonNumber:   1,
		Number:         42,
		AbsoluteNumber: 42, // Anime uses absolute numbering
	}

	notification, err := NewFromSeries(m, episode, "trace", "span")

	require.NoError(t, err)
	assert.True(t, notification.IsAnime)
	assert.NotNil(t, notification.AbsoluteEpisode)
	assert.Equal(t, 42, *notification.AbsoluteEpisode)
	assert.Equal(t, "555555", notification.TvdbID)
}

func TestNewFromSeries_AnimeWithoutAbsoluteNumber(t *testing.T) {
	m := media.Media{
		Id:       "anime-show-456",
		Name:     "Attack on Titan",
		Category: "series",
		Anime:    true,
	}

	episode := media.Episode{
		Id:             666666,
		SeasonNumber:   4,
		Number:         10,
		AbsoluteNumber: 0, // No absolute number
	}

	notification, err := NewFromSeries(m, episode, "trace", "span")

	require.NoError(t, err)
	assert.True(t, notification.IsAnime)
	assert.Nil(t, notification.AbsoluteEpisode) // Should be nil when zero
}

func TestNewFromSeries_InvalidCategory(t *testing.T) {
	m := media.Media{
		Id:       "123",
		Name:     "Wrong Category",
		Category: "movie", // Should be "series"
	}

	episode := media.Episode{
		Id:           12345,
		SeasonNumber: 1,
		Number:       1,
	}

	notification, err := NewFromSeries(m, episode, "trace", "span")

	assert.Error(t, err)
	assert.Nil(t, notification)
	assert.Contains(t, err.Error(), "media category must be 'series'")
}

func TestNewFromMovie_Success(t *testing.T) {
	m := media.Media{
		Id:       "movie-789", // Movie ID - used as tvdb_id
		Name:     "Test Movie",
		Category: "movie",
		ImageUrl: "https://example.com/movie-poster.jpg",
		Anime:    false,
	}

	traceID := "movie-trace"
	spanID := "movie-span"

	notification, err := NewFromMovie(m, traceID, spanID)

	require.NoError(t, err)
	assert.NotNil(t, notification)

	// For movies, tvdb_id is the movie ID
	assert.Equal(t, "movie-789", notification.TvdbID)
	assert.Equal(t, "Test Movie", notification.MediaTitle)
	assert.Equal(t, "movie", notification.Category)
	assert.Nil(t, notification.Season)
	assert.Nil(t, notification.Episode)
	assert.Nil(t, notification.AbsoluteEpisode)
	assert.Equal(t, "https://example.com/movie-poster.jpg", notification.PosterURL)
	assert.False(t, notification.IsAnime)
	assert.Equal(t, StatusSearching, notification.Status)
	assert.Equal(t, traceID, notification.TraceID)
	assert.Equal(t, spanID, notification.SpanID)
}

func TestNewFromMovie_AnimeMovie(t *testing.T) {
	m := media.Media{
		Id:       "anime-movie-999",
		Name:     "Spirited Away",
		Category: "movie",
		ImageUrl: "https://example.com/ghibli.jpg",
		Anime:    true,
	}

	notification, err := NewFromMovie(m, "trace", "span")

	require.NoError(t, err)
	assert.True(t, notification.IsAnime)
	assert.Equal(t, "anime-movie-999", notification.TvdbID)
}

func TestNewFromMovie_InvalidCategory(t *testing.T) {
	m := media.Media{
		Id:       "456",
		Name:     "Wrong Category",
		Category: "series", // Should be "movie"
	}

	notification, err := NewFromMovie(m, "trace", "span")

	assert.Error(t, err)
	assert.Nil(t, notification)
	assert.Contains(t, err.Error(), "media category must be 'movie'")
}

func TestNewFromSeriesWithContext_Success(t *testing.T) {
	m := media.Media{
		Id:       "show-111",
		Name:     "Context Test Show",
		Category: "series",
	}

	episode := media.Episode{
		Id:           777777,
		SeasonNumber: 3,
		Number:       7,
	}

	// Create a fake span context with trace and span IDs
	traceID := trace.TraceID{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}
	spanID := trace.SpanID{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18}

	spanCtx := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID,
		SpanID:  spanID,
	})

	notification, err := NewFromSeriesWithContext(m, episode, spanCtx)

	require.NoError(t, err)
	assert.NotNil(t, notification)
	assert.Equal(t, "777777", notification.TvdbID)
	assert.Equal(t, traceID.String(), notification.TraceID)
	assert.Equal(t, spanID.String(), notification.SpanID)
}

func TestNewFromMovieWithContext_Success(t *testing.T) {
	m := media.Media{
		Id:       "movie-222",
		Name:     "Context Test Movie",
		Category: "movie",
	}

	// Create a fake span context with trace and span IDs
	traceID := trace.TraceID{0xa1, 0xa2, 0xa3, 0xa4, 0xa5, 0xa6, 0xa7, 0xa8,
		0xa9, 0xaa, 0xab, 0xac, 0xad, 0xae, 0xaf, 0xa0}
	spanID := trace.SpanID{0xb1, 0xb2, 0xb3, 0xb4, 0xb5, 0xb6, 0xb7, 0xb8}

	spanCtx := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID,
		SpanID:  spanID,
	})

	notification, err := NewFromMovieWithContext(m, spanCtx)

	require.NoError(t, err)
	assert.NotNil(t, notification)
	assert.Equal(t, "movie-222", notification.TvdbID)
	assert.Equal(t, traceID.String(), notification.TraceID)
	assert.Equal(t, spanID.String(), notification.SpanID)
}
