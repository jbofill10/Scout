import React, { useState } from 'react';
import InfiniteScroll from 'react-infinite-scroll-component/dist/index.js';
import SearchResultsList from './SearchResultsList';
import type { SearchResult } from './SearchResultsList';
import TextField from '@mui/material/TextField';
import Button from '@mui/material/Button';
import Paper from '@mui/material/Paper';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';

const Search: React.FC = () => {
  const [searchTerm, setSearchTerm] = useState('');
  const [formSubmitted, setFormSubmitted] = useState(false);
  const [searchResults, setSearchResults] = useState<SearchResult[]>([]);
  const [hasMore, setHasMore] = useState(true);
  const [page, setPage] = useState(1);

  const fetchResults = async (query: string, pageNum: number) => {
    const params = new URLSearchParams({ query, media_type: 'series', page: pageNum.toString() });
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
          <SearchResultsList results={searchResults} />
        </InfiniteScroll>
      </Box>
    </Box>
  );
};

export default Search;
