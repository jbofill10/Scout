import React, { useState } from "react";
import Card from "@mui/material/Card";
import CardMedia from "@mui/material/CardMedia";
import Box from "@mui/material/Box";
import Typography from "@mui/material/Typography";
import Chip from "@mui/material/Chip";
import DownloadIcon from "@mui/icons-material/Download";
import { useTheme } from "@mui/material/styles";
import type { MediaStatusBadge } from "../../types/MediaStatus";

export interface MediaCardProps {
  media: {
    id: string;
    name: string;
    imageUrl: string;
    category?: string;
  };
  onClick: (media: MediaCardProps["media"]) => void;
  statusBadge?: MediaStatusBadge;
}

/**
 * MediaCard Component
 *
 * Displays a poster-style media card with hover effects for Netflix-style UI.
 * Features:
 * - 2:3 aspect ratio poster (standard movie/TV poster dimensions)
 * - Lazy loading for performance
 * - Hover overlay with title and download icon
 * - Optional status badge (shows download status for movies/shows)
 * - Keyboard accessible (Enter key support)
 * - Material-UI elevation change on hover
 */
const MediaCard: React.FC<MediaCardProps> = ({ media, onClick, statusBadge }) => {
  const theme = useTheme();
  const [isHovered, setIsHovered] = useState(false);

  // Determine badge label based on status type
  const getBadgeLabel = (): string | null => {
    if (!statusBadge) return null;

    if (statusBadge.type === 'movie' && statusBadge.inLibrary) {
      return 'In Library';
    }

    if (statusBadge.type === 'show' && statusBadge.episodeCount) {
      const { downloaded, total } = statusBadge.episodeCount;
      return `${downloaded}/${total}`;
    }

    return null;
  };

  const badgeLabel = getBadgeLabel();

  const handleClick = () => {
    onClick(media);
  };

  const handleKeyPress = (event: React.KeyboardEvent) => {
    if (event.key === "Enter") {
      onClick(media);
    }
  };

  return (
    <Card
      onMouseEnter={() => setIsHovered(true)}
      onMouseLeave={() => setIsHovered(false)}
      onClick={handleClick}
      onKeyPress={handleKeyPress}
      tabIndex={0}
      role="button"
      aria-label={`View details for ${media.name}`}
      elevation={isHovered ? 8 : 2}
      sx={{
        position: "relative",
        width: "100%",
        aspectRatio: "2/3",
        cursor: "pointer",
        transition: "all 0.3s ease-in-out",
        transform: isHovered ? "scale(1.05)" : "scale(1)",
        borderRadius: 2,
        overflow: "hidden",
        "&:focus": {
          outline: `2px solid ${theme.palette.primary.main}`,
          outlineOffset: "2px",
        },
        "&:focus-visible": {
          outline: `2px solid ${theme.palette.primary.main}`,
          outlineOffset: "2px",
        },
      }}
    >
      <CardMedia
        component="img"
        image={media.imageUrl}
        alt={media.name}
        loading="lazy"
        sx={{
          width: "100%",
          height: "100%",
          objectFit: "cover",
        }}
      />

      {/* Status Badge (always visible) */}
      {badgeLabel && (
        <Chip
          label={badgeLabel}
          size="small"
          sx={{
            position: "absolute",
            top: 8,
            right: 8,
            backgroundColor: "rgba(0, 0, 0, 0.7)",
            color: "white",
            fontWeight: 600,
            fontSize: "0.75rem",
            backdropFilter: "blur(4px)",
            zIndex: 1,
          }}
        />
      )}

      {/* Hover Overlay */}
      <Box
        sx={{
          position: "absolute",
          top: 0,
          left: 0,
          right: 0,
          bottom: 0,
          backgroundColor: "rgba(0, 0, 0, 0.75)",
          display: "flex",
          flexDirection: "column",
          alignItems: "center",
          justifyContent: "center",
          opacity: isHovered ? 1 : 0,
          transition: "opacity 0.3s ease-in-out",
          padding: 2,
        }}
      >
        <DownloadIcon
          sx={{
            fontSize: 48,
            color: theme.palette.primary.main,
            mb: 2,
          }}
        />
        <Typography
          variant="h6"
          align="center"
          sx={{
            color: theme.palette.text.primary,
            fontWeight: 600,
            overflow: "hidden",
            textOverflow: "ellipsis",
            display: "-webkit-box",
            WebkitLineClamp: 3,
            WebkitBoxOrient: "vertical",
          }}
        >
          {media.name}
        </Typography>
        {media.category && (
          <Typography
            variant="caption"
            sx={{
              color: theme.palette.text.secondary,
              mt: 1,
              textTransform: "uppercase",
              letterSpacing: "0.05em",
            }}
          >
            {media.category}
          </Typography>
        )}
      </Box>
    </Card>
  );
};

export default MediaCard;
