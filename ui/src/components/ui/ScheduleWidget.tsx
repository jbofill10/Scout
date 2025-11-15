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
import SearchResultDialog from "../SearchResultDialog";
import type { SearchResult } from "../SearchResultsList";
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
  const [dialogOpen, setDialogOpen] = useState(false);
  const [selectedItem, setSelectedItem] = useState<SearchResult | null>(null);
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

    // Fetch full media details including episodes
    try {
      const params = new URLSearchParams({
        query: item.mediaName,
        media_type: item.mediaType,
      });
      const res = await fetch(`/api/search?${params}`);
      const results: SearchResult[] = await res.json();

      // Find the matching result by ID
      const fullResult = results.find((r) => r.id === item.id);
      if (fullResult) {
        setSelectedItem(fullResult);
        setDialogOpen(true);
      } else {
        // Fallback to basic info if not found
        const searchResult: SearchResult = {
          id: item.id,
          mediaName: item.mediaName,
          image_url: item.posterUrl,
        };
        setSelectedItem(searchResult);
        setDialogOpen(true);
      }
    } catch (error) {
      console.error("Failed to fetch media details:", error);
      // Fallback to basic info on error
      const searchResult: SearchResult = {
        id: item.id,
        mediaName: item.mediaName,
        image_url: item.posterUrl,
      };
      setSelectedItem(searchResult);
      setDialogOpen(true);
    }
  };

  const handleCloseDialog = () => {
    setDialogOpen(false);
    setSelectedItem(null);
  };

  const handleDownload = (result: SearchResult) => {
    const endpoint =
      selectedMediaType === "movie" ? "/api/movies" : "/api/shows";

    fetch(endpoint, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(result),
    })
      .then((response) => {
        if (!response.ok) {
          throw new Error("Network response was not ok");
        }
        return response.json();
      })
      .then(() => {
        // Download initiated successfully
        handleCloseDialog();
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
            resultId: result.id,
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

  // const formatDate = (dateString: string) => {
  //     const date = new Date(dateString);
  //     return new Intl.DateTimeFormat('en-US', {
  //         month: 'short',
  //         day: 'numeric',
  //         hour: '2-digit',
  //         minute: '2-digit',
  //     }).format(date);
  // };

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
            <Card
              key={`${item.id}-${item.seasonNumber}-${item.episodeNumber}`}
              onClick={() => handleItemClick(item)}
              sx={{
                flex: "0 0 auto",
                width: 120,
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
          ))}
        </Box>
      </Paper>
      <SearchResultDialog
        open={dialogOpen}
        onClose={handleCloseDialog}
        result={selectedItem}
        onDownload={handleDownload}
        mediaType={selectedMediaType}
      />
    </>
  );
};

export default ScheduleWidget;
