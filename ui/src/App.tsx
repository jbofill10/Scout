import { useState } from 'react';
import { BrowserRouter as Router, Route, Routes } from 'react-router-dom';
import { ThemeProvider } from '@mui/material/styles';
import CssBaseline from '@mui/material/CssBaseline';
import { QueryClientProvider } from '@tanstack/react-query';
import './App.css';
import Navbar from './components/ui/Navbar';
import SearchDropdown from './components/ui/SearchDropdown';
import ErrorBoundary from './components/ErrorBoundary';
import Home from './pages/Home';
import Shows from './pages/Shows';
import Movies from './pages/Movies';
import Search from './pages/Search';
import Library from './pages/Library';
import theme from './theme/theme';
import { queryClient } from './lib/queryClient';
import { GenreProvider } from './contexts/GenreContext';

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
              <Navbar onSearchClick={handleOpenSearch} />
              <SearchDropdown isOpen={searchDropdownOpen} onClose={handleCloseSearch} />
              <Routes>
                <Route path="/" element={<ErrorBoundary><Home /></ErrorBoundary>} />
                <Route path="/shows" element={<ErrorBoundary><Shows /></ErrorBoundary>} />
                <Route path="/movies" element={<ErrorBoundary><Movies /></ErrorBoundary>} />
                <Route path="/search" element={<ErrorBoundary><Search /></ErrorBoundary>} />
                <Route path="/library" element={<ErrorBoundary><Library /></ErrorBoundary>} />
              </Routes>
            </Router>
          </GenreProvider>
        </ThemeProvider>
      </QueryClientProvider>
    </ErrorBoundary>
  );
}

export default App;
