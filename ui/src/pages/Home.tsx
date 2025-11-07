import React from 'react';
import Box from '@mui/material/Box';
import Container from '@mui/material/Container';
import Typography from '@mui/material/Typography';
import { useTheme } from '@mui/material/styles';
import ScheduleWidget from '../components/ui/ScheduleWidget';
import GenreRow from '../components/ui/GenreRow';

/**
 * Home Page Component
 *
 * Main landing page for Scout's Netflix-style UI.
 * Features:
 * - Page title "Scout"
 * - ScheduleWidget for upcoming downloads (next 7 days)
 * - Popular TV Shows carousel (no genre filter)
 * - Popular Movies carousel (no genre filter)
 * - Max-width container with centered layout
 * - Proper spacing between sections
 */
const Home: React.FC = () => {
  const theme = useTheme();

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
          Scout
        </Typography>

        {/* Schedule Widget - Shows upcoming downloads for next 7 days */}
        <ScheduleWidget />

        {/* Popular TV Shows - No genre filter, just show popular content */}
        <Box sx={{ mb: 4 }}>
          <GenreRow genre="Popular TV Shows" mediaType="series" />
        </Box>

        {/* Popular Movies - No genre filter, just show popular content */}
        <Box sx={{ mb: 4 }}>
          <GenreRow genre="Popular Movies" mediaType="movie" />
        </Box>
      </Container>
    </Box>
  );
};

export default Home;
