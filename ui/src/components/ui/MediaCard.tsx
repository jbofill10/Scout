import React, { useEffect, useRef, useState } from "react";
import Card from "@mui/material/Card";
import Box from "@mui/material/Box";
import Typography from "@mui/material/Typography";
import Skeleton from "@mui/material/Skeleton";
import AddRoundedIcon from "@mui/icons-material/AddRounded";
import { alpha, useTheme } from "@mui/material/styles";
import type { Theme } from "@mui/material/styles";
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
}

/** Tinted pill colors for a status badge — soft fill, bright text, hairline edge. */
interface BadgeTone {
  fill: string;
  text: string;
  border: string;
}

const toneFrom = (color: string): BadgeTone => ({
  fill: alpha(color, 0.22),
  text: color,
  border: alpha(color, 0.45),
});

const neutralTone = (theme: Theme): BadgeTone => ({
  fill: alpha(theme.palette.common.black, 0.55),
  text: theme.palette.text.primary,
  border: alpha(theme.palette.common.white, 0.18),
});

/**
 * MediaCard Component
 *
 * Poster-first media card. The artwork stays unobstructed at rest; hovering
 * raises the card, rings it in the brand color, and lifts a gradient scrim
 * carrying the title and an add-to-library affordance.
 *
 * Features:
 * - 2:3 aspect ratio poster with lazy loading and a fade-in once decoded
 * - Optional status badge (episode progress for shows, library state for movies)
 * - Keyboard accessible (Enter / Space)
 */
const MediaCard: React.FC<MediaCardProps> = ({
  media,
  onClick,
  statusBadge,
  isLoadingStatus = false,
}) => {
  const theme = useTheme();
  const [isHovered, setIsHovered] = useState(false);
  const [imageLoaded, setImageLoaded] = useState(false);
  const imageRef = useRef<HTMLImageElement>(null);

  // A cached poster can finish loading before React attaches onLoad, which would
  // otherwise leave the image stuck at opacity 0.
  useEffect(() => {
    setImageLoaded(imageRef.current?.complete ?? false);
  }, [media.imageUrl]);

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

  // Badge tone tracks how complete the media is in the library
  const getBadgeTone = (): BadgeTone => {
    if (!statusBadge) return neutralTone(theme);

    if (statusBadge.type === 'movie' && statusBadge.inLibrary) {
      return toneFrom(theme.palette.success.main);
    }

    if (statusBadge.type === 'show' && statusBadge.episodeCount) {
      const { downloaded, total } = statusBadge.episodeCount;
      if (downloaded === 0) {
        return neutralTone(theme);
      }
      if (downloaded === total) {
        return toneFrom(theme.palette.success.main);
      }
      return toneFrom(theme.palette.warning.main);
    }

    return neutralTone(theme);
  };

  const badgeTone = getBadgeTone();

  const handleClick = () => {
    onClick(media);
  };

  const handleKeyDown = (event: React.KeyboardEvent) => {
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      onClick(media);
    }
  };

  return (
    <Card
      onMouseEnter={() => setIsHovered(true)}
      onMouseLeave={() => setIsHovered(false)}
      onClick={handleClick}
      onKeyDown={handleKeyDown}
      tabIndex={0}
      role="button"
      aria-label={`View details for ${media.name}`}
      elevation={isHovered ? 10 : 1}
      sx={{
        position: "relative",
        width: "100%",
        aspectRatio: "2/3",
        cursor: "pointer",
        borderRadius: 3,
        overflow: "hidden",
        backgroundColor: "rgba(148, 163, 184, 0.06)",
        border: `1px solid ${isHovered ? alpha(theme.palette.primary.light, 0.55) : theme.palette.divider}`,
        transform: isHovered ? "translateY(-6px) scale(1.03)" : "none",
        transition: theme.transitions.create(
          ["transform", "box-shadow", "border-color"],
          { duration: 260, easing: "cubic-bezier(0.22, 1, 0.36, 1)" },
        ),
      }}
    >
      <Box
        component="img"
        ref={imageRef}
        src={media.imageUrl}
        alt={media.name}
        loading="lazy"
        onLoad={() => setImageLoaded(true)}
        sx={{
          width: "100%",
          height: "100%",
          objectFit: "cover",
          display: "block",
          opacity: imageLoaded ? 1 : 0,
          transition: "opacity .4s ease",
        }}
      />

      {/* Resting scrim — anchors the poster to the page without hiding artwork */}
      <Box
        aria-hidden
        sx={{
          position: "absolute",
          inset: 0,
          background:
            "linear-gradient(180deg, rgba(2,6,23,0.35) 0%, rgba(2,6,23,0) 32%, rgba(2,6,23,0) 55%, rgba(2,6,23,0.45) 100%)",
          pointerEvents: "none",
        }}
      />

      {/* Status Badge or Loading Shimmer */}
      {isLoadingStatus ? (
        <Skeleton
          variant="rounded"
          width={54}
          height={22}
          sx={{ position: "absolute", top: 10, right: 10, borderRadius: 999, zIndex: 1 }}
          aria-label="Loading status"
        />
      ) : (
        badgeLabel && (
          <Box
            sx={{
              position: "absolute",
              top: 10,
              right: 10,
              zIndex: 1,
              px: 1,
              py: "3px",
              borderRadius: 999,
              fontSize: "0.6875rem",
              fontWeight: 700,
              letterSpacing: "0.01em",
              lineHeight: 1.3,
              color: badgeTone.text,
              backgroundColor: badgeTone.fill,
              border: `1px solid ${badgeTone.border}`,
              backdropFilter: "blur(8px)",
            }}
          >
            {badgeLabel}
          </Box>
        )
      )}

      {/* Hover Overlay */}
      <Box
        sx={{
          position: "absolute",
          inset: 0,
          display: "flex",
          flexDirection: "column",
          alignItems: "center",
          justifyContent: "flex-end",
          textAlign: "center",
          p: 2,
          background:
            "linear-gradient(180deg, rgba(2,6,23,0.15) 0%, rgba(2,6,23,0.72) 55%, rgba(2,6,23,0.94) 100%)",
          opacity: isHovered ? 1 : 0,
          transition: "opacity .26s ease",
          pointerEvents: "none",
        }}
      >
        <Box
          sx={{
            width: 44,
            height: 44,
            mb: 1.5,
            borderRadius: "50%",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            color: "#fff",
            backgroundColor: alpha(theme.palette.primary.main, 0.92),
            boxShadow: `0 6px 18px ${alpha(theme.palette.primary.main, 0.45)}`,
            transform: isHovered ? "translateY(0)" : "translateY(8px)",
            transition: "transform .3s cubic-bezier(0.22, 1, 0.36, 1)",
          }}
        >
          <AddRoundedIcon />
        </Box>
        <Typography
          variant="subtitle1"
          sx={{
            fontWeight: 650,
            lineHeight: 1.3,
            color: theme.palette.text.primary,
            overflow: "hidden",
            display: "-webkit-box",
            WebkitLineClamp: 3,
            WebkitBoxOrient: "vertical",
          }}
        >
          {media.name}
        </Typography>
        {media.category && (
          <Typography variant="overline" sx={{ color: theme.palette.text.secondary, mt: 0.5 }}>
            {media.category}
          </Typography>
        )}
      </Box>
    </Card>
  );
};

/**
 * MediaCardSkeleton
 *
 * Placeholder matching MediaCard's footprint so loading rows do not shift
 * layout once the real posters arrive.
 */
export const MediaCardSkeleton: React.FC = () => (
  <Skeleton
    variant="rectangular"
    sx={{ width: "100%", aspectRatio: "2/3", height: "auto", borderRadius: 3 }}
  />
);

export default MediaCard;
