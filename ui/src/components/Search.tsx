import React, { useState } from 'react';
import InfiniteScroll from 'react-infinite-scroll-component/dist/index.js';
import SearchResultsList from './SearchResultsList';
import type { SearchResult } from './SearchResultsList';
import TextField from '@mui/material/TextField';
import Button from '@mui/material/Button';
import Paper from '@mui/material/Paper';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import ToggleButton from '@mui/material/ToggleButton';
import ToggleButtonGroup from '@mui/material/ToggleButtonGroup';
import Tv from '@mui/icons-material/Tv';
import Movie from '@mui/icons-material/Movie';

const Search: React.FC = () => {
  const [searchTerm, setSearchTerm] = useState('');
  const [formSubmitted, setFormSubmitted] = useState(false);
  const [searchResults, setSearchResults] = useState<SearchResult[]>([]);
  const [hasMore, setHasMore] = useState(true);
  const [page, setPage] = useState(1);
  const [mediaType, setMediaType] = useState<'series' | 'movie'>('series');

  const fetchResults = async (query: string, pageNum: number) => {
    const params = new URLSearchParams({ query, media_type: mediaType, page: pageNum.toString() });
    const res = await fetch(`/api/search?${params}`);
    const data = await res.json();
    return data;
  };

  const onSearch = async (e: React.FormEvent) => {
    e.preventDefault();
    setFormSubmitted(true);
    setPage(1);
    const data = await fetchResults(searchTerm, 1);
    setSearchResults(data);
    setHasMore(data.length > 0);
  };

  const fetchMoreData = async () => {
    const nextPage = page + 1;
    const data = await fetchResults(searchTerm, nextPage);
    if (data.length === 0) {
      setHasMore(false);
      return;
    }
    setSearchResults(prev => [...prev, ...data]);
    setPage(nextPage);
  };

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
        {searchResults.length > 0 && (
          <Typography variant="h6" sx={{ mb: 2 }}>
            Results
          </Typography>
        )}
        <InfiniteScroll
          dataLength={searchResults.length}
          next={fetchMoreData}
          hasMore={hasMore}
          loader={formSubmitted ? <Typography sx={{ mt: 2 }}>Loading...</Typography> : null}
          style={{ overflow: 'visible' }}
        >
          <SearchResultsList results={searchResults} mediaType={mediaType} />
        </InfiniteScroll>
      </Box>
    </Box>
  );
};

export default Search;
