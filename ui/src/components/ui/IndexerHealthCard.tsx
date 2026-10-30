import React, { useEffect, useState } from "react";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Chip from "@mui/material/Chip";
import CircularProgress from "@mui/material/CircularProgress";
import Collapse from "@mui/material/Collapse";
import IconButton from "@mui/material/IconButton";
import Tooltip from "@mui/material/Tooltip";
import Typography from "@mui/material/Typography";
import { alpha, useTheme } from "@mui/material/styles";
import CheckCircleIcon from "@mui/icons-material/CheckCircle";
import ErrorIcon from "@mui/icons-material/Error";
import SwapHorizIcon from "@mui/icons-material/SwapHoriz";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";
import { useIndexerHealth, useRotateRelay, useTestIndexer, type IndexerState } from "../../hooks/useIndexerHealth";
import { formatRelativeTime } from "../../utils/downloadStage";

const IndexerChip: React.FC<{ indexer: IndexerState }> = ({ indexer }) => {
  const theme = useTheme();
  const test = useTestIndexer();
  if (!indexer.enabled) {
    return <Chip size="small" variant="outlined" label={`${indexer.name} · off`} sx={{ opacity: 0.6 }} />;
  }
  if (indexer.healthy) {
    return (
      <Chip
        size="small"
        icon={<CheckCircleIcon />}
        label={indexer.name}
        sx={{ backgroundColor: alpha(theme.palette.success.main, 0.16), color: theme.palette.success.main, "& .MuiChip-icon": { color: "inherit" } }}
      />
    );
  }
  return (
    <Tooltip title={`${indexer.failure ?? "Disabled"}${indexer.ip_blocked ? " · looks like the exit IP is blocked" : ""}. Click to re-test.`}>
      <Chip
        size="small"
        icon={test.isPending ? <CircularProgress size={14} color="inherit" /> : <ErrorIcon />}
        label={`${indexer.name} · disabled`}
        onClick={() => test.mutate(indexer)}
        disabled={test.isPending}
        sx={{ backgroundColor: alpha(theme.palette.error.main, 0.16), color: theme.palette.error.main, "& .MuiChip-icon": { color: "inherit" } }}
      />
    </Tooltip>
  );
};

/**
 * IndexerHealthCard
 *
 * Which indexers Prowlarr has disabled, which relay the host is using, and a
 * way to rotate it by hand. Hidden entirely when the health endpoint has
 * nothing (no Prowlarr), so a fresh install does not show an empty box.
 */
export const IndexerHealthCard: React.FC = () => {
  const theme = useTheme();
  const { data, isError } = useIndexerHealth();
  const rotate = useRotateRelay();
  const [armed, setArmed] = useState(false);
  const [showEvents, setShowEvents] = useState(false);

  useEffect(() => {
    if (!armed) return;
    const timer = window.setTimeout(() => setArmed(false), 4000);
    return () => window.clearTimeout(timer);
  }, [armed]);

  if (isError || !data || data.indexers.length === 0) return null;

  const relay = data.relay;
  const unhealthy = data.indexers.filter((i) => i.enabled && !i.healthy).length;

  return (
    <Box
      sx={{
        mb: 4,
        px: { xs: 2, md: 2.5 },
        py: 2,
        borderRadius: 3,
        backgroundColor: theme.palette.background.paper,
        border: `1px solid ${theme.palette.divider}`,
        borderLeft: `3px solid ${unhealthy > 0 ? theme.palette.error.main : theme.palette.success.main}`,
      }}
    >
      <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, flexWrap: "wrap" }}>
        <Typography variant="subtitle1" sx={{ fontWeight: 650, mr: 0.5 }}>
          Indexers
        </Typography>
        {data.indexers.map((i) => (
          <IndexerChip key={i.id} indexer={i} />
        ))}
        {data.check_error && (
          <Typography variant="caption" sx={{ color: theme.palette.error.main }}>
            Prowlarr unreachable: {data.check_error}
          </Typography>
        )}
        <Box sx={{ flex: 1 }} />
        {relay.configured && (
          armed ? (
            <Button size="small" color="warning" variant="contained" onClick={() => { setArmed(false); rotate.mutate(); }} disabled={rotate.isPending}>
              Confirm rotate
            </Button>
          ) : (
            <Button
              size="small"
              variant="outlined"
              startIcon={rotate.isPending ? <CircularProgress size={16} /> : <SwapHorizIcon />}
              onClick={() => setArmed(true)}
              disabled={rotate.isPending}
            >
              Rotate relay
            </Button>
          )
        )}
      </Box>

      <Box sx={{ display: "flex", alignItems: "center", gap: 1, mt: 1, flexWrap: "wrap" }}>
        {relay.configured ? (
          <Typography variant="caption" sx={{ color: relay.error ? theme.palette.error.main : theme.palette.text.secondary }}>
            {relay.error
              ? `Relay agent: ${relay.error}`
              : `Exit via ${relay.relay || "unknown relay"}${relay.ip ? ` (${relay.ip})` : ""}${relay.location ? ` · ${relay.location}` : ""}${relay.connected ? "" : " · tunnel down"}`}
            {relay.last_rotation_at ? ` · rotated ${formatRelativeTime(relay.last_rotation_at)}${relay.last_rotation_reason ? ` because ${relay.last_rotation_reason}` : ""}` : ""}
            {` · automatic rotation ${data.auto_rotate ? "on" : "off"}`}
          </Typography>
        ) : (
          <Typography variant="caption" sx={{ color: theme.palette.text.disabled }}>
            No relay agent configured, so a blocked exit IP has to be fixed by hand (see tools/relay-agent).
          </Typography>
        )}
        {data.events.length > 0 && (
          <IconButton
            size="small"
            aria-label={showEvents ? "Hide recent events" : "Show recent events"}
            aria-expanded={showEvents}
            onClick={() => setShowEvents((v) => !v)}
            sx={{ ml: "auto", transform: showEvents ? "rotate(180deg)" : "none", transition: "transform .2s ease" }}
          >
            <ExpandMoreIcon fontSize="small" />
          </IconButton>
        )}
      </Box>

      <Collapse in={showEvents} unmountOnExit>
        <Box sx={{ mt: 1.5, borderTop: `1px solid ${theme.palette.divider}`, pt: 1 }}>
          {data.events.slice(0, 12).map((ev, idx) => (
            <Typography key={`${ev.at}-${idx}`} variant="caption" sx={{ display: "block", color: ev.ok ? theme.palette.text.secondary : theme.palette.error.main }}>
              {formatRelativeTime(ev.at)} · {ev.indexer ? `${ev.indexer}: ` : ""}{ev.message}
            </Typography>
          ))}
        </Box>
      </Collapse>
    </Box>
  );
};

export default IndexerHealthCard;
