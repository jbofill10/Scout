import React, { useState, useMemo } from "react";
import Dialog from "@mui/material/Dialog";
import DialogTitle from "@mui/material/DialogTitle";
import DialogContent from "@mui/material/DialogContent";
import DialogActions from "@mui/material/DialogActions";
import Button from "@mui/material/Button";
import Box from "@mui/material/Box";
import Tabs from "@mui/material/Tabs";
import Tab from "@mui/material/Tab";
import LinearProgress from "@mui/material/LinearProgress";
import Typography from "@mui/material/Typography";
import IconButton from "@mui/material/IconButton";
import CircularProgress from "@mui/material/CircularProgress";
import CloseIcon from "@mui/icons-material/Close";
import CheckRoundedIcon from "@mui/icons-material/CheckRounded";
import DownloadRoundedIcon from "@mui/icons-material/DownloadRounded";
import type { ShowStatus, EnrichedMedia } from "../../types/MediaStatus";
import { alpha, useTheme } from "@mui/material/styles";

export interface MediaStatusDialogProps {
  open: boolean;
  onClose: () => void;
  status: ShowStatus | null;
  mediaName: string;
  posterUrl: string;
  enrichedMedia?: EnrichedMedia | null;
  onDownload?: () => void;
  showDownloadButton?: boolean;
  isDownloading?: boolean;
}

interface EnrichedEpisodeInfo {
  episodeNum: number;
  seasonNum: number;
  downloaded: boolean;
  name?: string;
  aired?: string;
}

interface EnrichedSeasonInfo {
  seasonNum: number;
  episodes: EnrichedEpisodeInfo[];
}

/**
 * MediaStatusDialog Component
 *
 * Displays detailed download status for TV shows in a tabbed dialog.
 * Features:
 * - Tabs for each season
 * - Episode list with downloaded/not downloaded indicators
 * - Show poster and name header
 * - Keyboard accessible (ESC to close)
 * - Material-UI theme integration
 */
export const MediaStatusDialog: React.FC<MediaStatusDialogProps> = ({
  open,
  onClose,
  status,
  mediaName,
  posterUrl,
  enrichedMedia,
  onDownload,
  showDownloadButton = false,
  isDownloading = false,
}) => {
  const theme = useTheme();
  const [selectedSeasonIndex, setSelectedSeasonIndex] = useState<number | null>(null);

  // Merge TVDB episode metadata with Plex download status
  const enrichedSeasons = useMemo<EnrichedSeasonInfo[]>(() => {
    // Prefer enriched status seasons (has TVDB IDs) over batch status prop
    const statusSeasons = enrichedMedia?.status?.seasons ?? status?.seasons;

    // If no enriched data, fall back to status-only display
    if (!enrichedMedia || !enrichedMedia.media.metadata?.episodes) {
      if (!statusSeasons || statusSeasons.length === 0) {
        return [];
      }
      return statusSeasons.map((season): EnrichedSeasonInfo => ({
        seasonNum: season.seasonNum,
        episodes: season.episodes.map((ep): EnrichedEpisodeInfo => ({
          episodeNum: ep.episodeNum,
          seasonNum: season.seasonNum,
          downloaded: ep.downloaded,
        })),
      }));
    }

    // Build lookup sets from status data for matching
    const downloadedByTvdbId = new Set<string>();
    const downloadedByKey = new Set<string>();
    if (statusSeasons) {
      statusSeasons.forEach((season) => {
        season.episodes.forEach((ep) => {
          if (ep.downloaded) {
            if (ep.tvdbId) {
              downloadedByTvdbId.add(ep.tvdbId);
            }
            const key = `${season.seasonNum}-${ep.episodeNum}`;
            downloadedByKey.add(key);
          }
        });
      });
    }

    // Build absolute number lookup for anime shows.
    // Plex uses absolute episode numbering for anime (e.g., S01E25, S02E25, S03E48)
    // while TVDB uses standard per-season numbering (S02E01). The absolute number
    // from TVDB metadata bridges these two schemes.
    const downloadedByAbsolute = new Set<number>();
    const usesAbsoluteNumbering = (() => {
      if (!statusSeasons) return false;
      const nonSpecials = statusSeasons.filter((s) => s.seasonNum > 0);
      if (nonSpecials.length < 2) return false;
      return statusSeasons.some((s) => {
        if (s.seasonNum <= 1 || s.episodes.length === 0) return false;
        const minEp = Math.min(...s.episodes.map((ep) => ep.episodeNum));
        return minEp > 1;
      });
    })();
    if (enrichedMedia.media.anime === true && usesAbsoluteNumbering && statusSeasons) {
      statusSeasons.forEach((season) => {
        if (season.seasonNum === 0) return;
        season.episodes.forEach((ep) => {
          if (ep.downloaded) {
            downloadedByAbsolute.add(ep.episodeNum);
          }
        });
      });
    }

    // Build complete episode list from TVDB metadata
    const seasonsMap = new Map<number, EnrichedEpisodeInfo[]>();
    enrichedMedia.media.metadata.episodes.forEach((ep) => {
      // Match by TVDB ID first, then season-episode key, then absolute number
      const matchByTvdbId = downloadedByTvdbId.has(String(ep.id));
      const matchByKey = downloadedByKey.has(`${ep.seasonNumber}-${ep.number}`);
      const matchByAbsolute =
        ep.absoluteNumber != null &&
        ep.absoluteNumber > 0 &&
        downloadedByAbsolute.has(ep.absoluteNumber);
      const downloaded = matchByTvdbId || matchByKey || matchByAbsolute;

      if (!seasonsMap.has(ep.seasonNumber)) {
        seasonsMap.set(ep.seasonNumber, []);
      }

      seasonsMap.get(ep.seasonNumber)!.push({
        episodeNum: ep.number,
        seasonNum: ep.seasonNumber,
        downloaded: downloaded,
        name: ep.name,
        aired: ep.aired,
      });
    });

    // Convert map to array and sort
    return Array.from(seasonsMap.entries())
      .map(([seasonNum, episodes]): EnrichedSeasonInfo => ({
        seasonNum,
        episodes: episodes.sort((a, b) => a.episodeNum - b.episodeNum),
      }))
      .sort((a, b) => a.seasonNum - b.seasonNum);
  }, [status, enrichedMedia]);

  // Library coverage across every season, shown as a progress bar in the header
  const totals = useMemo(() => {
    const episodes = enrichedSeasons.flatMap((season) => season.episodes);
    const downloaded = episodes.filter((episode) => episode.downloaded).length;
    return { downloaded, total: episodes.length };
  }, [enrichedSeasons]);

  if (enrichedSeasons.length === 0) {
    return null;
  }

  const handleSeasonChange = (_event: React.SyntheticEvent, newValue: number) => {
    setSelectedSeasonIndex(newValue);
  };

  // Land on the first real season rather than Specials, which sorts first
  const defaultSeasonIndex = Math.max(
    enrichedSeasons.findIndex((season) => season.seasonNum > 0),
    0,
  );
  const activeSeasonIndex = selectedSeasonIndex ?? defaultSeasonIndex;
  const currentSeason = enrichedSeasons[activeSeasonIndex];
  const percentComplete = totals.total > 0 ? (totals.downloaded / totals.total) * 100 : 0;

  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth="md"
      fullWidth
      aria-labelledby="media-status-dialog-title"
    >
      {/* Header with poster, title and overall coverage */}
      <DialogTitle
        id="media-status-dialog-title"
        component="div"
        sx={{ display: "flex", alignItems: "flex-start", gap: 2.5, p: 3, pb: 2.5 }}
      >
        <Box
          component="img"
          src={posterUrl}
          alt={mediaName}
          sx={{
            width: 72,
            height: 108,
            flexShrink: 0,
            objectFit: "cover",
            borderRadius: 2,
            border: `1px solid ${theme.palette.divider}`,
          }}
        />
        <Box sx={{ flex: 1, minWidth: 0 }}>
          <Typography variant="h4" component="h2" sx={{ color: theme.palette.text.primary }}>
            {mediaName}
          </Typography>
          <Typography variant="body2" sx={{ mt: 0.5, color: theme.palette.text.secondary }}>
            {totals.downloaded} of {totals.total} {totals.total === 1 ? "episode" : "episodes"} in
            your library
          </Typography>
          <LinearProgress
            variant="determinate"
            value={percentComplete}
            aria-label="Library coverage"
            sx={{
              mt: 1.5,
              maxWidth: 320,
              backgroundColor: alpha(theme.palette.common.white, 0.08),
              "& .MuiLinearProgress-bar": {
                backgroundColor:
                  percentComplete === 100 ? theme.palette.success.main : theme.palette.primary.main,
              },
            }}
          />
        </Box>
        <IconButton aria-label="close" onClick={onClose} sx={{ color: theme.palette.text.secondary }}>
          <CloseIcon />
        </IconButton>
      </DialogTitle>

      <DialogContent sx={{ p: 0 }}>
        {/* Season Tabs — each label carries that season's coverage */}
        <Box sx={{ borderBottom: `1px solid ${theme.palette.divider}` }}>
          <Tabs
            value={activeSeasonIndex}
            onChange={handleSeasonChange}
            variant="scrollable"
            scrollButtons="auto"
            aria-label="season tabs"
            sx={{
              px: 3,
              minHeight: 48,
              "& .MuiTab-root": {
                minHeight: 48,
                textTransform: "none",
                fontWeight: 600,
                fontSize: "0.875rem",
                color: theme.palette.text.secondary,
                "&.Mui-selected": { color: theme.palette.text.primary },
              },
            }}
          >
            {enrichedSeasons.map((season) => {
              const have = season.episodes.filter((episode) => episode.downloaded).length;
              return (
                <Tab
                  key={season.seasonNum}
                  label={`${season.seasonNum === 0 ? "Specials" : `Season ${season.seasonNum}`} · ${have}/${season.episodes.length}`}
                  id={`season-tab-${season.seasonNum}`}
                  aria-controls={`season-panel-${season.seasonNum}`}
                />
              );
            })}
          </Tabs>
        </Box>

        {/* Episode List */}
        <Box
          role="tabpanel"
          id={`season-panel-${currentSeason.seasonNum}`}
          aria-labelledby={`season-tab-${currentSeason.seasonNum}`}
          sx={{ px: 3, py: 2, maxHeight: "48vh", overflowY: "auto" }}
        >
          {currentSeason.episodes.length === 0 ? (
            <Typography variant="body2" color="text.secondary" align="center" sx={{ py: 6 }}>
              No episodes found for this season
            </Typography>
          ) : (
            currentSeason.episodes.map((episode) => (
              <Box
                key={episode.episodeNum}
                sx={{
                  display: "flex",
                  alignItems: "center",
                  gap: 2,
                  py: 1.25,
                  borderBottom: `1px solid ${theme.palette.divider}`,
                  "&:last-of-type": { borderBottom: 0 },
                }}
              >
                {/* Episode number, fixed width so titles align down the column */}
                <Typography
                  variant="body2"
                  sx={{
                    width: 34,
                    flexShrink: 0,
                    textAlign: "right",
                    fontVariantNumeric: "tabular-nums",
                    color: theme.palette.text.disabled,
                  }}
                >
                  {episode.episodeNum}
                </Typography>

                <Box sx={{ flex: 1, minWidth: 0 }}>
                  <Typography
                    variant="body2"
                    sx={{
                      fontWeight: 600,
                      color: theme.palette.text.primary,
                      overflow: "hidden",
                      textOverflow: "ellipsis",
                      whiteSpace: "nowrap",
                    }}
                  >
                    {episode.name || `Episode ${episode.episodeNum}`}
                  </Typography>
                  {episode.aired && (
                    <Typography variant="caption" sx={{ color: theme.palette.text.secondary }}>
                      {new Date(episode.aired).toLocaleDateString()}
                    </Typography>
                  )}
                </Box>

                {episode.downloaded ? (
                  <Box
                    sx={{
                      display: "flex",
                      alignItems: "center",
                      gap: 0.5,
                      flexShrink: 0,
                      px: 1,
                      py: "3px",
                      borderRadius: 999,
                      fontSize: "0.6875rem",
                      fontWeight: 700,
                      color: theme.palette.success.main,
                      backgroundColor: alpha(theme.palette.success.main, 0.14),
                      border: `1px solid ${alpha(theme.palette.success.main, 0.35)}`,
                    }}
                  >
                    <CheckRoundedIcon sx={{ fontSize: 14 }} />
                    In library
                  </Box>
                ) : (
                  <Typography
                    variant="caption"
                    sx={{ flexShrink: 0, color: theme.palette.text.disabled, fontWeight: 600 }}
                  >
                    Missing
                  </Typography>
                )}
              </Box>
            ))
          )}
        </Box>
      </DialogContent>

      {/* Dialog Actions */}
      <DialogActions sx={{ px: 3, py: 2, borderTop: `1px solid ${theme.palette.divider}` }}>
        <Button onClick={onClose} variant="text" color="inherit">
          Close
        </Button>
        {showDownloadButton && onDownload && (
          <Button
            onClick={onDownload}
            variant="contained"
            color="primary"
            disabled={isDownloading}
            startIcon={
              isDownloading ? (
                <CircularProgress size={16} color="inherit" />
              ) : (
                <DownloadRoundedIcon />
              )
            }
          >
            {isDownloading ? "Requesting..." : "Download missing"}
          </Button>
        )}
      </DialogActions>
    </Dialog>
  );
};

export default MediaStatusDialog;
