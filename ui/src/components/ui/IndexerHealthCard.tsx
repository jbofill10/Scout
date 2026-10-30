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
import {
  describeQueue,
  useIndexerHealth,
  useRotateRelay,
  useTestIndexer,
  type IndexerQueueStats,
  type IndexerState,
  type RelayInfo,
} from "../../hooks/useIndexerHealth";
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

/** The few words about the relay that fit on the strip itself. */
function relaySummary(relay: RelayInfo): string {
  if (relay.error) return "Relay agent error";
  const parts = [relay.relay || "unknown relay"];
  if (relay.ip) parts.push(relay.ip);
  if (!relay.connected) parts.push("tunnel down");
  return `via ${parts.join(" · ")}`;
}

/** Queue entries worth a line: a search running, searches waiting, or a pause. A quiet setup stays quiet. */
function busyQueues(queue: IndexerQueueStats[] | undefined): IndexerQueueStats[] {
  return (queue ?? []).filter((q) => !!q.in_flight || q.queued > 0 || !!q.paused_until);
}

/** "1 searching · 3 waiting" across every indexer, for the strip itself. */
function queueSummary(queue: IndexerQueueStats[]): string {
  const searching = queue.filter((q) => q.in_flight).length;
  const waiting = queue.reduce((sum, q) => sum + q.queued, 0);
  const paused = queue.filter((q) => q.paused_until).length;
  const parts: string[] = [];
  if (searching) parts.push(`${searching} searching`);
  if (waiting) parts.push(`${waiting} waiting`);
  if (paused) parts.push(`${paused} paused`);
  return parts.join(" · ");
}

/** The full relay sentence, for the expanded details. */
function relayDetail(relay: RelayInfo, autoRotate: boolean): string {
  const base = relay.error
    ? `Relay agent: ${relay.error}`
    : `Exit via ${relay.relay || "unknown relay"}${relay.ip ? ` (${relay.ip})` : ""}${relay.location ? ` · ${relay.location}` : ""}${relay.connected ? "" : " · tunnel down"}`;
  const rotated = relay.last_rotation_at
    ? ` · rotated ${formatRelativeTime(relay.last_rotation_at)}${relay.last_rotation_reason ? ` because ${relay.last_rotation_reason}` : ""}`
    : "";
  return `${base}${rotated} · automatic rotation ${autoRotate ? "on" : "off"}`;
}

/**
 * IndexerHealthCard
 *
 * One strip: which indexers Prowlarr has disabled, which relay the host is
 * using, and a way to rotate it by hand. The relay details and recent events
 * sit behind a chevron so a healthy setup costs a single line. Hidden
 * entirely when the health endpoint has nothing (no Prowlarr), so a fresh
 * install does not show an empty box.
 */
export const IndexerHealthCard: React.FC = () => {
  const theme = useTheme();
  const { data, isError } = useIndexerHealth();
  const rotate = useRotateRelay();
  const [armed, setArmed] = useState(false);
  const [open, setOpen] = useState(false);

  useEffect(() => {
    if (!armed) return;
    const timer = window.setTimeout(() => setArmed(false), 4000);
    return () => window.clearTimeout(timer);
  }, [armed]);

  if (isError || !data || data.indexers.length === 0) return null;

  const relay = data.relay;
  const unhealthy = data.indexers.filter((i) => i.enabled && !i.healthy).length;
  const relayTrouble = relay.configured && (!!relay.error || !relay.connected);
  const busy = busyQueues(data.queue);
  const indexerName = (id: number) => data.indexers.find((i) => i.id === id)?.name ?? `Indexer ${id}`;

  return (
    <Box
      sx={{
        mb: 2.5,
        px: { xs: 1.5, md: 2 },
        py: 1,
        borderRadius: 2.5,
        backgroundColor: theme.palette.background.paper,
        border: `1px solid ${theme.palette.divider}`,
        borderLeft: `3px solid ${unhealthy > 0 ? theme.palette.error.main : theme.palette.success.main}`,
      }}
    >
      <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap" }}>
        <Typography variant="subtitle2" sx={{ mr: 0.5 }}>
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
        {busy.length > 0 && (
          <Typography variant="caption" sx={{ display: { xs: "none", sm: "block" }, color: theme.palette.text.secondary, whiteSpace: "nowrap" }}>
            {queueSummary(busy)}
          </Typography>
        )}
        {relay.configured && (
          <Typography
            variant="caption"
            sx={{
              display: { xs: "none", sm: "block" },
              color: relayTrouble ? theme.palette.error.main : theme.palette.text.secondary,
              whiteSpace: "nowrap",
            }}
          >
            {relaySummary(relay)}
          </Typography>
        )}
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
        <IconButton
          size="small"
          aria-label={open ? "Hide indexer details" : "Show indexer details"}
          aria-expanded={open}
          onClick={() => setOpen((v) => !v)}
          sx={{ transform: open ? "rotate(180deg)" : "none", transition: "transform .2s ease" }}
        >
          <ExpandMoreIcon fontSize="small" />
        </IconButton>
      </Box>

      <Collapse in={open} unmountOnExit>
        <Box sx={{ mt: 1, pt: 1, borderTop: `1px solid ${theme.palette.divider}` }}>
          {relay.configured ? (
            <Typography variant="caption" sx={{ display: "block", color: relayTrouble ? theme.palette.error.main : theme.palette.text.secondary }}>
              {relayDetail(relay, data.auto_rotate)}
            </Typography>
          ) : (
            <Typography variant="caption" sx={{ display: "block", color: theme.palette.text.disabled }}>
              No relay agent configured, so a blocked exit IP has to be fixed by hand (see tools/relay-agent).
            </Typography>
          )}
          {busy.length > 0 && (
            <Box sx={{ mt: 0.75 }}>
              {busy.map((q) => (
                <Typography key={q.id} variant="caption" sx={{ display: "block", color: q.paused_until ? theme.palette.warning.main : theme.palette.text.secondary }}>
                  {indexerName(q.id)}: {describeQueue(q)}
                </Typography>
              ))}
            </Box>
          )}
          {data.events.length > 0 && (
            <Box sx={{ mt: 0.75 }}>
              {data.events.slice(0, 12).map((ev, idx) => (
                <Typography key={`${ev.at}-${idx}`} variant="caption" sx={{ display: "block", color: ev.ok ? theme.palette.text.secondary : theme.palette.error.main }}>
                  {formatRelativeTime(ev.at)} · {ev.indexer ? `${ev.indexer}: ` : ""}{ev.message}
                </Typography>
              ))}
            </Box>
          )}
        </Box>
      </Collapse>
    </Box>
  );
};

export default IndexerHealthCard;
