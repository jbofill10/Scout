import React, { useCallback, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import Box from "@mui/material/Box";
import Typography from "@mui/material/Typography";
import Button from "@mui/material/Button";
import Skeleton from "@mui/material/Skeleton";
import { useTheme } from "@mui/material/styles";
import HorizontalCarousel from "./HorizontalCarousel";
import MediaCard, { MediaCardSkeleton } from "./MediaCard";
import type { MediaCardProps } from "./MediaCard";
import MediaStatusDialog from "./MediaStatusDialog";
import type { SearchResult } from "../SearchResultsList";
import type { ShowStatus, EnrichedMedia, MediaStatusBadge } from "../../types/MediaStatus";
import { useMediaStatus } from "../../hooks/useMediaStatus";
import { useEnrichedPopular } from "../../hooks/useProgressiveEnrichment";
import { getStatusBadgeForMediaWithEnriched } from "../../utils/statusHelpers";
import { useDownloadRequest } from "../../hooks/useDownloadRequest";

interface GenreRowProps {
  genre: string;
  mediaType: "series" | "movie";
}

/** Everything a poster card needs, computed once per data change. */
interface CardModel {
  media: MediaCardProps["media"];
  statusBadge?: MediaStatusBadge;
}

/**
 * MediaRowSkeleton
 *
 * Loading placeholder for a carousel row. Mirrors the carousel's item sizing so
 * the layout does not jump when real posters arrive.
 */
export const MediaRowSkeleton: React.FC<{ title?: string }> = ({ title }) => (
  <Box sx={{ mb: 5 }}>
    {title ? (
      <Typography variant="h5" sx={{ mb: 1.75 }}>
        {title}
      </Typography>
    ) : (
      <Skeleton variant="text" width={180} sx={{ mb: 1.75, fontSize: "1.125rem" }} />
    )}
    <Box sx={{ display: "flex", gap: 2, overflow: "hidden" }}>
      {Array.from({ length: 7 }).map((_, index) => (
        <Box
          key={index}
          sx={{ flex: "0 0 calc((100% - 96px) / 7)", minWidth: 150, maxWidth: 230 }}
        >
          <MediaCardSkeleton />
        </Box>
      ))}
    </Box>
  </Box>
);

/**
 * GenreRow Component
 *
 * Displays a horizontal carousel of popular media for a specific genre.
 * Features:
 * - Fetches popular content from API based on genre and media type
 * - Fetches download status for all displayed media items
 * - Displays status badges on media cards (episode counts for shows, library status for movies)
 * - Shows loading skeletons during fetch
 * - Error handling with retry button
 * - Opens MediaStatusDialog for both shows and movies
 * - Integrates MediaCard, HorizontalCarousel, and status system
 *
 * Card props and handlers are memoised so opening a dialog or a query
 * resolving re-renders only what changed, not every poster in the row.
 */
const GenreRow: React.FC<GenreRowProps> = ({ genre, mediaType }) => {
  const theme = useTheme();

  // State for MediaStatusDialog (both shows and movies)
  const [statusDialogOpen, setStatusDialogOpen] = useState(false);
  const [selectedShowStatus, setSelectedShowStatus] = useState<ShowStatus | null>(null);
  const [selectedShowInfo, setSelectedShowInfo] = useState<{
    name: string;
    posterUrl: string;
    tvdbId: string;
  } | null>(null);
  const [selectedEnrichedMedia, setSelectedEnrichedMedia] = useState<EnrichedMedia | null>(null);
  const { requestDownload, isPending: isDownloading } = useDownloadRequest();

  // "Popular TV Shows" / "Popular Movies" are UI labels, not real genres
  const isPopularOnly = genre === "Popular TV Shows" || genre === "Popular Movies";
  const statusMediaType = mediaType === "series" ? ("show" as const) : ("movie" as const);

  // Fetch popular content for this genre
  const { data, isLoading, isError, refetch } = useQuery<SearchResult[]>({
    queryKey: ["popular", mediaType, genre],
    queryFn: async () => {
      const endpoint =
        mediaType === "series" ? "/api/popular/shows" : "/api/popular/movies";
      const params = new URLSearchParams(isPopularOnly ? {} : { genre });

      const response = await fetch(`${endpoint}?${params}`);
      if (!response.ok) {
        throw new Error("Failed to fetch popular content");
      }
      return response.json();
    },
    staleTime: 5 * 60 * 1000, // 5 minutes
    retry: 2,
  });

  // Build status requests from media data
  const statusRequests = useMemo(
    () => data?.map((item) => ({ tvdbId: item.id, mediaType: statusMediaType })) ?? [],
    [data, statusMediaType],
  );

  // Fetch status data (only when we have media data)
  const { data: statusData } = useMediaStatus(statusRequests, !!data && data.length > 0);

  // Progressive enrichment: Fetch enriched data asynchronously (with episode metadata, extended info)
  const { data: enrichedData } = useEnrichedPopular(
    isPopularOnly ? "" : genre,
    20,
    mediaType,
    !!data && data.length > 0  // Only fetch after initial data loads
  );

  const enrichedById = useMemo(
    () => new Map((enrichedData ?? []).map((entry) => [entry.media.id, entry])),
    [enrichedData],
  );

  const cards = useMemo<CardModel[]>(
    () =>
      (data ?? []).map((item) => ({
        media: {
          id: item.id,
          name: item.mediaName,
          imageUrl: item.image_url,
        },
        statusBadge: getStatusBadgeForMediaWithEnriched(
          item.id,
          statusData,
          statusMediaType,
          enrichedById.get(item.id),
        ),
      })),
    [data, statusData, statusMediaType, enrichedById],
  );

  const handleMediaClick = useCallback(
    (media: MediaCardProps["media"]) => {
      // Use MediaStatusDialog for both TV shows and movies
      const status =
        statusData?.shows?.find((s) => s.tvdbId === media.id) ||
        statusData?.movies?.find((m) => m.tvdbId === media.id);
      // Find enriched media data if available
      const enriched = enrichedById.get(media.id);

      setSelectedShowInfo({
        name: media.name,
        posterUrl: media.imageUrl,
        tvdbId: media.id,
      });

      // Convert status to ShowStatus format for series, null for movies
      const showStatus: ShowStatus | null =
        mediaType === "series" && status && "seasons" in status
          ? { tvdbId: media.id, seasons: status.seasons }
          : null;

      setSelectedShowStatus(showStatus);
      setSelectedEnrichedMedia(enriched ?? null);
      setStatusDialogOpen(true);
    },
    [statusData, enrichedById, mediaType],
  );

  const renderCard = useCallback(
    (card: CardModel) => (
      <MediaCard media={card.media} onClick={handleMediaClick} statusBadge={card.statusBadge} />
    ),
    [handleMediaClick],
  );

  const handleCloseStatusDialog = () => {
    setStatusDialogOpen(false);
    setSelectedShowStatus(null);
    setSelectedShowInfo(null);
    setSelectedEnrichedMedia(null);
  };

  const handleDownload = () => {
    if (!selectedShowInfo) return;

    // Enriched media carries the episode list; fall back to the bare identity
    // when enrichment has not landed yet.
    const media = selectedEnrichedMedia?.media ?? {
      id: selectedShowInfo.tvdbId,
      mediaName: selectedShowInfo.name,
      image_url: selectedShowInfo.posterUrl,
    };

    requestDownload(
      {
        media,
        mediaType,
        title: selectedShowInfo.name,
        component: `GenreRow(${genre})`,
      },
      { onSuccess: () => handleCloseStatusDialog() },
    );
  };

  // Loading state with skeletons
  if (isLoading) {
    return <MediaRowSkeleton title={genre} />;
  }

  // Error state with retry button
  if (isError) {
    return (
      <Box sx={{ mb: 5 }}>
        <Typography variant="h5" sx={{ mb: 1.75, color: theme.palette.text.primary }}>
          {genre}
        </Typography>
        <Box
          sx={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            gap: 2,
            px: 3,
            py: 2.5,
            backgroundColor: theme.palette.background.paper,
            border: `1px solid ${theme.palette.divider}`,
            borderRadius: 3,
          }}
        >
          <Typography variant="body2" sx={{ color: theme.palette.text.secondary }}>
            Couldn't load {genre}.
          </Typography>
          <Button variant="outlined" color="primary" size="small" onClick={() => refetch()}>
            Retry
          </Button>
        </Box>
      </Box>
    );
  }

  // No data state
  if (cards.length === 0) {
    return null;
  }

  return (
    <>
      <HorizontalCarousel title={genre} items={cards} renderItem={renderCard} />
      {selectedShowInfo && (
        <MediaStatusDialog
          open={statusDialogOpen}
          onClose={handleCloseStatusDialog}
          status={selectedShowStatus}
          mediaName={selectedShowInfo.name}
          posterUrl={selectedShowInfo.posterUrl}
          enrichedMedia={selectedEnrichedMedia}
          onDownload={handleDownload}
          showDownloadButton={true}
          isDownloading={isDownloading}
          mediaType={mediaType === "movie" ? "movie" : "series"}
          inLibrary={
            statusData?.movies?.find((m) => m.tvdbId === selectedShowInfo.tvdbId)?.inLibrary
          }
        />
      )}
    </>
  );
};

export default GenreRow;
