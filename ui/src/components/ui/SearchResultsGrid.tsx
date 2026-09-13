import React, { memo, useCallback, useMemo } from "react";
import Box from "@mui/material/Box";
import type { SxProps, Theme } from "@mui/material/styles";
import MediaCard from "./MediaCard";
import type { MediaCardProps } from "./MediaCard";
import type { EnrichedMedia } from "../../types/MediaStatus";
import { buildStatusBadgeFromEnriched } from "../../utils/statusHelpers";

interface SearchResultsGridProps {
  results: EnrichedMedia[];
  onSelect: (result: EnrichedMedia) => void;
  /** Minimum poster width; the grid auto-fills columns from it. */
  minCardWidth?: number;
  /** Optional label shown under the title in the hover overlay. */
  cardCategory?: string;
  sx?: SxProps<Theme>;
}

/**
 * SearchResultsGrid
 *
 * The poster grid shared by the search overlay and the /search page.
 *
 * Card props are derived once per results array and the click handler is
 * stable, so MediaCard's memo holds: typing in the search box or opening a
 * dialog re-renders the parent without touching the grid.
 */
const SearchResultsGrid: React.FC<SearchResultsGridProps> = ({
  results,
  onSelect,
  minCardWidth = 190,
  cardCategory,
  sx,
}) => {
  const cards = useMemo(
    () =>
      results.map((result) => ({
        result,
        media: {
          id: result.media.id,
          name: result.media.mediaName,
          imageUrl: result.media.image_url,
          category: cardCategory,
        },
        statusBadge: buildStatusBadgeFromEnriched(result.status),
      })),
    [results, cardCategory],
  );

  const resultsById = useMemo(
    () => new Map(cards.map((card) => [card.media.id, card.result])),
    [cards],
  );

  const handleCardClick = useCallback(
    (media: MediaCardProps["media"]) => {
      const result = resultsById.get(media.id);
      if (result) {
        onSelect(result);
      }
    },
    [resultsById, onSelect],
  );

  return (
    <Box
      sx={[
        {
          display: "grid",
          gridTemplateColumns: `repeat(auto-fill, minmax(${minCardWidth}px, 1fr))`,
          gap: 3,
        },
        ...(Array.isArray(sx) ? sx : [sx]),
      ]}
    >
      {cards.map((card) => (
        <MediaCard
          key={card.media.id}
          media={card.media}
          statusBadge={card.statusBadge}
          onClick={handleCardClick}
          isLoadingStatus={false}
        />
      ))}
    </Box>
  );
};

export default memo(SearchResultsGrid);
