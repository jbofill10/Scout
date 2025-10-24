
import React from 'react';
import AppBar from '@mui/material/AppBar';
import Toolbar from '@mui/material/Toolbar';
import Typography from '@mui/material/Typography';
import IconButton from '@mui/material/IconButton';
import MenuIcon from '@mui/icons-material/Menu';
import Button from '@mui/material/Button';
import { Link, useLocation } from 'react-router-dom';

const navItems = [
  { label: 'Dashboard', path: '/' },
  { label: 'Schedule', path: '/schedule' },
  { label: 'Search', path: '/search' },
];

const ScoutToolbar: React.FC = () => {
  const location = useLocation();
  return (
    <AppBar position="fixed" sx={{ background: '#0e3358', width: '100%' }}>
      <Toolbar sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', px: 0, minHeight: 64 }}>
        <div style={{ display: 'flex', alignItems: 'center', flex: 1, minWidth: 0 }}>
          <IconButton edge="start" color="inherit" aria-label="menu" sx={{ mr: 2 }}>
            <MenuIcon />
          </IconButton>
          <img src="https://community.carbide3d.com/uploads/default/original/3X/2/6/2640ea01be3c21d4588c150942ea158a0d466700.svg" height="40" alt="Scout Logo" style={{ marginRight: 8 }} />
          <Typography variant="h6" component="div" sx={{ fontWeight: 600, letterSpacing: 2 }}>
            Scout
          </Typography>
        </div>
        <div style={{ display: 'flex', justifyContent: 'center', flex: 2, minWidth: 0 }}>
          {navItems.map(item => (
            <Button
              key={item.path}
              color={location.pathname === item.path ? 'secondary' : 'inherit'}
              component={Link}
              to={item.path}
              sx={{ color: location.pathname === item.path ? '#007bff' : '#fff', fontWeight: 500, mx: 2, fontSize: '1.1rem', letterSpacing: 1 }}
            >
              {item.label}
            </Button>
          ))}
        </div>
        <div style={{ flex: 1, minWidth: 0 }} />
      </Toolbar>
    </AppBar>
  );
};

export default ScoutToolbar;
