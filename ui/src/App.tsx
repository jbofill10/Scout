import { lazy, Suspense, useState } from 'react';
import { BrowserRouter as Router, Route, Routes } from 'react-router-dom';
import { ThemeProvider } from '@mui/material/styles';
import CssBaseline from '@mui/material/CssBaseline';
import Box from '@mui/material/Box';
import CircularProgress from '@mui/material/CircularProgress';
import { QueryClientProvider } from '@tanstack/react-query';
import Navbar from './components/ui/Navbar';
import BottomNav, { BOTTOM_NAV_HEIGHT } from './components/ui/BottomNav';
import SearchDropdown from './components/ui/SearchDropdown';
import ErrorBoundary from './components/ErrorBoundary';
import ToastProvider from './components/ui/ToastProvider';
import theme from './theme/theme';
import { queryClient } from './lib/queryClient';
import { GenreProvider } from './contexts/GenreContext';

// Each page is its own chunk, so the first paint only waits on the shell and
// the page being opened. Home is the landing route, so it is still fetched up
// front by the preload hint Vite emits for it.
const Home = lazy(() => import('./pages/Home'));
const Shows = lazy(() => import('./pages/Shows'));
const Movies = lazy(() => import('./pages/Movies'));
const Search = lazy(() => import('./pages/Search'));
const Library = lazy(() => import('./pages/Library'));
const Activity = lazy(() => import('./pages/Activity'));

/** Shown while a page chunk is downloading; sized so the layout does not jump. */
function RouteFallback() {
  return (
    <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: '60vh' }}>
      <CircularProgress aria-label="Loading page" />
    </Box>
  );
}

function App() {
  const [searchDropdownOpen, setSearchDropdownOpen] = useState(false);

  const handleOpenSearch = () => {
    setSearchDropdownOpen(true);
  };

  const handleCloseSearch = () => {
    setSearchDropdownOpen(false);
  };

  return (
    <ErrorBoundary>
      <QueryClientProvider client={queryClient}>
        <ThemeProvider theme={theme}>
          <CssBaseline />
          <GenreProvider>
            <Router>
              {/* Inside the Router so a toast can link to the Activity page */}
              <ToastProvider>
                <Navbar onSearchClick={handleOpenSearch} />
                <SearchDropdown isOpen={searchDropdownOpen} onClose={handleCloseSearch} />
                {/* Below md the bottom tab bar is fixed over the page, so leave room for it */}
                <Box
                  component="main"
                  sx={{
                    pb: {
                      xs: `calc(${BOTTOM_NAV_HEIGHT}px + env(safe-area-inset-bottom, 0px))`,
                      md: 0,
                    },
                  }}
                >
                  <Suspense fallback={<RouteFallback />}>
                    <Routes>
                      <Route path="/" element={<ErrorBoundary><Home /></ErrorBoundary>} />
                      <Route path="/shows" element={<ErrorBoundary><Shows /></ErrorBoundary>} />
                      <Route path="/movies" element={<ErrorBoundary><Movies /></ErrorBoundary>} />
                      <Route path="/activity" element={<ErrorBoundary><Activity /></ErrorBoundary>} />
                      <Route path="/search" element={<ErrorBoundary><Search /></ErrorBoundary>} />
                      <Route path="/library" element={<ErrorBoundary><Library /></ErrorBoundary>} />
                    </Routes>
                  </Suspense>
                </Box>
                <BottomNav />
              </ToastProvider>
            </Router>
          </GenreProvider>
        </ThemeProvider>
      </QueryClientProvider>
    </ErrorBoundary>
  );
}

export default App;
