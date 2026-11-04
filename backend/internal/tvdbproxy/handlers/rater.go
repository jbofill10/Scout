package handlers

import (
	"context"
	"time"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"
)

const (
	// interactiveWait is how long a response a person is waiting on (a
	// search) holds for scores not cached yet.
	interactiveWait = 4 * time.Second
	// backgroundWait is how long a batch the UI or the webserver fetches
	// behind the scenes (library ratings, row enrichment) holds for them.
	backgroundWait = 15 * time.Second
)

// Rater attaches critic and audience scores to media; ratings.Service
// implements it. A nil Rater means no rating source is configured and
// responses go out without scores. wait bounds how long a call holds for
// scores not cached yet; lookups finish regardless and land in the cache.
type Rater interface {
	// Attach fills Ratings on every item that has an IMDb id, resolving one
	// from the TVDB id where it is missing.
	Attach(ctx context.Context, media []tvdb.Media, wait time.Duration)
	// Lookup returns the ratings for one IMDb id, nil when none are known.
	Lookup(ctx context.Context, imdbID string, wait time.Duration) []tvdb.Rating
	// LookupMany returns the ratings for each IMDb id; ids with none are absent.
	LookupMany(ctx context.Context, imdbIDs []string, wait time.Duration) map[string][]tvdb.Rating
}

// rateExtended fills the ratings on an extended record from its IMDb id.
func rateExtended(ctx context.Context, rater Rater, info *tvdb.TVDBSeriesExtendedResponse) {
	if rater == nil || info == nil {
		return
	}
	if imdbID := info.Data.ImdbID(); imdbID != "" {
		info.Data.Ratings = rater.Lookup(ctx, imdbID, interactiveWait)
	}
}
