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
 * rhythm instead of each page hand-rolling a bare Typography.
 */
const PageHeader: React.FC<PageHeaderProps> = ({ eyebrow, title, description, action }) => {
  const theme = useTheme();

  return (
    <Box
      sx={{
        display: 'flex',
        alignItems: 'flex-end',
        justifyContent: 'space-between',
        gap: 3,
        mb: 4,
      }}
    >
      <Box>
        {eyebrow && (
          <Typography variant="overline" sx={{ display: 'block', color: theme.palette.primary.light, mb: 0.5 }}>
            {eyebrow}
          </Typography>
        )}
        <Typography variant="h3" component="h1" sx={{ color: theme.palette.text.primary }}>
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
