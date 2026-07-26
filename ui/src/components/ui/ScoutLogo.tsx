import React, { useId } from 'react';
import Box from '@mui/material/Box';
import type { SxProps, Theme } from '@mui/material/styles';

export interface ScoutLogoProps {
  /** Height of the mark in pixels. The wordmark scales relative to it. */
  size?: number;
  /** Render the "Scout" wordmark next to the mark. */
  showWordmark?: boolean;
  sx?: SxProps<Theme>;
}

const GRADIENT_START = '#818CF8';
const GRADIENT_MID = '#6366F1';
const GRADIENT_END = '#A855F7';

/**
 * ScoutMark
 *
 * The Scout brand mark: a broken viewfinder ring with cardinal crosshair ticks
 * wrapped around a play glyph — "scouting" for something to watch.
 *
 * Rendered inline so the gradient can be uniquely scoped per instance and the
 * mark inherits layout sizing without an extra network request.
 */
const ScoutMark: React.FC<{ size: number }> = ({ size }) => {
  const gradientId = `scout-mark-gradient-${useId()}`;

  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 32 32"
      role="img"
      aria-label="Scout"
      style={{ display: 'block', flexShrink: 0 }}
    >
      <defs>
        <linearGradient id={gradientId} x1="4" y1="2" x2="28" y2="30" gradientUnits="userSpaceOnUse">
          <stop offset="0" stopColor={GRADIENT_START} />
          <stop offset="0.55" stopColor={GRADIENT_MID} />
          <stop offset="1" stopColor={GRADIENT_END} />
        </linearGradient>
      </defs>
      <g fill="none" stroke={`url(#${gradientId})`} strokeWidth="2.4" strokeLinecap="round">
        <circle cx="16" cy="16" r="11" strokeDasharray="13.28 4" strokeDashoffset="-2" />
        <path d="M16 1.6v3.2M30.4 16h-3.2M16 30.4v-3.2M1.6 16h3.2" />
      </g>
      <path
        d="M13.4 11.3a1 1 0 0 1 1.52-.86l7.1 4.36a1.2 1.2 0 0 1 0 2.4l-7.1 4.36a1 1 0 0 1-1.52-.86z"
        fill={`url(#${gradientId})`}
      />
    </svg>
  );
};

/**
 * ScoutLogo
 *
 * Full brand lockup — mark plus optional gradient wordmark. Used in the navbar
 * and anywhere Scout needs to identify itself.
 */
const ScoutLogo: React.FC<ScoutLogoProps> = ({ size = 32, showWordmark = true, sx }) => (
  <Box sx={{ display: 'inline-flex', alignItems: 'center', gap: `${size * 0.28}px`, ...sx }}>
    <ScoutMark size={size} />
    {showWordmark && (
      <Box
        component="span"
        sx={{
          fontSize: `${size * 0.72}px`,
          fontWeight: 700,
          letterSpacing: '-0.03em',
          lineHeight: 1,
          background: `linear-gradient(100deg, ${GRADIENT_START} 0%, ${GRADIENT_MID} 45%, ${GRADIENT_END} 100%)`,
          WebkitBackgroundClip: 'text',
          backgroundClip: 'text',
          color: 'transparent',
          // Keeps the wordmark legible if background-clip: text is unsupported.
          WebkitTextFillColor: 'transparent',
        }}
      >
        Scout
      </Box>
    )}
  </Box>
);

export { ScoutMark };
export default ScoutLogo;
