import React from 'react';
import Box from '@mui/material/Box';
import Container from '@mui/material/Container';
import { useTheme } from '@mui/material/styles';
import ScheduleWidget from '../components/ui/ScheduleWidget';
import GenreRow from '../components/ui/GenreRow';
import PageHeader from '../components/ui/PageHeader';

/**
 * Home Page Component
 *
 * Main landing page.
 * Features:
 * - ScheduleWidget for upcoming downloads (next 7 days)
 * - Popular TV Shows carousel (no genre filter)
 * - Popular Movies carousel (no genre filter)
 */
const Home: React.FC = () => {
  const theme = useTheme();

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
          eyebrow="Your library"
          title="What's next"
          description="Upcoming downloads and what's popular right now."
        />

        {/* Schedule Widget - Shows upcoming downloads for next 7 days */}
        <ScheduleWidget />

        {/* Popular TV Shows - No genre filter, just show popular content */}
        <GenreRow genre="Popular TV Shows" mediaType="series" />

        {/* Popular Movies - No genre filter, just show popular content */}
        <GenreRow genre="Popular Movies" mediaType="movie" />
      </Container>
    </Box>
  );
};

export default Home;
