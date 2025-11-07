import React, { useRef, useState, useEffect } from 'react';
import Box from '@mui/material/Box';
import IconButton from '@mui/material/IconButton';
import Typography from '@mui/material/Typography';
import ChevronLeftIcon from '@mui/icons-material/ChevronLeft';
import ChevronRightIcon from '@mui/icons-material/ChevronRight';
import { useTheme } from '@mui/material/styles';

interface HorizontalCarouselProps<T> {
  items: T[];
  renderItem: (item: T, index: number) => React.ReactNode;
  title?: string;
}

/**
 * HorizontalCarousel Component
 *
 * Netflix-style horizontal scrolling carousel with CSS scroll-snap.
 * Features:
 * - CSS scroll-snap for smooth, snap-to-item scrolling
 * - Left/right navigation buttons (shown on hover)
 * - Keyboard navigation (arrow keys)
 * - Hidden scrollbar for clean appearance
 * - Desktop-optimized for 6-8 items per row at 1920x1080
 * - Smooth scrollBy() animation for navigation
 */
function HorizontalCarousel<T>({ items, renderItem, title }: HorizontalCarouselProps<T>) {
  const theme = useTheme();
  const scrollContainerRef = useRef<HTMLDivElement>(null);
  const [showLeftButton, setShowLeftButton] = useState(false);
  const [showRightButton, setShowRightButton] = useState(true);
  const [isHovered, setIsHovered] = useState(false);

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
      container.addEventListener('scroll', checkScroll);
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
    if (scrollContainerRef.current) {
      scrollContainerRef.current.scrollBy({
        left: -scrollAmount(),
        behavior: 'smooth',
      });
    }
  };

  const scrollRight = () => {
    if (scrollContainerRef.current) {
      scrollContainerRef.current.scrollBy({
        left: scrollAmount(),
        behavior: 'smooth',
      });
    }
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

  return (
    <Box
      onMouseEnter={() => setIsHovered(true)}
      onMouseLeave={() => setIsHovered(false)}
      onKeyDown={handleKeyDown}
      role="region"
      aria-label={title ? `${title} carousel` : 'Media carousel'}
      sx={{ position: 'relative', width: '100%', mb: 4 }}
    >
      {title && (
        <Typography variant="h5" sx={{ mb: 2, fontWeight: 600, color: theme.palette.text.primary }}>
          {title}
        </Typography>
      )}

      {/* Left Navigation Button */}
      {showLeftButton && (
        <IconButton
          onClick={scrollLeft}
          sx={{
            position: 'absolute',
            left: 0,
            top: title ? '50%' : '50%',
            transform: 'translateY(-50%)',
            zIndex: 2,
            backgroundColor: 'rgba(0, 0, 0, 0.7)',
            color: theme.palette.text.primary,
            opacity: isHovered ? 1 : 0,
            transition: 'opacity 0.3s ease-in-out',
            '&:hover': {
              backgroundColor: 'rgba(0, 0, 0, 0.85)',
            },
          }}
          aria-label="Scroll left"
        >
          <ChevronLeftIcon fontSize="large" />
        </IconButton>
      )}

      {/* Scroll Container with CSS Scroll Snap */}
      <Box
        ref={scrollContainerRef}
        sx={{
          display: 'flex',
          gap: 2,
          overflowX: 'auto',
          overflowY: 'hidden',
          scrollSnapType: 'x mandatory',
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
            maxWidth: '220px',
          },
        }}
      >
        {items.map((item, index) => (
          <Box
            key={index}
            sx={{
              animation: 'fadeIn 0.4s ease-in-out',
              animationDelay: `${index * 0.05}s`,
              animationFillMode: 'backwards',
              '@keyframes fadeIn': {
                from: {
                  opacity: 0,
                  transform: 'translateY(10px)',
                },
                to: {
                  opacity: 1,
                  transform: 'translateY(0)',
                },
              },
            }}
          >
            {renderItem(item, index)}
          </Box>
        ))}
      </Box>

      {/* Right Navigation Button */}
      {showRightButton && (
        <IconButton
          onClick={scrollRight}
          sx={{
            position: 'absolute',
            right: 0,
            top: title ? '50%' : '50%',
            transform: 'translateY(-50%)',
            zIndex: 2,
            backgroundColor: 'rgba(0, 0, 0, 0.7)',
            color: theme.palette.text.primary,
            opacity: isHovered ? 1 : 0,
            transition: 'opacity 0.3s ease-in-out',
            '&:hover': {
              backgroundColor: 'rgba(0, 0, 0, 0.85)',
            },
          }}
          aria-label="Scroll right"
        >
          <ChevronRightIcon fontSize="large" />
        </IconButton>
      )}
    </Box>
  );
}

export default HorizontalCarousel;
