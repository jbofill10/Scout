import React, { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import Box from "@mui/material/Box";
import Card from "@mui/material/Card";
import Typography from "@mui/material/Typography";
import Skeleton from "@mui/material/Skeleton";
import Button from "@mui/material/Button";
import CalendarMonthOutlinedIcon from "@mui/icons-material/CalendarMonthOutlined";
import { alpha, useTheme } from "@mui/material/styles";
import MediaStatusDialog from "./MediaStatusDialog";
import type { EnrichedMedia, ShowStatus } from "../../types/MediaStatus";
import { useDownloadRequest } from "../../hooks/useDownloadRequest";

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
  const { requestDownload, isPending: isDownloading } = useDownloadRequest();

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

    const mediaData = selectedEnrichedMedia
      ? selectedEnrichedMedia.media
      : {
          id: selectedShowInfo!.tvdbId,
          mediaName: selectedShowInfo!.name,
          image_url: selectedShowInfo!.posterUrl,
        };

    requestDownload(
      {
        media: mediaData,
        mediaType: selectedMediaType,
        title: selectedEnrichedMedia?.media.mediaName ?? selectedShowInfo?.name,
        component: "ScheduleWidget",
      },
      { onSuccess: () => handleCloseStatusDialog() },
    );
  };

  const formatEpisode = (seasonNumber?: number, episodeNumber?: number) => {
    if (seasonNumber !== undefined && episodeNumber !== undefined) {
      return `S${String(seasonNumber).padStart(2, "0")}E${String(episodeNumber).padStart(2, "0")}`;
    }
    return null;
  };

  // Short weekday + day of month, e.g. "SAT 26" — compact enough for a badge
  const formatReleaseBadge = (dateString: string) => {
    const date = new Date(dateString);
    const weekday = new Intl.DateTimeFormat("en-US", {
      weekday: "short",
      timeZone: "UTC",
    })
      .format(date)
      .toUpperCase();
    const dayNum = new Intl.DateTimeFormat("en-US", {
      day: "numeric",
      timeZone: "UTC",
    }).format(date);
    return `${weekday} ${dayNum}`;
  };

  // Section header shared by every state so the widget never shifts vertically
  const header = (
    <Box sx={{ display: "flex", alignItems: "baseline", gap: 1.5, mb: 1.75 }}>
      <Typography variant="h5" sx={{ color: theme.palette.text.primary }}>
        Scheduled This Week
      </Typography>
      {data && data.length > 0 && (
        <Typography variant="caption" sx={{ color: theme.palette.text.secondary }}>
          {data.length} {data.length === 1 ? "item" : "items"}
        </Typography>
      )}
    </Box>
  );

  // Loading state
  if (isLoading) {
    return (
      <Box sx={{ mb: 5 }}>
        {header}
        <Box sx={{ display: "flex", gap: 2, overflow: "hidden" }}>
          {Array.from({ length: 7 }).map((_, index) => (
            <Skeleton
              key={index}
              variant="rectangular"
              sx={{ flex: "0 0 140px", height: 240, borderRadius: 3 }}
            />
          ))}
        </Box>
      </Box>
    );
  }

  // Error state
  if (isError) {
    return (
      <Box sx={{ mb: 5 }}>
        {header}
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
            Couldn't load the schedule.
          </Typography>
          <Button variant="outlined" color="primary" size="small" onClick={() => refetch()}>
            Retry
          </Button>
        </Box>
      </Box>
    );
  }

  // Empty state
  if (!data || data.length === 0) {
    return (
      <Box sx={{ mb: 5 }}>
        {header}
        <Box
          sx={{
            display: "flex",
            alignItems: "center",
            gap: 2,
            px: 3,
            py: 3.5,
            border: `1px dashed ${theme.palette.divider}`,
            borderRadius: 3,
          }}
        >
          <CalendarMonthOutlinedIcon sx={{ color: theme.palette.text.disabled, fontSize: 28 }} />
          <Box>
            <Typography variant="subtitle1" sx={{ color: theme.palette.text.primary }}>
              Nothing scheduled this week
            </Typography>
            <Typography variant="body2" sx={{ color: theme.palette.text.secondary }}>
              Queue a show or movie and upcoming releases will show up here.
            </Typography>
          </Box>
        </Box>
      </Box>
    );
  }

  return (
    <>
      <Box sx={{ mb: 5 }}>
        {header}
        <Box
          sx={{
            display: "flex",
            gap: 2,
            overflowX: "auto",
            overflowY: "hidden",
            // Room for the hover lift so raised cards are not clipped
            py: 1.5,
            my: -1.5,
            "&::-webkit-scrollbar": { display: "none" },
            scrollbarWidth: "none",
          }}
        >
          {data.map((item) => {
            const episode = formatEpisode(item.seasonNumber, item.episodeNumber);

            return (
              <Card
                key={`${item.id}-${item.seasonNumber}-${item.episodeNumber}`}
                onClick={() => handleItemClick(item)}
                onKeyDown={(event) => {
                  if (event.key === "Enter" || event.key === " ") {
                    event.preventDefault();
                    handleItemClick(item);
                  }
                }}
                tabIndex={0}
                role="button"
                aria-label={`View details for ${item.mediaName}`}
                sx={{
                  flex: "0 0 140px",
                  cursor: "pointer",
                  overflow: "hidden",
                  borderRadius: 3,
                  transition: theme.transitions.create(
                    ["transform", "box-shadow", "border-color"],
                    { duration: 220, easing: "cubic-bezier(0.22, 1, 0.36, 1)" },
                  ),
                  "&:hover": {
                    transform: "translateY(-4px)",
                    boxShadow: theme.shadows[8],
                    borderColor: alpha(theme.palette.primary.light, 0.5),
                  },
                }}
              >
                {/* overflow:hidden keeps a broken poster's alt text inside the card */}
                <Box sx={{ position: "relative", height: 168, overflow: "hidden" }}>
                  <Box
                    component="img"
                    src={item.posterUrl}
                    alt={item.mediaName}
                    loading="lazy"
                    sx={{ width: "100%", height: "100%", objectFit: "cover", display: "block" }}
                  />
                  {/* Release day rides on the poster so the card stays compact */}
                  <Box
                    sx={{
                      position: "absolute",
                      top: 8,
                      left: 8,
                      px: 0.875,
                      py: "3px",
                      borderRadius: 999,
                      fontSize: "0.625rem",
                      fontWeight: 700,
                      letterSpacing: "0.06em",
                      color: theme.palette.common.white,
                      backgroundColor: alpha("#020617", 0.72),
                      border: `1px solid ${alpha("#F8FAFC", 0.16)}`,
                      backdropFilter: "blur(8px)",
                    }}
                  >
                    {formatReleaseBadge(item.releaseTime)}
                  </Box>
                </Box>
                <Box sx={{ p: 1.25 }}>
                  <Typography
                    variant="caption"
                    sx={{
                      fontWeight: 650,
                      display: "-webkit-box",
                      WebkitLineClamp: 2,
                      WebkitBoxOrient: "vertical",
                      overflow: "hidden",
                      lineHeight: 1.35,
                      color: theme.palette.text.primary,
                    }}
                  >
                    {item.mediaName}
                  </Typography>
                  {episode && (
                    <Typography
                      variant="caption"
                      sx={{
                        display: "block",
                        mt: 0.25,
                        color: theme.palette.primary.light,
                        fontWeight: 600,
                        letterSpacing: "0.02em",
                      }}
                    >
                      {episode}
                    </Typography>
                  )}
                </Box>
              </Card>
            );
          })}
        </Box>
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
          mediaType={selectedMediaType === "movie" ? "movie" : "series"}
        />
      )}
    </>
  );
};

export default ScheduleWidget;
