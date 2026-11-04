package ratings

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	tvdb "github.com/jbofill10/scout/backend/pkg/media"
	"github.com/jbofill10/scout/backend/pkg/telemetry"
)

const (
	// imdbIDTTL is how long a resolved TVDB id → IMDb id mapping is kept;
	// it never changes, so this only bounds memory.
	imdbIDTTL = 7 * 24 * time.Hour
	// imdbIDMissTTL is how long "TVDB lists no IMDb id" is remembered.
	imdbIDMissTTL      = 24 * time.Hour
	resolveConcurrency = 6
	resolveTimeout     = 20 * time.Second
)

// TVDBGetter performs an authenticated TVDB GET of a path relative to the TVDB host.
type TVDBGetter interface {
	Get(ctx context.Context, uri string) ([]byte, error)
}

// Resolver finds the IMDb id behind a TVDB id, for media that arrives
// without one: library items, which carry only a TVDB id, and search hits
// found through a person's credits. It reads TVDB's short extended record,
// which lists remote ids without the episode and cast lists.
type Resolver struct {
	client TVDBGetter
	cache  *asyncCache[string]
	logger *slog.Logger
}

// NewResolver returns a resolver reading TVDB through client.
func NewResolver(client TVDBGetter, logger *slog.Logger) *Resolver {
	return &Resolver{client: client, cache: newAsyncCache[string](resolveConcurrency, resolveTimeout), logger: logger}
}

// ImdbID returns the IMDb id of a TVDB series or movie ("series" or "movie",
// anything else counts as series), "" when TVDB lists none or the answer
// has not arrived by the time ctx ends.
func (r *Resolver) ImdbID(ctx context.Context, mediaType, id string) string {
	if id == "" {
		return ""
	}
	collection := "series"
	if mediaType == "movie" {
		collection = "movies"
	}
	key := collection + ":" + id
	imdbID, _ := r.cache.get(ctx, key, func(ctx context.Context) (string, time.Duration) {
		body, err := r.client.Get(ctx, fmt.Sprintf("/%s/%s/extended?short=true", collection, id))
		if err != nil {
			r.logger.WarnContext(ctx, "IMDb id lookup failed", telemetry.WithTraceContext(ctx,
				"media_type", mediaType, "id", id, "error", err)...)
			return "", 0
		}
		var response tvdb.TVDBSeriesExtendedResponse
		if err := json.Unmarshal(body, &response); err != nil {
			r.logger.WarnContext(ctx, "IMDb id lookup decode failed", telemetry.WithTraceContext(ctx,
				"media_type", mediaType, "id", id, "error", err)...)
			return "", 0
		}
		imdbID := response.Data.ImdbID()
		if imdbID == "" {
			return "", imdbIDMissTTL
		}
		return imdbID, imdbIDTTL
	})
	return imdbID
}

// SetStore keeps resolved ids across restarts.
func (r *Resolver) SetStore(store Store) {
	r.cache.persist = &persistence{store: store, prefix: "imdb-id:", logger: r.logger}
}
