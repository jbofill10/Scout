import React, { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import Box from "@mui/material/Box";
import Paper from "@mui/material/Paper";
import Card from "@mui/material/Card";
import CardMedia from "@mui/material/CardMedia";
import Typography from "@mui/material/Typography";
import Skeleton from "@mui/material/Skeleton";
import Button from "@mui/material/Button";
import { useTheme } from "@mui/material/styles";
import MediaStatusDialog from "./MediaStatusDialog";
import type { EnrichedMedia, ShowStatus } from "../../types/MediaStatus";
import { logError } from "../../lib/logger";

interface ScheduledItem {
  id: string;
  mediaName: string;
  posterUrl: string;
  seasonNumber?: number;
  episodeNumber?: number;
  releaseTime: string;
  mediaType: "series" | "movie";
}

/**
 * ScheduleWidget Component
 *
 * Displays a horizontal timeline of scheduled downloads for the upcoming week.
 * Features:
 * - Fetches weekly schedule from API
 * - Refreshes every 5 minutes
 * - Compact cards with poster thumbnails
 * - Shows episode format (S##E##) for shows
 * - Displays release dates
 * - Horizontal scroll for >6 items
 * - Empty state for no scheduled items
 * - Click to view details in SearchResultDialog
 */
const ScheduleWidget: React.FC = () => {
  const theme = useTheme();
  const [statusDialogOpen, setStatusDialogOpen] = useState(false);
  const [selectedShowStatus, setSelectedShowStatus] = useState<ShowStatus | null>(null);
  const [selectedShowInfo, setSelectedShowInfo] = useState<{
    name: string;
    posterUrl: string;
    tvdbId: string;
  } | null>(null);
  const [selectedEnrichedMedia, setSelectedEnrichedMedia] = useState<EnrichedMedia | null>(null);
  const [selectedMediaType, setSelectedMediaType] = useState<
    "series" | "movie"
  >("series");

  // Fetch weekly schedule with auto-refresh
  const { data, isLoading, isError, refetch } = useQuery<ScheduledItem[]>({
    queryKey: ["schedule", "weekly"],
    queryFn: async () => {
      const response = await fetch("/api/schedule/weekly");
      if (!response.ok) {
        throw new Error("Failed to fetch schedule");
      }
      return response.json();
    },
    staleTime: 5 * 60 * 1000, // 5 minutes
    refetchInterval: 5 * 60 * 1000, // Refresh every 5 minutes
    retry: 2,
  });

  const handleItemClick = async (item: ScheduledItem) => {
    setSelectedMediaType(item.mediaType);

    // Fetch enriched media details
    try {
      const params = new URLSearchParams({
        query: item.mediaName,
        media_type: item.mediaType,
      });
      const res = await fetch(`/api/search/enriched?${params}`);
      const results: EnrichedMedia[] = await res.json();

      // Find the matching result by ID
      const enrichedResult = results.find((r) => r.media.id === item.id);
      if (enrichedResult) {
        setSelectedShowInfo({
          name: enrichedResult.media.mediaName,
          posterUrl: enrichedResult.media.image_url,
          tvdbId: enrichedResult.media.id,
        });

        const showStatus: ShowStatus | null = enrichedResult.status.seasons
          ? {
              tvdbId: enrichedResult.media.id,
              seasons: enrichedResult.status.seasons,
            }
          : null;

        setSelectedShowStatus(showStatus);
        setSelectedEnrichedMedia(enrichedResult);
        setStatusDialogOpen(true);
      } else {
        // Fallback: create minimal enriched media
        setSelectedShowInfo({
          name: item.mediaName,
          posterUrl: item.posterUrl,
          tvdbId: item.id,
        });
        setSelectedShowStatus(null);
        setSelectedEnrichedMedia(null);
        setStatusDialogOpen(true);
      }
    } catch (error) {
      console.error("Failed to fetch media details:", error);
      // Fallback to basic info on error
      setSelectedShowInfo({
        name: item.mediaName,
        posterUrl: item.posterUrl,
        tvdbId: item.id,
      });
      setSelectedShowStatus(null);
      setSelectedEnrichedMedia(null);
      setStatusDialogOpen(true);
    }
  };

  const handleCloseStatusDialog = () => {
    setStatusDialogOpen(false);
    setSelectedShowStatus(null);
    setSelectedShowInfo(null);
    setSelectedEnrichedMedia(null);
  };

  const handleDownload = () => {
    if (!selectedEnrichedMedia && !selectedShowInfo) return;

    const endpoint =
      selectedMediaType === "movie" ? "/api/movies" : "/api/shows";

    const mediaData = selectedEnrichedMedia
      ? selectedEnrichedMedia.media
      : {
          id: selectedShowInfo!.tvdbId,
          mediaName: selectedShowInfo!.name,
          image_url: selectedShowInfo!.posterUrl,
        };

    fetch(endpoint, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(mediaData),
    })
      .then((response) => {
        if (!response.ok) {
          throw new Error("Network response was not ok");
        }
        return response.json();
      })
      .then(() => {
        // Download initiated successfully
        handleCloseStatusDialog();
      })
      .catch((error) => {
        console.error("Error initiating download:", error);
        logError(
          "Download initiation failed in ScheduleWidget",
          error as Error,
          {
            component: "ScheduleWidget",
            endpoint: endpoint,
            mediaType: selectedMediaType,
            resultId: selectedEnrichedMedia?.media.id || selectedShowInfo?.tvdbId || "unknown",
          },
        );
      });
  };

  const formatEpisode = (seasonNumber?: number, episodeNumber?: number) => {
    if (seasonNumber !== undefined && episodeNumber !== undefined) {
      return `S${String(seasonNumber).padStart(2, "0")}E${String(episodeNumber).padStart(2, "0")}`;
    }
    return null;
  };

  const getOrdinalSuffix = (day: number) => {
    if (day > 3 && day < 21) return "th";
    switch (day % 10) {
      case 1:
        return "st";
      case 2:
        return "nd";
      case 3:
        return "rd";
      default:
        return "th";
    }
  };

  const formatDayOfWeek = (dateString: string) => {
    const date = new Date(dateString);
    const weekday = new Intl.DateTimeFormat("en-US", {
      weekday: "long",
      timeZone: "UTC",
    }).format(date);
    const dayNum = parseInt(
      new Intl.DateTimeFormat("en-US", {
        day: "numeric",
        timeZone: "UTC",
      }).format(date),
    );
    return `${weekday} - ${dayNum}${getOrdinalSuffix(dayNum)}`;
  };

  // Loading state
  if (isLoading) {
    return (
      <Paper
        elevation={2}
        sx={{ p: 3, mb: 4, backgroundColor: theme.palette.background.paper }}
      >
        <Typography variant="h6" sx={{ mb: 2, fontWeight: 600 }}>
          Scheduled This Week
        </Typography>
        <Box sx={{ display: "flex", gap: 2, overflowX: "auto" }}>
          {Array.from({ length: 6 }).map((_, index) => (
            <Skeleton
              key={index}
              variant="rectangular"
              width={120}
              height={200}
              sx={{ borderRadius: 1 }}
            />
          ))}
        </Box>
      </Paper>
    );
  }

  // Error state
  if (isError) {
    return (
      <Paper
        elevation={2}
        sx={{ p: 3, mb: 4, backgroundColor: theme.palette.background.paper }}
      >
        <Typography variant="h6" sx={{ mb: 2, fontWeight: 600 }}>
          Scheduled This Week
        </Typography>
        <Box sx={{ display: "flex", alignItems: "center", gap: 2 }}>
          <Typography variant="body2" sx={{ color: theme.palette.error.main }}>
            Failed to load schedule
          </Typography>
          <Button
            variant="outlined"
            color="primary"
            size="small"
            onClick={() => refetch()}
          >
            Retry
          </Button>
        </Box>
      </Paper>
    );
  }

  // Empty state
  if (!data || data.length === 0) {
    return (
      <Paper
        elevation={2}
        sx={{ p: 3, mb: 4, backgroundColor: theme.palette.background.paper }}
      >
        <Typography variant="h6" sx={{ mb: 2, fontWeight: 600 }}>
          Scheduled This Week
        </Typography>
        <Typography
          variant="body2"
          sx={{ color: theme.palette.text.secondary }}
        >
          No scheduled downloads this week
        </Typography>
      </Paper>
    );
  }

  return (
    <>
      <Paper
        elevation={2}
        sx={{ p: 3, mb: 4, backgroundColor: theme.palette.background.paper }}
      >
        <Typography variant="h6" sx={{ mb: 2, fontWeight: 600 }}>
          Scheduled This Week
        </Typography>
        <Box
          sx={{
            display: "flex",
            gap: 2,
            overflowX: "auto",
            overflowY: "hidden",
            pb: 1,
            "&::-webkit-scrollbar": {
              height: 6,
            },
            "&::-webkit-scrollbar-thumb": {
              backgroundColor: theme.palette.divider,
              borderRadius: 3,
            },
          }}
        >
          {data.map((item) => (
            <Box
              key={`${item.id}-${item.seasonNumber}-${item.episodeNumber}`}
              sx={{
                flex: "0 0 auto",
                width: 120,
                display: "flex",
                flexDirection: "column",
                alignItems: "center",
              }}
            >
              <Card
                onClick={() => handleItemClick(item)}
                sx={{
                  width: "100%",
                  cursor: "pointer",
                  transition: "transform 0.2s ease-in-out",
                  "&:hover": {
                    transform: "scale(1.05)",
                  },
                }}
              >
                <CardMedia
                  component="img"
                  image={item.posterUrl}
                  alt={item.mediaName}
                  sx={{
                    width: "100%",
                    height: 160,
                    objectFit: "cover",
                  }}
                />
                <Box sx={{ p: 1 }}>
                  <Typography
                    variant="caption"
                    sx={{
                      fontWeight: 600,
                      display: "-webkit-box",
                      WebkitLineClamp: 2,
                      WebkitBoxOrient: "vertical",
                      overflow: "hidden",
                      lineHeight: 1.3,
                      mb: 0.5,
                    }}
                  >
                    {item.mediaName}
                  </Typography>
                  {formatEpisode(item.seasonNumber, item.episodeNumber) && (
                    <Typography
                      variant="caption"
                      sx={{
                        display: "block",
                        color: theme.palette.primary.main,
                        mb: 0.5,
                      }}
                    >
                      {formatEpisode(item.seasonNumber, item.episodeNumber)}
                    </Typography>
                  )}
                </Box>
              </Card>
              <Typography
                variant="caption"
                sx={{
                  mt: 0.5,
                  color: theme.palette.text.secondary,
                  fontSize: "0.7rem",
                  textAlign: "center",
                }}
              >
                {formatDayOfWeek(item.releaseTime)}
              </Typography>
            </Box>
          ))}
        </Box>
      </Paper>
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
        />
      )}
    </>
  );
};

export default ScheduleWidget;
