## ADDED Requirements

### Requirement: Media Ratings Enrichment

When an OMDb API key is configured (`OMDB_API_KEY`), the tvdb-proxy SHALL attach critic and audience scores to
the media it returns, looked up by IMDb id: a `ratings` list of `{source, value, votes}` where `source` is
`imdb` (value 0-10, with `votes`), `rotten_tomatoes` (the Tomatometer, 0-100) or `metacritic` (the Metascore,
0-100), in that order. Without a key, or for a title the source does not know, `ratings` SHALL be absent.

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

- **WHEN** the rating source refuses the key (daily allowance spent, or the key is wrong)
- **THEN** the proxy SHALL stop calling the source for one hour
- **AND** media SHALL be returned without `ratings` meanwhile

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
