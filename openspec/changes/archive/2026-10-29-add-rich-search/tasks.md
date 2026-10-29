# Tasks

## 1. Query parsing

- [x] 1.1 `pkg/search`: `Parse` (bracketed/trailing year, `key:value` filters with quoted or run-on values, IMDb id/URL), `ProxyParams`/`FromProxyParams`, `Normalize`
- [x] 1.2 Unit tests for every syntax rule and the non-year numbers ("1917", "Blade Runner 2049", "Class of 1984")

## 2. tvdb-proxy

- [x] 2.1 `TVDBSearchItem` gains director, studios, network, country, language, tvdb_id; new people, remote-id and base-record types
- [x] 2.2 `search.go`: tiered `searchMedia` (title search with native filters, studio post-filter, verbatim retry, implicit and explicit people credits, IMDb lookup, episode fetch, ranking, caps), replacing `queryShow`
- [x] 2.3 `/series` reads the structured query; proper URL encoding of the TVDB request
- [x] 2.4 `searchItemToMedia` maps the new fields, genres, and falls back to the original name
- [x] 2.5 Tests against a fake TVDB for each mode, ranking, caching and failure handling

## 3. webserver

- [x] 3.1 `TVDBProxyClient.Search` takes a `search.Query`, encodes it, checks the status code
- [x] 3.2 `SearchInteractor` parses the text, skips the proxy when only unusable filters were typed, caches by raw text
- [x] 3.3 `/search/enriched` returns `{ results, query }`
- [x] 3.4 Client and interactor tests

## 4. UI

- [x] 4.1 `useEnrichedSearch` reads the new response shape and exposes `query`
- [x] 4.2 `utils/searchQuery.ts`: types, filter labels, `needsTitle`, `withoutFilter`, examples
- [x] 4.3 `SearchDropdown`: placeholder, syntax tooltip, example chips, interpretation chips with removal, tailored empty states
- [x] 4.4 `SearchResultsGrid`: year and provenance label per card
- [x] 4.5 `MediaRecord` type gains director, studios, network, matchedVia

## 5. Docs and verification

- [x] 5.1 CLAUDE.md search flow and proxy endpoint notes
- [x] 5.2 `go build`, `go vet`, `go test` for proxy, webserver and pkg; `npm run build` and `npm run lint`
- [x] 5.3 Run the new proxy locally against TVDB: "X (2022)", "Mia Goth", "actor:", "director:", "studio:A24", "network:HBO", IMDb id, "Class of 1984", "Bryan Cranston"
- [x] 5.4 Rebuild and deploy tvdb-proxy, webserver and ui; try the searches in the overlay (deployed 2026-10-29, overlay checked with Playwright)
