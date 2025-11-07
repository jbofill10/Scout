import { createTheme } from '@mui/material/styles';

/**
 * Scout Netflix-style UI Theme
 *
 * Warm dark color palette with indigo and purple accents
 * Designed for desktop-first experience at 1920x1080
 */
const theme = createTheme({
  palette: {
    mode: 'dark',
    primary: {
      main: '#4F46E5',      // Deep indigo
      dark: '#4338CA',      // Darker indigo
      light: '#6366F1',     // Lighter indigo
      contrastText: '#FFFFFF',
    },
    secondary: {
      main: '#8B5CF6',      // Warm purple
      dark: '#7C3AED',      // Darker purple
      light: '#A78BFA',     // Lighter purple
      contrastText: '#FFFFFF',
    },
    background: {
      default: '#0F172A',   // Very dark blue
      paper: '#1E293B',     // Slightly lighter dark blue
    },
    text: {
      primary: '#F1F5F9',   // Light gray-blue for primary text
      secondary: '#94A3B8', // Muted gray-blue for secondary text
    },
    divider: '#334155',     // Subtle divider color
    error: {
      main: '#EF4444',      // Red for errors
      dark: '#DC2626',
      light: '#F87171',
    },
    warning: {
      main: '#F59E0B',      // Amber for warnings
      dark: '#D97706',
      light: '#FBBF24',
    },
    success: {
      main: '#10B981',      // Green for success
      dark: '#059669',
      light: '#34D399',
    },
    info: {
      main: '#3B82F6',      // Blue for info
      dark: '#2563EB',
      light: '#60A5FA',
    },
  },
  typography: {
    fontFamily: '"Inter", "system-ui", "-apple-system", "BlinkMacSystemFont", "Segoe UI", "Roboto", sans-serif',
    h1: {
      fontSize: '3rem',
      fontWeight: 700,
      letterSpacing: '-0.02em',
    },
    h2: {
      fontSize: '2.25rem',
      fontWeight: 700,
      letterSpacing: '-0.01em',
    },
    h3: {
      fontSize: '1.875rem',
      fontWeight: 600,
      letterSpacing: '-0.01em',
    },
    h4: {
      fontSize: '1.5rem',
      fontWeight: 600,
    },
    h5: {
      fontSize: '1.25rem',
      fontWeight: 600,
    },
    h6: {
      fontSize: '1rem',
      fontWeight: 600,
    },
    body1: {
      fontSize: '1rem',
      lineHeight: 1.6,
    },
    body2: {
      fontSize: '0.875rem',
      lineHeight: 1.5,
    },
    button: {
      textTransform: 'none', // Disable uppercase transform on buttons
      fontWeight: 500,
    },
  },
  shape: {
    borderRadius: 8,
  },
  spacing: 8, // Default spacing unit (1 = 8px)
  components: {
    MuiButton: {
      styleOverrides: {
        root: {
          borderRadius: 8,
          padding: '10px 20px',
          fontSize: '0.875rem',
          fontWeight: 500,
        },
        contained: {
          boxShadow: 'none',
          '&:hover': {
            boxShadow: 'none',
          },
        },
      },
    },
    MuiCard: {
      styleOverrides: {
        root: {
          backgroundImage: 'none',
          backgroundColor: '#1E293B',
          borderRadius: 12,
        },
      },
    },
    MuiDialog: {
      styleOverrides: {
        paper: {
          backgroundImage: 'none',
          backgroundColor: '#1E293B',
          borderRadius: 12,
        },
      },
    },
    MuiTextField: {
      styleOverrides: {
        root: {
          '& .MuiOutlinedInput-root': {
            backgroundColor: '#1E293B',
            '& fieldset': {
              borderColor: '#334155',
            },
            '&:hover fieldset': {
              borderColor: '#475569',
            },
            '&.Mui-focused fieldset': {
              borderColor: '#4F46E5',
            },
          },
        },
      },
    },
    MuiAppBar: {
      styleOverrides: {
        root: {
          backgroundImage: 'none',
          backgroundColor: '#0F172A',
          borderBottom: '1px solid #334155',
        },
      },
    },
    MuiChip: {
      styleOverrides: {
        root: {
          borderRadius: 6,
        },
      },
    },
  },
});

export default theme;
