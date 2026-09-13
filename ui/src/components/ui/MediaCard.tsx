import React, { memo, useEffect, useRef, useState } from "react";
import Card from "@mui/material/Card";
import Box from "@mui/material/Box";
import Typography from "@mui/material/Typography";
import Skeleton from "@mui/material/Skeleton";
import AddRoundedIcon from "@mui/icons-material/AddRounded";
import { alpha, useTheme } from "@mui/material/styles";
import type { Theme } from "@mui/material/styles";
import type { MediaStatusBadge } from "../../types/MediaStatus";
import { toThumbnailUrl } from "../../utils/images";

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
 * Pill colors for a status badge. The pill sits on a dark base so it stays
 * legible over bright artwork without a backdrop blur, which is expensive
 * when a page shows a hundred posters at once.
 */
interface BadgeTone {
  tint: string | null;
  text: string;
  border: string;
}

const toneFrom = (color: string): BadgeTone => ({
  tint: alpha(color, 0.3),
  text: color,
  border: alpha(color, 0.45),
});

const neutralTone = (theme: Theme): BadgeTone => ({
  tint: null,
  text: theme.palette.text.primary,
  border: alpha(theme.palette.common.white, 0.18),
});

const HOVER_EASING = "cubic-bezier(0.22, 1, 0.36, 1)";

/**
 * MediaCard Component
 *
 * Poster-first media card. The artwork stays unobstructed at rest; hovering
 * raises the card, rings it in the brand color, and lifts a gradient scrim
 * carrying the title and an add-to-library affordance.
 *
 * Hover styling is pure CSS and the component is memoised, so a page of
 * posters does not re-render as the pointer moves across it or when a parent
 * re-renders for unrelated state (typing in the search box, a dialog opening).
 *
 * Features:
 * - 2:3 aspect ratio poster, served as a TVDB thumbnail with a fade-in once decoded
 * - Optional status badge (episode progress for shows, library state for movies)
 * - Keyboard accessible (Enter / Space, hover treatment on focus)
 */
const MediaCard: React.FC<MediaCardProps> = ({
  media,
  onClick,
  statusBadge,
  isLoadingStatus = false,
  onImageLoad,
}) => {
  const theme = useTheme();
  const [imageLoaded, setImageLoaded] = useState(false);
  // The thumbnail URL that 404'd, if any, so we can fall back to the full poster
  const [failedThumbnail, setFailedThumbnail] = useState<string | null>(null);
  const imageRef = useRef<HTMLImageElement>(null);

  const thumbnailUrl = toThumbnailUrl(media.imageUrl);
  const src = failedThumbnail === thumbnailUrl ? media.imageUrl : thumbnailUrl;

  // A cached poster can finish loading before React attaches onLoad, which would
  // otherwise leave the image stuck at opacity 0.
  useEffect(() => {
    setImageLoaded(imageRef.current?.complete ?? false);
  }, [src]);

  const handleImageError = () => {
    if (src === thumbnailUrl && thumbnailUrl !== media.imageUrl) {
      setFailedThumbnail(thumbnailUrl);
      return;
    }
    onImageLoad?.();
  };

  // Determine badge label based on status type
  const getBadgeLabel = (): string | null => {
    if (!statusBadge) return null;

    if (statusBadge.type === "movie" && statusBadge.inLibrary) {
      return "In Library";
    }

    if (statusBadge.type === "show" && statusBadge.episodeCount) {
      const { downloaded, total } = statusBadge.episodeCount;
      return `${downloaded}/${total}`;
    }

    return null;
  };

  const badgeLabel = getBadgeLabel();

  // Badge tone tracks how complete the media is in the library
  const getBadgeTone = (): BadgeTone => {
    if (!statusBadge) return neutralTone(theme);

    if (statusBadge.type === "movie" && statusBadge.inLibrary) {
      return toneFrom(theme.palette.success.main);
    }

    if (statusBadge.type === "show" && statusBadge.episodeCount) {
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
      onClick={handleClick}
      onKeyDown={handleKeyDown}
      tabIndex={0}
      role="button"
      aria-label={`View details for ${media.name}`}
      elevation={1}
      sx={{
        position: "relative",
        width: "100%",
        aspectRatio: "2/3",
        cursor: "pointer",
        borderRadius: 3,
        overflow: "hidden",
        backgroundColor: "rgba(148, 163, 184, 0.06)",
        border: `1px solid ${theme.palette.divider}`,
        transition: theme.transitions.create(["transform", "box-shadow", "border-color"], {
          duration: 260,
          easing: HOVER_EASING,
        }),
        "&:hover, &:focus-visible": {
          borderColor: alpha(theme.palette.primary.light, 0.55),
          transform: "translateY(-6px) scale(1.03)",
          boxShadow: theme.shadows[10],
        },
        "&:hover .media-card-overlay, &:focus-visible .media-card-overlay": {
          opacity: 1,
        },
        "&:hover .media-card-action, &:focus-visible .media-card-action": {
          transform: "translateY(0)",
        },
      }}
    >
      <Box
        component="img"
        ref={imageRef}
        src={src}
        alt={media.name}
        loading="lazy"
        decoding="async"
        onLoad={() => {
          setImageLoaded(true);
          onImageLoad?.();
        }}
        onError={handleImageError}
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
              backgroundColor: alpha("#020617", 0.78),
              backgroundImage: badgeTone.tint
                ? `linear-gradient(${badgeTone.tint}, ${badgeTone.tint})`
                : "none",
              border: `1px solid ${badgeTone.border}`,
            }}
          >
            {badgeLabel}
          </Box>
        )
      )}

      {/* Hover Overlay */}
      <Box
        className="media-card-overlay"
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
          opacity: 0,
          transition: "opacity .26s ease",
          pointerEvents: "none",
        }}
      >
        <Box
          className="media-card-action"
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
            transform: "translateY(8px)",
            transition: `transform .3s ${HOVER_EASING}`,
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

export default memo(MediaCard);
