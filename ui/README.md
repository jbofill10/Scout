# Scout UI

Web interface for the Scout media torrenting system.

## Overview

The Scout UI is a React 19 application built with TypeScript, Material-UI v7, and Vite. It provides a user-friendly interface for searching media on TVDB, selecting episodes to download, and managing scheduled downloads.

## Architecture

```
ui/
├── src/
│   ├── components/           # React components
│   │   ├── Dashboard.tsx     # Main dashboard view
│   │   ├── Search.tsx        # Search interface
│   │   ├── SearchResultsList.tsx     # Display search results
│   │   ├── SearchResultDialog.tsx    # Episode selection dialog
│   │   ├── Schedule.tsx      # View scheduled downloads
│   │   └── Toolbar.tsx       # Navigation toolbar
│   ├── App.tsx               # Root component, routing
│   ├── main.tsx              # Entry point
│   └── index.css             # Global styles
├── public/                   # Static assets
├── vite.config.ts            # Vite configuration
├── tsconfig.json             # TypeScript configuration
├── eslint.config.js          # ESLint configuration
└── package.json              # Dependencies
```

## Features

### 1. Media Search
- Search for TV shows and movies via TVDB
- Real-time search with query parameters
- Display results with metadata (year, overview, poster)
- Filter by media type (series/movie)

### 2. Episode Selection
- View all episodes for selected TV show
- Group episodes by season
- Display air dates and episode titles
- Select multiple episodes for download
- "Select All" and "Deselect All" functionality

### 3. Scheduled Downloads
- View upcoming scheduled downloads
- See which episodes are pending vs. queued
- Display release dates for future episodes
- Manage download queue

### 4. Movie Downloads
- One-click movie downloads
- Display movie metadata (year, overview)
- Immediate download initiation

## Key Components

### Dashboard.tsx
Main view containing the search interface and results list.

**State Management:**
- Search query and media type
- Search results from webserver API
- Loading states

**Features:**
- Tabbed interface (Search, Schedule)
- Material-UI Paper layout
- Responsive design

### Search.tsx
Search input component with media type selector.

**Props:**
- `onSearch(query, mediaType)` - Callback when search is triggered
- `initialQuery` - Pre-populate search field
- `initialMediaType` - Default media type

**Features:**
- Text input with debouncing
- Radio buttons for series/movie selection
- Search button

### SearchResultsList.tsx
Displays search results as cards.

**Props:**
- `results` - Array of tvdb.Media objects
- `onShowSelect(show)` - Callback when show is selected
- `onMovieDownload(movie)` - Callback when movie is downloaded

**Features:**
- Grid layout with Material-UI Cards
- Show metadata display (title, year, overview)
- Episode count for series
- Download button for movies

### SearchResultDialog.tsx
Modal dialog for episode selection.

**Props:**
- `open` - Dialog visibility
- `show` - Selected show object
- `onClose()` - Callback to close dialog
- `onConfirm(selectedEpisodes)` - Callback with selected episodes

**Features:**
- Grouped by season
- Checkboxes for episode selection
- "Select All" and "Deselect All" buttons
- Air date display
- Confirm and Cancel actions

### Schedule.tsx
View scheduled downloads from the database.

**Features:**
- Fetch scheduled downloads from webserver `/api/scheduled`
- Display media title, episode info, release date
- Show status (pending/queued)
- Refresh functionality

### Toolbar.tsx
Navigation bar with app branding.

**Features:**
- Scout logo/title
- Navigation links (if routes added)
- Material-UI AppBar design

## Tech Stack

- **React 19** - UI framework with concurrent features
- **TypeScript** - Type safety
- **Material-UI v7** - Component library
- **Vite** - Build tool and dev server
- **ESLint** - Code linting
- **Axios** (or fetch) - HTTP client for webserver API

## API Integration

The UI communicates with the webserver via REST API:

### Search Endpoint
```typescript
GET /api/search?query={query}&media_type={type}

Response: tvdb.Media[]
```

### Download Show
```typescript
POST /api/shows
Body: {
  id: number,
  title: string,
  episodes: Episode[],
  anime: boolean
}

Response: {
  message: string,
  scheduled: number,
  immediate: number
}
```

### Download Movie
```typescript
POST /api/movies
Body: {
  id: number,
  title: string,
  year: number
}

Response: {
  message: string
}
```

### Get Scheduled Downloads
```typescript
GET /api/scheduled

Response: ScheduledDownload[]
```

## Configuration

### Environment Variables

Create `.env` file in `ui/` directory:

```env
VITE_API_BASE_URL=http://localhost:22920
```

**In Production (Kubernetes):**
- API calls go through ingress: `http://scout.local:30030/api`
- No environment variable needed (uses relative path `/api`)

### Vite Configuration

**vite.config.ts:**
```typescript
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:22920',
        changeOrigin: true
      }
    }
  }
})
```

This proxies `/api` requests to webserver during local development.

## Running Locally

### Prerequisites
- Node.js 18+ and npm
- Webserver running on port 22920

### Development Mode

```bash
cd ui

# Install dependencies
npm install

# Start dev server
npm run dev
```

The UI will be available at `http://localhost:5173`

### Build for Production

```bash
cd ui

# Build optimized bundle
npm run build

# Output in ui/dist/
```

### Preview Production Build

```bash
cd ui
npm run build
npm run preview
```

## Development

### Adding New Components

1. Create component file in `src/components/`
2. Define TypeScript interfaces for props
3. Use Material-UI components for consistency
4. Export component
5. Import in `App.tsx` or parent component

**Example:**
```typescript
// src/components/DownloadHistory.tsx
import { Card, CardContent, Typography } from '@mui/material';

interface DownloadHistoryProps {
  downloads: Download[];
}

export const DownloadHistory: React.FC<DownloadHistoryProps> = ({ downloads }) => {
  return (
    <Card>
      <CardContent>
        <Typography variant="h5">Download History</Typography>
        {/* Render downloads */}
      </CardContent>
    </Card>
  );
};
```

### Styling

**Global Styles:**
- Edit `src/index.css` for app-wide styles
- Material-UI theme customization in `App.tsx`

**Component Styles:**
- Use Material-UI's `sx` prop for inline styles
- Use `styled()` for reusable styled components
- Avoid CSS modules (not necessary with MUI)

**Example:**
```typescript
<Box sx={{
  display: 'flex',
  gap: 2,
  padding: 3
}}>
  {/* Content */}
</Box>
```

### Linting

```bash
cd ui

# Run ESLint
npm run lint

# Fix auto-fixable issues
npm run lint -- --fix
```

### Type Checking

```bash
cd ui

# Run TypeScript compiler check
npx tsc --noEmit
```

## Deployment

### Kubernetes Deployment

The UI is deployed as a static site served by Nginx:

**Dockerfile:**
```dockerfile
# Build stage
FROM node:18-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

# Production stage
FROM nginx:alpine
COPY --from=builder /app/dist /usr/share/nginx/html
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
```

**Kubernetes Service:**
- Service: `ui-service` on port 80
- Ingress route: `/` → `ui-service:80`
- Accessed via: `http://scout.local:30030/`

### Local Testing with Docker

```bash
cd ui

# Build image
docker build -t scout-ui:latest .

# Run container
docker run -p 8080:80 scout-ui:latest

# Access at http://localhost:8080
```

## Troubleshooting

### API Connection Issues

**Problem:** UI can't reach webserver API

**Solutions:**
1. Check webserver is running: `curl http://localhost:22920/search?query=test&media_type=series`
2. Verify Vite proxy configuration in `vite.config.ts`
3. Check browser console for CORS errors
4. Ensure `/api` prefix in API calls

### Build Errors

**Problem:** `npm run build` fails

**Solutions:**
1. Clear node_modules: `rm -rf node_modules && npm install`
2. Clear Vite cache: `rm -rf node_modules/.vite`
3. Check TypeScript errors: `npx tsc --noEmit`
4. Update dependencies: `npm update`

### ESLint Errors

**Problem:** Linting failures

**Solutions:**
1. Auto-fix issues: `npm run lint -- --fix`
2. Update ESLint config in `eslint.config.js`
3. Disable specific rules if needed (avoid overuse)

### Material-UI Styling Issues

**Problem:** Components not styled correctly

**Solutions:**
1. Check Material-UI version: `npm list @mui/material`
2. Ensure theme provider wraps app in `App.tsx`
3. Use MUI v7 documentation: https://mui.com/
4. Check `sx` prop syntax for typos

## Future Enhancements

Potential features to add:

1. **User Authentication**
   - Login/logout functionality
   - User-specific download history
   - Preferences and settings

2. **Download Management**
   - View active downloads with progress
   - Cancel/pause/resume downloads
   - Download history with filters

3. **Advanced Search**
   - Filters (genre, year, rating)
   - Sort options
   - Pagination for large result sets

4. **Notifications**
   - Toast notifications for download status
   - Error messages
   - Success confirmations

5. **Dark Mode**
   - Toggle between light/dark themes
   - Persist user preference

6. **Quality Preferences**
   - Select preferred video quality
   - Choose uploader preferences
   - Set default download options

7. **Anime Support**
   - Dedicated anime search interface
   - Display absolute numbering
   - Anime-specific metadata

## Contributing

When adding features:
1. Follow TypeScript best practices
2. Use Material-UI components consistently
3. Add proper type definitions for all props/state
4. Test in both development and production builds
5. Run linter before committing
6. Update this README with new features

## Dependencies

### Core
- `react` ^19.0.0
- `react-dom` ^19.0.0
- `typescript` ^5.x

### UI Framework
- `@mui/material` ^7.x
- `@mui/icons-material` ^7.x
- `@emotion/react` ^11.x
- `@emotion/styled` ^11.x

### Build Tools
- `vite` ^6.x
- `@vitejs/plugin-react` ^4.x

### Linting
- `eslint` ^9.x
- `typescript-eslint` ^8.x

## License

MIT
