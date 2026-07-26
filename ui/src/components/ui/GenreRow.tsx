import React, { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import Box from "@mui/material/Box";
import Typography from "@mui/material/Typography";
import Button from "@mui/material/Button";
import Skeleton from "@mui/material/Skeleton";
import { useTheme } from "@mui/material/styles";
import HorizontalCarousel from "./HorizontalCarousel";
import MediaCard, { MediaCardSkeleton } from "./MediaCard";
import MediaStatusDialog from "./MediaStatusDialog";
import type { SearchResult } from "../SearchResultsList";
import type { ShowStatus, EnrichedMedia } from "../../types/MediaStatus";
import { useMediaStatus } from "../../hooks/useMediaStatus";
import { useEnrichedPopular } from "../../hooks/useProgressiveEnrichment";
import { getStatusBadgeForMediaWithEnriched } from "../../utils/statusHelpers";
import { useDownloadRequest } from "../../hooks/useDownloadRequest";

interface GenreRowProps {
  genre: string;
  mediaType: "series" | "movie";
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
 * - Opens MediaStatusDialog for TV shows (shows episode breakdown)
 * - Opens SearchResultDialog for movies (triggers download)
 * - Integrates MediaCard, HorizontalCarousel, and status system
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

  // Fetch popular content for this genre
  const { data, isLoading, isError, refetch } = useQuery<SearchResult[]>({
    queryKey: ["popular", mediaType, genre],
    queryFn: async () => {
      const endpoint =
        mediaType === "series" ? "/api/popular/shows" : "/api/popular/movies";

      // Don't send genre parameter for "Popular TV Shows" or "Popular Movies" - these are UI labels, not real genres
      const isPopularOnly =
        genre === "Popular TV Shows" || genre === "Popular Movies";
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
  const statusRequests =
    data?.map((item) => ({
      tvdbId: item.id,
      mediaType: mediaType === "series" ? ("show" as const) : ("movie" as const),
    })) || [];

  // Fetch status data (only when we have media data)
  const { data: statusData } = useMediaStatus(statusRequests, !!data && data.length > 0);

  // Progressive enrichment: Fetch enriched data asynchronously (with episode metadata, extended info)
  // Determine if genre should be sent (skip for "Popular TV Shows" / "Popular Movies")
  const isPopularOnly =
    genre === "Popular TV Shows" || genre === "Popular Movies";
  const enrichedGenre = isPopularOnly ? "" : genre;

  const { data: enrichedData } = useEnrichedPopular(
    enrichedGenre,
    20,
    mediaType,
    !!data && data.length > 0  // Only fetch after initial data loads
  );

  const handleMediaClick = (media: {
    id: string;
    name: string;
    imageUrl: string;
  }) => {
    // Use MediaStatusDialog for both TV shows and movies
    const status = statusData?.shows?.find((s) => s.tvdbId === media.id) ||
                   statusData?.movies?.find((m) => m.tvdbId === media.id);
    // Find enriched media data if available
    const enriched = enrichedData?.find((e) => e.media.id === media.id);

    setSelectedShowInfo({
      name: media.name,
      posterUrl: media.imageUrl,
      tvdbId: media.id,
    });

    // Convert status to ShowStatus format for series, null for movies
    const showStatus: ShowStatus | null = mediaType === "series" && status && "seasons" in status
      ? { tvdbId: media.id, seasons: status.seasons }
      : null;

    setSelectedShowStatus(showStatus);
    setSelectedEnrichedMedia(enriched || null);
    setStatusDialogOpen(true);
  };

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
  if (!data || data.length === 0) {
    return null;
  }

  return (
    <>
      <HorizontalCarousel
        title={genre}
        items={data}
        renderItem={(item) => (
          <MediaCard
            media={{
              id: item.id,
              name: item.mediaName,
              imageUrl: item.image_url,
            }}
            onClick={handleMediaClick}
            statusBadge={getStatusBadgeForMediaWithEnriched(
              item.id,
              statusData,
              mediaType === "series" ? "show" : "movie",
              enrichedData?.find((e) => e.media.id === item.id)
            )}
          />
        )}
      />
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
