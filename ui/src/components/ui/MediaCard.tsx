import React, { useState } from "react";
import Card from "@mui/material/Card";
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
  isLoadingStatus?: boolean;
  onImageLoad?: () => void;
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
const MediaCard: React.FC<MediaCardProps> = ({
  media,
  onClick,
  statusBadge,
  isLoadingStatus = false,
  onImageLoad,
}) => {
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

  // Determine badge color based on download status
  const getBadgeColor = (): string => {
    if (!statusBadge) return 'rgba(0, 0, 0, 0.7)';

    if (statusBadge.type === 'movie' && statusBadge.inLibrary) {
      return theme.palette.success.dark; // Green for movies in library
    }

    if (statusBadge.type === 'show' && statusBadge.episodeCount) {
      const { downloaded, total } = statusBadge.episodeCount;
      if (downloaded === 0) {
        return theme.palette.grey[700]; // Gray for no episodes
      }
      if (downloaded === total) {
        return theme.palette.success.dark; // Green for complete
      }
      return theme.palette.warning.dark; // Yellow/amber for partial
    }

    return 'rgba(0, 0, 0, 0.7)';
  };

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
      <img
        src={media.imageUrl}
        alt={media.name}
        loading="eager"
        onLoad={onImageLoad}
        onError={onImageLoad}
        style={{
          width: "100%",
          height: "100%",
          objectFit: "cover",
          display: "block",
        }}
      />

      {/* Status Badge or Loading Shimmer */}
      {isLoadingStatus ? (
        <Box
          sx={{
            position: "absolute",
            top: 8,
            right: 8,
            width: 60,
            height: 24,
            borderRadius: 3,
            backgroundColor: "rgba(0, 0, 0, 0.5)",
            backdropFilter: "blur(4px)",
            zIndex: 1,
            overflow: "hidden",
            "&::after": {
              content: '""',
              position: "absolute",
              top: 0,
              left: "-100%",
              width: "100%",
              height: "100%",
              background:
                "linear-gradient(90deg, transparent, rgba(255, 255, 255, 0.2), transparent)",
              animation: "shimmer 1.5s infinite",
            },
            "@keyframes shimmer": {
              "0%": { left: "-100%" },
              "100%": { left: "100%" },
            },
          }}
          aria-label="Loading status"
        />
      ) : (
        badgeLabel && (
          <Chip
            label={badgeLabel}
            size="small"
            sx={{
              position: "absolute",
              top: 8,
              right: 8,
              backgroundColor: getBadgeColor(),
              color: "white",
              fontWeight: 600,
              fontSize: "0.75rem",
              backdropFilter: "blur(4px)",
              zIndex: 1,
            }}
          />
        )
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
