# Implementation Tasks

## 1. Search Component (ui/src/components/Search.tsx)

- [x] 1.1 Import ToggleButton and ToggleButtonGroup from @mui/material
- [x] 1.2 Import Tv and Movie icons from @mui/icons-material
- [x] 1.3 Add `mediaType` state variable: `useState<'series' | 'movie'>('series')`
- [x] 1.4 Add ToggleButtonGroup component above or beside TextField
  - [x] 1.4.1 Set value to `mediaType` state
  - [x] 1.4.2 Add onChange handler to update `mediaType` state
  - [x] 1.4.3 Add two ToggleButton children: "TV Shows" and "Movies"
  - [x] 1.4.4 Style with `sx` prop to match existing patterns
  - [x] 1.4.5 Optional: Add Tv and Movie icons to buttons
- [x] 1.5 Update `fetchResults()` function (line 18)
  - [x] 1.5.1 Change `media_type: 'series'` to `media_type: mediaType`
  - [x] 1.5.2 Verify URLSearchParams includes dynamic mediaType
- [x] 1.6 Pass `mediaType` prop to SearchResultsList component (line 73)

## 2. Search Results List (ui/src/components/SearchResultsList.tsx)

- [x] 2.1 Add `mediaType: 'series' | 'movie'` to component props interface
- [x] 2.2 Update `handleDownload()` function (line 43)
  - [x] 2.2.1 Add conditional logic: if mediaType === 'movie', POST to '/api/movies'
  - [x] 2.2.2 Else POST to '/api/shows' (existing behavior)
  - [x] 2.2.3 Verify error handling works for both endpoints
- [x] 2.3 Pass `mediaType` prop to SearchResultDialog component (line 56)

## 3. Search Result Dialog (ui/src/components/SearchResultDialog.tsx)

- [x] 3.1 Add `mediaType: 'series' | 'movie'` to component props interface
- [x] 3.2 Update metadata display section
  - [x] 3.2.1 Add conditional rendering based on `mediaType`
  - [x] 3.2.2 For 'series': Display episode count (existing behavior)
  - [x] 3.2.3 For 'movie': Display release date or runtime from metadata
  - [x] 3.2.4 Handle missing metadata gracefully

## 4. Testing

- [x] 4.1 Run ESLint: `cd ui && npm run lint`
- [x] 4.2 Build UI: `cd ui && npm run build`
- [x] 4.3 Test with Docker Compose deployment
  - [x] 4.3.1 Deploy with `./compose-deploy.sh`
  - [x] 4.3.2 Search for TV shows, verify existing behavior unchanged
  - [x] 4.3.3 Switch to Movies, search for a movie
  - [x] 4.3.4 Verify movie results display correctly
  - [x] 4.3.5 Click download on a movie, verify POST to /api/movies
  - [x] 4.3.6 Check backend logs for proper movie download handling

## 5. Documentation

- [x] 5.1 Update CLAUDE.md if needed to document UI movie search feature
- [x] 5.2 Add inline comments explaining media type toggle logic

## 6. Validation

- [x] 6.1 Run `openspec validate add-movie-search-ui --strict`
- [x] 6.2 Verify all TypeScript types are correct
- [x] 6.3 Verify Material-UI patterns match existing codebase style
- [x] 6.4 Confirm default behavior is 'series' (backward compatible)

## Notes

- Minimal, unintrusive UI changes only
- Follow existing Material-UI v7 patterns (ToggleButtonGroup, sx prop)
- Default to 'series' to maintain current user experience
- No new dependencies required
- Backend endpoints already exist and are functional
- This change depends on `add-movie-downloading` backend work
