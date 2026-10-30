import { useState, useMemo } from "react";
import { Box, Tabs, Tab, Typography, CircularProgress, Container, Skeleton, Button, Chip, IconButton, Tooltip } from "@mui/material";
import { alpha, useTheme } from "@mui/material/styles";
import RefreshIcon from "@mui/icons-material/Refresh";
import BookmarkRemoveOutlinedIcon from "@mui/icons-material/BookmarkRemoveOutlined";
import { useCheckFollows, useFollows, useUnfollowShow, type FollowedShow } from "../hooks/useFollows";
import { formatRelativeTime } from "../utils/downloadStage";
import MediaCard from "../components/ui/MediaCard";
import { MediaStatusDialog } from "../components/ui/MediaStatusDialog";
import PageHeader from "../components/ui/PageHeader";
import { useLibraryShows, useLibraryMovies, useShowEpisodes, useShowMetadataStatus, type LibraryShow } from "../hooks/useLibrary";
import type { MediaStatusBadge } from "../types/MediaStatus";

// ShowCard component that fetches and displays episode count metadata
interface ShowCardProps {
  item: LibraryShow;
  imageUrl: string;
  handleMediaClick: (tvdbId: string, title: string, thumb: string) => void;
  handleImageLoad: (tvdbId: string) => void;
}

const ShowCard: React.FC<ShowCardProps> = ({ item, imageUrl, handleMediaClick, handleImageLoad }) => {
  // Fetch metadata status for this specific show
  const { data: metadataStatus, isLoading } = useShowMetadataStatus(item.tvdbId);

  // Build status badge with episode counts
  const getStatusBadge = (): MediaStatusBadge => {
    if (!metadataStatus || !metadataStatus.hasTvdbData) {
      // No TVDB data yet - show generic badge
      return { type: "show", inLibrary: true };
    }

    return {
      type: "show",
      inLibrary: true,
      episodeCount: {
        downloaded: metadataStatus.plexEpisodeCount,
        total: metadataStatus.tvdbEpisodeCount,
      },
    };
  };

  return (
    <MediaCard
      media={{
        id: item.tvdbId,
        name: item.title,
        imageUrl: imageUrl,
      }}
      onClick={() => handleMediaClick(item.tvdbId, item.title, item.thumb)}
      statusBadge={getStatusBadge()}
      isLoadingStatus={isLoading}
      onImageLoad={() => handleImageLoad(item.tvdbId)}
    />
  );
};

/** One followed show: when it was last checked, what that found, and the controls. */
const FollowedRow: React.FC<{ show: FollowedShow; onOpen: () => void }> = ({ show, onOpen }) => {
  const theme = useTheme();
  const check = useCheckFollows();
  const unfollow = useUnfollowShow();
  const busy = check.isPending || unfollow.isPending;

  return (
    <Box
      sx={{
        display: "flex",
        gap: 2,
        alignItems: "center",
        px: { xs: 2, sm: 2.5 },
        py: 1.5,
        borderBottom: `1px solid ${theme.palette.divider}`,
        "&:last-of-type": { borderBottom: 0 },
      }}
    >
      <Box
        component={show.poster_url ? "img" : "div"}
        src={show.poster_url || undefined}
        alt=""
        onClick={onOpen}
        sx={{
          width: 44,
          height: 66,
          flexShrink: 0,
          objectFit: "cover",
          borderRadius: 1.5,
          cursor: "pointer",
          border: `1px solid ${theme.palette.divider}`,
          backgroundColor: alpha(theme.palette.common.white, 0.05),
        }}
      />
      <Box sx={{ flex: 1, minWidth: 0 }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap" }}>
          <Typography variant="body1" sx={{ fontWeight: 650, cursor: "pointer" }} onClick={onOpen}>
            {show.title}
          </Typography>
          {show.include_missing && <Chip size="small" label="Including missing" sx={{ height: 20 }} />}
        </Box>
        <Typography variant="caption" sx={{ display: "block", color: show.last_check_error ? theme.palette.error.main : theme.palette.text.secondary }}>
          {show.last_check_error
            ? `Last check failed: ${show.last_check_error}`
            : show.last_checked_at
              ? `Checked ${formatRelativeTime(show.last_checked_at)}`
              : "Not checked yet"}
          {` · ${show.episodes_added} episode${show.episodes_added === 1 ? "" : "s"} added · following since ${new Date(show.followed_at).toLocaleDateString()}`}
        </Typography>
      </Box>
      <Tooltip title="Check for new episodes now">
        <span>
          <IconButton size="small" aria-label={`Check ${show.title} now`} disabled={busy} onClick={() => check.mutate({ tvdbId: show.tvdb_id })}>
            {check.isPending ? <CircularProgress size={18} /> : <RefreshIcon fontSize="small" />}
          </IconButton>
        </span>
      </Tooltip>
      <Tooltip title="Unfollow">
        <span>
          <IconButton size="small" aria-label={`Unfollow ${show.title}`} disabled={busy} onClick={() => unfollow.mutate({ tvdbId: show.tvdb_id, title: show.title })}>
            <BookmarkRemoveOutlinedIcon fontSize="small" />
          </IconButton>
        </span>
      </Tooltip>
    </Box>
  );
};

export const Library = () => {
  const theme = useTheme();
  const [activeTab, setActiveTab] = useState<"shows" | "movies" | "following">("shows");
  const { data: follows, isLoading: followsLoading } = useFollows(activeTab === "following");
  const checkAll = useCheckFollows();
  const [selectedTvdbId, setSelectedTvdbId] = useState<string | null>(null);
  const [selectedMediaName, setSelectedMediaName] = useState<string>("");
  const [selectedPosterUrl, setSelectedPosterUrl] = useState<string>("");
  const [loadedImages, setLoadedImages] = useState<Set<string>>(new Set());

  // Fetch library data
  const { data: shows, isLoading: showsLoading } = useLibraryShows();
  const { data: movies, isLoading: moviesLoading } = useLibraryMovies();
  const { data: episodes } = useShowEpisodes(selectedTvdbId);

  // Sort items alphabetically
  const sortedShows = useMemo(() => {
    if (!shows) return [];
    return [...shows].sort((a, b) => a.title.localeCompare(b.title));
  }, [shows]);

  const sortedMovies = useMemo(() => {
    if (!movies) return [];
    return [...movies].sort((a, b) => a.title.localeCompare(b.title));
  }, [movies]);

  // Determine which items to display
  const items = activeTab === "shows" ? sortedShows : activeTab === "movies" ? sortedMovies : [];
  const isLoading = activeTab === "shows" ? showsLoading : activeTab === "movies" ? moviesLoading : followsLoading;

  // Convert Plex thumb path to proxied URL
  const getProxiedThumbUrl = (plexThumbPath: string): string => {
    if (!plexThumbPath) return "";
    if (/^https?:\/\//.test(plexThumbPath)) return plexThumbPath; // already a full URL (TVDB poster)
    // Encode the path to safely pass as query parameter
    return `/api/plex/thumb?path=${encodeURIComponent(plexThumbPath)}`;
  };

  // Handle media card click
  const handleMediaClick = (tvdbId: string, name: string, thumb: string) => {
    setSelectedTvdbId(tvdbId);
    setSelectedMediaName(name);
    setSelectedPosterUrl(getProxiedThumbUrl(thumb));
  };

  // Handle dialog close
  const handleCloseDialog = () => {
    setSelectedTvdbId(null);
    setSelectedMediaName("");
    setSelectedPosterUrl("");
  };

  // Convert episodes to MediaStatusDialog format
  const mediaStatus = useMemo(() => {
    if (!episodes || episodes.length === 0) return null;

    // Group episodes by season
    const seasonMap = new Map<number, Array<{ episodeNum: number; downloaded: boolean }>>();

    episodes.forEach((ep) => {
      if (!seasonMap.has(ep.seasonNumber)) {
        seasonMap.set(ep.seasonNumber, []);
      }
      seasonMap.get(ep.seasonNumber)!.push({
        episodeNum: ep.episodeNumber,
        downloaded: ep.downloaded,
      });
    });

    // Build ShowStatus structure
    const seasons = Array.from(seasonMap.entries()).map(([seasonNum, episodesList]) => ({
      seasonNum,
      episodes: episodesList.sort((a, b) => a.episodeNum - b.episodeNum),
    }));

    return {
      tvdbId: selectedTvdbId || "",
      seasons: seasons.sort((a, b) => a.seasonNum - b.seasonNum),
    };
  }, [episodes, selectedTvdbId]);

  // Convert episodes to enrichedMedia format for dialog
  const enrichedMedia = useMemo(() => {
    if (!episodes || episodes.length === 0) return null;

    return {
      media: {
        id: selectedTvdbId || "",
        mediaName: selectedMediaName,
        image_url: selectedPosterUrl,
        metadata: {
          episodes: episodes.map((ep) => ({
            id: parseInt(ep.tvdbId) || 0,
            seasonNumber: ep.seasonNumber,
            number: ep.episodeNumber,
            name: ep.name,
            aired: ep.aired,
            image: "",
          })),
        },
      },
      status: {
        type: "series" as const,
        downloaded: episodes.filter((ep) => ep.downloaded).length,
        total: episodes.length,
      },
    };
  }, [episodes, selectedTvdbId, selectedMediaName, selectedPosterUrl]);

  // Handle image load completion
  const handleImageLoad = (tvdbId: string) => {
    setLoadedImages((prev) => new Set(prev).add(tvdbId));
  };

  return (
    <Box sx={{ minHeight: "100vh", pt: { xs: 11, md: 13 }, pb: 8 }}>
      <Container maxWidth="xl">
        <PageHeader
          eyebrow="Plex"
          title="My Library"
          description="Everything already in your Plex library. Pick a show to see which episodes you have."
        />

        {/* Tabs */}
        <Box sx={{ borderBottom: 1, borderColor: "divider", mb: { xs: 3, md: 4 } }}>
          <Tabs
            value={activeTab}
            onChange={(_, newValue) => setActiveTab(newValue)}
            aria-label="library tabs"
          >
            <Tab label="TV Shows" value="shows" />
            <Tab label="Movies" value="movies" />
            <Tab label={`Following${follows && follows.length > 0 ? ` (${follows.length})` : ""}`} value="following" />
          </Tabs>
        </Box>

        {/* Followed shows: re-checked for new episodes on their own */}
        {activeTab === "following" && !followsLoading && (
          <Box sx={{ mb: 4 }}>
            <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 1.5, gap: 2, flexWrap: "wrap" }}>
              <Typography variant="body2" color="text.secondary">
                New episodes of these shows are requested on their own. Checked on a timer, or now.
              </Typography>
              <Button
                size="small"
                variant="outlined"
                startIcon={checkAll.isPending ? <CircularProgress size={16} /> : <RefreshIcon />}
                disabled={checkAll.isPending || !follows || follows.length === 0}
                onClick={() => checkAll.mutate({ tvdbId: null })}
              >
                Check all now
              </Button>
            </Box>
            {(!follows || follows.length === 0) && (
              <Box sx={{ textAlign: "center", py: 8 }}>
                <Typography variant="h6" color="text.secondary">
                  Not following any shows
                </Typography>
                <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
                  Open a show and press Follow to have new episodes requested automatically.
                </Typography>
              </Box>
            )}
            {follows && follows.length > 0 && (
              <Box sx={{ backgroundColor: theme.palette.background.paper, border: `1px solid ${theme.palette.divider}`, borderRadius: 3, overflow: "hidden" }}>
                {follows.map((show) => (
                  <FollowedRow key={show.tvdb_id} show={show} onOpen={() => handleMediaClick(show.tvdb_id, show.title, show.poster_url ?? "")} />
                ))}
              </Box>
            )}
          </Box>
        )}

        {/* Loading State */}
        {isLoading && (
          <Box sx={{ display: "flex", justifyContent: "center", py: 8 }}>
            <CircularProgress />
          </Box>
        )}

        {/* Empty State */}
        {!isLoading && activeTab !== "following" && items.length === 0 && (
          <Box sx={{ textAlign: "center", py: 8 }}>
            <Typography variant="h6" color="text.secondary">
              No {activeTab === "shows" ? "TV shows" : "movies"} in your library yet
            </Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
              Download some content to see it here
            </Typography>
          </Box>
        )}

        {/* Grid of Media Cards — two columns on a phone, as many as fit elsewhere */}
        {!isLoading && activeTab !== "following" && items.length > 0 && (
          <Box
            sx={{
              display: "grid",
              gridTemplateColumns: {
                xs: "repeat(auto-fill, minmax(130px, 1fr))",
                sm: "repeat(auto-fill, minmax(160px, 1fr))",
                md: "repeat(auto-fill, minmax(180px, 1fr))",
              },
              gap: { xs: 1.5, sm: 2 },
            }}
          >
            {items.map((item) => {
              const isImageLoaded = loadedImages.has(item.tvdbId);
              const imageUrl = getProxiedThumbUrl(item.thumb);

              return (
                <Box key={item.tvdbId} sx={{ position: "relative" }}>
                  {/* Show cards with episode counts or movie cards */}
                  {activeTab === "shows" ? (
                    <ShowCard
                      item={item as LibraryShow}
                      imageUrl={imageUrl}
                      handleMediaClick={handleMediaClick}
                      handleImageLoad={handleImageLoad}
                    />
                  ) : (
                    <MediaCard
                      media={{
                        id: item.tvdbId,
                        name: item.title,
                        imageUrl: imageUrl,
                      }}
                      onClick={() => handleMediaClick(item.tvdbId, item.title, item.thumb)}
                      statusBadge={{ type: "movie", inLibrary: true }}
                      onImageLoad={() => handleImageLoad(item.tvdbId)}
                    />
                  )}

                  {/* Skeleton overlay - removed once image loads. Sized by the
                      card underneath, so it fits whatever column width the grid picked. */}
                  {!isImageLoaded && (
                    <Box
                      sx={{
                        position: "absolute",
                        inset: 0,
                        borderRadius: 3,
                        overflow: "hidden",
                        bgcolor: "background.paper",
                        cursor: "pointer",
                      }}
                      onClick={() => handleMediaClick(item.tvdbId, item.title, item.thumb)}
                    >
                      <Skeleton
                        variant="rectangular"
                        sx={{ width: "100%", height: "100%", bgcolor: "rgba(255, 255, 255, 0.05)" }}
                      />
                    </Box>
                  )}
                </Box>
              );
            })}
          </Box>
        )}

        {/* Media Status Dialog */}
        {selectedTvdbId && activeTab !== "movies" && (
          <MediaStatusDialog
            open={!!selectedTvdbId}
            onClose={handleCloseDialog}
            status={mediaStatus}
            mediaName={selectedMediaName}
            posterUrl={selectedPosterUrl}
            tvdbId={selectedTvdbId}
            enrichedMedia={enrichedMedia}
            showDownloadButton={false} // Read-only mode for library
          />
        )}

        {/* Movie Dialog — details only, everything here is already in the library */}
        {selectedTvdbId && activeTab === "movies" && (
          <MediaStatusDialog
            open={!!selectedTvdbId}
            onClose={handleCloseDialog}
            status={null} // Movies don't have episode status
            mediaName={selectedMediaName}
            posterUrl={selectedPosterUrl}
            tvdbId={selectedTvdbId}
            mediaType="movie"
            inLibrary={true}
            showDownloadButton={false}
          />
        )}
      </Container>
    </Box>
  );
};

export default Library;
