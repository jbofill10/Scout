# Design

## Where ratings come from

Rotten Tomatoes has no public API; the Tomatometer is only available through OMDb (omdbapi.com), which
licenses it and returns it next to IMDb's rating and Metacritic's Metascore in one record keyed by IMDb id.
One source therefore covers both asks: Tomatometer for movies, IMDb rating for shows (OMDb rarely has a
Tomatometer for series). The backend carries every score it gets; the UI decides which one leads.

Alternatives considered: TMDB (no Tomatometer), Trakt ratings (needs a client id the deployment lacks),
IMDb's daily datasets (no Tomatometer, 100 MB in memory).

## Where the lookup lives

In tvdb-proxy, which mints every `Media` the UI sees (search hits, popular rows) and serves the extended
record the webserver converts for the detail view and the enriched rows. Attaching there means three hook
points (search handler, popular handler, extended handler) instead of the webserver's six merge points, and
the proxy already runs the other non-TVDB sources (Trakt, Wikipedia, AniList).

The popular handler attaches per request rather than at row build time: `Rank` returns a copy of the cached
row, the rating cache makes it a map lookup per item, and a slow OMDb does not pin a rating-less row for an
hour.

## Budget

A free OMDb key allows 1,000 lookups a day. The service caches hits for 7 days and misses for 24 hours,
shares one in-flight lookup between concurrent askers (the Home rows overlap the genre rows heavily), bounds
concurrency at 4, and on a refused key (allowance spent, or wrong key) pauses for an hour instead of
hammering. The cache is in memory: a restart re-fetches the warm rows (about 250 titles). Persisting it is a
follow-up if the budget turns out to bite.

## Latency

Ratings are decoration. A caller waits at most 4 seconds for an uncached answer; the lookup runs detached
(`context.WithoutCancel`) so the answer still lands in the cache for the next request. OMDb answers in well
under a second, so in practice the first search for a title pays it once.

## Search hits

TVDB's search index returns `remote_ids` on every hit, which was not decoded before; `TVDBSearchItem.ImdbID()`
picks the IMDb id out, so no extra TVDB call is needed for search results. Person-credit hits (actor:/
director: searches) come from `/people/{id}/extended`, whose embedded records have no remote ids, so those
cards carry no rating.

## Card treatment

A pill in the poster's top-left corner (the library badge keeps the top-right): a tomato (fresh, 60% and up) or
green splat (rotten) with the percentage for the Tomatometer, a yellow star with the one-decimal rating for
IMDb, a colour-banded square for the Metascore. Dark base like the library badge so it reads over any
artwork; smaller insets and type below the `sm` breakpoint so both pills fit a 130px phone card.
