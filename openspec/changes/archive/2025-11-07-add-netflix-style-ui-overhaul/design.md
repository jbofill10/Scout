# Design: Netflix-Style UI Overhaul

## Context

Scout's current UI uses React 19, Material-UI v7, and TypeScript with Vite as the build tool. The existing architecture has a flat component structure with minimal styling consistency and placeholder pages. This redesign aims to create a modern streaming platform interface while leveraging existing infrastructure and maintaining Scout's microservices architecture.

**Constraints:**
- Must maintain compatibility with existing API contracts for downloads
- Must work with current webserver, torrenter, tvdb-proxy services
- Should use Material-UI v7 (already in use)
- Must support OpenTelemetry instrumentation
- Desktop-focused design (1920x1080 typical resolution, no mobile/tablet optimization)

**Stakeholders:**
- End users seeking a polished, intuitive media browsing experience
- Developers maintaining the UI codebase
- API consumers (UI → webserver → tvdb-proxy)

## Goals / Non-Goals

### Goals
- Create a Netflix-inspired UI with horizontal carousels and sleek design
- Implement popular content discovery via TVDB API integration
- Consolidate schedule functionality into Home page
- Establish a proper component architecture (pages/, components/ui/)
- Implement a centralized theme system with design tokens
- Maintain existing download functionality with improved UX

### Non-Goals
- Real-time streaming playback (Scout is for downloading, not streaming)
- User authentication or multi-user support
- Content recommendations based on watch history
- Social features (sharing, ratings, reviews)
- Backend architecture changes (scheduler, download logic)
- Mobile/tablet optimization (desktop-focused only)

## Decisions

### 1. Carousel Implementation

**Decision:** Use CSS scroll-snap with custom controls

**Why:**
- Avoids heavy library dependency (react-slick is 60KB+)
- Provides smooth native scrolling performance
- Easier to customize for Scout's specific needs
- Better accessibility with keyboard navigation
- Simpler implementation for horizontal scrolling

**Alternatives Considered:**
- `react-slick`: Popular but adds significant bundle size, requires react-slick + slick-carousel CSS
- `swiper`: Feature-rich but overkill for simple horizontal scrolling
- Custom JS scroll: More complex, reinvents native browser capabilities

**Implementation:**
```typescript
// HorizontalCarousel.tsx
- Container with overflow-x: auto, scroll-snap-type: x mandatory
- Items with scroll-snap-align: start
- Left/right buttons using scrollBy()
- Hide scrollbar with CSS, show navigation buttons on hover
```

### 2. State Management for Server Data

**Decision:** Add TanStack Query (React Query) for server state

**Why:**
- Handles caching, background refetching, and stale data automatically
- Reduces boilerplate for API calls
- Built-in loading and error states
- Prevents redundant API requests
- Industry standard for React server state (800K+ weekly downloads)

**Alternatives Considered:**
- Plain fetch with useState: Current approach, lots of boilerplate, no caching
- SWR: Similar to React Query but less feature-rich
- Redux + RTK Query: Overkill for Scout's simple state needs

**Impact:**
- Add `@tanstack/react-query` dependency (~45KB)
- Create `src/lib/queryClient.ts` for configuration
- Wrap App in QueryClientProvider

### 3. Theme System

**Decision:** Use Material-UI's `createTheme()` with custom warm dark palette

**Why:**
- Leverages existing Material-UI v7 integration
- Provides centralized design tokens (colors, spacing, typography)
- Automatic dark mode support
- Type-safe with TypeScript
- Component-level overrides for consistent styling
- Unique aesthetic distinct from Netflix (warm indigo/purple tones)

**Color Palette:**
```typescript
{
  primary: {
    main: '#4F46E5',      // Deep indigo - main accent
    dark: '#4338CA',      // Darker indigo
    light: '#6366F1'      // Lighter indigo
  },
  secondary: {
    main: '#8B5CF6',      // Warm purple - secondary actions
    dark: '#7C3AED',
    light: '#A78BFA'
  },
  background: {
    default: '#0F172A',   // Very dark blue - main background
    paper: '#1E293B'      // Dark indigo - cards/elevated surfaces
  },
  text: {
    primary: '#F1F5F9',   // Off-white - primary text
    secondary: '#94A3B8'  // Light gray-blue - secondary text
  }
}
```

**Typography:**
- Font family: 'Inter', system-ui, sans-serif (clean, modern)
- Scale: 12px (caption) → 14px (body) → 16px (h6) → 24px (h4)
- Weight: 400 (regular), 500 (medium), 700 (bold)

### 4. TVDB Popular Content Strategy

**Decision:** Use `/series/filter` and `/movies/filter` with score sorting

**Why:**
- TVDB v4 doesn't have dedicated trending endpoints
- Score field provides relative popularity ranking
- Filter endpoints support genre filtering for Shows/Movies pages
- Simple single-request implementation
- Returns high-quality, popular content

**API Pattern:**
```
TVDB: /series/filter?country=usa&lang=eng&sort=score&sortType=desc&genre={id}
tvdb-proxy: GET /series/popular?genre={id}&limit=20
webserver: GET /api/popular/shows?genre={id}
```

**Alternatives Considered:**
- `/updates` endpoint: Shows recent additions but not popularity-based
- Local tracking: Requires database changes, needs user base to be meaningful
- Hardcoded lists: Not dynamic, requires manual curation

**Limitations:**
- Not "trending" in real-time sense (no recent popularity spikes)
- Score calculation is opaque (TVDB proprietary)
- Mitigation: Label as "Popular" not "Trending"

### 5. Genre Organization

**Decision:** Fetch all genres on app mount, display exactly 6 curated genres on both Shows and Movies pages

**Why:**
- Curated genre list provides focused, quality content discovery
- Same genres for Shows and Movies creates consistent UX
- Reduces cognitive load for users
- Genre list from TVDB is relatively static (~30-40 total genres)
- Context provides global access without prop drilling

**Implementation:**
```typescript
// GenreContext.tsx
- Fetch /api/genres on mount from tvdb-proxy
- Store in context: { id: number, name: string }[]
- Filter to exactly 6 genres: Action, Comedy, Drama, Sci-Fi, Anime, Documentary
- Match by name (case-insensitive) with slug fallback
- Handle "Sci-Fi" → "Science Fiction" alias
```

**Genre Selection for Rows:**
- Shows: Display 6 genre rows (Action, Comedy, Drama, Sci-Fi, Anime, Documentary)
- Movies: Display 6 genre rows (Action, Comedy, Drama, Sci-Fi, Anime, Documentary)
- Home: Mix of top 20 shows + top 20 movies (no genre filter)

**Genre Caching Strategy (tvdb-proxy):**
Per tvdb-proxy-maintainer recommendations:
- Cache genres in memory on tvdb-proxy startup (sync.RWMutex)
- Refresh weekly (TVDB genres change infrequently)
- Genre name → ID mapping with aliases for variations
- Single genre per API call (TVDB `/series/filter` and `/movies/filter` limitation)
- No episode enrichment for popular content (performance optimization)

### 6. Schedule Widget Design

**Decision:** Horizontal timeline widget above popular content on Home page

**Why:**
- Prominent placement for upcoming downloads
- Sleek, minimalistic design fits Netflix aesthetic
- Horizontal layout matches carousel pattern
- Easy to scan this week's schedule at a glance

**Data Source:**
```
GET /api/schedule/weekly → Returns scheduled downloads for next 7 days
{
  "schedule": [
    {
      "id": "abc123",
      "title": "Breaking Bad",
      "season": 1,
      "episode": 3,
      "releaseTime": "2025-11-08T20:00:00Z",
      "posterUrl": "https://...",
      "isAnime": false
    }
  ]
}
```

**UI Design:**
- Compact cards showing poster thumbnail, title, S01E03 format, date
- Scroll horizontally if more than ~6 items
- Empty state: "No scheduled downloads this week"
- Click to view details (reuse SearchResultDialog)

### 7. Search Overlay UX

**Decision:** Dropdown overlay with semi-transparent backdrop

**Why:**
- Keeps user in context of current page
- Familiar pattern (YouTube, Netflix search)
- Smooth transition with Material-UI's Drawer or custom dropdown
- Infinite scroll works well in vertical dropdown

**Implementation:**
```typescript
// SearchDropdown.tsx
- Material-UI Drawer with anchor="top" or custom positioned div
- Backdrop with opacity: 0.85
- Search input at top, results below with infinite scroll
- Click outside or ESC to close
- Clicking a result opens SearchResultDialog
```

**Behavioral Note:**
- Search icon in navbar toggles dropdown visibility
- Results appear as user types (debounced 300ms)
- Maintains existing infinite scroll pagination
- Can still navigate to `/search` for dedicated search page (bookmarkable)

### 8. Project Structure

**Decision:** Reorganize into pages/ and components/ui/ directories

**Why:**
- Clear separation of concerns (pages = routes, components = reusable)
- Industry standard pattern (Next.js, Remix, etc.)
- Easier to locate and maintain code
- Scales better as UI grows

**New Structure:**
```
ui/src/
├── pages/
│   ├── Home.tsx
│   ├── Shows.tsx
│   ├── Movies.tsx
│   └── Search.tsx (converted to overlay)
├── components/
│   ├── ui/
│   │   ├── Navbar.tsx
│   │   ├── HorizontalCarousel.tsx
│   │   ├── MediaCard.tsx
│   │   ├── GenreRow.tsx
│   │   ├── ScheduleWidget.tsx
│   │   ├── SearchDropdown.tsx
│   │   └── ... (shared UI components)
│   ├── SearchResultsList.tsx (existing)
│   └── SearchResultDialog.tsx (existing)
├── contexts/
│   └── GenreContext.tsx
├── theme/
│   └── theme.ts
├── lib/
│   └── queryClient.ts
└── App.tsx
```

### 9. Media Card Design

**Decision:** Poster-only cards with hover overlay showing title

**Why:**
- Maximizes visual impact of poster art
- Clean, minimalistic design (Netflix pattern)
- Hover reveals title + quick actions (download icon)
- Focus on imagery, not text clutter

**Implementation:**
```typescript
// MediaCard.tsx
- Aspect ratio: 2:3 (standard poster dimensions)
- Image with object-fit: cover
- Hover state: Dark overlay with title + download icon
- Click: Open SearchResultDialog for details
- Lazy loading for images (loading="lazy")
```

### 10. Desktop-Focused Design Strategy

**Decision:** Desktop-only design optimized for 1920x1080 resolution

**Why:**
- Primary use case is desktop browsing (media libraries are desktop applications)
- Horizontal carousels are designed for wide screens
- Simplifies implementation (no breakpoint logic needed)
- Allows for richer interactions and larger content display
- Users typically interact with download managers on their main computer

**Desktop Optimizations:**
- Fixed layouts optimized for 1920x1080 (typical desktop resolution)
- Carousel displays 6-8 items per row for optimal browsing
- Navbar and search overlay designed for mouse/keyboard interaction
- Poster cards sized for comfortable viewing at arm's length from monitor
- No viewport meta tags or responsive CSS needed

## Risks / Trade-offs

### Risk 1: TVDB API Rate Limiting
**Risk:** Popular content requests on each page load could hit TVDB rate limits

**Mitigation:**
- Implement TanStack Query with 5-minute stale time
- Cache responses in webserver layer (optional future enhancement)
- Limit concurrent requests (fetch genres once, popular content per page)
- Monitor TVDB API usage in SigNoz

### Risk 2: Increased Initial Page Load Time
**Risk:** Fetching popular content + genres adds latency to Home page load

**Mitigation:**
- Use TanStack Query's `suspense` mode with React Suspense boundaries
- Show skeleton loaders during fetch (maintain visual stability)
- Lazy load genre rows below fold
- Prefetch popular content on navbar hover (speculative loading)

### Risk 3: Large Bundle Size
**Risk:** New dependencies (TanStack Query) increase bundle size

**Mitigation:**
- Use native CSS scroll-snap instead of carousel library (saves ~60KB)
- Code-split pages with React.lazy() (Home, Shows, Movies loaded on demand)
- Tree-shake unused Material-UI components
- No responsive CSS reduces overall bundle size
- Monitor bundle size with vite-bundle-visualizer

**Baseline:** Current UI bundle is ~300KB. Target: Keep under 400KB (desktop-only reduces overhead).

### Risk 4: Genre List Changes
**Risk:** TVDB genres could change, breaking hardcoded genre selections

**Mitigation:**
- Fetch genres dynamically from API (don't hardcode)
- Genre selection for rows based on name matching (e.g., "Action", "Comedy")
- Fallback to "All Popular" if specific genre not found
- Log genre mismatches for monitoring

### Risk 5: Breaking Change for Existing Users
**Risk:** Removing `/schedule` route disrupts user workflows

**Mitigation:**
- Add redirect from `/schedule` to `/` with hash anchor `/#schedule`
- Show browser notification on first visit explaining new location
- Document change prominently in release notes
- Consider keeping `/schedule` route with redirect for 1-2 releases

## Migration Plan

### Phase 1: Backend API (Low Risk)
1. Implement tvdb-proxy endpoints (`/series/popular`, `/movies/popular`, `/genres`)
2. Implement webserver proxy endpoints (`/api/popular/shows`, `/api/popular/movies`, `/api/genres`, `/api/schedule/weekly`)
3. Deploy and test endpoints independently (UI still uses old design)
4. **Rollback:** Endpoints are additive, no breaking changes. Can be removed if unused.

### Phase 2: UI Foundation (Medium Risk)
1. Create Material-UI theme and theme provider
2. Restructure directories (pages/, components/ui/)
3. Add TanStack Query provider
4. Implement GenreContext
5. Build base components (Navbar, HorizontalCarousel, MediaCard)
6. **Rollback:** Changes are additive. Old components still work. Can revert to old routing.

### Phase 3: New Pages (Medium Risk)
1. Build Home page (popular + schedule)
2. Build Shows page (genre rows)
3. Build Movies page (genre rows)
4. Convert Search to overlay
5. Update routing in App.tsx
6. **Rollback:** Keep old components as fallback. Feature flag to toggle between old/new UI.

### Phase 4: Cleanup (Low Risk)
1. Remove old Dashboard, Schedule, Toolbar components
2. Remove unused CSS files
3. Remove `/schedule` route (add redirect)
4. Update nginx configuration if needed
5. **Rollback:** Can restore removed files from git history.

### Deployment Strategy
- Deploy backend changes first (Phase 1) - non-breaking
- Deploy UI changes as feature flag (optional: `?new_ui=true` query param for testing)
- Monitor error rates, API latency, user feedback
- Enable for all users once validated
- Remove old code after 1-2 releases

### Testing Checklist
- [ ] Backend endpoints return expected data
- [ ] Popular content loads and displays correctly
- [ ] Genre filtering works on Shows/Movies pages (6 genres each)
- [ ] Schedule widget shows upcoming downloads
- [ ] Search overlay opens/closes correctly
- [ ] Media card hover effects work
- [ ] Carousels scroll left/right smoothly
- [ ] Desktop layout (1920x1080) displays correctly
- [ ] Keyboard navigation (Tab, Enter, Esc)
- [ ] Screen reader accessibility (ARIA labels)
- [ ] No console errors or warnings
- [ ] OpenTelemetry traces captured correctly
- [ ] Download functionality still works (shows/movies)
- [ ] Genre caching in tvdb-proxy works correctly

## Open Questions

1. **Should we implement genre preferences?** (e.g., user can reorder/hide genre rows)
   - **Answer:** No, not in this change. Future enhancement.

2. **Should Home page show recently downloaded content?**
   - **Answer:** No, not in this change. Focus on popular + schedule. Future enhancement.

3. **Should we add a "Favorites" or "Watchlist" feature?**
   - **Answer:** No, out of scope. Scout is for downloading, not tracking watches.

4. **Should we implement server-side caching for popular content?**
   - **Answer:** No, not in initial implementation. TanStack Query client-side caching is sufficient. Can add later if TVDB rate limits become an issue.

5. **Should we support custom genre selection?**
   - **Answer:** No, hardcode initial genre set. Can make configurable later.
