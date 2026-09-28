import React from 'react';
import { Link, useLocation } from 'react-router-dom';
import Box from '@mui/material/Box';
import BottomNavigation from '@mui/material/BottomNavigation';
import BottomNavigationAction from '@mui/material/BottomNavigationAction';
import Badge from '@mui/material/Badge';
import { useTheme } from '@mui/material/styles';
import { NAV_LINKS } from './navLinks';
import { useActiveDownloadCount } from '../../hooks/useActivity';

/** Height of the bar itself, excluding the home-indicator inset on notched phones. */
export const BOTTOM_NAV_HEIGHT = 60;

/**
 * BottomNav
 *
 * Phone and tablet navigation. The navbar's centre pills need roughly 450px,
 * which does not fit a portrait phone, so below the `md` breakpoint they hide
 * and these thumb-reachable tabs take over. The Activity tab carries the same
 * in-flight badge as the desktop link.
 */
const BottomNav: React.FC = () => {
  const theme = useTheme();
  const location = useLocation();
  const { data: activeDownloads } = useActiveDownloadCount();

  return (
    <Box
      component="nav"
      aria-label="Main navigation"
      sx={{
        display: { xs: 'block', md: 'none' },
        position: 'fixed',
        left: 0,
        right: 0,
        bottom: 0,
        zIndex: theme.zIndex.appBar,
        pb: 'env(safe-area-inset-bottom, 0px)',
        backgroundColor: 'rgba(11, 17, 32, 0.86)',
        backdropFilter: 'blur(16px) saturate(180%)',
        borderTop: `1px solid ${theme.palette.divider}`,
      }}
    >
      <BottomNavigation
        value={location.pathname}
        showLabels
        sx={{ height: BOTTOM_NAV_HEIGHT, backgroundColor: 'transparent' }}
      >
        {NAV_LINKS.map(({ label, path, icon: Icon }) => {
          const badgeCount = path === '/activity' ? activeDownloads ?? 0 : 0;
          return (
            <BottomNavigationAction
              key={path}
              component={Link}
              to={path}
              value={path}
              label={label}
              aria-label={badgeCount > 0 ? `${label}, ${badgeCount} in flight` : label}
              icon={
                badgeCount > 0 ? (
                  <Badge
                    badgeContent={badgeCount}
                    max={99}
                    sx={{
                      '& .MuiBadge-badge': {
                        minWidth: 18,
                        height: 18,
                        fontSize: '0.6875rem',
                        fontWeight: 700,
                        backgroundColor: theme.palette.secondary.main,
                        color: theme.palette.common.white,
                      },
                    }}
                  >
                    <Icon />
                  </Badge>
                ) : (
                  <Icon />
                )
              }
              sx={{
                minWidth: 0,
                px: 0.5,
                color: theme.palette.text.secondary,
                '& .MuiBottomNavigationAction-label': {
                  fontSize: '0.6875rem',
                  fontWeight: 600,
                  '&.Mui-selected': { fontSize: '0.6875rem' },
                },
                '&.Mui-selected': { color: theme.palette.primary.light },
              }}
            />
          );
        })}
      </BottomNavigation>
    </Box>
  );
};

export default BottomNav;
