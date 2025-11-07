import React from 'react';
import Box from '@mui/material/Box';
import Container from '@mui/material/Container';
import Typography from '@mui/material/Typography';
import Skeleton from '@mui/material/Skeleton';
import Alert from '@mui/material/Alert';
import { useTheme } from '@mui/material/styles';
import { useGenres } from '../contexts/GenreContext';
import GenreRow from '../components/ui/GenreRow';

/**
 * Movies Page Component
 *
 * Displays movies organized by genre.
 * Features:
 * - Page title "Movies"
 * - Uses GenreContext to fetch available genres
 * - Filters to 6 curated genres: Action, Comedy, Drama, Sci-Fi, Anime, Documentary
 * - Renders GenreRow for each genre with mediaType="movie"
 * - Loading state with skeletons
 * - Error handling
 * - Gracefully handles missing genres
 */
const Movies: React.FC = () => {
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
        pt: 10,
        pb: 6,
      }}
    >
      <Container maxWidth="xl">
        {/* Page Title */}
        <Typography
          variant="h3"
          sx={{
            mb: 4,
            fontWeight: 700,
            color: theme.palette.text.primary,
          }}
        >
          Movies
        </Typography>

        {/* Loading State */}
        {isLoading && (
          <Box>
            {Array.from({ length: 6 }).map((_, index) => (
              <Box key={index} sx={{ mb: 4 }}>
                <Skeleton
                  variant="text"
                  width={200}
                  height={40}
                  sx={{ mb: 2 }}
                />
                <Box sx={{ display: 'flex', gap: 2 }}>
                  {Array.from({ length: 7 }).map((_, cardIndex) => (
                    <Skeleton
                      key={cardIndex}
                      variant="rectangular"
                      width={180}
                      height={270}
                      sx={{ borderRadius: 2, flex: '0 0 180px' }}
                    />
                  ))}
                </Box>
              </Box>
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
              <Box key={genre.slug} sx={{ mb: 4 }}>
                <GenreRow genre={genre.name} mediaType="movie" />
              </Box>
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

export default Movies;
