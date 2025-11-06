# Change: Add Movie Search to UI

## Why

Scout's backend now supports movie downloads (via the `add-movie-downloading` change), but the UI only allows searching for TV shows. The search page has `media_type: 'series'` hardcoded in the API call (Search.tsx:19), preventing users from discovering and downloading movies.

Users need a minimal, unintrusive way to toggle between searching for TV shows and movies. Without this UI feature, the completed backend movie functionality remains inaccessible to end users.

## What Changes

- **ui/src/components/Search.tsx**: Add media type state and Material-UI ToggleButtonGroup for series/movie selection, update `fetchResults()` to use dynamic media type instead of hardcoded 'series'
- **ui/src/components/SearchResultsList.tsx**: Accept `mediaType` prop and conditionally POST to `/api/shows` or `/api/movies` based on selection
- **ui/src/components/SearchResultDialog.tsx**: Accept `mediaType` prop and conditionally render media-specific metadata (episode count for shows, release date for movies)

The UI will follow existing Material-UI v7 patterns with a ToggleButtonGroup positioned near the search TextField. The default selection will be "series" to maintain current user behavior.

## Impact

**Affected specs:**
- `ui-search` - ADDED media type selection and dynamic API endpoint requirements

**Affected code:**
- `ui/src/components/Search.tsx` - Add media type selector, update API call
- `ui/src/components/SearchResultsList.tsx` - Conditionally POST to correct endpoint
- `ui/src/components/SearchResultDialog.tsx` - Show appropriate metadata

**Dependencies:**
- Requires `add-movie-downloading` backend change to be complete (currently 7/32 tasks)
- Backend endpoints already exist: `GET /api/search?media_type=movie`, `POST /api/movies`

**No breaking changes** - This is additive functionality that defaults to existing "series" behavior.
