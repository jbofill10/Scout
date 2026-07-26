import React, { useState } from "react";
import MediaStatusDialog from "./ui/MediaStatusDialog";
import Box from "@mui/material/Box";
import { buildStatusBadgeFromEnriched } from "../utils/statusHelpers";
import { useDownloadRequest } from "../hooks/useDownloadRequest";
import MediaCard from "./ui/MediaCard";
import type { EnrichedMedia, ShowStatus } from "../types/MediaStatus";

export interface SearchResult {
  id: string;
  image_url: string;
  mediaName: string;
  metadata?: {
    episodes?: Array<{
      aired: string;
      id: number;
      image: string;
      name: string;
      number: number;
      seasonNumber: number;
    }>;
  };
}

interface SearchResultsListProps {
  results: EnrichedMedia[];
  mediaType: "series" | "movie";
}

const SearchResultsList: React.FC<SearchResultsListProps> = ({
  results,
  mediaType,
}) => {
  // State for MediaStatusDialog (both TV shows and movies)
  const [statusDialogOpen, setStatusDialogOpen] = useState(false);
  const [selectedShowStatus, setSelectedShowStatus] = useState<ShowStatus | null>(null);
  const [selectedShowInfo, setSelectedShowInfo] = useState<{
    name: string;
    posterUrl: string;
    tvdbId: string;
  } | null>(null);
  const [selectedEnrichedMedia, setSelectedEnrichedMedia] = useState<EnrichedMedia | null>(null);
  const { requestDownload, isPending: isDownloading } = useDownloadRequest();

  const handleCardClick = (enrichedMedia: EnrichedMedia) => {
    // Use MediaStatusDialog for both TV shows and movies
    setSelectedShowInfo({
      name: enrichedMedia.media.mediaName,
      posterUrl: enrichedMedia.media.image_url,
      tvdbId: enrichedMedia.media.id,
    });

    // Convert MediaStatusInfo to ShowStatus format
    const showStatus: ShowStatus | null = enrichedMedia.status.seasons
      ? {
          tvdbId: enrichedMedia.media.id,
          seasons: enrichedMedia.status.seasons,
        }
      : null;

    setSelectedShowStatus(showStatus);
    setSelectedEnrichedMedia(enrichedMedia);
    setStatusDialogOpen(true);
  };

  const handleCloseStatusDialog = () => {
    setStatusDialogOpen(false);
    setSelectedShowStatus(null);
    setSelectedShowInfo(null);
    setSelectedEnrichedMedia(null);
  };

  const handleDownload = () => {
    // Use selected enriched media for download
    if (!selectedEnrichedMedia) return;

    requestDownload(
      {
        media: selectedEnrichedMedia.media,
        mediaType,
        title: selectedEnrichedMedia.media.mediaName,
        component: "SearchResultsList",
      },
      { onSuccess: () => handleCloseStatusDialog() },
    );
  };

  return (
    <>
      <Box
        sx={{
          display: "grid",
          gridTemplateColumns: "repeat(auto-fill, minmax(180px, 1fr))",
          gap: 3,
          width: "100%",
          padding: 2,
        }}
      >
        {results.map((enrichedMedia) => {
          // Build status badge from enriched response (no async loading needed)
          const statusBadge = buildStatusBadgeFromEnriched(enrichedMedia.status);

          return (
            <MediaCard
              key={enrichedMedia.media.id}
              media={{
                id: enrichedMedia.media.id,
                name: enrichedMedia.media.mediaName,
                imageUrl: enrichedMedia.media.image_url,
                category: mediaType,
              }}
              onClick={() => handleCardClick(enrichedMedia)}
              statusBadge={statusBadge}
              isLoadingStatus={false}
            />
          );
        })}
      </Box>
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
        />
      )}
    </>
  );
};

export default SearchResultsList;
