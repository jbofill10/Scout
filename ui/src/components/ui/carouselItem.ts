/**
 * Poster slot sizing per breakpoint, shared by HorizontalCarousel and the
 * loading skeleton so a row does not jump when real posters arrive. The
 * fractional counts leave a partially visible poster at the edge as a hint
 * that the row scrolls.
 */
export const CAROUSEL_ITEM_SX = {
  flex: {
    xs: '0 0 calc((100% - 32px) / 2.6)',
    sm: '0 0 calc((100% - 48px) / 4)',
    md: '0 0 calc((100% - 80px) / 5.5)',
    lg: '0 0 calc((100% - 96px) / 7)',
  },
  minWidth: { xs: 0, lg: 150 },
  maxWidth: 230,
} as const;
