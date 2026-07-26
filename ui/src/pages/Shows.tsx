import React from 'react';
import Box from '@mui/material/Box';
import Container from '@mui/material/Container';
import Typography from '@mui/material/Typography';
import Alert from '@mui/material/Alert';
import { useTheme } from '@mui/material/styles';
import { useGenres } from '../contexts/GenreContext';
import GenreRow, { MediaRowSkeleton } from '../components/ui/GenreRow';
import PageHeader from '../components/ui/PageHeader';

/**
 * Shows Page Component
 *
 * Displays TV shows organized by genre.
 * Features:
 * - Page title "TV Shows"
 * - Uses GenreContext to fetch available genres
 * - Filters to 6 curated genres: Action, Comedy, Drama, Sci-Fi, Anime, Documentary
 * - Renders GenreRow for each genre with mediaType="series"
 * - Loading state with skeletons
 * - Error handling
 * - Gracefully handles missing genres
 */
const Shows: React.FC = () => {
  const theme = useTheme();
  const { genres, isLoading, error } = useGenres();

  // Filter to the 6 curated genres
  const curatedGenres = ['Action', 'Comedy', 'Drama', 'Sci-Fi', 'Anime', 'Documentary'];
  const filteredGenres = genres.filter((genre) => curatedGenres.includes(genre.name));

  return (
    <Box
      sx={{
        minHeight: '100vh',
        backgroundColor: theme.palette.background.default,
        pt: 13,
        pb: 8,
      }}
    >
      <Container maxWidth="xl">
        <PageHeader
          eyebrow="Browse"
          title="TV Shows"
          description="Popular series by genre. Pick a show to see which episodes you already have."
        />

        {/* Loading State */}
        {isLoading && (
          <Box>
            {Array.from({ length: 6 }).map((_, index) => (
              <MediaRowSkeleton key={index} />
            ))}
          </Box>
        )}

        {/* Error State */}
        {error && !isLoading && (
          <Alert severity="error" sx={{ mb: 4 }}>
            Failed to load genres: {error.message}
          </Alert>
        )}

        {/* Genre Rows */}
        {!isLoading && !error && filteredGenres.length > 0 && (
          <>
            {filteredGenres.map((genre) => (
              <GenreRow key={genre.slug} genre={genre.name} mediaType="series" />
            ))}
          </>
        )}

        {/* Empty State - No matching genres found */}
        {!isLoading && !error && filteredGenres.length === 0 && (
          <Typography
            variant="body1"
            sx={{
              color: theme.palette.text.secondary,
              textAlign: 'center',
              py: 8,
            }}
          >
            No genres available at the moment
          </Typography>
        )}
      </Container>
    </Box>
  );
};

export default Shows;
