import React, { useState } from "react";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Chip from "@mui/material/Chip";
import Container from "@mui/material/Container";
import LinearProgress from "@mui/material/LinearProgress";
import Skeleton from "@mui/material/Skeleton";
import Tab from "@mui/material/Tab";
import Tabs from "@mui/material/Tabs";
import Typography from "@mui/material/Typography";
import useMediaQuery from "@mui/material/useMediaQuery";
import RefreshIcon from "@mui/icons-material/Refresh";
import DeleteSweepOutlinedIcon from "@mui/icons-material/DeleteSweepOutlined";
import UnfoldLessIcon from "@mui/icons-material/UnfoldLess";
import UnfoldMoreIcon from "@mui/icons-material/UnfoldMore";
import { alpha, useTheme } from "@mui/material/styles";
import PageHeader from "../components/ui/PageHeader";
import ReleasePickerDialog from "../components/ui/ReleasePickerDialog";
import IndexerHealthCard from "../components/ui/IndexerHealthCard";
import ActivityList, { type PickerTarget } from "../components/ui/ActivityList";
import { useActivity } from "../hooks/useActivity";
import { useUpcoming } from "../hooks/useUpcoming";
import { useClearFinished } from "../hooks/useNotifications";
import { toSelection, useDownloadControls, type ReleaseCandidate } from "../hooks/useDownloadControls";
import { buildViews, countByStage, groupRows, type ActivityRowModel } from "../utils/activityRows";
import { getStageColor, getStageDescription, getStageLabel, type DownloadStage } from "../utils/downloadStage";

type TabKey = "inflight" | "upcoming" | "finished";

/** A summary tile doubles as a shortcut: it opens the tab that holds its stage, filtered to it. */
interface SummaryTile {
  stage: DownloadStage;
  tab: TabKey;
  filter: DownloadStage | null;
}

const TILES: SummaryTile[] = [
  { stage: "downloading", tab: "inflight", filter: "downloading" },
  { stage: "searching", tab: "inflight", filter: "searching" },
  { stage: "scheduled", tab: "upcoming", filter: null },
  { stage: "failed", tab: "finished", filter: "failed" },
];

/** Groups shown in Upcoming and Finished before "Show all". In flight is never cut. */
const COLLAPSED_GROUPS = 12;

const EMPTY: Record<TabKey, { title: string; body: string }> = {
  inflight: { title: "Nothing in flight", body: "Request a show or movie and its progress will appear here." },
  upcoming: { title: "Nothing scheduled", body: "Future episodes and unreleased movies wait here until they come out." },
  finished: { title: "Nothing finished yet", body: "Completed, failed and cancelled downloads end up here." },
};

type ExpandedByTab = Record<TabKey, ReadonlySet<string>>;

const NONE_EXPANDED: ExpandedByTab = { inflight: new Set(), upcoming: new Set(), finished: new Set() };

/**
 * Activity Page
 *
 * Answers "what is Scout doing right now?" without scrolling: four stage
 * tiles, the indexer strip, then one tab at a time (In flight, Upcoming,
 * Finished) listing one line per show. A whole season is a single row until
 * it is opened; a tile click jumps to the matching tab filtered to that
 * stage. Polls every 5 seconds.
 */
const Activity: React.FC = () => {
  const theme = useTheme();
  const phone = useMediaQuery(theme.breakpoints.down("sm"));
  const activity = useActivity();
  const upcomingQuery = useUpcoming();
  const controls = useDownloadControls();
  const clearFinished = useClearFinished();

  const [tab, setTab] = useState<TabKey>("inflight");
  const [stageFilter, setStageFilter] = useState<DownloadStage | null>(null);
  const [expanded, setExpanded] = useState<ExpandedByTab>(NONE_EXPANDED);
  const [showAll, setShowAll] = useState<Partial<Record<TabKey, boolean>>>({});
  const [picker, setPicker] = useState<PickerTarget | null>(null);

  // Recomputed every render on purpose: the relative times inside the rows
  // move even when the polled data does not, and the work is a few hundred rows.
  const views = buildViews(activity.data, upcomingQuery.data);
  const inFlightCounts = countByStage(views.inFlight);
  const finishedCounts = countByStage(views.finished);
  const rowsByTab: Record<TabKey, ActivityRowModel[]> = {
    inflight: views.inFlight,
    upcoming: views.upcoming,
    finished: views.finished,
  };
  const rows = stageFilter ? rowsByTab[tab].filter((row) => row.stage === stageFilter) : rowsByTab[tab];
  const groups = groupRows(rows, tab === "upcoming" ? "soonest" : "latest");
  const hasGroups = groups.some((group) => group.rows.length > 1);
  const allOpen = hasGroups && groups.every((group) => group.rows.length === 1 || expanded[tab].has(group.key));

  const tileCount = (tile: SummaryTile): number => {
    if (tile.stage === "scheduled") return views.upcoming.length;
    return (tile.tab === "inflight" ? inFlightCounts : finishedCounts)[tile.stage] ?? 0;
  };
  const tileSelected = (tile: SummaryTile) => tab === tile.tab && stageFilter === tile.filter;
  const selectTile = (tile: SummaryTile) => {
    if (tileSelected(tile) && tile.filter) {
      setStageFilter(null);
      return;
    }
    setTab(tile.tab);
    setStageFilter(tile.filter);
  };

  const toggleGroup = (key: string) => {
    setExpanded((current) => {
      const next = new Set(current[tab]);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return { ...current, [tab]: next };
    });
  };
  const setAllOpen = (open: boolean) => {
    setExpanded((current) => ({ ...current, [tab]: open ? new Set(groups.map((group) => group.key)) : new Set() }));
  };

  const handleGrab = (candidate: ReleaseCandidate) => {
    if (!picker) return;
    controls.grab({ itemId: picker.itemId, title: picker.title, release: toSelection(candidate) });
    setPicker(null);
  };

  const refreshing = activity.isFetching || upcomingQuery.isFetching;
  const refetchAll = () => {
    activity.refetch();
    upcomingQuery.refetch();
  };

  const tabLabel = (label: string, count: number) => (count > 0 ? `${label} (${count})` : label);
  const empty = stageFilter
    ? { title: `No ${stageFilter} items`, body: "Clear the filter to see the rest of this tab." }
    : EMPTY[tab];

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
          description="What Scout is doing now, what comes next, and what finished. Open a show to see each episode."
          action={
            <Button variant="outlined" size="small" startIcon={<RefreshIcon />} onClick={refetchAll} disabled={refreshing}>
              {refreshing ? "Refreshing" : "Refresh"}
            </Button>
          }
        />

        {/* Stage tiles: a count each, and a shortcut to the tab that holds them */}
        <Box
          sx={{
            display: "grid",
            gridTemplateColumns: { xs: "repeat(2, 1fr)", md: "repeat(4, 1fr)" },
            gap: 1.5,
            mb: 2.5,
          }}
        >
          {TILES.map((tile) => {
            const color = getStageColor(tile.stage);
            const selected = tileSelected(tile);
            return (
              <Box
                key={tile.stage}
                component="button"
                type="button"
                aria-pressed={selected}
                onClick={() => selectTile(tile)}
                sx={{
                  display: "flex",
                  alignItems: "center",
                  gap: 1.5,
                  px: { xs: 1.5, md: 2 },
                  py: 1.25,
                  textAlign: "left",
                  cursor: "pointer",
                  font: "inherit",
                  color: "inherit",
                  borderRadius: 2.5,
                  backgroundColor: selected ? alpha(color, 0.12) : theme.palette.background.paper,
                  border: `1px solid ${selected ? alpha(color, 0.5) : theme.palette.divider}`,
                  borderLeft: `3px solid ${color}`,
                  transition: "background-color .15s ease, border-color .15s ease",
                  "&:hover": { backgroundColor: alpha(color, 0.08) },
                  "&:focus-visible": { outline: `2px solid ${theme.palette.primary.main}`, outlineOffset: 2 },
                }}
              >
                <Typography
                  variant="h4"
                  component="span"
                  sx={{ color: theme.palette.text.primary, fontVariantNumeric: "tabular-nums", minWidth: 28 }}
                >
                  {tileCount(tile)}
                </Typography>
                <Box sx={{ minWidth: 0 }}>
                  <Typography variant="subtitle2" sx={{ display: "block", color: theme.palette.text.primary, lineHeight: 1.3 }}>
                    {getStageLabel(tile.stage)}
                  </Typography>
                  <Typography
                    variant="caption"
                    sx={{ display: "block", color: theme.palette.text.secondary, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}
                  >
                    {getStageDescription(tile.stage)}
                  </Typography>
                </Box>
              </Box>
            );
          })}
        </Box>

        <IndexerHealthCard />

        <Box sx={{ borderBottom: 1, borderColor: "divider" }}>
          <Tabs
            value={tab}
            onChange={(_, value: TabKey) => {
              setTab(value);
              setStageFilter(null);
            }}
            aria-label="activity tabs"
            variant={phone ? "fullWidth" : "standard"}
          >
            <Tab label={tabLabel("In flight", views.inFlight.length)} value="inflight" />
            <Tab label={tabLabel("Upcoming", views.upcoming.length)} value="upcoming" />
            <Tab label={tabLabel("Finished", views.finished.length)} value="finished" />
          </Tabs>
        </Box>

        {/* Filter chip and list-wide actions; a thin progress line keeps polling visible without moving the page */}
        <Box sx={{ display: "flex", alignItems: "center", gap: 1, minHeight: 40, py: 0.5 }}>
          {stageFilter && (
            <Chip
              size="small"
              label={`${getStageLabel(stageFilter)} only`}
              onDelete={() => setStageFilter(null)}
              sx={{
                backgroundColor: alpha(getStageColor(stageFilter), 0.16),
                color: getStageColor(stageFilter),
                "& .MuiChip-deleteIcon": { color: "inherit", opacity: 0.7, "&:hover": { opacity: 1 } },
              }}
            />
          )}
          <Box sx={{ flex: 1 }} />
          {hasGroups && (
            <Button
              size="small"
              color="inherit"
              startIcon={allOpen ? <UnfoldLessIcon /> : <UnfoldMoreIcon />}
              onClick={() => setAllOpen(!allOpen)}
              sx={{ color: theme.palette.text.secondary }}
            >
              {allOpen ? "Collapse all" : "Expand all"}
            </Button>
          )}
          {tab === "finished" && views.finished.length > 0 && (
            <Button
              size="small"
              color="inherit"
              startIcon={<DeleteSweepOutlinedIcon />}
              onClick={() => clearFinished.mutate()}
              disabled={clearFinished.isPending}
              sx={{ color: theme.palette.text.secondary }}
            >
              Clear finished
            </Button>
          )}
        </Box>
        <Box sx={{ height: 2, mb: 1 }}>
          {refreshing && !activity.isLoading && <LinearProgress sx={{ height: 2, borderRadius: 999 }} />}
        </Box>

        {activity.isLoading && (
          <Box
            sx={{
              backgroundColor: theme.palette.background.paper,
              border: `1px solid ${theme.palette.divider}`,
              borderRadius: 3,
              px: 2,
            }}
          >
            {Array.from({ length: 6 }).map((_, index) => (
              <Box key={index} sx={{ display: "flex", alignItems: "center", gap: 2, py: 1 }}>
                <Skeleton variant="rectangular" width={32} height={48} sx={{ borderRadius: 1 }} />
                <Skeleton variant="text" width="22%" height={22} />
                <Skeleton variant="text" width="40%" height={18} />
                <Box sx={{ flex: 1 }} />
                <Skeleton variant="text" width={48} height={18} />
              </Box>
            ))}
          </Box>
        )}

        {activity.isError && !activity.isLoading && (
          <Box sx={{ textAlign: "center", py: 8 }}>
            <Typography variant="subtitle1" sx={{ color: theme.palette.text.primary }}>
              Couldn't load activity
            </Typography>
            <Typography variant="body2" sx={{ color: theme.palette.text.secondary, mb: 2 }}>
              Scout couldn't reach the server.
            </Typography>
            <Button variant="outlined" size="small" onClick={refetchAll}>
              Retry
            </Button>
          </Box>
        )}

        {!activity.isLoading && !activity.isError && (
          <ActivityList
            groups={groups}
            expanded={expanded[tab]}
            onToggle={toggleGroup}
            onPick={setPicker}
            limit={tab === "inflight" ? undefined : COLLAPSED_GROUPS}
            showAll={!!showAll[tab]}
            onToggleShowAll={() => setShowAll((current) => ({ ...current, [tab]: !current[tab] }))}
            emptyTitle={empty.title}
            emptyBody={empty.body}
          />
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
