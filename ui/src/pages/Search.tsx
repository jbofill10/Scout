import React, { useState } from 'react';
import SearchResultsList from '../components/SearchResultsList';
import { useEnrichedSearch } from '../hooks/useEnrichedSearch';
import TextField from '@mui/material/TextField';
import Button from '@mui/material/Button';
import Paper from '@mui/material/Paper';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import ToggleButton from '@mui/material/ToggleButton';
import ToggleButtonGroup from '@mui/material/ToggleButtonGroup';
import Tv from '@mui/icons-material/Tv';
import Movie from '@mui/icons-material/Movie';

/**
 * Search Page
 *
 * Submit-driven search backed by the same cached query as the overlay. The
 * enriched search endpoint returns the full result set in one response, so
 * there is no paging here.
 */
const Search: React.FC = () => {
  const [searchTerm, setSearchTerm] = useState('');
  const [submittedTerm, setSubmittedTerm] = useState('');
  const [mediaType, setMediaType] = useState<'series' | 'movie'>('series');

  const { results, isFetching, error } = useEnrichedSearch(submittedTerm, mediaType);

  const onSearch = (e: React.FormEvent) => {
    e.preventDefault();
    setSubmittedTerm(searchTerm);
  };

  const hasSearched = submittedTerm.trim().length > 0;

  return (
    <Box sx={{ mt: 10, display: 'flex', flexDirection: 'column', alignItems: 'center' }}>
      <Paper elevation={3} sx={{ p: 4, borderRadius: 3, mb: 4, width: '100%', maxWidth: 600 }}>
        <Box sx={{ mb: 3, display: 'flex', justifyContent: 'center' }}>
          <ToggleButtonGroup
            value={mediaType}
            exclusive
            onChange={(_, newValue) => newValue && setMediaType(newValue)}
            aria-label="media type"
            sx={{ boxShadow: 1 }}
          >
            <ToggleButton value="series" aria-label="TV shows">
              <Tv sx={{ mr: 1 }} />
              TV Shows
            </ToggleButton>
            <ToggleButton value="movie" aria-label="Movies">
              <Movie sx={{ mr: 1 }} />
              Movies
            </ToggleButton>
          </ToggleButtonGroup>
        </Box>
        <form onSubmit={onSearch} style={{ display: 'flex', gap: 16 }}>
          <TextField
            fullWidth
            label="Search for media"
            variant="outlined"
            value={searchTerm}
            onChange={e => setSearchTerm(e.target.value)}
            required
          />
          <Button type="submit" variant="contained" color="primary" sx={{ minWidth: 120 }}>
            Search
          </Button>
        </form>
      </Paper>
      <Box sx={{ width: '100%', maxWidth: 900, mt: 2 }}>
        {results.length > 0 && (
          <Typography variant="h6" sx={{ mb: 2 }}>
            Results
          </Typography>
        )}
        {isFetching && results.length === 0 && (
          <Typography sx={{ mt: 2 }}>Loading...</Typography>
        )}
        {error && (
          <Typography color="error" sx={{ mt: 2 }}>
            Search failed. Please try again.
          </Typography>
        )}
        {hasSearched && !isFetching && !error && results.length === 0 && (
          <Typography sx={{ mt: 2 }} color="text.secondary">
            No results for "{submittedTerm}".
          </Typography>
        )}
        <SearchResultsList results={results} mediaType={mediaType} />
      </Box>
    </Box>
  );
};

export default Search;
