import React from "react";
import Box from "@mui/material/Box";
import Container from "@mui/material/Container";
import Typography from "@mui/material/Typography";
import Button from "@mui/material/Button";
import Skeleton from "@mui/material/Skeleton";
import Tooltip from "@mui/material/Tooltip";
import LinearProgress from "@mui/material/LinearProgress";
import RefreshIcon from "@mui/icons-material/Refresh";
import InboxIcon from "@mui/icons-material/MoveToInbox";
import { alpha, useTheme } from "@mui/material/styles";
import PageHeader from "../components/ui/PageHeader";
import StageChip from "../components/ui/StageChip";
import { useActivity, type ActivityItem } from "../hooks/useActivity";
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
  // record, treat it as having reached the search step.
  const reachedIndex = failed ? 1 : PIPELINE.indexOf(stage);
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

/** One tracked episode or movie. */
const ActivityRow: React.FC<{ item: ActivityItem }> = ({ item }) => {
  const theme = useTheme();
  const isFailed = item.status === "failed";
  const detail = item.category === "series"
    ? formatEpisodeLabel(item.season, item.episode, item.absolute_episode, item.is_anime)
    : "Movie";

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
        px: 2.5,
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
              maxWidth: 420,
            }}
          >
            {item.media_title}
          </Typography>
          <StageChip stage={item.status} />
          <Typography variant="caption" sx={{ color: theme.palette.text.disabled }}>
            {detail}
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
      </Box>

      <Box sx={{ flexShrink: 0, textAlign: "right" }}>
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

  const active = data?.active ?? [];
  const recent = data?.recent ?? [];
  const counts = data?.counts ?? {};

  return (
    <Box
      sx={{
        minHeight: "100vh",
        backgroundColor: theme.palette.background.default,
        pt: 13,
        pb: 8,
      }}
    >
      <Container maxWidth="lg">
        <PageHeader
          eyebrow="Downloads"
          title="Activity"
          description="Everything Scout is working on: what stage each request is at, what is waiting on a release, and what failed."
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
            gridTemplateColumns: "repeat(4, 1fr)",
            gap: 2,
            mb: 5,
          }}
        >
          {SUMMARY_STAGES.map((stage) => (
            <Box
              key={stage}
              sx={{
                px: 2.5,
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
                  <ActivityRow key={itemKey(item)} item={item} />
                ))}
              </SectionCard>
            )}

            {recent.length > 0 && (
              <SectionCard title="Finished" count={recent.length}>
                {recent.map((item) => (
                  <ActivityRow key={itemKey(item)} item={item} />
                ))}
              </SectionCard>
            )}

            {active.length === 0 && recent.length === 0 && (
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
    </Box>
  );
};

export default Activity;
