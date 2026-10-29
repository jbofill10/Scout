# Change: Netflix-Style UI Overhaul

## Why

Scout's current UI is functional but lacks visual polish and modern UX patterns. The Dashboard and Schedule pages are minimal placeholders, and the overall design doesn't reflect the quality of a modern media streaming platform. Users expect a familiar, sleek interface similar to Netflix, HBO Max, or Disney+ when browsing and discovering content.

This overhaul will transform Scout into a visually appealing, intuitive platform that makes content discovery enjoyable through horizontal carousels, popular content recommendations, and integrated scheduling information—all in a minimalistic, sleek design.

## What Changes

- **New Netflix-style layout system** with horizontal carousels for browsing media
- **Redesigned navigation** with top navbar, search dropdown overlay with backdrop
- **New Home page** featuring popular movies/shows mix and weekly schedule widget
- **New Shows page** with genre-based rows of TV series
- **New Movies page** with genre-based rows of films
- **Remove Schedule page** - functionality moved to Home page as a widget
- **New theme system** with warm, dark palette (indigo and purple tones) and design tokens
- **Media cards** displaying poster-only with hover effects
- **Search overlay** that appears on top of current page with infinite scroll
- **TVDB popular content integration** using filter endpoints with score sorting
- **Webserver API endpoints** for popular content and weekly schedule data
- **tvdb-proxy endpoints** for TVDB filter API integration

### Breaking Changes

- **BREAKING**: `/schedule` route will be removed (functionality moved to Home page)
- **BREAKING**: Search will become an overlay instead of a dedicated page route (though `/search` route will remain for direct access)
- **BREAKING**: UI component structure will be reorganized (pages/ directory added)

## Impact

### Affected Specs
- `ui-netflix-layout` (NEW) - Horizontal carousel components and media cards
- `ui-navigation` (MODIFIED) - Top navbar with search dropdown
- `ui-pages` (MODIFIED) - New Home, Shows, Movies pages; Schedule page removed
- `tvdb-integration` (MODIFIED) - Add popular/filter endpoints with genre caching
- `webserver-api` (MODIFIED) - Add popular content and weekly schedule endpoints
- `ui-theme-system` (NEW) - Material-UI custom theme with warm dark palette

### Affected Code

**UI Service:**
- `ui/src/App.tsx` - Add theme provider, update routing
- `ui/src/components/` - Restructure with pages/ and components/ui/ directories
- `ui/src/pages/Home.tsx` (NEW) - Popular content + schedule widget
- `ui/src/pages/Shows.tsx` (NEW) - Genre rows for TV series
- `ui/src/pages/Movies.tsx` (NEW) - Genre rows for movies
- `ui/src/components/ui/HorizontalCarousel.tsx` (NEW) - Carousel component
- `ui/src/components/ui/MediaCard.tsx` (NEW) - Poster card component
- `ui/src/components/ui/GenreRow.tsx` (NEW) - Genre row with carousel
- `ui/src/components/ui/ScheduleWidget.tsx` (NEW) - Weekly schedule display
- `ui/src/components/ui/SearchDropdown.tsx` (NEW) - Search overlay
- `ui/src/components/ui/Navbar.tsx` (NEW) - Top navigation bar
- `ui/src/theme/theme.ts` (NEW) - Custom Material-UI theme
- `ui/src/contexts/GenreContext.tsx` (NEW) - Genre data provider
- `ui/src/components/Dashboard.tsx` (REMOVE)
- `ui/src/components/Schedule.tsx` (REMOVE)
- `ui/src/components/Toolbar.tsx` (REMOVE - replaced by Navbar)

**tvdb-proxy Service:**
- `tvdb_proxy/main.go` - Add popular/filter endpoints:
  - `GET /series/popular?genre={id}&limit={n}`
  - `GET /movies/popular?genre={id}&limit={n}`
  - `GET /genres`

**webserver Service:**
- `webserver/internal/handler/popular_handler.go` (NEW) - Popular content endpoints
- `webserver/internal/handler/schedule_handler.go` (NEW) - Weekly schedule endpoint
- `webserver/cmd/webserver/main.go` - Register new routes:
  - `GET /api/popular/shows?genre={id}`
  - `GET /api/popular/movies?genre={id}`
  - `GET /api/genres`
  - `GET /api/schedule/weekly`

### User Impact

**Positive:**
- Modern, visually appealing interface optimized for desktop browsing
- Easier content discovery with popular recommendations
- Integrated schedule view on home page for quick access
- Familiar navigation patterns (streaming platform style)
- Improved visual hierarchy with genre-based organization
- Unique warm dark aesthetic with indigo/purple palette

**Considerations:**
- Users accustomed to the `/schedule` route will need to use Home page
- Learning curve for new navigation patterns (search overlay)
- Initial page load may be slightly longer due to popular content fetching
- Desktop-focused design (no mobile/tablet optimization)

### Migration Notes

- No database migrations required
- No API contract changes for existing download endpoints
- Users should be notified of the UI changes in release notes
- Old `/schedule` route can optionally redirect to `/` for transition period
