## ADDED Requirements

### Requirement: Media Ratings Enrichment

The tvdb-proxy SHALL attach critic and audience scores to the media it returns, looked up by IMDb id: a
`ratings` list of `{source, value, votes}` where `source` is `imdb` (value 0-10, with `votes`),
`rotten_tomatoes` (the Tomatometer, 0-100) or `metacritic` (the Metascore, 0-100), in that order. The IMDb
rating SHALL come from IMDb's public daily ratings table held in memory (`RATINGS_IMDB`, on by default); the
Tomatometer and Metascore SHALL come from OMDb when an API key is configured (`OMDB_API_KEY`). Media whose
IMDb id TVDB does not list, or that no source knows, SHALL carry no `ratings`.

#### Scenario: IMDb table loaded at startup

- **WHEN** the proxy starts with `RATINGS_IMDB` on
- **THEN** it SHALL download and parse IMDb's ratings table before serving requests, giving up after 90
  seconds and retrying in the background every 10 minutes until it has one
- **AND** it SHALL download the table again once a day, keeping the old table when a download fails

#### Scenario: Media with only a TVDB id

- **WHEN** media arrives without an IMDb id (a person-credit search hit, a ratings batch entry)
- **THEN** the proxy SHALL resolve the IMDb id through TVDB's short extended record
  (`/{series|movies}/{id}/extended?short=true`) once per title, caching it for 7 days (a miss for 24 hours)
- **AND** the resolved id SHALL be returned as `imdbId` next to the `ratings`

#### Scenario: Search hits carry ratings

- **WHEN** a client calls `GET /series` and a hit's TVDB `remote_ids` include an IMDb title id
- **THEN** the hit SHALL carry `imdbId`
- **AND** the hit SHALL carry `ratings` with every score the rating source holds for that id

#### Scenario: Popular rows carry ratings

- **WHEN** a client calls `GET /series/popular` or `GET /movies/popular`
- **THEN** every item with an IMDb id SHALL carry `ratings` with the scores the rating source holds for it

#### Scenario: Extended records carry ratings

- **WHEN** a client calls `GET /series/:id/extended` or `POST /series/batch/extended` for a record whose
  remote ids include an IMDb title id
- **THEN** the record's `data` SHALL carry `ratings` with the scores the rating source holds for it

#### Scenario: Answers are cached

- **WHEN** a title's ratings have been looked up within the last 7 days, or the source reported no record for
  it within the last 24 hours
- **THEN** the proxy SHALL answer from its cache without calling the rating source
- **AND** concurrent requests for one title SHALL share a single source lookup

#### Scenario: Refused key pauses lookups

- **WHEN** OMDb refuses the key (daily allowance spent, or the key is wrong)
- **THEN** the proxy SHALL stop calling OMDb for one hour
- **AND** media SHALL be returned with the IMDb rating alone meanwhile

### Requirement: Ratings Batch Endpoint

The tvdb-proxy SHALL provide `POST /ratings/batch` taking an array of `{id, mediaType}` (TVDB id; `series`
or `movie`) and returning an array of `{id, mediaType, imdbId, ratings}` in the same order.

#### Scenario: Rate library titles

- **WHEN** a client posts up to 200 titles
- **THEN** every title TVDB lists an IMDb id for SHALL come back with `imdbId` and the `ratings` the sources
  hold for it
- **AND** a title without an IMDb id or without scores SHALL come back with neither field

#### Scenario: Reject bad batches

- **WHEN** the body is not an array of `{id, mediaType}`, is empty, or holds more than 200 titles
- **THEN** the proxy SHALL answer 400

#### Scenario: Slow lookups do not hold the response

- **WHEN** a title's ratings are not cached and the source has not answered within 4 seconds
- **THEN** the response SHALL go out without that title's `ratings`
- **AND** the lookup SHALL finish in the background so the next request finds it cached

## MODIFIED Requirements

### Requirement: Search Hit Mapping

Search hits SHALL carry the extra fields TVDB's index provides.

#### Scenario: Movie hit fields

- **WHEN** a movie hit is mapped to Media
- **THEN** `director`, `studios`, `genres`, `originalCountry` and `originalLanguage` SHALL be populated from the hit when present

#### Scenario: Series hit fields

- **WHEN** a series hit is mapped to Media
- **THEN** `network`, `aliases`, `originalCountry` and `originalLanguage` SHALL be populated from the hit when present

#### Scenario: IMDb id from remote ids

- **WHEN** a hit's `remote_ids` include an entry of type 2 / source "IMDB" whose id is `tt` followed by digits
- **THEN** `imdbId` SHALL be populated from it, without fetching the extended record
- **AND** a hit without such an entry SHALL carry no `imdbId`

#### Scenario: Missing English translation

- **WHEN** a hit has no English translated name
- **THEN** `mediaName` SHALL fall back to the hit's original name
