import React, { useState } from "react";
import Dialog from "@mui/material/Dialog";
import DialogTitle from "@mui/material/DialogTitle";
import DialogContent from "@mui/material/DialogContent";
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
import type { ShowStatus } from "../../types/MediaStatus";
import { useTheme } from "@mui/material/styles";

export interface MediaStatusDialogProps {
  open: boolean;
  onClose: () => void;
  status: ShowStatus | null;
  mediaName: string;
  posterUrl: string;
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
}) => {
  const theme = useTheme();
  const [selectedSeasonIndex, setSelectedSeasonIndex] = useState(0);

  if (!status || !status.seasons || status.seasons.length === 0) {
    return null;
  }

  const handleSeasonChange = (_event: React.SyntheticEvent, newValue: number) => {
    setSelectedSeasonIndex(newValue);
  };

  const currentSeason = status.seasons[selectedSeasonIndex];

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
            {status.seasons.map((season) => (
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
                        Episode {episode.episodeNum}
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
                              sx={{ color: theme.palette.error.main }}
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
      </DialogContent>
    </Dialog>
  );
};

export default MediaStatusDialog;
