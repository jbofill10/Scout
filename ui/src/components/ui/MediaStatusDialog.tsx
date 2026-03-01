import React, { useState, useMemo } from "react";
import Dialog from "@mui/material/Dialog";
import DialogTitle from "@mui/material/DialogTitle";
import DialogContent from "@mui/material/DialogContent";
import DialogActions from "@mui/material/DialogActions";
import Button from "@mui/material/Button";
import Box from "@mui/material/Box";
import Tabs from "@mui/material/Tabs";
import Tab from "@mui/material/Tab";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableRow from "@mui/material/TableRow";
import Paper from "@mui/material/Paper";
import Typography from "@mui/material/Typography";
import IconButton from "@mui/material/IconButton";
import CloseIcon from "@mui/icons-material/Close";
import CheckCircleIcon from "@mui/icons-material/CheckCircle";
import CancelIcon from "@mui/icons-material/Cancel";
import type { ShowStatus, EnrichedMedia } from "../../types/MediaStatus";
import { useTheme } from "@mui/material/styles";

export interface MediaStatusDialogProps {
  open: boolean;
  onClose: () => void;
  status: ShowStatus | null;
  mediaName: string;
  posterUrl: string;
  enrichedMedia?: EnrichedMedia | null;
  onDownload?: () => void;
  showDownloadButton?: boolean;
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
}) => {
  const theme = useTheme();
  const [selectedSeasonIndex, setSelectedSeasonIndex] = useState(0);

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

  if (enrichedSeasons.length === 0) {
    return null;
  }

  const handleSeasonChange = (_event: React.SyntheticEvent, newValue: number) => {
    setSelectedSeasonIndex(newValue);
  };

  const currentSeason = enrichedSeasons[selectedSeasonIndex];

  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth="md"
      fullWidth
      aria-labelledby="media-status-dialog-title"
      PaperProps={{
        sx: {
          backgroundColor: theme.palette.background.paper,
          borderRadius: 2,
        },
      }}
    >
      {/* Header with poster and title */}
      <DialogTitle
        id="media-status-dialog-title"
        sx={{
          display: "flex",
          alignItems: "center",
          gap: 2,
          pb: 1,
        }}
      >
        <Box
          component="img"
          src={posterUrl}
          alt={mediaName}
          sx={{
            width: 60,
            height: 90,
            objectFit: "cover",
            borderRadius: 1,
            border: `2px solid ${theme.palette.divider}`,
          }}
        />
        <Box sx={{ flex: 1 }}>
          <Typography variant="h5" component="div" sx={{ fontWeight: 600 }}>
            {mediaName}
          </Typography>
          <Typography variant="body2" color="text.secondary">
            Download Status
          </Typography>
        </Box>
        <IconButton
          aria-label="close"
          onClick={onClose}
          sx={{
            color: theme.palette.text.secondary,
          }}
        >
          <CloseIcon />
        </IconButton>
      </DialogTitle>

      <DialogContent sx={{ p: 0 }}>
        {/* Season Tabs */}
        <Box sx={{ borderBottom: 1, borderColor: "divider" }}>
          <Tabs
            value={selectedSeasonIndex}
            onChange={handleSeasonChange}
            variant="scrollable"
            scrollButtons="auto"
            aria-label="season tabs"
            sx={{
              px: 2,
            }}
          >
            {enrichedSeasons.map((season) => (
              <Tab
                key={season.seasonNum}
                label={`Season ${season.seasonNum}`}
                id={`season-tab-${season.seasonNum}`}
                aria-controls={`season-panel-${season.seasonNum}`}
              />
            ))}
          </Tabs>
        </Box>

        {/* Episode List */}
        <Box
          role="tabpanel"
          id={`season-panel-${currentSeason.seasonNum}`}
          aria-labelledby={`season-tab-${currentSeason.seasonNum}`}
          sx={{ p: 2 }}
        >
          {currentSeason.episodes.length === 0 ? (
            <Typography
              variant="body2"
              color="text.secondary"
              align="center"
              sx={{ py: 4 }}
            >
              No episodes found for this season
            </Typography>
          ) : (
            <TableContainer component={Paper} elevation={0}>
              <Table aria-label="episode status table">
                <TableBody>
                  {currentSeason.episodes.map((episode) => (
                    <TableRow
                      key={episode.episodeNum}
                      sx={{
                        "&:last-child td, &:last-child th": { border: 0 },
                        "&:hover": {
                          backgroundColor: theme.palette.action.hover,
                        },
                      }}
                    >
                      <TableCell
                        component="th"
                        scope="row"
                        sx={{
                          fontWeight: 500,
                          color: theme.palette.text.primary,
                        }}
                      >
                        <Box>
                          <Typography variant="body1" sx={{ fontWeight: 600 }}>
                            Episode {episode.episodeNum}
                            {episode.name && `: ${episode.name}`}
                          </Typography>
                          {episode.aired && (
                            <Typography
                              variant="caption"
                              sx={{ color: theme.palette.text.secondary }}
                            >
                              Aired: {new Date(episode.aired).toLocaleDateString()}
                            </Typography>
                          )}
                        </Box>
                      </TableCell>
                      <TableCell align="right">
                        {episode.downloaded ? (
                          <Box
                            sx={{
                              display: "flex",
                              alignItems: "center",
                              justifyContent: "flex-end",
                              gap: 1,
                            }}
                          >
                            <Typography
                              variant="body2"
                              sx={{ color: theme.palette.success.main }}
                            >
                              Downloaded
                            </Typography>
                            <CheckCircleIcon
                              sx={{
                                color: theme.palette.success.main,
                                fontSize: 20,
                              }}
                            />
                          </Box>
                        ) : (
                          <Box
                            sx={{
                              display: "flex",
                              alignItems: "center",
                              justifyContent: "flex-end",
                              gap: 1,
                            }}
                          >
                            <Typography
                              variant="body2"
                              sx={{ color: theme.palette.error.main, fontWeight: 600 }}
                            >
                              Missing
                            </Typography>
                            <CancelIcon
                              sx={{
                                color: theme.palette.error.main,
                                fontSize: 20,
                              }}
                            />
                          </Box>
                        )}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
          )}
        </Box>
      </DialogContent>

      {/* Dialog Actions */}
      <DialogActions sx={{ p: 2, borderTop: `1px solid ${theme.palette.divider}` }}>
        <Button onClick={onClose} variant="outlined" color="inherit">
          Close
        </Button>
        {showDownloadButton && onDownload && (
          <Button onClick={onDownload} variant="contained" color="primary">
            Download
          </Button>
        )}
      </DialogActions>
    </Dialog>
  );
};

export default MediaStatusDialog;
