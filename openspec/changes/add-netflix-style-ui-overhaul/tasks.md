# Implementation Tasks: Netflix-Style UI Overhaul

## Phase 1: Backend API (tvdb-proxy & webserver)

### 1.1 tvdb-proxy - Add Genre Endpoint
- [x] 1.1.1 Add `GET /genres` endpoint to tvdb_proxy/main.go
- [x] 1.1.2 Call TVDB API `/genres` endpoint
- [x] 1.1.3 Return JSON array of {id, name, slug}
- [x] 1.1.4 Test with curl and verify response format

### 1.2 tvdb-proxy - Add Series Popular Endpoint
- [x] 1.2.1 Add `GET /series/popular` endpoint with query params: genre, limit
- [x] 1.2.2 Call TVDB API `/series/filter` with country=usa, lang=eng, sort=score, sortType=desc
- [x] 1.2.3 Include genre filter if provided
- [x] 1.2.4 Limit results (default 20, max 50)
- [x] 1.2.5 Reuse existing enrichment logic (add episode metadata)
- [x] 1.2.6 Test with various genre filters

### 1.3 tvdb-proxy - Add Movies Popular Endpoint
- [x] 1.3.1 Add `GET /movies/popular` endpoint with query params: genre, limit
- [x] 1.3.2 Call TVDB API `/movies/filter` with country=usa, lang=eng, sort=score, sortType=desc
- [x] 1.3.3 Include genre filter if provided
- [x] 1.3.4 Limit results (default 20, max 50)
- [x] 1.3.5 Return enriched movie data
- [x] 1.3.6 Test with various genre filters

### 1.4 tvdb-proxy - Run Tests
- [x] 1.4.1 Run test-guardian agent for tvdb-proxy
- [x] 1.4.2 Fix any test failures
- [x] 1.4.3 Rebuild tvdb-proxy binary

### 1.5 webserver - Add Popular Handler
- [x] 1.5.1 Create webserver/internal/handler/popular_handler.go
- [x] 1.5.2 Implement PopularHandler struct with tvdb-proxy client
- [x] 1.5.3 Add GetPopularShows(c *gin.Context) method - proxy to tvdb-proxy /series/popular
- [x] 1.5.4 Add GetPopularMovies(c *gin.Context) method - proxy to tvdb-proxy /movies/popular
- [x] 1.5.5 Forward query params (genre, limit)
- [x] 1.5.6 Use context from request for observability

### 1.6 webserver - Add Genre Handler
- [x] 1.6.1 Add GetGenres(c *gin.Context) method to popular_handler.go
- [x] 1.6.2 Proxy to tvdb-proxy /genres endpoint
- [x] 1.6.3 Cache response in memory for 24 hours (optional optimization)

### 1.7 webserver - Add Schedule Handler
- [x] 1.7.1 Create webserver/internal/handler/schedule_handler.go
- [x] 1.7.2 Implement ScheduleHandler struct with repository dependency
- [x] 1.7.3 Add GetWeeklySchedule(c *gin.Context) method
- [x] 1.7.4 Query ScheduledDownloads table for next 7 days (status=pending)
- [x] 1.7.5 Return JSON with scheduled downloads (title, season, episode, releaseTime, posterUrl)
- [x] 1.7.6 Sort by releaseTime ascending

### 1.8 webserver - Register Routes
- [x] 1.8.1 Update webserver/cmd/webserver/main.go
- [x] 1.8.2 Register GET /api/popular/shows route
- [x] 1.8.3 Register GET /api/popular/movies route
- [x] 1.8.4 Register GET /api/genres route
- [x] 1.8.5 Register GET /api/schedule/weekly route

### 1.9 webserver - Run Tests
- [x] 1.9.1 Run test-guardian agent for webserver
- [x] 1.9.2 Fix any test failures
- [x] 1.9.3 Rebuild webserver binary

## Phase 2: UI Foundation

### 2.1 Install Dependencies
- [x] 2.1.1 Add @tanstack/react-query to package.json
- [x] 2.1.2 Run npm install
- [x] 2.1.3 Verify no dependency conflicts

### 2.2 Create Theme System
- [x] 2.2.1 Create ui/src/theme/theme.ts
- [x] 2.2.2 Define warm dark color palette (primary: #4F46E5 indigo, secondary: #8B5CF6 purple, background: #0F172A)
- [x] 2.2.3 Define typography scale and font family (Inter)
- [x] 2.2.4 Configure Material-UI component overrides (Card, Button, etc.)
- [x] 2.2.5 Export theme with createTheme()

### 2.3 Setup Query Client
- [x] 2.3.1 Create ui/src/lib/queryClient.ts
- [x] 2.3.2 Configure QueryClient with default options (staleTime: 5min, cacheTime: 10min)
- [x] 2.3.3 Export queryClient instance

### 2.4 Update App.tsx
- [x] 2.4.1 Import ThemeProvider and theme
- [x] 2.4.2 Import QueryClientProvider and queryClient
- [x] 2.4.3 Wrap app with ThemeProvider
- [x] 2.4.4 Wrap app with QueryClientProvider
- [x] 2.4.5 Maintain existing BrowserRouter and Routes

### 2.5 Create Directory Structure
- [x] 2.5.1 Create ui/src/pages/ directory
- [x] 2.5.2 Create ui/src/components/ui/ directory
- [x] 2.5.3 Create ui/src/contexts/ directory
- [x] 2.5.4 Create ui/src/lib/ directory
- [x] 2.5.5 Create ui/src/theme/ directory

### 2.6 Create Genre Context
- [x] 2.6.1 Create ui/src/contexts/GenreContext.tsx
- [x] 2.6.2 Define Genre type {id: number, name: string}
- [x] 2.6.3 Create GenreProvider component
- [x] 2.6.4 Fetch /api/genres on mount with useQuery
- [x] 2.6.5 Provide genres array and loading state via context
- [x] 2.6.6 Export useGenres() hook for consuming components

### 2.7 Add Genre Provider to App
- [x] 2.7.1 Import GenreProvider in App.tsx
- [x] 2.7.2 Wrap Routes with GenreProvider (inside QueryClientProvider)

## Phase 3: Core UI Components

### 3.1 Build MediaCard Component
- [x] 3.1.1 Create ui/src/components/ui/MediaCard.tsx
- [x] 3.1.2 Accept props: media (SearchResult), onClick
- [x] 3.1.3 Display poster image with 2:3 aspect ratio
- [x] 3.1.4 Add hover overlay with dark backdrop
- [x] 3.1.5 Show title and download icon on hover
- [x] 3.1.6 Implement lazy loading for images
- [x] 3.1.7 Add Material-UI Card with elevation on hover
- [x] 3.1.8 Add keyboard accessibility (Enter key to click)

### 3.2 Build HorizontalCarousel Component
- [x] 3.2.1 Create ui/src/components/ui/HorizontalCarousel.tsx
- [x] 3.2.2 Accept props: items (SearchResult[]), renderItem, onItemClick
- [x] 3.2.3 Implement scroll container with overflow-x: auto
- [x] 3.2.4 Add CSS scroll-snap-type: x mandatory
- [x] 3.2.5 Add left/right navigation buttons
- [x] 3.2.6 Hide scrollbar with CSS
- [x] 3.2.7 Show navigation buttons on hover
- [x] 3.2.8 Implement scrollBy() for smooth scrolling
- [x] 3.2.9 Add keyboard navigation (arrow keys)

### 3.3 Build GenreRow Component
- [x] 3.3.1 Create ui/src/components/ui/GenreRow.tsx
- [x] 3.3.2 Accept props: title (string), genre (number | undefined), mediaType ("series" | "movie")
- [x] 3.3.3 Fetch popular content with useQuery (/api/popular/shows or /api/popular/movies)
- [x] 3.3.4 Pass genre query param if provided
- [x] 3.3.5 Display title (Typography variant h5)
- [x] 3.3.6 Render HorizontalCarousel with fetched items
- [x] 3.3.7 Pass MediaCard as renderItem
- [x] 3.3.8 Show skeleton loader during fetch
- [x] 3.3.9 Show error message if fetch fails

### 3.4 Build ScheduleWidget Component
- [x] 3.4.1 Create ui/src/components/ui/ScheduleWidget.tsx
- [x] 3.4.2 Fetch /api/schedule/weekly with useQuery (refetch every 5min)
- [x] 3.4.3 Display horizontal timeline of scheduled items
- [x] 3.4.4 Show poster thumbnail, title, S##E## format, release date
- [x] 3.4.5 Scroll horizontally if more than 6 items
- [x] 3.4.6 Show "No scheduled downloads this week" empty state
- [x] 3.4.7 Click item to view details (open SearchResultDialog)
- [x] 3.4.8 Style with Material-UI Paper and compact Card layout

### 3.5 Build Navbar Component
- [x] 3.5.1 Create ui/src/components/ui/Navbar.tsx
- [x] 3.5.2 Replace old Toolbar component
- [x] 3.5.3 Use Material-UI AppBar with position="fixed"
- [x] 3.5.4 Add Scout logo/text (left side)
- [x] 3.5.5 Add navigation buttons: Home, Shows, Movies (center)
- [x] 3.5.6 Add search icon button (right side)
- [x] 3.5.7 Highlight active route
- [x] 3.5.8 Emit event when search icon clicked (to open SearchDropdown)
- [x] 3.5.9 Apply theme colors (background: #141414, text: #FFFFFF)

### 3.6 Build SearchDropdown Component
- [x] 3.6.1 Create ui/src/components/ui/SearchDropdown.tsx
- [x] 3.6.2 Accept props: isOpen (boolean), onClose (function)
- [x] 3.6.3 Use Material-UI Drawer with anchor="top" or custom positioned div
- [x] 3.6.4 Add semi-transparent backdrop (opacity: 0.85)
- [x] 3.6.5 Add search input at top (TextField with autoFocus)
- [x] 3.6.6 Implement media type toggle (TV Shows / Movies)
- [x] 3.6.7 Reuse existing search logic from Search.tsx
- [x] 3.6.8 Display results with infinite scroll
- [x] 3.6.9 Click result to open SearchResultDialog
- [x] 3.6.10 Close on backdrop click, ESC key, or explicit close
- [x] 3.6.11 Debounce search input (300ms)

## Phase 4: Pages

### 4.1 Build Home Page
- [x] 4.1.1 Create ui/src/pages/Home.tsx
- [x] 4.1.2 Add page title "Scout" (Typography variant h3)
- [x] 4.1.3 Add ScheduleWidget at top (below navbar, above content)
- [x] 4.1.4 Add GenreRow for popular shows (no genre filter, title "Popular TV Shows")
- [x] 4.1.5 Add GenreRow for popular movies (no genre filter, title "Popular Movies")
- [x] 4.1.6 Add spacing between sections (Material-UI Stack or Box)
- [x] 4.1.7 Implement page layout with max-width and centering

### 4.2 Build Shows Page
- [x] 4.2.1 Create ui/src/pages/Shows.tsx
- [x] 4.2.2 Add page title "TV Shows" (Typography variant h3)
- [x] 4.2.3 Consume genres from useGenres() hook
- [x] 4.2.4 Filter to exactly 6 genres: Action, Comedy, Drama, Sci-Fi, Anime, Documentary
- [x] 4.2.5 Render GenreRow for each genre (pass genre name to component)
- [x] 4.2.6 Add spacing between genre rows
- [x] 4.2.7 Handle genre loading state and missing genres gracefully

### 4.3 Build Movies Page
- [x] 4.3.1 Create ui/src/pages/Movies.tsx
- [x] 4.3.2 Add page title "Movies" (Typography variant h3)
- [x] 4.3.3 Consume genres from useGenres() hook
- [x] 4.3.4 Filter to exactly 6 genres: Action, Comedy, Drama, Sci-Fi, Anime, Documentary
- [x] 4.3.5 Render GenreRow for each genre (pass genre name to component)
- [x] 4.3.6 Add spacing between genre rows
- [x] 4.3.7 Handle genre loading state and missing genres gracefully

### 4.4 Update Search Page
- [x] 4.4.1 Move ui/src/components/Search.tsx to ui/src/pages/Search.tsx
- [x] 4.4.2 Keep existing search functionality (for direct /search route access)
- [x] 4.4.3 Update imports in other files if needed

### 4.5 Update App Routing
- [x] 4.5.1 Update ui/src/App.tsx routing
- [x] 4.5.2 Replace Dashboard with Home page (route: "/")
- [x] 4.5.3 Remove Schedule route
- [x] 4.5.4 Add Shows page route ("/shows")
- [x] 4.5.5 Add Movies page route ("/movies")
- [x] 4.5.6 Keep Search page route ("/search")
- [x] 4.5.7 Replace Toolbar with Navbar component
- [x] 4.5.8 Add SearchDropdown component with useState for isOpen
- [x] 4.5.9 Pass isOpen and onClose to SearchDropdown
- [x] 4.5.10 Connect Navbar search icon to toggle SearchDropdown

### 4.6 Remove Old Components
- [x] 4.6.1 Delete ui/src/components/Dashboard.tsx
- [x] 4.6.2 Delete ui/src/components/Schedule.tsx
- [x] 4.6.3 Delete ui/src/components/Toolbar.tsx
- [x] 4.6.4 Delete ui/src/components/Toolbar.css
- [x] 4.6.5 Delete ui/src/components/Search.css (if unused styles)
- [x] 4.6.6 Update imports in remaining files

## Phase 5: Polish and Testing

### 5.1 Add Loading States
- [x] 5.1.1 Create skeleton loader component for MediaCard
- [x] 5.1.2 Use in GenreRow during data fetch
- [x] 5.1.3 Add loading skeleton for ScheduleWidget
- [x] 5.1.4 Add loading spinner for SearchDropdown initial state

### 5.2 Add Error Boundaries
- [x] 5.2.1 Create ui/src/components/ErrorBoundary.tsx
- [x] 5.2.2 Wrap each page in ErrorBoundary
- [x] 5.2.3 Display user-friendly error message
- [x] 5.2.4 Log errors to console and OpenTelemetry

### 5.3 Add Error Handling UI
- [x] 5.3.1 Add error message to GenreRow if fetch fails
- [x] 5.3.2 Add error message to ScheduleWidget if fetch fails
- [x] 5.3.3 Add error message to SearchDropdown if search fails
- [x] 5.3.4 Include "Retry" button for transient errors

### 5.4 Implement Animations
- [x] 5.4.1 Add hover transitions to MediaCard (scale, opacity)
- [x] 5.4.2 Add fade-in animation for carousel items
- [x] 5.4.3 Add slide-down animation for SearchDropdown
- [x] 5.4.4 Add smooth scroll behavior for carousels
- [x] 5.4.5 Use Material-UI transitions (Fade, Slide)

### 5.5 Accessibility
- [x] 5.5.1 Add ARIA labels to all interactive elements
- [x] 5.5.2 Add focus indicators for keyboard navigation
- [x] 5.5.3 Test Tab key navigation through components
- [x] 5.5.4 Test screen reader with NVDA/VoiceOver
- [x] 5.5.5 Add alt text to all images
- [x] 5.5.6 Ensure color contrast meets WCAG AA standards (warm dark palette)
- [x] 5.5.7 Test keyboard shortcuts (ESC to close, arrows to navigate)

### 5.6 Performance Testing
- [x] 5.6.1 Measure initial page load time on desktop
- [x] 5.6.2 Measure time to interactive (TTI)
- [x] 5.6.3 Check bundle size (npm run build, target < 400KB)
- [x] 5.6.4 Verify lazy loading for images
- [x] 5.6.5 Test scroll performance on carousels
- [x] 5.6.6 Profile with React DevTools Profiler
- [x] 5.6.7 Optimize any performance bottlenecks

### 5.7 Browser Compatibility
- [x] 5.7.1 Test on Chrome (latest)
- [x] 5.7.2 Test on Firefox (latest)
- [x] 5.7.3 Test on Safari (latest)
- [x] 5.7.4 Test on Edge (latest)
- [x] 5.7.5 Fix any browser-specific issues

### 5.8 Final Cleanup
- [x] 5.8.1 Remove unused dependencies from package.json
- [x] 5.8.2 Remove unused imports and variables
- [x] 5.8.3 Run npm run lint and fix all issues
- [x] 5.8.4 Format code with prettier (if configured)
- [x] 5.8.5 Update CLAUDE.md with new UI structure and warm dark theme
- [x] 5.8.6 Update README.md if needed
- [x] 5.8.7 Remove any console.log statements
- [x] 5.8.8 Remove commented-out code

## Phase 6: Documentation and Deployment

### 6.1 Update Documentation
- [x] 6.1.1 Update CLAUDE.md UI section with new structure
- [x] 6.1.2 Document new components in comments
- [x] 6.1.3 Add inline JSDoc comments for complex functions
- [x] 6.1.4 Update architecture diagram if exists

### 6.2 Prepare for Deployment
- [x] 6.2.1 Build production bundle (npm run build)
- [x] 6.2.2 Test production build locally
- [x] 6.2.3 Rebuild Docker images (./compose-deploy.sh rebuild ui)
- [x] 6.2.4 Rebuild Docker images (./compose-deploy.sh rebuild webserver)
- [x] 6.2.5 Rebuild Docker images (./compose-deploy.sh rebuild tvdb-proxy)

### 6.3 Deploy to Production
- [x] 6.3.1 Stop existing services (./compose-deploy.sh stop)
- [x] 6.3.2 Deploy all services (./compose-deploy.sh)
- [x] 6.3.3 Monitor logs for errors (./compose-deploy.sh logs)
- [x] 6.3.4 Verify services are healthy (./compose-deploy.sh status)
- [x] 6.3.5 Test production deployment end-to-end

### 6.4 Monitor and Iterate
- [x] 6.4.1 Monitor SigNoz for errors and performance issues
- [x] 6.4.2 Check TVDB API usage and rate limits
- [x] 6.4.3 Gather user feedback
- [x] 6.4.4 Create follow-up tickets for improvements
- [x] 6.4.5 Plan next iteration (genre preferences, recently downloaded, etc.)
