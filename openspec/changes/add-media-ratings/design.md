# Design

## Where ratings come from

Four sources, all keyed by IMDb id, merged into one list (IMDb, Rotten Tomatoes, Metacritic):

- **IMDb's public dataset** (`title.ratings.tsv.gz`, refreshed daily, no key). Every rated title's average
  and vote count, about 1.5 million rows. Parsed into 12-byte rows (numeric id, rating in tenths, votes)
  sorted by id and searched by bisection: about 20 MB, lookups in microseconds. Loaded before the proxy
  serves (90 s cap, then retried in the background) so the webserver's first popular refresh already carries
  scores; replaced once a day, and a failed download keeps the old table. This is the baseline: every card
  can show a score with nothing configured.
- **Wikidata** (query service, no key, CC0). Film and series items carry "review score" (P444) statements
  qualified by "review score by" (P447): the Tomatometer as "93%" (Q105584, Rotten Tomatoes; its "8.7/10"
  average is skipped) and the Metascore as "81/100" (Q150248). One SPARQL query covers 50 IMDb ids (P345);
  the latest-dated score per source is kept. Bots keep these current for established titles (29 of 40 of the
  user's library movies had a Tomatometer) but they lag on brand-new releases (3 of 20 of the week's popular
  movies).
- **Wikipedia article prose** (MediaWiki API, no key). The same SPARQL query returns each item's English
  Wikipedia sitelink and whether it is a film. For films Wikidata has not scored, the article's wikitext is
  fetched (20 per request) and the Tomatometer read from the `{{Rotten Tomatoes prose|93|7.8|250}}` template
  or the first critics' percentage after a "Rotten Tomatoes" mention (sentences giving only the audience
  score are skipped); the Metascore likewise from `{{Metacritic film prose}}` or "N out of 100". Editors add
  these from release week on: all 17 of the week's uncovered popular movies had one. Series articles are not
  read, since their prose scores single seasons.
- **OMDb** (omdbapi.com, free key). Rotten Tomatoes has no public API; OMDb licenses a current Tomatometer
  and returns it next to IMDb's rating and the Metascore in one record. When a key is set its Tomatometer and
  Metascore win over Wikidata's; its IMDb figure is dropped in favour of the table's, which always carries
  votes.

`Service.LookupMany(ctx, ids, wait)` runs the sources in parallel per batch of ids and merges per id;
`Lookup` is the one-id case and the batch extended handler rates a whole batch in one call. Waits are
explicit: 4 s for a search a person is waiting on, 15 s for the batch endpoints the UI and webserver fetch
in the background, and 45 s when the popular package rates a row as it is built, so the webserver's refresh
(which caches rows for 6 hours) never picks up an unrated row.

Alternatives considered: TMDB (no Tomatometer), Trakt ratings (needs a client id the deployment lacks),
scraping rottentomatoes.com (undocumented endpoints, against its terms).

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

## Finding the IMDb id

TVDB's search index returns `remote_ids` on every hit, which was not decoded before; `TVDBSearchItem.ImdbID()`
picks the IMDb id out, so no extra TVDB call is needed for title searches. Two kinds of media arrive with
only a TVDB id: person-credit hits (actor:/director: searches come from `/people/{id}/extended`, whose
embedded records have no remote ids) and Plex library items. `Resolver` reads TVDB's short extended record
(`/{series|movies}/{id}/extended?short=true`: remote ids without the episode and cast lists, one call) and
caches the mapping for a week (it never changes) or a miss for a day. `Service.Attach` resolves such items
first and keeps the id on the item, so `POST /ratings/batch` can rate by TVDB id for the Library page, which
asks in chunks of 40 so the first posters get their score while the rest resolve.

Shared machinery: `asyncCache` memoises detached lookups by key (callers wait only as long as their context
allows, the lookup runs on under its own timeout and lands in the cache, concurrent callers share it). Both
the OMDb cache and the resolver cache are instances of it.

## Card treatment

A pill in the poster's top-left corner (the library badge keeps the top-right): a tomato (fresh, 60% and up) or
green splat (rotten) with the percentage for the Tomatometer, a yellow star with the one-decimal rating for
IMDb, a colour-banded square for the Metascore. Dark base like the library badge so it reads over any
artwork; smaller insets and type below the `sm` breakpoint so both pills fit a 130px phone card.
