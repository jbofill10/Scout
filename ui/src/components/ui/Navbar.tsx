import React from 'react';
import { Link, useLocation } from 'react-router-dom';
import AppBar from '@mui/material/AppBar';
import Toolbar from '@mui/material/Toolbar';
import Typography from '@mui/material/Typography';
import Button from '@mui/material/Button';
import IconButton from '@mui/material/IconButton';
import Box from '@mui/material/Box';
import SearchIcon from '@mui/icons-material/Search';
import LiveTvIcon from '@mui/icons-material/LiveTv';
import { useTheme } from '@mui/material/styles';
import NotificationDropdown from './NotificationDropdown';

interface NavbarProps {
  onSearchClick: () => void;
}

/**
 * Navbar Component
 *
 * Fixed navigation bar for Netflix-style UI.
 * Features:
 * - Scout logo/branding on the left
 * - Navigation buttons (Home, Shows, Movies) in the center
 * - Search icon button on the right
 * - Highlights active route
 * - Theme background color (#0F172A)
 * - Fixed positioning at top of viewport
 */
const Navbar: React.FC<NavbarProps> = ({ onSearchClick }) => {
  const theme = useTheme();
  const location = useLocation();

  const isActive = (path: string) => location.pathname === path;

  return (
    <AppBar position="fixed" elevation={0}>
      <Toolbar sx={{ justifyContent: 'space-between', px: 4 }}>
        {/* Left: Logo/Branding */}
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          <LiveTvIcon sx={{ fontSize: 32, color: theme.palette.primary.main }} />
          <Typography
            variant="h5"
            component={Link}
            to="/"
            sx={{
              fontWeight: 700,
              color: theme.palette.text.primary,
              textDecoration: 'none',
              letterSpacing: '-0.02em',
              '&:hover': {
                color: theme.palette.primary.light,
              },
            }}
          >
            Scout
          </Typography>
        </Box>

        {/* Center: Navigation Links */}
        <Box sx={{ display: 'flex', gap: 1 }} role="navigation" aria-label="Main navigation">
          <Button
            component={Link}
            to="/"
            aria-label="Navigate to home page"
            sx={{
              color: isActive('/') ? theme.palette.primary.main : theme.palette.text.primary,
              fontWeight: isActive('/') ? 600 : 500,
              fontSize: '1rem',
              px: 2,
              borderBottom: isActive('/') ? `2px solid ${theme.palette.primary.main}` : '2px solid transparent',
              borderRadius: 0,
              transition: 'all 0.2s ease-in-out',
              '&:hover': {
                backgroundColor: 'transparent',
                color: theme.palette.primary.light,
                borderBottom: `2px solid ${theme.palette.primary.light}`,
              },
            }}
          >
            Home
          </Button>
          <Button
            component={Link}
            to="/shows"
            aria-label="Navigate to TV shows page"
            sx={{
              color: isActive('/shows') ? theme.palette.primary.main : theme.palette.text.primary,
              fontWeight: isActive('/shows') ? 600 : 500,
              fontSize: '1rem',
              px: 2,
              borderBottom: isActive('/shows') ? `2px solid ${theme.palette.primary.main}` : '2px solid transparent',
              borderRadius: 0,
              transition: 'all 0.2s ease-in-out',
              '&:hover': {
                backgroundColor: 'transparent',
                color: theme.palette.primary.light,
                borderBottom: `2px solid ${theme.palette.primary.light}`,
              },
            }}
          >
            Shows
          </Button>
          <Button
            component={Link}
            to="/movies"
            aria-label="Navigate to movies page"
            sx={{
              color: isActive('/movies') ? theme.palette.primary.main : theme.palette.text.primary,
              fontWeight: isActive('/movies') ? 600 : 500,
              fontSize: '1rem',
              px: 2,
              borderBottom: isActive('/movies') ? `2px solid ${theme.palette.primary.main}` : '2px solid transparent',
              borderRadius: 0,
              transition: 'all 0.2s ease-in-out',
              '&:hover': {
                backgroundColor: 'transparent',
                color: theme.palette.primary.light,
                borderBottom: `2px solid ${theme.palette.primary.light}`,
              },
            }}
          >
            Movies
          </Button>
          <Button
            component={Link}
            to="/library"
            aria-label="Navigate to library page"
            sx={{
              color: isActive('/library') ? theme.palette.primary.main : theme.palette.text.primary,
              fontWeight: isActive('/library') ? 600 : 500,
              fontSize: '1rem',
              px: 2,
              borderBottom: isActive('/library') ? `2px solid ${theme.palette.primary.main}` : '2px solid transparent',
              borderRadius: 0,
              transition: 'all 0.2s ease-in-out',
              '&:hover': {
                backgroundColor: 'transparent',
                color: theme.palette.primary.light,
                borderBottom: `2px solid ${theme.palette.primary.light}`,
              },
            }}
          >
            Library
          </Button>
        </Box>

        {/* Right: Notifications and Search */}
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          {/* Notification Bell */}
          <NotificationDropdown />

          {/* Search Icon */}
          <IconButton
            onClick={onSearchClick}
            sx={{
              color: theme.palette.text.primary,
              '&:hover': {
                color: theme.palette.primary.light,
                backgroundColor: 'rgba(79, 70, 229, 0.1)',
              },
            }}
            aria-label="Open search"
          >
            <SearchIcon fontSize="large" />
          </IconButton>
        </Box>
      </Toolbar>
    </AppBar>
  );
};

export default Navbar;
