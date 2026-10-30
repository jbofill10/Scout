import React from "react";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Chip from "@mui/material/Chip";
import Dialog from "@mui/material/Dialog";
import DialogActions from "@mui/material/DialogActions";
import DialogContent from "@mui/material/DialogContent";
import DialogTitle from "@mui/material/DialogTitle";
import IconButton from "@mui/material/IconButton";
import LinearProgress from "@mui/material/LinearProgress";
import Tooltip from "@mui/material/Tooltip";
import Typography from "@mui/material/Typography";
import useMediaQuery from "@mui/material/useMediaQuery";
import { alpha, useTheme } from "@mui/material/styles";
import CloseIcon from "@mui/icons-material/Close";
import CheckCircleIcon from "@mui/icons-material/CheckCircle";
import BlockIcon from "@mui/icons-material/Block";
import DownloadIcon from "@mui/icons-material/Download";
import HistoryIcon from "@mui/icons-material/History";
import { useReleases, type ReleaseCandidate } from "../../hooks/useDownloadControls";
import { formatRelativeTime } from "../../utils/downloadStage";
import { formatBytes } from "../../utils/formatBytes";

export interface ReleasePickerDialogProps {
  /** The item's tvdb id; null keeps the dialog idle. */
  itemId: string | null;
  title: string;
  /** "Season 01 Episode 04" or "Movie". */
  detail?: string;
  open: boolean;
  onClose: () => void;
  onGrab: (candidate: ReleaseCandidate) => void;
  grabbing?: boolean;
}

const CandidateRow: React.FC<{
  candidate: ReleaseCandidate;
  onGrab: () => void;
  disabled: boolean;
}> = ({ candidate, onGrab, disabled }) => {
  const theme = useTheme();
  const verdictColor = candidate.accepted ? theme.palette.success.main : theme.palette.text.disabled;

  return (
    <Box
      data-testid="release-candidate"
      sx={{
        display: "flex",
        gap: 1.5,
        alignItems: "flex-start",
        px: { xs: 0, sm: 1 },
        py: 1.5,
        borderBottom: `1px solid ${theme.palette.divider}`,
        "&:last-of-type": { borderBottom: 0 },
        opacity: candidate.accepted ? 1 : 0.8,
      }}
    >
      <Box sx={{ pt: 0.25, color: verdictColor, flexShrink: 0 }} aria-hidden>
        {candidate.accepted ? <CheckCircleIcon fontSize="small" /> : <BlockIcon fontSize="small" />}
      </Box>

      <Box sx={{ flex: 1, minWidth: 0 }}>
        <Typography
          variant="body2"
          sx={{ fontWeight: 600, color: theme.palette.text.primary, wordBreak: "break-word" }}
        >
          {candidate.title}
        </Typography>

        <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.75, mt: 0.75, alignItems: "center" }}>
          {candidate.quality && <Chip size="small" label={candidate.quality} />}
          {candidate.source && <Chip size="small" variant="outlined" label={candidate.source} />}
          {candidate.codec && <Chip size="small" variant="outlined" label={candidate.codec} />}
          <Typography variant="caption" sx={{ color: theme.palette.text.secondary }}>
            {[
              formatBytes(candidate.size),
              `${candidate.seeders} seed${candidate.seeders === 1 ? "" : "s"}`,
              candidate.indexer,
              candidate.publish_date ? formatRelativeTime(candidate.publish_date) : "",
            ]
              .filter(Boolean)
              .join(" · ")}
          </Typography>
          {candidate.previously_failed && (
            <Tooltip title="Scout already tried this release for this item and gave up on it">
              <Chip
                size="small"
                icon={<HistoryIcon />}
                label="Tried before"
                sx={{ backgroundColor: alpha(theme.palette.warning.main, 0.16), color: theme.palette.warning.main }}
              />
            </Tooltip>
          )}
        </Box>

        <Typography
          variant="caption"
          sx={{ display: "block", mt: 0.5, color: candidate.accepted ? theme.palette.success.main : theme.palette.text.secondary }}
        >
          {candidate.accepted
            ? `Scout would take this (score ${candidate.score.toFixed(2)})`
            : `Passed over: ${candidate.reason || "did not match"}`}
        </Typography>
      </Box>

      <Button
        size="small"
        variant={candidate.accepted ? "contained" : "outlined"}
        startIcon={<DownloadIcon />}
        onClick={onGrab}
        disabled={disabled}
        sx={{ flexShrink: 0, alignSelf: "center" }}
      >
        Download
      </Button>
    </Box>
  );
};

/**
 * ReleasePickerDialog
 *
 * Shows every release the indexers have for one episode or movie, with the
 * verdict Scout's automatic picker would reach on each, and lets the user
 * download one by hand. Accepted releases are listed first.
 */
export const ReleasePickerDialog: React.FC<ReleasePickerDialogProps> = ({
  itemId,
  title,
  detail,
  open,
  onClose,
  onGrab,
  grabbing = false,
}) => {
  const theme = useTheme();
  const fullScreen = useMediaQuery(theme.breakpoints.down("md"));
  const { data, isLoading, isError, error, refetch, isFetching } = useReleases(open ? itemId : null);

  const candidates = data?.candidates ?? [];
  const accepted = candidates.filter((c) => c.accepted).length;

  return (
    <Dialog open={open} onClose={onClose} fullScreen={fullScreen} fullWidth maxWidth="md">
      <DialogTitle sx={{ pr: 7 }}>
        <Typography component="span" variant="h6" sx={{ display: "block" }}>
          Pick a release
        </Typography>
        <Typography variant="body2" sx={{ color: theme.palette.text.secondary }}>
          {title}
          {detail ? ` · ${detail}` : ""}
        </Typography>
        <IconButton aria-label="Close" onClick={onClose} sx={{ position: "absolute", right: 12, top: 12 }}>
          <CloseIcon />
        </IconButton>
      </DialogTitle>

      <DialogContent dividers sx={{ px: { xs: 2, sm: 3 } }}>
        {isLoading && (
          <Box sx={{ py: 4, textAlign: "center" }}>
            <LinearProgress sx={{ mb: 2, borderRadius: 999 }} />
            <Typography variant="body2" sx={{ color: theme.palette.text.secondary }}>
              Asking every indexer… this can take a minute.
            </Typography>
          </Box>
        )}

        {isError && !isLoading && (
          <Box sx={{ py: 4, textAlign: "center" }}>
            <Typography variant="subtitle1" sx={{ color: theme.palette.text.primary }}>
              Couldn't search for releases
            </Typography>
            <Typography variant="body2" sx={{ color: theme.palette.text.secondary, mb: 2 }}>
              {error instanceof Error ? error.message : "Scout couldn't reach the indexers."}
            </Typography>
            <Button variant="outlined" size="small" onClick={() => refetch()}>
              Try again
            </Button>
          </Box>
        )}

        {!isLoading && !isError && candidates.length === 0 && (
          <Box sx={{ py: 4, textAlign: "center" }}>
            <Typography variant="subtitle1" sx={{ color: theme.palette.text.primary }}>
              No releases found
            </Typography>
            <Typography variant="body2" sx={{ color: theme.palette.text.secondary }}>
              {data ? `${data.queries} searches came back empty.` : ""}
            </Typography>
          </Box>
        )}

        {!isLoading && !isError && candidates.length > 0 && (
          <>
            <Typography variant="caption" sx={{ display: "block", mb: 1, color: theme.palette.text.secondary }}>
              {candidates.length} release{candidates.length === 1 ? "" : "s"} from {data?.queries ?? 0} searches,{" "}
              {accepted} Scout would accept. Downloading one replaces whatever Scout had in flight for this item.
            </Typography>
            {candidates.map((candidate) => (
              <CandidateRow
                key={candidate.info_hash || candidate.guid || candidate.title}
                candidate={candidate}
                onGrab={() => onGrab(candidate)}
                disabled={grabbing || isFetching}
              />
            ))}
          </>
        )}
      </DialogContent>

      <DialogActions>
        <Button onClick={onClose}>Close</Button>
      </DialogActions>
    </Dialog>
  );
};

export default ReleasePickerDialog;
