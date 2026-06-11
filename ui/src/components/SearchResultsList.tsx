import React, { useState } from "react";
import MediaStatusDialog from "./ui/MediaStatusDialog";
import Box from "@mui/material/Box";
import Snackbar from "@mui/material/Snackbar";
import Alert from "@mui/material/Alert";
import { logError } from "../lib/logger";
import { buildStatusBadgeFromEnriched } from "../utils/statusHelpers";
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
  const [isDownloading, setIsDownloading] = useState(false);
  const [snackbar, setSnackbar] = useState<{
    open: boolean;
    message: string;
    severity: "success" | "error";
  }>({ open: false, message: "", severity: "success" });

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

    setIsDownloading(true);

    // Route to correct endpoint based on media type
    const endpoint = mediaType === "movie" ? "/api/movies" : "/api/shows";

    fetch(endpoint, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(selectedEnrichedMedia.media),
    })
      .then((response) => {
        if (!response.ok) {
          throw new Error("Network response was not ok");
        }
        return response.json();
      })
      .then(() => {
        const name = selectedEnrichedMedia.media.mediaName;
        handleCloseStatusDialog();
        setSnackbar({
          open: true,
          message: `Download requested for "${name}"`,
          severity: "success",
        });
      })
      .catch((error) => {
        console.error("Error initiating download:", error);
        logError(
          "Download initiation failed in SearchResultsList",
          error as Error,
          {
            component: "SearchResultsList",
            endpoint: endpoint,
            mediaType: mediaType,
            resultId: selectedEnrichedMedia.media.id,
          },
        );
        setSnackbar({
          open: true,
          message: "Failed to request download. Please try again.",
          severity: "error",
        });
      })
      .finally(() => {
        setIsDownloading(false);
      });
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
      <Snackbar
        open={snackbar.open}
        autoHideDuration={4000}
        onClose={() => setSnackbar((s) => ({ ...s, open: false }))}
        anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
      >
        <Alert
          onClose={() => setSnackbar((s) => ({ ...s, open: false }))}
          severity={snackbar.severity}
          variant="filled"
          sx={{ width: "100%" }}
        >
          {snackbar.message}
        </Alert>
      </Snackbar>
    </>
  );
};

export default SearchResultsList;
