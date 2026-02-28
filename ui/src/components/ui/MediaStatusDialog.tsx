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
import Alert from "@mui/material/Alert";
import AlertTitle from "@mui/material/AlertTitle";
import CloseIcon from "@mui/icons-material/Close";
import CheckCircleIcon from "@mui/icons-material/CheckCircle";
import CancelIcon from "@mui/icons-material/Cancel";
import type { ShowStatus, EnrichedMedia } from "../../types/MediaStatus";
import { useTheme } from "@mui/material/styles";
import { useShowMetadataStatus } from "../../hooks/useLibrary";

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

  // Fetch metadata status for TV shows
  const { data: metadataStatus } = useShowMetadataStatus(
    enrichedMedia?.status.type === "series" ? enrichedMedia?.media.id ?? null : null
  );

  // Merge TVDB episode metadata with Plex download status
  const enrichedSeasons = useMemo<EnrichedSeasonInfo[]>(() => {
    // If no enriched data, fall back to status-only display
    if (!enrichedMedia || !enrichedMedia.media.metadata?.episodes) {
      if (!status || !status.seasons || status.seasons.length === 0) {
        return [];
      }
      return status.seasons.map((season): EnrichedSeasonInfo => ({
        seasonNum: season.seasonNum,
        episodes: season.episodes.map((ep): EnrichedEpisodeInfo => ({
          episodeNum: ep.episodeNum,
          seasonNum: season.seasonNum,
          downloaded: ep.downloaded,
        })),
      }));
    }

    // Build set of downloaded episodes from status data
    const downloadedEpisodesSet = new Set<string>();
    if (status?.seasons) {
      status.seasons.forEach((season) => {
        season.episodes.forEach((ep) => {
          if (ep.downloaded) {  // Only add downloaded episodes to the set
            const key = `${season.seasonNum}-${ep.episodeNum}`;
            downloadedEpisodesSet.add(key);
          }
        });
      });
    }

    // Build complete episode list from TVDB metadata
    const seasonsMap = new Map<number, EnrichedEpisodeInfo[]>();
    enrichedMedia.media.metadata.episodes.forEach((ep) => {
      const key = `${ep.seasonNumber}-${ep.number}`;
      const downloaded = downloadedEpisodesSet.has(key);

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

  const handleSeasonChange = (_event: React.SyntheticEvent, newValue: number) => {
    setSelectedSeasonIndex(newValue);
  };

  const currentSeason = enrichedSeasons.length > 0 ? enrichedSeasons[selectedSeasonIndex] : null;

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
        {/* TVDB Metadata Status Banner */}
        {enrichedMedia?.status.type === "series" && metadataStatus && (
          <Box sx={{ p: 2, pb: 0 }}>
            {!metadataStatus.hasTvdbData && metadataStatus.plexEpisodeCount > 0 ? (
              <Alert severity="info">
                <AlertTitle>TVDB Sync in Progress</AlertTitle>
                Episode metadata is being synced from TVDB. Missing episode detection will be available shortly.
                Refresh this page in a few minutes to see complete episode status.
              </Alert>
            ) : metadataStatus.hasTvdbData && metadataStatus.missingCount > 0 ? (
              <Alert severity="warning">
                <AlertTitle>Missing Episodes Detected</AlertTitle>
                {metadataStatus.missingCount} episode{metadataStatus.missingCount !== 1 ? 's' : ''} not yet downloaded.
              </Alert>
            ) : metadataStatus.hasTvdbData && metadataStatus.plexEpisodeCount > 0 ? (
              <Alert severity="success">
                <AlertTitle>All Episodes Downloaded 🎉</AlertTitle>
                You have all {metadataStatus.tvdbEpisodeCount} aired episodes in your library!
              </Alert>
            ) : null}
          </Box>
        )}

        {enrichedSeasons.length === 0 ? (
          <Box sx={{ p: 4, textAlign: "center" }}>
            <Typography variant="h6" color="text.secondary" sx={{ mb: 2 }}>
              No Episode Data Available
            </Typography>
            <Typography variant="body2" color="text.secondary">
              This show was added to your library before episode tracking was enabled.
              Download a new show to automatically sync episode data, or the data will
              be synced the next time this show is updated.
            </Typography>
          </Box>
        ) : (
          <>
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
            {currentSeason && (
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
                                    Not Downloaded
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
            )}
          </>
        )}
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
