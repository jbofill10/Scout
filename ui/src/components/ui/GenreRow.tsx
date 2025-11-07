import React, { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import Button from '@mui/material/Button';
import Skeleton from '@mui/material/Skeleton';
import { useTheme } from '@mui/material/styles';
import HorizontalCarousel from './HorizontalCarousel';
import MediaCard from './MediaCard';
import SearchResultDialog from '../SearchResultDialog';
import type { SearchResult } from '../SearchResultsList';

interface GenreRowProps {
  genre: string;
  mediaType: 'series' | 'movie';
}

/**
 * GenreRow Component
 *
 * Displays a horizontal carousel of popular media for a specific genre.
 * Features:
 * - Fetches popular content from API based on genre and media type
 * - Shows loading skeletons during fetch
 * - Error handling with retry button
 * - Opens SearchResultDialog on media card click
 * - Integrates MediaCard and HorizontalCarousel components
 */
const GenreRow: React.FC<GenreRowProps> = ({ genre, mediaType }) => {
  const theme = useTheme();
  const [dialogOpen, setDialogOpen] = useState(false);
  const [selectedMedia, setSelectedMedia] = useState<SearchResult | null>(null);

  // Fetch popular content for this genre
  const { data, isLoading, isError, refetch } = useQuery<SearchResult[]>({
    queryKey: ['popular', mediaType, genre],
    queryFn: async () => {
      const endpoint = mediaType === 'series' ? '/api/popular/shows' : '/api/popular/movies';
      const params = new URLSearchParams({ genre });
      const response = await fetch(`${endpoint}?${params}`);
      if (!response.ok) {
        throw new Error('Failed to fetch popular content');
      }
      return response.json();
    },
    staleTime: 5 * 60 * 1000, // 5 minutes
    retry: 2,
  });

  const handleMediaClick = (media: { id: string; name: string; imageUrl: string }) => {
    // Convert to SearchResult format for dialog
    const searchResult: SearchResult = {
      id: media.id,
      mediaName: media.name,
      imageUrl: media.imageUrl,
    };
    setSelectedMedia(searchResult);
    setDialogOpen(true);
  };

  const handleCloseDialog = () => {
    setDialogOpen(false);
    setSelectedMedia(null);
  };

  const handleDownload = (result: SearchResult) => {
    // Route to correct endpoint based on media type
    const endpoint = mediaType === 'movie' ? '/api/movies' : '/api/shows';

    fetch(endpoint, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(result),
    })
      .then((response) => {
        if (!response.ok) {
          throw new Error('Network response was not ok');
        }
        return response.json();
      })
      .then(() => {
        // Download initiated successfully
        handleCloseDialog();
      })
      .catch((error) => {
        console.error('Error initiating download:', error);
      });
  };

  // Loading state with skeletons
  if (isLoading) {
    return (
      <Box sx={{ mb: 4 }}>
        <Typography variant="h5" sx={{ mb: 2, fontWeight: 600, color: theme.palette.text.primary }}>
          {genre}
        </Typography>
        <Box sx={{ display: 'flex', gap: 2 }}>
          {Array.from({ length: 7 }).map((_, index) => (
            <Skeleton
              key={index}
              variant="rectangular"
              width={180}
              height={270}
              sx={{ borderRadius: 2, flex: '0 0 180px' }}
            />
          ))}
        </Box>
      </Box>
    );
  }

  // Error state with retry button
  if (isError) {
    return (
      <Box sx={{ mb: 4 }}>
        <Typography variant="h5" sx={{ mb: 2, fontWeight: 600, color: theme.palette.text.primary }}>
          {genre}
        </Typography>
        <Box
          sx={{
            display: 'flex',
            alignItems: 'center',
            gap: 2,
            p: 3,
            backgroundColor: theme.palette.background.paper,
            borderRadius: 2,
          }}
        >
          <Typography variant="body1" sx={{ color: theme.palette.error.main }}>
            Failed to load content for {genre}
          </Typography>
          <Button variant="outlined" color="primary" onClick={() => refetch()}>
            Retry
          </Button>
        </Box>
      </Box>
    );
  }

  // No data state
  if (!data || data.length === 0) {
    return null;
  }

  return (
    <>
      <HorizontalCarousel
        title={genre}
        items={data}
        renderItem={(item) => (
          <MediaCard
            media={{
              id: item.id,
              name: item.mediaName,
              imageUrl: item.imageUrl,
            }}
            onClick={handleMediaClick}
          />
        )}
      />
      <SearchResultDialog
        open={dialogOpen}
        onClose={handleCloseDialog}
        result={selectedMedia}
        onDownload={handleDownload}
        mediaType={mediaType}
      />
    </>
  );
};

export default GenreRow;
