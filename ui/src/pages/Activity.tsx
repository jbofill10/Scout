import React, { useEffect, useState } from "react";
import Box from "@mui/material/Box";
import Container from "@mui/material/Container";
import Typography from "@mui/material/Typography";
import Button from "@mui/material/Button";
import IconButton from "@mui/material/IconButton";
import Skeleton from "@mui/material/Skeleton";
import Tooltip from "@mui/material/Tooltip";
import LinearProgress from "@mui/material/LinearProgress";
import CircularProgress from "@mui/material/CircularProgress";
import RefreshIcon from "@mui/icons-material/Refresh";
import InboxIcon from "@mui/icons-material/MoveToInbox";
import CancelIcon from "@mui/icons-material/CancelOutlined";
import ReplayIcon from "@mui/icons-material/Replay";
import ManageSearchIcon from "@mui/icons-material/ManageSearch";
import { alpha, useTheme } from "@mui/material/styles";
import PageHeader from "../components/ui/PageHeader";
import StageChip from "../components/ui/StageChip";
import ReleasePickerDialog from "../components/ui/ReleasePickerDialog";
import { useActivity, type ActivityItem } from "../hooks/useActivity";
import { useUpcoming, type UpcomingItem } from "../hooks/useUpcoming";
import { toSelection, useDownloadControls, type ReleaseCandidate } from "../hooks/useDownloadControls";
import {
  formatEpisodeLabel,
  formatRelativeTime,
  formatTimeUntil,
  getStageColor,
  getStageDescription,
  type DownloadStage,
} from "../utils/downloadStage";

/** The order a download travels through, used to draw the progress track. */
const PIPELINE: DownloadStage[] = ["scheduled", "searching", "downloading", "completed"];

/** Stage tiles across the top, in the order work flows. */
const SUMMARY_STAGES: DownloadStage[] = [
  "downloading",
  "searching",
  "scheduled",
  "failed",
];

function itemKey(item: ActivityItem): string {
  return `${item.id}-${item.tvdb_id}`;
}

/**
 * Horizontal track showing how far an item has moved through the pipeline.
 * A failed item stops at the stage it reached and turns red, so a glance
 * answers "how far did it get before it broke?".
 */
const StageTrack: React.FC<{ stage: DownloadStage }> = ({ stage }) => {
  const theme = useTheme();
  const failed = stage === "failed";
  // A failure is reported at whichever stage it happened; without a per-stage
  // record, treat it as having reached the search step. A cancelled item never
  // got anywhere worth showing.
  const reachedIndex = failed ? 1 : stage === "cancelled" ? -1 : PIPELINE.indexOf(stage);
  const color = getStageColor(stage);

  return (
    <Box sx={{ display: "flex", gap: 0.5, mt: 1.25 }} aria-hidden>
      {PIPELINE.map((step, index) => (
        <Box
          key={step}
          sx={{
            height: 3,
            flex: 1,
            borderRadius: 999,
            backgroundColor:
              index <= reachedIndex ? color : alpha(theme.palette.common.white, 0.08),
          }}
        />
      ))}
    </Box>
  );
};

/** What the release picker needs to know about the item it is opened for. */
interface PickerTarget {
  itemId: string;
  title: string;
  detail: string;
}

function describeItem(category: "series" | "movie", season?: number, episode?: number, absolute?: number, anime?: boolean) {
  return category === "series" ? formatEpisodeLabel(season, episode, absolute, anime) : "Movie";
}

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
        sx={{ minWidth: 0, px: 1.25 }}
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

/** The per-item actions, shared by in-flight, finished and upcoming rows. */
const RowActions: React.FC<{
  itemId: string;
  title: string;
  detail: string;
  canCancel: boolean;
  canRetry: boolean;
  canPick: boolean;
  onPick: (target: PickerTarget) => void;
}> = ({ itemId, title, detail, canCancel, canRetry, canPick, onPick }) => {
  const theme = useTheme();
  const controls = useDownloadControls();
  const label = `${title} ${detail}`.trim();

  if (!canCancel && !canRetry && !canPick) return null;

  return (
    <Box sx={{ display: "flex", alignItems: "center", gap: 0.25 }}>
      {canRetry && (
        <Tooltip title="Retry now">
          <span>
            <IconButton
              size="small"
              aria-label={`Retry ${label} now`}
              disabled={controls.isRetrying(itemId)}
              onClick={() => controls.retryNow({ itemId, title: label })}
              sx={{ color: theme.palette.text.secondary }}
            >
              {controls.isRetrying(itemId) ? <CircularProgress size={18} /> : <ReplayIcon fontSize="small" />}
            </IconButton>
          </span>
        </Tooltip>
      )}
      {canPick && (
        <Tooltip title="Pick a release">
          <IconButton
            size="small"
            aria-label={`Pick a release for ${label}`}
            onClick={() => onPick({ itemId, title, detail })}
            sx={{ color: theme.palette.text.secondary }}
          >
            <ManageSearchIcon fontSize="small" />
          </IconButton>
        </Tooltip>
      )}
      {canCancel && (
        <CancelButton
          title={label}
          busy={controls.isCancelling(itemId)}
          onConfirm={() => controls.cancel({ itemId, title: label })}
        />
      )}
    </Box>
  );
};

/** One tracked episode or movie. */
const ActivityRow: React.FC<{ item: ActivityItem; onPick: (target: PickerTarget) => void }> = ({ item, onPick }) => {
  const theme = useTheme();
  const isFailed = item.status === "failed";
  const detail = describeItem(item.category, item.season, item.episode, item.absolute_episode, item.is_anime);

  // The most useful "what now?" line, in priority order: why it failed, when
  // it will be retried, when it is due to release, or what stage it is in.
  const retryLine = (() => {
    if (item.next_attempt_at && !isFailed) {
      const attempt = item.attempts ? ` (attempt ${item.attempts + 1})` : "";
      return `Retrying ${formatTimeUntil(item.next_attempt_at)}${attempt}`;
    }
    if (item.status === "scheduled" && item.release_time) {
      return `Releases ${formatTimeUntil(item.release_time)}`;
    }
    return getStageDescription(item.status);
  })();

  return (
    <Box
      sx={{
        display: "flex",
        gap: 2,
        px: { xs: 2, sm: 2.5 },
        py: 2,
        borderBottom: `1px solid ${theme.palette.divider}`,
        "&:last-of-type": { borderBottom: 0 },
      }}
    >
      {item.poster_url ? (
        <Box
          component="img"
          src={item.poster_url}
          alt=""
          sx={{
            width: 48,
            height: 72,
            flexShrink: 0,
            objectFit: "cover",
            borderRadius: 1.5,
            border: `1px solid ${theme.palette.divider}`,
          }}
        />
      ) : (
        <Box
          sx={{
            width: 48,
            height: 72,
            flexShrink: 0,
            borderRadius: 1.5,
            backgroundColor: alpha(theme.palette.common.white, 0.05),
          }}
        />
      )}

      <Box sx={{ flex: 1, minWidth: 0 }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, flexWrap: "wrap" }}>
          <Typography
            variant="body1"
            sx={{
              fontWeight: 650,
              color: theme.palette.text.primary,
              overflow: "hidden",
              textOverflow: "ellipsis",
              whiteSpace: "nowrap",
              maxWidth: { xs: "100%", sm: 420 },
            }}
          >
            {item.media_title}
          </Typography>
          <StageChip stage={item.status} />
          <Typography variant="caption" sx={{ color: theme.palette.text.disabled }}>
            {detail}
          </Typography>
          {/* Phones drop the right-hand column, so the time moves in here */}
          <Typography
            variant="caption"
            sx={{ display: { xs: "inline", sm: "none" }, color: theme.palette.text.disabled }}
          >
            {formatRelativeTime(item.updated_at)}
          </Typography>
        </Box>

        <Typography
          variant="body2"
          sx={{
            mt: 0.5,
            color: isFailed ? theme.palette.error.main : theme.palette.text.secondary,
          }}
        >
          {item.reason || retryLine}
        </Typography>

        <StageTrack stage={item.status} />

        {/* Phones: actions sit under the text */}
        <Box sx={{ display: { xs: "flex", sm: "none" }, mt: 1, justifyContent: "flex-end" }}>
          <RowActions
            itemId={item.tvdb_id}
            title={item.media_title}
            detail={detail}
            canCancel={!!item.can_cancel}
            canRetry={!!item.can_retry}
            canPick={!!item.can_pick_release}
            onPick={onPick}
          />
        </Box>
      </Box>

      <Box sx={{ flexShrink: 0, textAlign: "right", display: { xs: "none", sm: "block" } }}>
        <Box sx={{ display: "flex", justifyContent: "flex-end", mb: 0.5 }}>
          <RowActions
            itemId={item.tvdb_id}
            title={item.media_title}
            detail={detail}
            canCancel={!!item.can_cancel}
            canRetry={!!item.can_retry}
            canPick={!!item.can_pick_release}
            onPick={onPick}
          />
        </Box>
        <Typography variant="caption" sx={{ display: "block", color: theme.palette.text.secondary }}>
          {formatRelativeTime(item.updated_at)}
        </Typography>
        {item.trace_id && (
          <Tooltip title="Trace ID — use it to find this request in SigNoz">
            <Typography
              variant="caption"
              sx={{
                display: "block",
                mt: 0.5,
                fontFamily: "monospace",
                fontSize: "0.6875rem",
                color: theme.palette.text.disabled,
                cursor: "default",
              }}
            >
              {item.trace_id.slice(0, 8)}
            </Typography>
          </Tooltip>
        )}
      </Box>
    </Box>
  );
};

/** One pending scheduled row: what Scout will do and when. */
const UpcomingRow: React.FC<{ item: UpcomingItem; onPick: (target: PickerTarget) => void }> = ({ item, onPick }) => {
  const theme = useTheme();
  const detail = describeItem(item.category, item.season, item.episode, item.absolute_episode, item.is_anime);
  const retrying = !!item.next_attempt_at;
  const when = retrying
    ? `Retry ${formatTimeUntil(item.next_attempt_at as string)}${item.attempts ? ` (attempt ${item.attempts + 1})` : ""}`
    : `Releases ${formatTimeUntil(item.release_time)}`;

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
      {item.poster_url ? (
        <Box
          component="img"
          src={item.poster_url}
          alt=""
          sx={{ width: 36, height: 54, flexShrink: 0, objectFit: "cover", borderRadius: 1, border: `1px solid ${theme.palette.divider}` }}
        />
      ) : (
        <Box sx={{ width: 36, height: 54, flexShrink: 0, borderRadius: 1, backgroundColor: alpha(theme.palette.common.white, 0.05) }} />
      )}
      <Box sx={{ flex: 1, minWidth: 0 }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.25, flexWrap: "wrap" }}>
          <Typography variant="body2" sx={{ fontWeight: 650, color: theme.palette.text.primary }}>
            {item.media_title}
          </Typography>
          <Typography variant="caption" sx={{ color: theme.palette.text.disabled }}>
            {detail}
          </Typography>
        </Box>
        <Typography variant="caption" sx={{ display: "block", color: retrying ? getStageColor("searching") : theme.palette.text.secondary }}>
          {when}
          {retrying && item.last_failure_reason ? ` · ${item.last_failure_reason}` : ""}
        </Typography>
      </Box>
      <RowActions
        itemId={item.tvdb_id}
        title={item.media_title}
        detail={detail}
        canCancel
        canRetry={retrying}
        canPick={retrying}
        onPick={onPick}
      />
    </Box>
  );
};

const UPCOMING_COLLAPSED = 10;

const SectionCard: React.FC<{
  title: string;
  count: number;
  children: React.ReactNode;
}> = ({ title, count, children }) => {
  const theme = useTheme();

  return (
    <Box sx={{ mb: 5 }}>
      <Box sx={{ display: "flex", alignItems: "baseline", gap: 1.25, mb: 1.75 }}>
        <Typography variant="h5" sx={{ color: theme.palette.text.primary }}>
          {title}
        </Typography>
        <Typography variant="caption" sx={{ color: theme.palette.text.secondary }}>
          {count}
        </Typography>
      </Box>
      <Box
        sx={{
          backgroundColor: theme.palette.background.paper,
          border: `1px solid ${theme.palette.divider}`,
          borderRadius: 3,
          overflow: "hidden",
        }}
      >
        {children}
      </Box>
    </Box>
  );
};

/**
 * Activity Page
 *
 * Answers "what is Scout doing right now?". Every requested episode or movie
 * appears here with the stage it has reached, why it is waiting, when it will
 * be retried, and the trace id to dig further. Polls every 5 seconds.
 */
const Activity: React.FC = () => {
  const theme = useTheme();
  const { data, isLoading, isError, isFetching, refetch } = useActivity();
  const upcomingQuery = useUpcoming();
  const controls = useDownloadControls();
  const [picker, setPicker] = useState<PickerTarget | null>(null);
  const [showAllUpcoming, setShowAllUpcoming] = useState(false);

  const active = data?.active ?? [];
  const recent = data?.recent ?? [];
  const counts = data?.counts ?? {};
  const upcoming = upcomingQuery.data ?? [];
  const visibleUpcoming = showAllUpcoming ? upcoming : upcoming.slice(0, UPCOMING_COLLAPSED);

  const handleGrab = (candidate: ReleaseCandidate) => {
    if (!picker) return;
    controls.grab({ itemId: picker.itemId, title: picker.title, release: toSelection(candidate) });
    setPicker(null);
  };

  return (
    <Box
      sx={{
        minHeight: "100vh",
        backgroundColor: theme.palette.background.default,
        pt: { xs: 11, md: 13 },
        pb: 8,
      }}
    >
      <Container maxWidth="lg">
        <PageHeader
          eyebrow="Downloads"
          title="Activity"
          description="Everything Scout is working on and will work on next. Cancel an item, retry one now, or pick the release yourself."
          action={
            <Button
              variant="outlined"
              size="small"
              startIcon={<RefreshIcon />}
              onClick={() => refetch()}
              disabled={isFetching}
            >
              {isFetching ? "Refreshing" : "Refresh"}
            </Button>
          }
        />

        {/* Stage tiles */}
        <Box
          sx={{
            display: "grid",
            gridTemplateColumns: { xs: "repeat(2, 1fr)", md: "repeat(4, 1fr)" },
            gap: 2,
            mb: 5,
          }}
        >
          {SUMMARY_STAGES.map((stage) => (
            <Box
              key={stage}
              sx={{
                px: { xs: 2, md: 2.5 },
                py: 2,
                borderRadius: 3,
                backgroundColor: theme.palette.background.paper,
                border: `1px solid ${theme.palette.divider}`,
                borderLeft: `3px solid ${getStageColor(stage)}`,
              }}
            >
              <Typography
                variant="h4"
                sx={{ color: theme.palette.text.primary, fontVariantNumeric: "tabular-nums" }}
              >
                {counts[stage] ?? 0}
              </Typography>
              <Typography variant="caption" sx={{ color: theme.palette.text.secondary }}>
                {getStageDescription(stage)}
              </Typography>
            </Box>
          ))}
        </Box>

        {/* A thin progress line keeps polling visible without moving the page */}
        <Box sx={{ height: 2, mb: 3 }}>
          {isFetching && !isLoading && <LinearProgress sx={{ height: 2, borderRadius: 999 }} />}
        </Box>

        {isLoading && (
          <Box
            sx={{
              backgroundColor: theme.palette.background.paper,
              border: `1px solid ${theme.palette.divider}`,
              borderRadius: 3,
              p: 2.5,
            }}
          >
            {Array.from({ length: 4 }).map((_, index) => (
              <Box key={index} sx={{ display: "flex", gap: 2, py: 1.5 }}>
                <Skeleton variant="rectangular" width={48} height={72} sx={{ borderRadius: 1.5 }} />
                <Box sx={{ flex: 1 }}>
                  <Skeleton variant="text" width="35%" height={24} />
                  <Skeleton variant="text" width="55%" height={18} />
                  <Skeleton variant="rectangular" height={3} sx={{ mt: 1.5, borderRadius: 999 }} />
                </Box>
              </Box>
            ))}
          </Box>
        )}

        {isError && !isLoading && (
          <Box sx={{ textAlign: "center", py: 8 }}>
            <Typography variant="subtitle1" sx={{ color: theme.palette.text.primary }}>
              Couldn't load activity
            </Typography>
            <Typography variant="body2" sx={{ color: theme.palette.text.secondary, mb: 2 }}>
              Scout couldn't reach the server.
            </Typography>
            <Button variant="outlined" size="small" onClick={() => refetch()}>
              Retry
            </Button>
          </Box>
        )}

        {!isLoading && !isError && (
          <>
            {active.length > 0 && (
              <SectionCard title="In flight" count={active.length}>
                {active.map((item) => (
                  <ActivityRow key={itemKey(item)} item={item} onPick={setPicker} />
                ))}
              </SectionCard>
            )}

            {upcoming.length > 0 && (
              <SectionCard title="Upcoming" count={upcoming.length}>
                {visibleUpcoming.map((item) => (
                  <UpcomingRow key={`${item.id}-${item.tvdb_id}`} item={item} onPick={setPicker} />
                ))}
                {upcoming.length > UPCOMING_COLLAPSED && (
                  <Box sx={{ px: 2.5, py: 1.25, borderTop: `1px solid ${theme.palette.divider}` }}>
                    <Button size="small" onClick={() => setShowAllUpcoming((v) => !v)}>
                      {showAllUpcoming ? "Show fewer" : `Show all ${upcoming.length}`}
                    </Button>
                  </Box>
                )}
              </SectionCard>
            )}

            {recent.length > 0 && (
              <SectionCard title="Finished" count={recent.length}>
                {recent.map((item) => (
                  <ActivityRow key={itemKey(item)} item={item} onPick={setPicker} />
                ))}
              </SectionCard>
            )}

            {active.length === 0 && recent.length === 0 && upcoming.length === 0 && (
              <Box sx={{ textAlign: "center", py: 10 }}>
                <InboxIcon sx={{ fontSize: 38, color: theme.palette.text.disabled, mb: 1 }} />
                <Typography variant="subtitle1" sx={{ color: theme.palette.text.primary }}>
                  Nothing in flight
                </Typography>
                <Typography variant="body2" sx={{ color: theme.palette.text.secondary }}>
                  Request a show or movie and its progress will appear here.
                </Typography>
              </Box>
            )}
          </>
        )}
      </Container>

      <ReleasePickerDialog
        itemId={picker?.itemId ?? null}
        title={picker?.title ?? ""}
        detail={picker?.detail}
        open={picker !== null}
        onClose={() => setPicker(null)}
        onGrab={handleGrab}
        grabbing={picker ? controls.isGrabbing(picker.itemId) : false}
      />
    </Box>
  );
};

export default Activity;
