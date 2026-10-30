import React, { useEffect, useState } from "react";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import CircularProgress from "@mui/material/CircularProgress";
import Collapse from "@mui/material/Collapse";
import IconButton from "@mui/material/IconButton";
import Tooltip from "@mui/material/Tooltip";
import Typography from "@mui/material/Typography";
import { alpha, useTheme } from "@mui/material/styles";
import CancelIcon from "@mui/icons-material/CancelOutlined";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";
import InboxIcon from "@mui/icons-material/MoveToInbox";
import ManageSearchIcon from "@mui/icons-material/ManageSearch";
import ReplayIcon from "@mui/icons-material/Replay";
import StageChip from "./StageChip";
import { useDownloadControls } from "../../hooks/useDownloadControls";
import { formatEpisodeLabel, getStageColor } from "../../utils/downloadStage";
import { STAGE_ORDER, summarizeStages, type ActivityGroup, type ActivityRowModel } from "../../utils/activityRows";

/** What the release picker needs to know about the item it is opened for. */
export interface PickerTarget {
  itemId: string;
  title: string;
  detail: string;
}

/** The long "Season 01 Episode 05" form, for dialogs and accessible names. */
function longLabel(row: ActivityRowModel): string {
  return row.category === "series"
    ? formatEpisodeLabel(row.season, row.episode, row.absoluteEpisode, row.isAnime)
    : "Movie";
}

const POSTER = { width: { xs: 28, sm: 32 }, height: { xs: 42, sm: 48 } };

const Poster: React.FC<{ url?: string }> = ({ url }) => {
  const theme = useTheme();
  return (
    <Box
      component={url ? "img" : "div"}
      src={url}
      alt=""
      sx={{
        ...POSTER,
        flexShrink: 0,
        objectFit: "cover",
        borderRadius: 1,
        border: `1px solid ${theme.palette.divider}`,
        backgroundColor: alpha(theme.palette.common.white, 0.05),
      }}
    />
  );
};

/**
 * Cancel needs a second tap: on a downloading item it deletes the torrent and
 * its files. The first tap arms the button for a few seconds.
 */
const CancelButton: React.FC<{ onConfirm: () => void; busy: boolean; title: string }> = ({ onConfirm, busy, title }) => {
  const theme = useTheme();
  const [armed, setArmed] = useState(false);

  useEffect(() => {
    if (!armed) return;
    const timer = window.setTimeout(() => setArmed(false), 4000);
    return () => window.clearTimeout(timer);
  }, [armed]);

  if (busy) {
    return <CircularProgress size={18} sx={{ m: 1 }} aria-label="Cancelling" />;
  }
  if (armed) {
    return (
      <Button
        size="small"
        color="error"
        variant="contained"
        onClick={() => {
          setArmed(false);
          onConfirm();
        }}
        sx={{ minWidth: 0, px: 1.25, py: 0.25 }}
      >
        Confirm
      </Button>
    );
  }
  return (
    <Tooltip title={`Cancel ${title}`}>
      <IconButton size="small" aria-label={`Cancel ${title}`} onClick={() => setArmed(true)} sx={{ color: theme.palette.text.secondary }}>
        <CancelIcon fontSize="small" />
      </IconButton>
    </Tooltip>
  );
};

/** The per-item actions: retry now, pick a release, cancel. */
const RowActions: React.FC<{ row: ActivityRowModel; onPick: (target: PickerTarget) => void }> = ({ row, onPick }) => {
  const theme = useTheme();
  const controls = useDownloadControls();
  const detail = longLabel(row);
  const label = `${row.title} ${detail}`.trim();

  if (!row.canCancel && !row.canRetry && !row.canPick) return null;

  return (
    <Box sx={{ display: "flex", alignItems: "center", gap: 0.25 }}>
      {row.canRetry && (
        <Tooltip title="Retry now">
          <span>
            <IconButton
              size="small"
              aria-label={`Retry ${label} now`}
              disabled={controls.isRetrying(row.itemId)}
              onClick={() => controls.retryNow({ itemId: row.itemId, title: label })}
              sx={{ color: theme.palette.text.secondary }}
            >
              {controls.isRetrying(row.itemId) ? <CircularProgress size={18} /> : <ReplayIcon fontSize="small" />}
            </IconButton>
          </span>
        </Tooltip>
      )}
      {row.canPick && (
        <Tooltip title="Pick a release">
          <IconButton
            size="small"
            aria-label={`Pick a release for ${label}`}
            onClick={() => onPick({ itemId: row.itemId, title: row.title, detail })}
            sx={{ color: theme.palette.text.secondary }}
          >
            <ManageSearchIcon fontSize="small" />
          </IconButton>
        </Tooltip>
      )}
      {row.canCancel && (
        <CancelButton
          title={label}
          busy={controls.isCancelling(row.itemId)}
          onConfirm={() => controls.cancel({ itemId: row.itemId, title: label })}
        />
      )}
    </Box>
  );
};

/** Proportional bar of a group's stages, in STAGE_ORDER. */
const StageBar: React.FC<{ counts: ActivityGroup["stageCounts"] }> = ({ counts }) => (
  <Box sx={{ display: "flex", gap: "2px", width: 112, height: 5, borderRadius: 999, overflow: "hidden" }} aria-hidden>
    {STAGE_ORDER.filter((stage) => counts[stage]).map((stage) => (
      <Box key={stage} sx={{ flex: counts[stage], backgroundColor: getStageColor(stage) }} />
    ))}
  </Box>
);

/** "● 3 downloading  ● 5 searching" with the stage colours as dots. */
const StageCounts: React.FC<{ counts: ActivityGroup["stageCounts"] }> = ({ counts }) => {
  const theme = useTheme();
  return (
    <Box sx={{ display: "flex", gap: 1.25, flexWrap: "wrap", justifyContent: "flex-end" }}>
      {STAGE_ORDER.filter((stage) => counts[stage]).map((stage) => (
        <Box
          key={stage}
          component="span"
          sx={{
            display: "inline-flex",
            alignItems: "center",
            gap: 0.5,
            fontSize: "0.75rem",
            color: theme.palette.text.secondary,
            whiteSpace: "nowrap",
            fontVariantNumeric: "tabular-nums",
          }}
        >
          <Box component="span" sx={{ width: 7, height: 7, borderRadius: "50%", backgroundColor: getStageColor(stage) }} />
          {counts[stage]} {stage}
        </Box>
      ))}
    </Box>
  );
};

/**
 * One-line row frame. On a phone the time column goes and the status wraps
 * under the title; from tablet width up everything sits on one line.
 */
const RowShell: React.FC<{
  lead: React.ReactNode;
  primary: React.ReactNode;
  status: React.ReactNode;
  meta: React.ReactNode;
  actions: React.ReactNode;
  inset?: boolean;
}> = ({ lead, primary, status, meta, actions, inset }) => {
  const theme = useTheme();
  return (
    <Box
      sx={{
        display: "grid",
        alignItems: "center",
        columnGap: 1.5,
        rowGap: 0.25,
        px: { xs: 1.5, sm: 2 },
        py: inset ? 0.625 : 0.875,
        minHeight: inset ? 40 : 52,
        borderTop: `1px solid ${theme.palette.divider}`,
        backgroundColor: inset ? alpha(theme.palette.common.white, 0.015) : "transparent",
        gridTemplateColumns: { xs: "auto minmax(0, 1fr) auto", sm: "auto minmax(0, 1fr) minmax(0, 1.4fr) auto auto" },
        gridTemplateAreas: {
          xs: `"lead primary actions" "lead status status"`,
          sm: `"lead primary status meta actions"`,
        },
      }}
    >
      <Box sx={{ gridArea: "lead", display: "flex", alignItems: "center" }}>{lead}</Box>
      <Box sx={{ gridArea: "primary", display: "flex", alignItems: "center", gap: 1, minWidth: 0 }}>{primary}</Box>
      <Box sx={{ gridArea: "status", minWidth: 0 }}>{status}</Box>
      <Box sx={{ gridArea: "meta", display: { xs: "none", sm: "flex" }, alignItems: "center", gap: 1.25, justifyContent: "flex-end" }}>
        {meta}
      </Box>
      <Box sx={{ gridArea: "actions", justifySelf: "end" }}>{actions}</Box>
    </Box>
  );
};

/** The "what now?" line plus, on phones, the relative time it would otherwise lose. */
const StatusText: React.FC<{ row: ActivityRowModel }> = ({ row }) => {
  const theme = useTheme();
  const color =
    row.tone === "error" ? theme.palette.error.main : row.tone === "warning" ? getStageColor("searching") : theme.palette.text.secondary;
  return (
    <Typography variant="body2" title={row.status} sx={{ color, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
      {row.status}
      {row.updated && (
        <Box component="span" sx={{ display: { xs: "inline", sm: "none" }, color: theme.palette.text.disabled }}>
          {` · ${row.updated}`}
        </Box>
      )}
    </Typography>
  );
};

const RowMeta: React.FC<{ row: ActivityRowModel }> = ({ row }) => {
  const theme = useTheme();
  return (
    <>
      {row.traceId && (
        <Tooltip title="Trace ID — use it to find this request in SigNoz">
          <Typography
            variant="caption"
            sx={{ display: { xs: "none", md: "block" }, fontFamily: "monospace", fontSize: "0.6875rem", color: theme.palette.text.disabled, cursor: "default" }}
          >
            {row.traceId.slice(0, 8)}
          </Typography>
        </Tooltip>
      )}
      {row.updated && (
        <Typography variant="caption" sx={{ color: theme.palette.text.secondary, whiteSpace: "nowrap", minWidth: 52, textAlign: "right" }}>
          {row.updated}
        </Typography>
      )}
    </>
  );
};

/** A show or movie with a single row: everything on one line, no disclosure. */
const LeafRow: React.FC<{ row: ActivityRowModel; onPick: (target: PickerTarget) => void }> = ({ row, onPick }) => {
  const theme = useTheme();
  return (
    <RowShell
      lead={<Poster url={row.posterUrl} />}
      primary={
        <>
          <Typography variant="body2" sx={{ fontWeight: 650, color: theme.palette.text.primary, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
            {row.title}
          </Typography>
          <Typography variant="caption" sx={{ color: theme.palette.text.disabled, whiteSpace: "nowrap" }}>
            {row.code}
          </Typography>
          <StageChip stage={row.stage} />
        </>
      }
      status={<StatusText row={row} />}
      meta={<RowMeta row={row} />}
      actions={<RowActions row={row} onPick={onPick} />}
    />
  );
};

/** One episode inside an opened group. */
const ChildRow: React.FC<{ row: ActivityRowModel; onPick: (target: PickerTarget) => void }> = ({ row, onPick }) => {
  const theme = useTheme();
  return (
    <RowShell
      inset
      lead={
        <Box sx={{ width: POSTER.width, display: "flex", justifyContent: "center" }}>
          <Box sx={{ width: 3, height: 18, borderRadius: 999, backgroundColor: alpha(getStageColor(row.stage), 0.7) }} />
        </Box>
      }
      primary={
        <>
          <Typography variant="body2" sx={{ fontWeight: 600, color: theme.palette.text.primary, whiteSpace: "nowrap", fontVariantNumeric: "tabular-nums" }}>
            {row.code}
          </Typography>
          <StageChip stage={row.stage} />
        </>
      }
      status={<StatusText row={row} />}
      meta={<RowMeta row={row} />}
      actions={<RowActions row={row} onPick={onPick} />}
    />
  );
};

/** A show with several rows: one line summarising them, opening to the episodes. */
const GroupRow: React.FC<{
  group: ActivityGroup;
  open: boolean;
  onToggle: () => void;
  onPick: (target: PickerTarget) => void;
}> = ({ group, open, onToggle, onPick }) => {
  const theme = useTheme();
  const count = `${group.rows.length} ${group.category === "series" ? "episodes" : "items"}`;
  const detail = [group.seasonLabel, count].filter(Boolean).join(" · ");
  const leadLine = `${group.lead.code} ${group.lead.status}`;

  return (
    <Box sx={{ borderTop: `1px solid ${theme.palette.divider}` }}>
      <Box
        role="button"
        tabIndex={0}
        aria-expanded={open}
        onClick={onToggle}
        onKeyDown={(event) => {
          if (event.key === "Enter" || event.key === " ") {
            event.preventDefault();
            onToggle();
          }
        }}
        sx={{
          display: "grid",
          alignItems: "center",
          columnGap: 1.5,
          px: { xs: 1, sm: 1.5 },
          py: 0.875,
          minHeight: 56,
          cursor: "pointer",
          userSelect: "none",
          gridTemplateColumns: { xs: "auto auto minmax(0, 1fr) auto", sm: "auto auto minmax(0, 1fr) auto auto" },
          transition: "background-color .15s ease",
          "&:hover": { backgroundColor: alpha(theme.palette.common.white, 0.03) },
          "&:focus-visible": { outline: `2px solid ${theme.palette.primary.main}`, outlineOffset: -2 },
        }}
      >
        <ExpandMoreIcon
          fontSize="small"
          sx={{ color: theme.palette.text.secondary, transform: open ? "rotate(0deg)" : "rotate(-90deg)", transition: "transform .2s ease" }}
        />
        <Poster url={group.posterUrl} />
        <Box sx={{ minWidth: 0 }}>
          <Typography variant="body2" sx={{ fontWeight: 650, color: theme.palette.text.primary, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
            {group.title}
          </Typography>
          {/* Phones have no room for the counts column, so the stages go here */}
          <Typography
            variant="caption"
            sx={{ display: { xs: "block", sm: "none" }, color: theme.palette.text.secondary, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}
          >
            {summarizeStages(group.stageCounts)}
          </Typography>
          <Typography
            variant="caption"
            title={`${detail} · ${leadLine}`}
            sx={{ display: { xs: "none", sm: "block" }, color: theme.palette.text.secondary, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}
          >
            {detail}
            <Box component="span" sx={{ color: theme.palette.text.disabled }}>{` · ${leadLine}`}</Box>
          </Typography>
        </Box>
        <Box sx={{ display: { xs: "none", sm: "flex" }, flexDirection: "column", alignItems: "flex-end", gap: 0.625 }}>
          <StageCounts counts={group.stageCounts} />
          <Box sx={{ display: { xs: "none", md: "block" } }}>
            <StageBar counts={group.stageCounts} />
          </Box>
        </Box>
        <Typography
          variant="caption"
          sx={{ color: theme.palette.text.secondary, whiteSpace: "nowrap", minWidth: { xs: 0, sm: 52 }, textAlign: "right" }}
        >
          {group.lead.updated}
        </Typography>
      </Box>

      <Collapse in={open} unmountOnExit>
        {group.rows.map((row) => (
          <ChildRow key={row.key} row={row} onPick={onPick} />
        ))}
      </Collapse>
    </Box>
  );
};

export interface ActivityListProps {
  groups: ActivityGroup[];
  /** Keys of the groups currently opened. */
  expanded: ReadonlySet<string>;
  onToggle: (key: string) => void;
  onPick: (target: PickerTarget) => void;
  /** Groups shown before the "Show all" control; omit to show every group. */
  limit?: number;
  showAll?: boolean;
  onToggleShowAll?: () => void;
  emptyTitle: string;
  emptyBody: string;
}

/**
 * ActivityList
 *
 * The grouped download list behind each Activity tab. A show with several
 * tracked episodes is one line (title, season, stage counts, the latest
 * thing that happened) until opened; a movie or a lone episode is a single
 * line with its actions. Rows are 40–56px so a screen holds a dozen shows
 * instead of four episodes.
 */
export const ActivityList: React.FC<ActivityListProps> = ({
  groups,
  expanded,
  onToggle,
  onPick,
  limit,
  showAll = false,
  onToggleShowAll,
  emptyTitle,
  emptyBody,
}) => {
  const theme = useTheme();
  const visible = limit !== undefined && !showAll ? groups.slice(0, limit) : groups;
  const hidden = groups.length - visible.length;

  return (
    <Box
      sx={{
        backgroundColor: theme.palette.background.paper,
        border: `1px solid ${theme.palette.divider}`,
        borderRadius: 3,
        overflow: "hidden",
        // Each row draws its own top border; the first one would double the card edge.
        "& > :first-of-type": { borderTop: 0 },
      }}
    >
      {groups.length === 0 && (
        <Box sx={{ textAlign: "center", py: 6, px: 2 }}>
          <InboxIcon sx={{ fontSize: 32, color: theme.palette.text.disabled, mb: 0.5 }} />
          <Typography variant="subtitle1" sx={{ color: theme.palette.text.primary }}>
            {emptyTitle}
          </Typography>
          <Typography variant="body2" sx={{ color: theme.palette.text.secondary }}>
            {emptyBody}
          </Typography>
        </Box>
      )}

      {visible.map((group) =>
        group.rows.length === 1 ? (
          <LeafRow key={group.key} row={group.rows[0]} onPick={onPick} />
        ) : (
          <GroupRow key={group.key} group={group} open={expanded.has(group.key)} onToggle={() => onToggle(group.key)} onPick={onPick} />
        ),
      )}

      {(hidden > 0 || (showAll && limit !== undefined && groups.length > limit)) && (
        <Box sx={{ px: 2, py: 1, borderTop: `1px solid ${theme.palette.divider}` }}>
          <Button size="small" onClick={onToggleShowAll}>
            {showAll ? "Show fewer" : `Show all ${groups.length}`}
          </Button>
        </Box>
      )}
    </Box>
  );
};

export default ActivityList;
