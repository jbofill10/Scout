package notifications

import (
	"fmt"
	"strconv"
	"time"

	"github.com/jbofill10/scout/backend/pkg/media"
	"go.opentelemetry.io/otel/trace"
)

// NewFromSeries creates a notification for a TV series episode
// IMPORTANT: Uses episode.Id as TvdbID, NOT media.Id (show ID)
func NewFromSeries(m media.Media, episode media.Episode, traceID, spanID string) (*Notification, error) {
	if m.Category != "series" {
		return nil, fmt.Errorf("media category must be 'series', got '%s'", m.Category)
	}

	// CRITICAL: Use episode.Id as tvdb_id, not show's media.Id
	episodeTvdbID := strconv.Itoa(episode.Id)

	season := episode.SeasonNumber
	episodeNum := episode.Number

	notification := &Notification{
		TvdbID:     episodeTvdbID, // Episode ID from TVDB
		MediaTitle: m.Name,
		Category:   "series",
		Season:     &season,
		Episode:    &episodeNum,
		PosterURL:  m.ImageUrl,
		IsAnime:    m.Anime,
		Status:     StatusSearching,
		IsRead:     false,
		TraceID:    traceID,
		SpanID:     spanID,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	// Set absolute episode number for anime
	if m.Anime && episode.AbsoluteNumber > 0 {
		absNum := episode.AbsoluteNumber
		notification.AbsoluteEpisode = &absNum
	}

	return notification, nil
}

// NewFromMovie creates a notification for a movie
// Uses movie's media.Id as TvdbID
func NewFromMovie(m media.Media, traceID, spanID string) (*Notification, error) {
	if m.Category != "movie" {
		return nil, fmt.Errorf("media category must be 'movie', got '%s'", m.Category)
	}

	notification := &Notification{
		TvdbID:     m.Id, // Movie ID from TVDB
		MediaTitle: m.Name,
		Category:   "movie",
		PosterURL:  m.ImageUrl,
		IsAnime:    m.Anime,
		Status:     StatusSearching,
		IsRead:     false,
		TraceID:    traceID,
		SpanID:     spanID,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	return notification, nil
}

// NewFromSeriesWithContext creates a notification for a TV series episode with OpenTelemetry context
func NewFromSeriesWithContext(m media.Media, episode media.Episode, spanCtx trace.SpanContext) (*Notification, error) {
	traceID := spanCtx.TraceID().String()
	spanID := spanCtx.SpanID().String()
	return NewFromSeries(m, episode, traceID, spanID)
}

// NewFromMovieWithContext creates a notification for a movie with OpenTelemetry context
func NewFromMovieWithContext(m media.Media, spanCtx trace.SpanContext) (*Notification, error) {
	traceID := spanCtx.TraceID().String()
	spanID := spanCtx.SpanID().String()
	return NewFromMovie(m, traceID, spanID)
}
