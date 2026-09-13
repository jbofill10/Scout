import React, { useCallback, useState } from "react";
import MediaStatusDialog from "./ui/MediaStatusDialog";
import SearchResultsGrid from "./ui/SearchResultsGrid";
import { useDownloadRequest } from "../hooks/useDownloadRequest";
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

  // Stable so the memoised grid does not re-render when the dialog opens
  const handleCardClick = useCallback((enrichedMedia: EnrichedMedia) => {
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
  }, []);

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
      <SearchResultsGrid
        results={results}
        onSelect={handleCardClick}
        minCardWidth={180}
        cardCategory={mediaType}
        sx={{ width: "100%", padding: 2 }}
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
          mediaType={mediaType}
          inLibrary={selectedEnrichedMedia?.status.inLibrary}
        />
      )}
    </>
  );
};

export default SearchResultsList;
