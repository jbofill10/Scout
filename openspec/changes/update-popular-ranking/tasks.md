## 1. tvdb-proxy
- [x] 1.1 `popular` package: `Source`, cached `Service` with primary/fallback, TVDB record resolver with bounded concurrency and miss caching
- [x] 1.2 Trakt source: watched/trending per genre, header auth, TVDB resolution (series by id, movies by IMDb), Trakt title as name, genre slug mapping, option normalisation
- [x] 1.3 TVDB source: year/continuing candidates per region, Japan-first regions for Anime, recency weighting, unaired/unreleased dropped, English names for non-English titles
- [x] 1.4 Wikipedia attention: article resolution (guesses, then serial search), 7-day pageviews, most-read harvest with TVDB search/extended resolution; pool re-ordered, harvested titles added
- [x] 1.5 Service single flight and startup warm-up of the UI's rows; webserver popular refresher's first refresh runs in the background
- [x] 1.6 AniList trending leads the Anime rows (English then romaji title lookup on TVDB, released only, half the row); `Media.Popularity` says what ranked each item
- [x] 1.8 Handler ranks through the service; config reads `TRAKT_*`, `POPULAR_WIKIPEDIA`, `POPULAR_ANILIST`; main wires Trakt, Wikipedia and AniList
- [x] 1.7 Tests: service caching/fallback/single flight/warm-up, Trakt request/mapping/skips/errors, TVDB queries/ranking/anime/partial failures, Wikipedia resolution/views/trending, attention ordering and harvest, AniList parsing and lead, popularity fields, handler, config

## 2. Configuration and docs
- [x] 2.1 `.env.template` and `docker-compose.yml.template` carry `TRAKT_CLIENT_ID`, `TRAKT_LIST`, `TRAKT_PERIOD`, `POPULAR_WIKIPEDIA`, `POPULAR_ANILIST`
- [x] 2.2 README, DEPLOYMENT and CLAUDE.md describe the ranking and the optional Trakt key

## 3. Verify
- [x] 3.1 `go vet ./... && go test ./...` (backend); UI untouched
- [x] 3.2 Deployed tvdb-proxy and webserver on 2026-11-02 (four rounds: TVDB ranking, attention layer, detached builds, AniList lead + popularity field). Live home movie row: Clayface, Wildwood, Wicker, Obsession, Verity, Street Fighter; shows: Crystal Lake, The Diplomat, Nocturne, Carrie, MobLand; anime shows led by AniList trending (So What's Wrong with Getting Reborn as a Goblin?, Overgeared, One Piece). Cold proxy warm-up 201s; webserver served at once. Trakt not yet configured (no client id in the local .env) Live rows through nginx: movies Clayface, Wildwood, Wicker, Obsession, Verity, Street Fighter (this week's most-read films, 733k to 258k views); shows Crystal Lake, The Diplomat, Nocturne, Carrie, MobLand; anime Cyberpunk: Edgerunners, Bleach, Solo Leveling. Cold proxy warm-up of all 14 rows took 216s; the webserver served from the warm proxy at once. Trakt not yet configured (no client id in the local .env)
- [x] 3.3 Independent check against JustWatch's US popular top 40 on 2026-11-02: home movie row 11/20 (old TVDB-score row 0/20), home shows row 12/20 (old 1/20)
