package handlers

import (
	"context"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"
)

// Rater attaches critic and audience scores to media; ratings.Service
// implements it. A nil Rater means no rating source is configured and
// responses go out without scores.
type Rater interface {
	// Attach fills Ratings on every item that has an IMDb id, resolving one
	// from the TVDB id where it is missing.
	Attach(ctx context.Context, media []tvdb.Media)
	// Lookup returns the ratings for one IMDb id, nil when none are known.
	Lookup(ctx context.Context, imdbID string) []tvdb.Rating
	// LookupMany returns the ratings for each IMDb id; ids with none are absent.
	LookupMany(ctx context.Context, imdbIDs []string) map[string][]tvdb.Rating
}

// rateExtended fills the ratings on an extended record from its IMDb id.
func rateExtended(ctx context.Context, rater Rater, info *tvdb.TVDBSeriesExtendedResponse) {
	if rater == nil || info == nil {
		return
	}
	if imdbID := info.Data.ImdbID(); imdbID != "" {
		info.Data.Ratings = rater.Lookup(ctx, imdbID)
	}
}
