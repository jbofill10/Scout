# Design: Movie Downloading Support

## Context

Scout's architecture was initially designed for TV show downloads but included partial movie support. The core infrastructure (Plex integration, database tables, TVDB search) works for movies, but the download orchestration flow is incomplete. This design extends the existing layered architecture (handlers → interactors → services → repositories) to support movie downloads without breaking show functionality.

**Constraints:**
- Must reuse existing ScheduledDownloads table for future movie releases
- Must maintain backwards compatibility with show downloads
- Must follow existing service communication patterns (HTTP APIs between microservices)
- TVDB API v4 uses different endpoint prefixes for movies vs series

**Stakeholders:**
- Users wanting to download movies via Scout
- Maintainers of tvdb-proxy, webserver, and torrenter services

## Goals / Non-Goals

**Goals:**
- Enable end-to-end movie download flow (search → schedule → download → Plex)
- Support scheduling movies with future release dates
- Implement anime-aware indexer selection for movies
- Check Plex library before downloading to avoid duplicates
- Reuse existing infrastructure wherever possible

**Non-Goals:**
- UI changes (backend API only)
- Download history tracking (table exists but tracking not implemented)
- Modifying shared data structures (current structs work as-is)
- Quality preference customization for movies (use same logic as shows)
- Multi-format movie downloads (single best torrent only)

## Decisions

### Decision 1: TVDB Endpoint Handling

**Choice:** Make endpoint paths conditional based on media type query parameter

**Rationale:**
- TVDB API v4 uses `/movies/{id}/extended` for movies, `/series/{id}/extended` for series
- Same pattern applies to translations: `/movies/{id}/translations/` vs `/series/{id}/translations/`
- Search endpoint `/search?type=movie` is universal and already works

**Implementation:**
```go
// In getExtendedInformation()
mediaType := c.Query("mediaType")
if mediaType == "movie" {
    url = fmt.Sprintf("/movies/%s/extended", mediaId)
} else {
    url = fmt.Sprintf("/series/%s/extended", mediaId)
}
```

**Alternatives considered:**
- Create separate `/movies/{id}/extended` endpoint in tvdb-proxy → Rejected: Adds duplicate code
- Use TVDB API autodiscovery → Rejected: Requires extra API calls

### Decision 2: Movie Metadata Structure

**Choice:** Reuse `TVDBSeriesMetadata` struct with empty `Episodes` array for movies

**Rationale:**
- Avoids breaking changes to shared module
- Empty array is valid Go pattern
- `FirstAired` field can represent movie release date
- Reduces implementation scope

**Implementation:**
```go
// In queryShow() for movies, skip querySeriesMetadata()
if mediaData.Category == "movie" {
    // Set empty metadata
    mediaData.Metadata = tvdb.TVDBSeriesMetadata{Episodes: []tvdb.Episode{}}
    results = append(results, mediaData)
    return
}
```

**Alternatives considered:**
- Create separate `MovieMetadata` struct → Rejected: Cosmetic change, not worth refactoring
- Add discriminated union with interface → Rejected: Over-engineering

### Decision 3: Indexer Selection for Movies

**Choice:** Anime movies use NYAA+1337x, regular movies use 1337x only

**Rationale:**
- NYAA is anime-specific indexer, not useful for Hollywood movies
- Reduces unnecessary API calls to Prowlarr
- Matches user's requested behavior
- Anime detection already works via TVDB genre tags

**Implementation:**
```go
// In createMovieSearchStrategy()
indexers := []int{1337} // 1337x
if req.Anime {
    indexers = []int{NYAA, 1337} // NYAA + 1337x for anime
}
```

**Alternatives considered:**
- Use same indexers for all movies → Rejected: NYAA doesn't have regular movies
- Make indexers configurable per media type → Rejected: Over-engineering for MVP

### Decision 4: Movie Anime Detection

**Choice:** Check for "Anime" OR "Animation" genre tags for movies

**Rationale:**
- TVDB may tag anime movies as "Animation" without "Anime"
- Studio Ghibli, Makoto Shinkai films need proper classification
- False positives (Disney, Pixar) acceptable - 1337x still searched as fallback

**Implementation:**
```go
// In getExtendedInformation() for movies
isAnime := false
for _, genre := range info.Data.Genres {
    if genre.Name == "Anime" || genre.Name == "Animation" {
        isAnime = true
        break
    }
}
```

**Alternatives considered:**
- Only check "Anime" genre → Rejected: Misses legitimately tagged anime movies
- Check origin country → Rejected: Not available in TVDB response

### Decision 5: Future Release Scheduling

**Choice:** Use existing ScheduledDownloads table with release date checking

**Rationale:**
- Table already stores `tvdb.Media` as JSONB (works for both shows and movies)
- Scheduler already queries by date and sends to torrenter
- `FirstAired` field in metadata represents movie release date
- No schema changes needed

**Implementation:**
```go
// In DownloadMovie() interactor
releaseDate, _ := time.Parse("2006-01-02", media.Metadata.FirstAired)
if releaseDate.After(time.Now()) {
    // Schedule for future
    repo.Schedule(ctx, media, releaseDate)
} else {
    // Download immediately
    torrenterClient.Download(ctx, media)
}
```

**Alternatives considered:**
- Create separate MovieScheduledDownloads table → Rejected: Unnecessary duplication
- Don't support scheduling for movies → Rejected: User explicitly wants this feature

### Decision 6: Duplicate Detection

**Choice:** Add `MovieExistsByTvdbId()` repository method to check Plex library

**Rationale:**
- Plex movie sync already populates Movies table with tvdb_id
- Prevents re-downloading existing movies
- Matches pattern used for shows (`EpisodeExistsByTvdbId()`)

**Implementation:**
```go
// In torrenter repository
func (r *Repo) MovieExistsByTvdbId(ctx context.Context, tvdbId string) (bool, error) {
    query := `SELECT EXISTS(SELECT 1 FROM Movies WHERE tvdb_id = $1)`
    var exists bool
    err := r.db.QueryRowContext(ctx, query, tvdbId).Scan(&exists)
    return exists, err
}
```

**Alternatives considered:**
- Check by title + year → Rejected: Unreliable due to title variations
- No duplicate checking → Rejected: Wastes bandwidth and storage

## Risks / Trade-offs

**Risk:** TVDB genre tagging for anime movies may be inconsistent
- **Mitigation:** Allow both "Anime" and "Animation" tags, prefer to over-classify

**Risk:** Empty Episodes array in metadata is semantically confusing
- **Mitigation:** Document this pattern, can refactor in future if needed
- **Trade-off:** Avoids breaking changes to shared module

**Risk:** Movie search strategy may need different quality preferences
- **Mitigation:** Use same quality sorting as shows for MVP
- **Trade-off:** Can customize later based on user feedback

**Trade-off:** Not tracking download history for movies
- **Benefit:** Reduces scope, simplifies implementation
- **Cost:** No audit trail for troubleshooting failed downloads
- **Decision:** Acceptable for MVP, can add later if needed

## Migration Plan

**No database migrations required** - All necessary tables already exist:
- ScheduledDownloads - generic JSONB storage works for movies
- Movies table - already populated by Plex sync
- MovieDownloadHistory - exists but unused (out of scope)

**Deployment steps:**
1. Deploy tvdb-proxy with conditional endpoint logic
2. Deploy webserver with movie download handler
3. Deploy torrenter with movie search support
4. Test end-to-end with released and unreleased movies

**Rollback:** Each service change is backwards compatible. Rollback individual services if needed.

**Monitoring:** Use existing OpenTelemetry tracing to track movie downloads through the system.

## Open Questions

1. **Should animated movies from Disney/Pixar trigger NYAA indexer?**
   - Current design: Yes (based on "Animation" genre)
   - Impact: Extra Prowlarr API call that will return no results
   - Decision: Acceptable overhead for MVP, can refine later

2. **How to handle movies without release dates in TVDB?**
   - Current design: Empty FirstAired will fail time.Parse()
   - Proposed: Treat parse failure as "released" and download immediately
   - Needs validation during implementation

3. **Should movie search include year in query string?**
   - Current design: Yes, include year for better matching
   - Format: "Movie Title (Year)" or "Movie Title Year"
   - Needs testing with Prowlarr to determine best format
