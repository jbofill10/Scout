import React from 'react';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import { useTheme } from '@mui/material/styles';

export interface PageHeaderProps {
  /** Small uppercase label above the title. */
  eyebrow?: string;
  title: string;
  /** One-line explanation of what the page is for. */
  description?: string;
  /** Right-aligned actions, e.g. filters or a refresh control. */
  action?: React.ReactNode;
}

/**
 * PageHeader Component
 *
 * Consistent page-level heading: an optional eyebrow, the title, a supporting
 * line, and an optional action slot. Keeps every route's masthead on the same
 * rhythm instead of each page hand-rolling a bare Typography. On phones the
 * action drops below the text instead of squeezing the title.
 */
const PageHeader: React.FC<PageHeaderProps> = ({ eyebrow, title, description, action }) => {
  const theme = useTheme();

  return (
    <Box
      sx={{
        display: 'flex',
        flexDirection: { xs: 'column', sm: 'row' },
        alignItems: { xs: 'flex-start', sm: 'flex-end' },
        justifyContent: 'space-between',
        gap: { xs: 2, sm: 3 },
        mb: { xs: 3, md: 4 },
      }}
    >
      <Box sx={{ minWidth: 0 }}>
        {eyebrow && (
          <Typography variant="overline" sx={{ display: 'block', color: theme.palette.primary.light, mb: 0.5 }}>
            {eyebrow}
          </Typography>
        )}
        <Typography
          variant="h3"
          component="h1"
          sx={{ color: theme.palette.text.primary, fontSize: { xs: '1.5rem', sm: '1.75rem' } }}
        >
          {title}
        </Typography>
        {description && (
          <Typography variant="body2" sx={{ mt: 1, color: theme.palette.text.secondary, maxWidth: 620 }}>
            {description}
          </Typography>
        )}
      </Box>
      {action && <Box sx={{ flexShrink: 0 }}>{action}</Box>}
    </Box>
  );
};

export default PageHeader;
