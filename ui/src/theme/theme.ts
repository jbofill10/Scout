import { createTheme } from '@mui/material/styles';

/**
 * Scout UI Theme
 *
 * Dark, desktop-first theme built on a deep slate base with an indigo→purple
 * accent. Surfaces are separated by soft borders and diffuse shadows rather
 * than hard contrast steps, which keeps poster artwork the brightest thing on
 * the page.
 */

// Core surface ramp — each step reads as one level closer to the viewer.
const BACKDROP = '#0B1120';
const SURFACE = '#141C2E';
const SURFACE_RAISED = '#1B2438';

const HAIRLINE = 'rgba(148, 163, 184, 0.14)';
const HAIRLINE_STRONG = 'rgba(148, 163, 184, 0.24)';

export const BRAND_GRADIENT = 'linear-gradient(100deg, #818CF8 0%, #6366F1 45%, #A855F7 100%)';

/**
 * Softer, more diffuse elevation than MUI's default ramp. Dark UIs need
 * shadows with more spread and less opacity or they read as smudges.
 */
const shadow = (y: number, blur: number, alpha: number) =>
  `0 ${y}px ${blur}px rgba(2, 6, 23, ${alpha}), 0 ${Math.round(y / 3)}px ${Math.round(blur / 3)}px rgba(2, 6, 23, ${alpha / 2})`;

const shadows = [
  'none',
  shadow(1, 2, 0.24),
  shadow(2, 6, 0.28),
  shadow(3, 10, 0.3),
  shadow(4, 14, 0.32),
  shadow(6, 18, 0.34),
  shadow(8, 22, 0.36),
  shadow(10, 26, 0.38),
  shadow(12, 32, 0.4),
  shadow(14, 36, 0.42),
  shadow(16, 40, 0.44),
  shadow(18, 44, 0.45),
  shadow(20, 48, 0.46),
  shadow(22, 52, 0.47),
  shadow(24, 56, 0.48),
  shadow(26, 60, 0.49),
  shadow(28, 64, 0.5),
  shadow(30, 68, 0.51),
  shadow(32, 72, 0.52),
  shadow(34, 76, 0.53),
  shadow(36, 80, 0.54),
  shadow(38, 84, 0.55),
  shadow(40, 88, 0.56),
  shadow(42, 92, 0.57),
  shadow(44, 96, 0.58),
] as const;

const theme = createTheme({
  palette: {
    mode: 'dark',
    primary: {
      main: '#6366F1',      // Indigo — brighter than the old #4F46E5 so it holds up on dark
      dark: '#4F46E5',
      light: '#A5B4FC',
      contrastText: '#FFFFFF',
    },
    secondary: {
      main: '#A855F7',      // Purple
      dark: '#9333EA',
      light: '#C4B5FD',
      contrastText: '#FFFFFF',
    },
    background: {
      default: BACKDROP,
      paper: SURFACE,
    },
    text: {
      primary: '#E8EDF7',
      secondary: '#93A2BC',
      disabled: 'rgba(148, 163, 184, 0.45)',
    },
    divider: HAIRLINE,
    error: {
      main: '#F87171',
      dark: '#DC2626',
      light: '#FCA5A5',
    },
    warning: {
      main: '#FBBF24',
      dark: '#D97706',
      light: '#FCD34D',
    },
    success: {
      main: '#34D399',
      dark: '#059669',
      light: '#6EE7B7',
    },
    info: {
      main: '#60A5FA',
      dark: '#2563EB',
      light: '#93C5FD',
    },
    action: {
      hover: 'rgba(148, 163, 184, 0.08)',
      selected: 'rgba(99, 102, 241, 0.16)',
      focus: 'rgba(99, 102, 241, 0.24)',
    },
  },
  typography: {
    fontFamily: '"Inter Variable", "Inter", system-ui, -apple-system, "Segoe UI", Roboto, sans-serif',
    // Display headings run tight and heavy; body copy stays comfortable.
    h1: { fontSize: '3rem', fontWeight: 700, letterSpacing: '-0.03em', lineHeight: 1.1 },
    h2: { fontSize: '2.25rem', fontWeight: 700, letterSpacing: '-0.025em', lineHeight: 1.15 },
    h3: { fontSize: '1.75rem', fontWeight: 700, letterSpacing: '-0.025em', lineHeight: 1.2 },
    h4: { fontSize: '1.375rem', fontWeight: 700, letterSpacing: '-0.02em', lineHeight: 1.25 },
    h5: { fontSize: '1.125rem', fontWeight: 650, letterSpacing: '-0.015em', lineHeight: 1.3 },
    h6: { fontSize: '1rem', fontWeight: 650, letterSpacing: '-0.01em', lineHeight: 1.4 },
    subtitle1: { fontSize: '0.9375rem', fontWeight: 500, lineHeight: 1.5 },
    subtitle2: { fontSize: '0.8125rem', fontWeight: 600, letterSpacing: '0.01em' },
    body1: { fontSize: '0.9375rem', lineHeight: 1.6 },
    body2: { fontSize: '0.8125rem', lineHeight: 1.55 },
    caption: { fontSize: '0.75rem', lineHeight: 1.45 },
    overline: {
      fontSize: '0.6875rem',
      fontWeight: 700,
      letterSpacing: '0.12em',
      textTransform: 'uppercase',
    },
    button: { textTransform: 'none', fontWeight: 600, letterSpacing: '-0.005em' },
  },
  shape: {
    borderRadius: 10,
  },
  spacing: 8,
  shadows: shadows as unknown as ReturnType<typeof createTheme>['shadows'],
  components: {
    MuiCssBaseline: {
      styleOverrides: {
        body: {
          backgroundColor: BACKDROP,
        },
        // One consistent focus ring across every interactive element.
        ':focus-visible': {
          outline: '2px solid #6366F1',
          outlineOffset: '2px',
        },
      },
    },
    MuiPaper: {
      styleOverrides: {
        root: {
          backgroundImage: 'none',
        },
        outlined: {
          borderColor: HAIRLINE,
        },
      },
    },
    MuiButton: {
      defaultProps: { disableElevation: true },
      styleOverrides: {
        root: {
          borderRadius: 10,
          padding: '9px 18px',
          fontSize: '0.875rem',
          transition: 'background-color .18s ease, border-color .18s ease, color .18s ease, transform .18s ease',
          '&:active': { transform: 'translateY(1px)' },
        },
        containedPrimary: {
          backgroundImage: BRAND_GRADIENT,
          backgroundColor: 'transparent',
          '&:hover': {
            backgroundImage: BRAND_GRADIENT,
            filter: 'brightness(1.1)',
          },
        },
        outlined: {
          borderColor: HAIRLINE_STRONG,
          '&:hover': {
            borderColor: 'rgba(165, 180, 252, 0.6)',
            backgroundColor: 'rgba(99, 102, 241, 0.08)',
          },
        },
        text: {
          '&:hover': { backgroundColor: 'rgba(148, 163, 184, 0.08)' },
        },
      },
    },
    MuiIconButton: {
      styleOverrides: {
        root: {
          transition: 'background-color .18s ease, color .18s ease',
          '&:hover': { backgroundColor: 'rgba(148, 163, 184, 0.1)' },
        },
      },
    },
    MuiCard: {
      styleOverrides: {
        root: {
          backgroundImage: 'none',
          backgroundColor: SURFACE,
          borderRadius: 12,
          border: `1px solid ${HAIRLINE}`,
        },
      },
    },
    MuiDialog: {
      styleOverrides: {
        paper: {
          backgroundImage: 'none',
          backgroundColor: SURFACE_RAISED,
          borderRadius: 16,
          border: `1px solid ${HAIRLINE_STRONG}`,
        },
      },
    },
    MuiBackdrop: {
      styleOverrides: {
        root: {
          // No backdrop blur: it re-blurs every poster on the page for the whole
          // open/close fade, which made dialogs feel sluggish to open.
          backgroundColor: 'rgba(2, 6, 23, 0.78)',
        },
      },
    },
    MuiTextField: {
      styleOverrides: {
        root: {
          '& .MuiOutlinedInput-root': {
            borderRadius: 12,
            backgroundColor: 'rgba(148, 163, 184, 0.06)',
            transition: 'background-color .18s ease, box-shadow .18s ease',
            '& fieldset': { borderColor: HAIRLINE_STRONG },
            '&:hover': { backgroundColor: 'rgba(148, 163, 184, 0.1)' },
            '&:hover fieldset': { borderColor: 'rgba(148, 163, 184, 0.38)' },
            '&.Mui-focused': {
              backgroundColor: 'rgba(99, 102, 241, 0.08)',
              boxShadow: '0 0 0 4px rgba(99, 102, 241, 0.16)',
            },
            '&.Mui-focused fieldset': { borderColor: '#6366F1', borderWidth: 1 },
          },
        },
      },
    },
    MuiChip: {
      styleOverrides: {
        root: {
          borderRadius: 8,
          fontWeight: 600,
        },
        sizeSmall: {
          height: 22,
          fontSize: '0.6875rem',
        },
      },
    },
    MuiToggleButtonGroup: {
      styleOverrides: {
        root: {
          backgroundColor: 'rgba(148, 163, 184, 0.06)',
          borderRadius: 12,
          padding: 4,
          gap: 4,
        },
        grouped: {
          border: '0 !important',
          borderRadius: '8px !important',
        },
      },
    },
    MuiToggleButton: {
      styleOverrides: {
        root: {
          textTransform: 'none',
          fontWeight: 600,
          color: '#93A2BC',
          padding: '7px 18px',
          transition: 'background-color .18s ease, color .18s ease',
          '&:hover': { backgroundColor: 'rgba(148, 163, 184, 0.1)' },
          '&.Mui-selected': {
            backgroundColor: 'rgba(99, 102, 241, 0.2)',
            color: '#E8EDF7',
            '&:hover': { backgroundColor: 'rgba(99, 102, 241, 0.28)' },
          },
        },
      },
    },
    MuiSkeleton: {
      defaultProps: { animation: 'wave' },
      styleOverrides: {
        root: {
          backgroundColor: 'rgba(148, 163, 184, 0.09)',
        },
      },
    },
    MuiTooltip: {
      styleOverrides: {
        tooltip: {
          backgroundColor: SURFACE_RAISED,
          border: `1px solid ${HAIRLINE_STRONG}`,
          fontSize: '0.75rem',
          fontWeight: 500,
          padding: '6px 10px',
          borderRadius: 8,
        },
      },
    },
    MuiAppBar: {
      styleOverrides: {
        root: {
          backgroundImage: 'none',
          backgroundColor: 'rgba(11, 17, 32, 0.72)',
          backdropFilter: 'blur(16px) saturate(180%)',
          borderBottom: `1px solid ${HAIRLINE}`,
        },
      },
    },
    MuiAlert: {
      styleOverrides: {
        root: {
          borderRadius: 12,
          border: `1px solid ${HAIRLINE}`,
        },
      },
    },
    MuiLinearProgress: {
      styleOverrides: {
        root: { borderRadius: 999, height: 6 },
        bar: { borderRadius: 999 },
      },
    },
    MuiDivider: {
      styleOverrides: {
        root: { borderColor: HAIRLINE },
      },
    },
  },
});

export default theme;
