import React from 'react';
import Box from '@mui/material/Box';
import Container from '@mui/material/Container';
import Typography from '@mui/material/Typography';
import Alert from '@mui/material/Alert';
import { useTheme } from '@mui/material/styles';
import { useGenres } from '../contexts/GenreContext';
import { CURATED_GENRES } from '../utils/curatedGenres';
import GenreRow, { MediaRowSkeleton } from '../components/ui/GenreRow';
import PageHeader from '../components/ui/PageHeader';

/**
 * Movies Page Component
 *
 * Displays movies organized by genre.
 * Features:
 * - Page title "Movies"
 * - Uses GenreContext to fetch available genres
 * - Shows the curated genres from utils/curatedGenres.ts
 *   (Action, Comedy, Drama, Sci-Fi, Horror, Thriller, Anime, Documentary)
 * - Renders GenreRow for each genre with mediaType="movie"
 * - Loading state with skeletons
 * - Error handling
 * - Gracefully handles missing genres
 */
const Movies: React.FC = () => {
  const theme = useTheme();
  // GenreContext already keeps only the curated genres, in row order
  const { genres, isLoading, error } = useGenres();

  return (
    <Box
      sx={{
        minHeight: '100vh',
        backgroundColor: theme.palette.background.default,
        pt: { xs: 11, md: 13 },
        pb: 8,
      }}
    >
      <Container maxWidth="xl">
        <PageHeader
          eyebrow="Browse"
          title="Movies"
          description="Popular films by genre. Pick a title to queue it for download."
        />

        {/* Loading State */}
        {isLoading && (
          <Box>
            {Array.from({ length: CURATED_GENRES.length }).map((_, index) => (
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
        {!isLoading && !error && genres.length > 0 && (
          <>
            {genres.map((genre) => (
              <GenreRow key={genre.slug} genre={genre.name} mediaType="movie" />
            ))}
          </>
        )}

        {/* Empty State - No matching genres found */}
        {!isLoading && !error && genres.length === 0 && (
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

export default Movies;
