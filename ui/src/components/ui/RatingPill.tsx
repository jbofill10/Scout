import React from "react";
import Box from "@mui/material/Box";
import StarRoundedIcon from "@mui/icons-material/StarRounded";
import { alpha } from "@mui/material/styles";
import type { SxProps, Theme } from "@mui/material/styles";
import type { RatingBadgeModel } from "../../utils/ratings";

/** Brand colours the scores are recognised by. */
const IMDB_YELLOW = "#F5C518";
const TOMATO_RED = "#FA320A";
const SPLAT_GREEN = "#0AC855";

/** Metacritic colours its score green, yellow or red by band. */
const metascoreColor = (label: string): string => {
  const value = Number(label);
  if (value >= 61) return "#66CC33";
  if (value >= 40) return "#FFCC33";
  return "#FF0000";
};

/** A ripe tomato (fresh) or a green splat (rotten), drawn to sit on a text line. */
const TomatoIcon: React.FC<{ fresh: boolean; size: number }> = ({ fresh, size }) =>
  fresh ? (
    <svg width={size} height={size} viewBox="0 0 16 16" aria-hidden focusable="false">
      <circle cx="8" cy="9" r="6.5" fill={TOMATO_RED} />
      <path d="M8 2.6c-1.1-.9-2.4-1-3.4-.6 1 .2 1.7.8 2.2 1.6-1.3-.2-2.4.2-3.2 1 1.2-.2 2.4.1 3.4.9h2c1-.8 2.2-1.1 3.4-.9-.8-.8-1.9-1.2-3.2-1 .5-.8 1.2-1.4 2.2-1.6-1-.4-2.3-.3-3.4.6z" fill="#3E9B2E" />
    </svg>
  ) : (
    <svg width={size} height={size} viewBox="0 0 16 16" aria-hidden focusable="false">
      <path d="M8 1.5l1.6 2.9 3.1-1.1-.7 3.2 3 1.5-2.6 2 1.3 3-3.2-.4L8 15l-2.5-2.4-3.2.4 1.3-3-2.6-2 3-1.5-.7-3.2 3.1 1.1z" fill={SPLAT_GREEN} />
    </svg>
  );

export interface RatingPillProps {
  rating: RatingBadgeModel;
  /** "card" sits on a poster corner; "detail" sits in the dialog's facts strip. */
  size?: "card" | "detail";
  sx?: SxProps<Theme>;
}

/**
 * RatingPill
 *
 * One score with the glyph its source is known by: a tomato or splat for the
 * Tomatometer, a yellow star for IMDb, a coloured square for the Metascore.
 * The pill sits on a dark base so it stays legible over bright artwork.
 */
const RatingPill: React.FC<RatingPillProps> = ({ rating, size = "card", sx }) => {
  const iconSize = size === "card" ? 12 : 14;
  let icon: React.ReactNode;
  switch (rating.source) {
    case "rotten_tomatoes":
      icon = <TomatoIcon fresh={rating.fresh ?? true} size={iconSize} />;
      break;
    case "imdb":
      icon = <StarRoundedIcon sx={{ fontSize: iconSize + 3, color: IMDB_YELLOW, ml: "-2px" }} />;
      break;
    case "metacritic":
      icon = (
        <Box
          component="span"
          aria-hidden
          sx={{
            width: iconSize - 2,
            height: iconSize - 2,
            borderRadius: "2px",
            backgroundColor: metascoreColor(rating.label),
            flexShrink: 0,
          }}
        />
      );
      break;
  }

  return (
    <Box
      component="span"
      role="img"
      aria-label={rating.title}
      title={rating.title}
      sx={[
        {
          display: "inline-flex",
          alignItems: "center",
          gap: "4px",
          px: size === "card" ? { xs: "6px", sm: "8px" } : "9px",
          py: size === "card" ? "3px" : "4px",
          borderRadius: 999,
          fontSize: size === "card" ? { xs: "0.625rem", sm: "0.6875rem" } : "0.75rem",
          fontWeight: 700,
          letterSpacing: "0.01em",
          lineHeight: 1.3,
          fontVariantNumeric: "tabular-nums",
          color: "#F8FAFC",
          backgroundColor: alpha("#020617", 0.78),
          border: `1px solid ${alpha("#FFFFFF", 0.18)}`,
          whiteSpace: "nowrap",
        },
        ...(Array.isArray(sx) ? sx : [sx]),
      ]}
    >
      {icon}
      {rating.label}
    </Box>
  );
};

export default RatingPill;
