## MODIFIED Requirements

### Requirement: Universal Search Endpoint

The tvdb-proxy service SHALL answer `GET /series` with ranked media for a structured query: a title (`mediaName`) and/or filters (`year`, `director`, `person`, `personRole`, `company`, `country`, `language`, `imdb`, `altName`) as defined by `pkg/search`, for `mediaType` `movie` or `series`.

#### Scenario: Search for movies

- **WHEN** a request is made to `/series?mediaName={query}&mediaType=movie`
- **THEN** the service SHALL call TVDB API `/search?query={query}&type=movie` with the query URL-encoded
- **AND** return movie search results with Category="movie"

#### Scenario: Search for series

- **WHEN** a request is made to `/series?mediaName={query}&mediaType=series`
- **THEN** the service SHALL call TVDB API `/search?query={query}&type=series` with the query URL-encoded
- **AND** return series search results with Category="series"

#### Scenario: Native TVDB filters are passed through

- **WHEN** the request carries `year`, `country` or `language`
- **THEN** the service SHALL pass each as the same-named TVDB `/search` parameter
- **AND** `director` SHALL be passed to TVDB for movies only
- **AND** `company` SHALL be passed as TVDB's `network` parameter for series only

#### Scenario: Studio filter is applied locally

- **WHEN** the request is for movies and carries `company`
- **THEN** the service SHALL keep only hits whose `studios` include a name containing the value, compared case-insensitively
- **AND** SHALL NOT send TVDB's `company` parameter

#### Scenario: Exact title matches lead

- **WHEN** a hit's name, original name or an alias equals the title after normalisation (case, punctuation and spacing ignored)
- **THEN** it SHALL be ranked before hits that merely contain the title

#### Scenario: Inferred year keeps a verbatim search

- **WHEN** the request carries `altName`
- **THEN** the service SHALL additionally search TVDB for `altName` without the year filter
- **AND** append those hits after the year-filtered hits, without duplicates

#### Scenario: Series hits include episodes

- **WHEN** any result, from any source, is a series
- **THEN** the service SHALL fetch its episode list and score as before
- **AND** a failed episode fetch SHALL leave that series in the results with an empty episode list

#### Scenario: Filters without anything to search

- **WHEN** the request has no `mediaName`, `person`, `director` or `imdb`
- **THEN** the service SHALL answer 400

#### Scenario: Result caps

- **WHEN** the merged results exceed 60
- **THEN** only the first 60 SHALL be returned

## ADDED Requirements

### Requirement: Search Hit Mapping

Search hits SHALL carry the extra fields TVDB's index provides.

#### Scenario: Movie hit fields

- **WHEN** a movie hit is mapped to Media
- **THEN** `director`, `studios`, `genres`, `originalCountry` and `originalLanguage` SHALL be populated from the hit when present

#### Scenario: Series hit fields

- **WHEN** a series hit is mapped to Media
- **THEN** `network`, `aliases`, `originalCountry` and `originalLanguage` SHALL be populated from the hit when present

#### Scenario: Missing English translation

- **WHEN** a hit has no English translated name
- **THEN** `mediaName` SHALL fall back to the hit's original name

### Requirement: People Credits Search

The tvdb-proxy service SHALL answer person queries from TVDB's people index and the person's credits.

#### Scenario: Explicit actor filter

- **WHEN** the request carries `person` with `personRole=actor`
- **THEN** the service SHALL search TVDB `/search?query={person}&type=person`, take the top hit (and further hits with the same normalised name, up to 3 people), fetch `/people/{id}/extended` for each
- **AND** return only their credits of the requested media type with peopleType "Actor" or "Guest Star", with no title search
- **AND** each result SHALL carry `matchedVia` as "{person name} · {peopleType}"

#### Scenario: Explicit person filter

- **WHEN** the request carries `person` without `personRole`
- **THEN** credits of every role SHALL be returned

#### Scenario: Director filter where TVDB cannot filter

- **WHEN** the request carries `director` and the media type is series, or there is no `mediaName`
- **THEN** the service SHALL answer from the person's credits with peopleType "Director"

#### Scenario: Title and year narrow credits

- **WHEN** a person request also carries `mediaName` or `year`
- **THEN** only credits whose title contains the name (normalised) and whose year equals the year SHALL be kept

#### Scenario: Implicit person match

- **WHEN** a title search is made and a TVDB person's name equals the title after normalisation
- **THEN** that person's "Actor" and "Director" credits of the media type SHALL be appended after exact title matches and before the remaining title hits
- **AND** people whose name merely contains the title SHALL NOT be expanded

#### Scenario: Credit ranking

- **WHEN** a person's credits are returned
- **THEN** they SHALL be ordered by the person's dominant line of work (acting or directing, by credit count) first, main-cast acting credits before supporting ones and guest spots, then the other line of work, then every other role; ties by TVDB score then year descending
- **AND** a title with several roles for the person SHALL appear once, showing the strongest role
- **AND** at most 40 credits per person SHALL be returned

#### Scenario: People lookups are cached and non-fatal

- **WHEN** a people search or extended record was fetched within the last hour
- **THEN** it SHALL be served from memory
- **AND** a failed people lookup during a title search SHALL leave the title results intact

### Requirement: IMDb Id Lookup

The tvdb-proxy service SHALL resolve IMDb title ids.

#### Scenario: IMDb id of the requested type

- **WHEN** the request carries `imdb={ttid}`
- **THEN** the service SHALL call TVDB `/search/remoteid/{ttid}` and return the matched record of the requested media type, with `imdbId` set, and its episodes if a series

#### Scenario: IMDb id of the other type

- **WHEN** the remote-id match is a series but `mediaType=movie`, or the reverse
- **THEN** the service SHALL return an empty list
