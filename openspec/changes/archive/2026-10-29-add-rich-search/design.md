# Design: Rich Search

## Where parsing lives

The parser is a shared package (`backend/pkg/search`) rather than living in
either service. The webserver owns the API contract with the UI and needs the
parsed query to echo it back; tvdb-proxy owns TVDB knowledge and needs the
same structure to decide which TVDB parameters to use. Sharing one parser
avoids two implementations drifting, and `ProxyParams`/`FromProxyParams` is
the single definition of the proxy's query string.

The UI never parses. It shows the backend's reading (`query` in the enriched
response) and, when a chip is removed, rebuilds the text from that reading.
One source of truth for what "X 2022 director:ti west" means.

## Filter value boundaries

Filter values run until the next filter key or the end of the text. People
type `director:ti west` far more often than `director:"ti west"`, and the
alternative (one word per value) would silently search for director "ti".
The cost is that words after an unquoted filter belong to it, so the title
goes first; quotes end a value explicitly when that is not wanted. Only a
fixed set of keys counts, and common words are excluded from the aliases, so
titles with colons are unaffected.

## Years

TVDB's index treats a year in the query as text (it matched wrestling pay-per-
views for "X 2022"), so years are extracted client-side. Bracketed years are
unambiguous. A bare trailing year is probably a year, but "Class of 1984" and
"Death Race 2000" end in one, so the proxy searches both the stripped title
with a year filter and the verbatim text, year-filtered hits first. Numbers
outside 1880..now+5 are never years ("Blade Runner 2049"), nor is a lone
number ("1917").

## Which TVDB mechanism per filter

| Filter | Movies | Series |
| --- | --- | --- |
| year, country, language | `/search` parameter | `/search` parameter |
| director (with a title) | `/search?director=` | people credits, Director role |
| director (alone) | people credits, Director role | same |
| actor / person | people credits | people credits |
| studio / network | local filter on `studios` (TVDB's `company` does not cover studios) | `/search?network=` |
| imdb | `/search/remoteid/{id}` | same |

## People credits

`/people/{id}/extended` lists every credit with the linked title's name,
poster and year, so a filmography needs no per-title lookups. Series credits
still get their episode list, like any series hit, because the enriched
status and the download flow depend on it.

Implicit expansion (a plain query that is also someone's name) only happens
on an exact, normalised name match, so "Pearl" does not pull in everyone
called Pearl; an explicit `actor:` takes TVDB's top hit. Acting and
directing credits are implied; writing and producing are not, unless
`person:` asks for every role. Credits are ranked by the person's dominant
line of work (count of acting vs directing credits), main-cast roles first
(TVDB's `isFeatured`, which it only sets on acting credits), then guest spots,
then the other line of work. A title the person both acted in and directed
shows the stronger role once.

Caps: 40 credits per person, 3 people per name, 60 results per search.
People searches and extended records are cached for an hour in the proxy;
the webserver's existing 5-minute cache covers the whole response.

## Ranking

Tiers, then TVDB score (series only; movie hits carry none), then year for
credits: exact title/alias matches → person credits (sub-tiered by role
rank) → remaining title hits → verbatim-title hits. Exact-match detection
compares normalised names and aliases, so "X" leads with every film called
"X" rather than "Julia X".

## Response shape

`/api/search/enriched` becomes `{ results, query }`. `/api/search` keeps
returning an array. tvdb-proxy's `/series` keeps returning an array;
provenance travels on each item as `matchedVia`.
