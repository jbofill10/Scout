import React, { useEffect, useState } from 'react';
import { Link, useLocation } from 'react-router-dom';
import AppBar from '@mui/material/AppBar';
import Toolbar from '@mui/material/Toolbar';
import Button from '@mui/material/Button';
import IconButton from '@mui/material/IconButton';
import Tooltip from '@mui/material/Tooltip';
import Box from '@mui/material/Box';
import Badge from '@mui/material/Badge';
import SearchIcon from '@mui/icons-material/Search';
import { alpha, useTheme } from '@mui/material/styles';
import NotificationDropdown from './NotificationDropdown';
import ScoutLogo from './ScoutLogo';
import { useActiveDownloadCount } from '../../hooks/useActivity';

interface NavbarProps {
  onSearchClick: () => void;
}

const NAV_LINKS = [
  { label: 'Home', path: '/' },
  { label: 'Shows', path: '/shows' },
  { label: 'Movies', path: '/movies' },
  { label: 'Library', path: '/library' },
  { label: 'Activity', path: '/activity' },
];

/**
 * Navbar Component
 *
 * Fixed top navigation.
 * Features:
 * - Scout logo on the left, navigation pills in the center, actions on the right
 * - Transparent over the page at rest, frosted once scrolled so poster rows
 *   pass cleanly underneath
 * - Active route marked with a filled pill rather than an underline
 */
const Navbar: React.FC<NavbarProps> = ({ onSearchClick }) => {
  const theme = useTheme();
  const location = useLocation();
  const [isScrolled, setIsScrolled] = useState(false);
  const { data: activeDownloads } = useActiveDownloadCount();

  useEffect(() => {
    const onScroll = () => setIsScrolled(window.scrollY > 8);
    onScroll();
    window.addEventListener('scroll', onScroll, { passive: true });
    return () => window.removeEventListener('scroll', onScroll);
  }, []);

  const isActive = (path: string) => location.pathname === path;

  return (
    <AppBar
      position="fixed"
      elevation={0}
      sx={{
        backgroundColor: isScrolled ? 'rgba(11, 17, 32, 0.72)' : 'transparent',
        borderBottom: `1px solid ${isScrolled ? theme.palette.divider : 'transparent'}`,
        backdropFilter: isScrolled ? 'blur(16px) saturate(180%)' : 'none',
        transition: 'background-color .25s ease, border-color .25s ease',
      }}
    >
      <Toolbar sx={{ justifyContent: 'space-between', gap: 2, px: { xs: 2, md: 4 }, minHeight: 68 }}>
        {/* Left: Logo/Branding */}
        <Box
          component={Link}
          to="/"
          aria-label="Scout home"
          sx={{
            display: 'flex',
            alignItems: 'center',
            textDecoration: 'none',
            transition: 'opacity .2s ease',
            '&:hover': { opacity: 0.82 },
          }}
        >
          <ScoutLogo size={30} />
        </Box>

        {/* Center: Navigation Links */}
        <Box
          component="nav"
          aria-label="Main navigation"
          sx={{
            display: 'flex',
            gap: 0.5,
            p: 0.5,
            borderRadius: 999,
            backgroundColor: alpha('#94A3B8', 0.07),
            border: `1px solid ${theme.palette.divider}`,
          }}
        >
          {NAV_LINKS.map(({ label, path }) => {
            const active = isActive(path);
            // The Activity link doubles as the "work is happening" indicator.
            const badgeCount = path === '/activity' ? activeDownloads ?? 0 : 0;
            return (
              <Button
                key={path}
                component={Link}
                to={path}
                aria-current={active ? 'page' : undefined}
                aria-label={
                  badgeCount > 0 ? `${label}, ${badgeCount} in flight` : undefined
                }
                sx={{
                  px: 2.5,
                  py: 0.75,
                  borderRadius: 999,
                  fontSize: '0.9375rem',
                  fontWeight: active ? 650 : 500,
                  color: active ? theme.palette.text.primary : theme.palette.text.secondary,
                  backgroundColor: active ? alpha(theme.palette.primary.main, 0.22) : 'transparent',
                  transition: 'color .2s ease, background-color .2s ease',
                  '&:hover': {
                    color: theme.palette.text.primary,
                    backgroundColor: active
                      ? alpha(theme.palette.primary.main, 0.28)
                      : alpha('#94A3B8', 0.1),
                  },
                }}
              >
                {badgeCount > 0 ? (
                  <Badge
                    badgeContent={badgeCount}
                    max={99}
                    sx={{
                      '& .MuiBadge-badge': {
                        top: -2,
                        right: -12,
                        minWidth: 18,
                        height: 18,
                        fontSize: '0.6875rem',
                        fontWeight: 700,
                        backgroundColor: theme.palette.secondary.main,
                        color: theme.palette.common.white,
                      },
                    }}
                  >
                    {label}
                  </Badge>
                ) : (
                  label
                )}
              </Button>
            );
          })}
        </Box>

        {/* Right: Notifications and Search */}
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
          <NotificationDropdown />

          <Tooltip title="Search">
            <IconButton
              onClick={onSearchClick}
              sx={{
                color: theme.palette.text.secondary,
                '&:hover': {
                  color: theme.palette.text.primary,
                  backgroundColor: alpha(theme.palette.primary.main, 0.14),
                },
              }}
              aria-label="Open search"
            >
              <SearchIcon />
            </IconButton>
          </Tooltip>
        </Box>
      </Toolbar>
    </AppBar>
  );
};

export default Navbar;
