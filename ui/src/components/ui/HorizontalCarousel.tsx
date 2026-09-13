import React, { useRef, useState, useEffect } from 'react';
import Box from '@mui/material/Box';
import IconButton from '@mui/material/IconButton';
import Typography from '@mui/material/Typography';
import ChevronLeftIcon from '@mui/icons-material/ChevronLeft';
import ChevronRightIcon from '@mui/icons-material/ChevronRight';
import { alpha, useTheme } from '@mui/material/styles';

interface HorizontalCarouselProps<T> {
  items: T[];
  renderItem: (item: T, index: number) => React.ReactNode;
  title?: string;
  /** Optional count rendered as a muted suffix after the title. */
  subtitle?: string;
}

/** Class on the row root; nav buttons reveal themselves via a CSS hover on it. */
const ROW_CLASS = 'scout-carousel';

/**
 * HorizontalCarousel Component
 *
 * Horizontal scrolling row with CSS scroll-snap.
 * Features:
 * - Floating circular nav buttons that fade in on hover, centered on the row
 * - Edge fades that hint at off-screen content without a hard clip
 * - Keyboard navigation (arrow keys)
 * - Hidden scrollbar for clean appearance
 * - Desktop-optimized for ~6.5 items per row at 1920x1080
 *
 * Hover reveal is CSS-only so moving the pointer across rows does not
 * re-render every poster in them.
 */
function HorizontalCarousel<T>({ items, renderItem, title, subtitle }: HorizontalCarouselProps<T>) {
  const theme = useTheme();
  const scrollContainerRef = useRef<HTMLDivElement>(null);
  const [showLeftButton, setShowLeftButton] = useState(false);
  const [showRightButton, setShowRightButton] = useState(true);

  // Check scroll position to show/hide navigation buttons
  const checkScroll = () => {
    if (scrollContainerRef.current) {
      const { scrollLeft, scrollWidth, clientWidth } = scrollContainerRef.current;
      setShowLeftButton(scrollLeft > 0);
      setShowRightButton(scrollLeft < scrollWidth - clientWidth - 10);
    }
  };

  useEffect(() => {
    checkScroll();
    const container = scrollContainerRef.current;
    if (container) {
      container.addEventListener('scroll', checkScroll, { passive: true });
      return () => container.removeEventListener('scroll', checkScroll);
    }
  }, [items]);

  // Scroll by one "page" (approximately 6 items worth of width)
  const scrollAmount = () => {
    if (scrollContainerRef.current) {
      return scrollContainerRef.current.clientWidth * 0.85;
    }
    return 0;
  };

  const scrollLeft = () => {
    scrollContainerRef.current?.scrollBy({ left: -scrollAmount(), behavior: 'smooth' });
  };

  const scrollRight = () => {
    scrollContainerRef.current?.scrollBy({ left: scrollAmount(), behavior: 'smooth' });
  };

  // Keyboard navigation
  const handleKeyDown = (event: React.KeyboardEvent) => {
    if (event.key === 'ArrowLeft') {
      scrollLeft();
      event.preventDefault();
    } else if (event.key === 'ArrowRight') {
      scrollRight();
      event.preventDefault();
    }
  };

  const navButtonSx = {
    position: 'absolute' as const,
    top: '50%',
    transform: 'translateY(-50%)',
    zIndex: 3,
    width: 44,
    height: 44,
    color: theme.palette.text.primary,
    backgroundColor: alpha('#020617', 0.82),
    border: `1px solid ${alpha('#F8FAFC', 0.14)}`,
    boxShadow: theme.shadows[6],
    opacity: 0,
    transition: 'opacity .24s ease, background-color .24s ease, transform .24s ease',
    [`.${ROW_CLASS}:hover &, &:focus-visible`]: {
      opacity: 1,
    },
    '&:hover': {
      backgroundColor: alpha(theme.palette.primary.main, 0.9),
      transform: 'translateY(-50%) scale(1.06)',
    },
  };

  // Fade the row edges so cut-off posters read as "more to come"
  const edgeFadeSx = (side: 'left' | 'right') => ({
    position: 'absolute' as const,
    top: 0,
    bottom: 0,
    [side]: 0,
    width: 56,
    zIndex: 2,
    pointerEvents: 'none' as const,
    background: `linear-gradient(to ${side === 'left' ? 'right' : 'left'}, ${
      theme.palette.background.default
    } 0%, transparent 100%)`,
  });

  return (
    <Box
      className={ROW_CLASS}
      onKeyDown={handleKeyDown}
      role="region"
      aria-label={title ? `${title} carousel` : 'Media carousel'}
      sx={{ width: '100%', mb: 5 }}
    >
      {title && (
        <Box sx={{ display: 'flex', alignItems: 'baseline', gap: 1.5, mb: 1.75 }}>
          <Typography variant="h5" sx={{ color: theme.palette.text.primary }}>
            {title}
          </Typography>
          {subtitle && (
            <Typography variant="caption" sx={{ color: theme.palette.text.secondary }}>
              {subtitle}
            </Typography>
          )}
        </Box>
      )}

      {/* Row viewport — nav buttons and edge fades are positioned against this,
          not the title, so they stay centered on the posters. */}
      <Box sx={{ position: 'relative' }}>
        {showLeftButton && <Box aria-hidden sx={edgeFadeSx('left')} />}
        {showRightButton && <Box aria-hidden sx={edgeFadeSx('right')} />}

        {showLeftButton && (
          <IconButton onClick={scrollLeft} sx={{ ...navButtonSx, left: 8 }} aria-label="Scroll left">
            <ChevronLeftIcon />
          </IconButton>
        )}

        {/* Scroll Container with CSS Scroll Snap */}
        <Box
          ref={scrollContainerRef}
          sx={{
            display: 'flex',
            gap: 2,
            // Room for the hover lift so raised cards are not clipped
            py: 1.5,
            my: -1.5,
            overflowX: 'auto',
            overflowY: 'hidden',
            scrollSnapType: 'x proximity',
            scrollBehavior: 'smooth',
            // Hide scrollbar
            '&::-webkit-scrollbar': {
              display: 'none',
            },
            scrollbarWidth: 'none',
            msOverflowStyle: 'none',
            // Each item gets approximately 1/7 of container width (shows ~6.5 items)
            '& > *': {
              flex: '0 0 calc((100% - 96px) / 7)',
              scrollSnapAlign: 'start',
              minWidth: '150px',
              maxWidth: '230px',
            },
          }}
        >
          {items.map((item, index) => (
            <Box
              key={index}
              sx={{
                animation: 'mediaRowFadeIn .45s cubic-bezier(0.22, 1, 0.36, 1)',
                // Cap the stagger so long rows do not trail in noticeably late
                animationDelay: `${Math.min(index, 8) * 0.04}s`,
                animationFillMode: 'backwards',
                '@keyframes mediaRowFadeIn': {
                  from: { opacity: 0, transform: 'translateY(12px)' },
                  to: { opacity: 1, transform: 'translateY(0)' },
                },
              }}
            >
              {renderItem(item, index)}
            </Box>
          ))}
        </Box>

        {showRightButton && (
          <IconButton onClick={scrollRight} sx={{ ...navButtonSx, right: 8 }} aria-label="Scroll right">
            <ChevronRightIcon />
          </IconButton>
        )}
      </Box>
    </Box>
  );
}

export default HorizontalCarousel;
