import React, { useState, useMemo } from "react";
import Dialog from "@mui/material/Dialog";
import DialogTitle from "@mui/material/DialogTitle";
import DialogContent from "@mui/material/DialogContent";
import DialogActions from "@mui/material/DialogActions";
import Button from "@mui/material/Button";
import Box from "@mui/material/Box";
import Chip from "@mui/material/Chip";
import Skeleton from "@mui/material/Skeleton";
import Tabs from "@mui/material/Tabs";
import Tab from "@mui/material/Tab";
import LinearProgress from "@mui/material/LinearProgress";
import Typography from "@mui/material/Typography";
import IconButton from "@mui/material/IconButton";
import CircularProgress from "@mui/material/CircularProgress";
import Alert from "@mui/material/Alert";
import AlertTitle from "@mui/material/AlertTitle";
import CloseIcon from "@mui/icons-material/Close";
import CheckRoundedIcon from "@mui/icons-material/CheckRounded";
import DownloadRoundedIcon from "@mui/icons-material/DownloadRounded";
import type { ShowStatus, EnrichedMedia } from "../../types/MediaStatus";
import { alpha, useTheme } from "@mui/material/styles";
import useMediaQuery from "@mui/material/useMediaQuery";
import { useShowMetadataStatus } from "../../hooks/useLibrary";
import { useMediaDetails } from "../../hooks/useMediaDetails";
import FollowButton from "./FollowButton";

export interface MediaStatusDialogProps {
  open: boolean;
  onClose: () => void;
  status: ShowStatus | null;
  mediaName: string;
  posterUrl: string;
  enrichedMedia?: EnrichedMedia | null;
  onDownload?: () => void;
  showDownloadButton?: boolean;
  isDownloading?: boolean;
  /** Movies have no seasons, so they get a compact single-item layout. */
  mediaType?: "series" | "movie";
  /** Library state for movies, when the caller already knows it. */
  inLibrary?: boolean;
  /** TVDB id, used to fetch the detail record. Falls back to enrichedMedia. */
  tvdbId?: string;
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

/** TVDB dates are plain calendar days — parse them as local to avoid a timezone slip. */
const formatAirDate = (value?: string): string | null => {
  if (!value) return null;
  const [year, month, day] = value.split("-").map(Number);
  if (!year || !month || !day) return null;
  return new Date(year, month - 1, day).toLocaleDateString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
};

const formatRuntime = (minutes?: number): string | null => {
  if (!minutes || minutes <= 0) return null;
  const hours = Math.floor(minutes / 60);
  const remainder = minutes % 60;
  if (hours === 0) return `${remainder}m`;
  if (remainder === 0) return `${hours}h`;
  return `${hours}h ${remainder}m`;
};

/** How much overview text fits before the fold. */
const OVERVIEW_CLAMP_CHARS = 320;

/**
 * MediaStatusDialog Component
 *
 * The detail view for a media item. Shows what the item is (overview, genres,
 * runtime, rating, air dates) alongside what Scout has of it — episode-level
 * coverage for shows, library state for movies.
 *
 * List endpoints carry only poster-grid data, so the dialog fetches the full
 * detail record on open and merges it over whatever the caller already had.
 *
 * Features:
 * - Overview with expand/collapse, genre chips and a facts row
 * - Season tabs with per-episode library state (series)
 * - Release date, runtime and library state (movies)
 * - Keyboard accessible (ESC to close)
 * - Material-UI theme integration
 * - Fills the screen on phones so the episode list gets the full height
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
  isDownloading = false,
  mediaType = "series",
  inLibrary,
  tvdbId,
}) => {
  const theme = useTheme();
  // A centred card on a phone leaves the episode list a few rows tall, and a
  // phone held sideways is short enough that the same applies.
  const isPhoneWidth = useMediaQuery(theme.breakpoints.down("sm"));
  const isShort = useMediaQuery("(max-height: 520px)");
  const fullScreen = isPhoneWidth || isShort;
  const [selectedSeasonIndex, setSelectedSeasonIndex] = useState<number | null>(null);
  const [overviewExpanded, setOverviewExpanded] = useState(false);

  const isMovie = mediaType === "movie";
  const resolvedTvdbId = tvdbId ?? enrichedMedia?.media.id ?? null;

  // Full detail record — only fetched while the dialog is open
  const { data: details, isLoading: isLoadingDetails } = useMediaDetails(
    resolvedTvdbId,
    mediaType,
    open,
  );

  // TVDB sync progress, used to surface missing-episode counts for shows
  const { data: metadataStatus } = useShowMetadataStatus(
    enrichedMedia?.status.type === "series" ? enrichedMedia?.media.id ?? null : null,
  );

  // Detail fields, preferring the fetched record and falling back to whatever
  // the list endpoint already gave the caller.
  const info = useMemo(() => {
    const base = enrichedMedia?.media;
    const listEpisodes = base?.metadata?.episodes ?? [];
    return {
      overview: details?.overview || base?.overview || "",
      tagline: details?.tagline || base?.tagline || "",
      year: details?.year || base?.year || "",
      statusName: details?.status?.name || base?.status?.name || "",
      genres: details?.genres?.length ? details.genres : base?.genres ?? [],
      runtime: details?.runtime || base?.runtime || 0,
      contentRating: details?.contentRating || base?.contentRating || "",
      firstAired: details?.metadata?.firstAired || base?.metadata?.firstAired || "",
      lastAired: details?.metadata?.lastAired || base?.metadata?.lastAired || "",
      anime: base?.anime ?? details?.anime ?? false,
      episodes: listEpisodes.length > 0 ? listEpisodes : details?.metadata?.episodes ?? [],
    };
  }, [enrichedMedia, details]);

  // Merge TVDB episode metadata with Plex download status
  const enrichedSeasons = useMemo<EnrichedSeasonInfo[]>(() => {
    // Prefer enriched status seasons (has TVDB IDs) over batch status prop
    const statusSeasons = enrichedMedia?.status?.seasons ?? status?.seasons;

    // Without TVDB episode metadata, fall back to status-only display
    if (info.episodes.length === 0) {
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
    if (info.anime === true && usesAbsoluteNumbering && statusSeasons) {
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
    info.episodes.forEach((ep) => {
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
  }, [status, enrichedMedia, info]);

  // Library coverage across every season, shown as a progress bar in the header
  const totals = useMemo(() => {
    const episodes = enrichedSeasons.flatMap((season) => season.episodes);
    const downloaded = episodes.filter((episode) => episode.downloaded).length;
    return { downloaded, total: episodes.length };
  }, [enrichedSeasons]);

  const downloadLabel = isMovie ? "Download" : "Download missing";
  const movieInLibrary = inLibrary ?? enrichedMedia?.status?.inLibrary ?? false;

  // Facts strip under the title: year · runtime · rating · status
  const facts = [
    info.year,
    formatRuntime(info.runtime),
    info.contentRating,
    info.statusName,
    info.anime ? "Anime" : null,
  ].filter((fact): fact is string => !!fact);

  const isOverviewLong = info.overview.length > OVERVIEW_CLAMP_CHARS;
  const showOverviewSkeleton = !info.overview && isLoadingDetails;

  const libraryPill = (
    <Box
      sx={{
        display: "inline-flex",
        alignItems: "center",
        gap: 0.5,
        px: 1,
        py: "4px",
        borderRadius: 999,
        fontSize: "0.6875rem",
        fontWeight: 700,
        color: movieInLibrary ? theme.palette.success.main : theme.palette.text.secondary,
        backgroundColor: alpha(
          movieInLibrary ? theme.palette.success.main : theme.palette.common.white,
          movieInLibrary ? 0.14 : 0.06,
        ),
        border: `1px solid ${alpha(
          movieInLibrary ? theme.palette.success.main : theme.palette.common.white,
          movieInLibrary ? 0.35 : 0.14,
        )}`,
      }}
    >
      {movieInLibrary && <CheckRoundedIcon sx={{ fontSize: 14 }} />}
      {movieInLibrary ? "In library" : "Not in library"}
    </Box>
  );

  const factsRow = facts.length > 0 && (
    <Typography
      variant="body2"
      sx={{ mt: 0.75, color: theme.palette.text.secondary, fontWeight: 500 }}
    >
      {facts.join(" · ")}
    </Typography>
  );

  // TVDB sometimes files a whole synopsis under "tagline" — only show it when it
  // reads like one and is not just the opening of the overview.
  const showTagline =
    !!info.tagline && info.tagline.length <= 120 && !info.overview.startsWith(info.tagline);

  const taglineRow = showTagline && (
    <Typography
      variant="body2"
      sx={{ mt: 0.75, color: theme.palette.primary.light, fontStyle: "italic" }}
    >
      {info.tagline}
    </Typography>
  );

  const genreChips = info.genres.length > 0 && (
    <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.75, mt: 1.25 }}>
      {info.genres.slice(0, 6).map((genre) => (
        <Chip
          key={genre}
          label={genre}
          size="small"
          variant="outlined"
          sx={{
            height: 24,
            fontSize: "0.6875rem",
            fontWeight: 600,
            color: theme.palette.text.secondary,
            borderColor: theme.palette.divider,
            backgroundColor: alpha(theme.palette.common.white, 0.03),
          }}
        />
      ))}
    </Box>
  );

  /** Overview paragraph, clamped until expanded. */
  const overviewBlock = (showOverviewSkeleton || info.overview) && (
    <Box>
      <Typography
        variant="overline"
        sx={{ color: theme.palette.text.disabled, letterSpacing: "0.08em" }}
      >
        Overview
      </Typography>
      {showOverviewSkeleton ? (
        <Box sx={{ mt: 0.5 }}>
          <Skeleton variant="text" />
          <Skeleton variant="text" />
          <Skeleton variant="text" width="70%" />
        </Box>
      ) : (
        <>
          <Typography
            variant="body2"
            sx={{
              mt: 0.5,
              color: theme.palette.text.secondary,
              lineHeight: 1.65,
              ...(isOverviewLong && !overviewExpanded
                ? {
                    overflow: "hidden",
                    display: "-webkit-box",
                    WebkitLineClamp: 4,
                    WebkitBoxOrient: "vertical",
                  }
                : {}),
            }}
          >
            {info.overview}
          </Typography>
          {isOverviewLong && (
            <Button
              size="small"
              variant="text"
              onClick={() => setOverviewExpanded((expanded) => !expanded)}
              sx={{ mt: 0.5, px: 0, minWidth: 0, fontSize: "0.75rem" }}
            >
              {overviewExpanded ? "Show less" : "Show more"}
            </Button>
          )}
        </>
      )}
    </Box>
  );

  /** Label/value pairs shown beneath the overview. */
  const factPairs: Array<{ label: string; value: string }> = [];
  const releasedLabel = isMovie ? "Released" : "First aired";
  const firstAired = formatAirDate(info.firstAired);
  const lastAired = formatAirDate(info.lastAired);
  if (firstAired) factPairs.push({ label: releasedLabel, value: firstAired });
  if (!isMovie && lastAired) factPairs.push({ label: "Last aired", value: lastAired });
  const runtimeLabel = formatRuntime(info.runtime);
  if (runtimeLabel) {
    factPairs.push({ label: isMovie ? "Runtime" : "Episode runtime", value: runtimeLabel });
  }
  if (info.contentRating) factPairs.push({ label: "Rated", value: info.contentRating });
  if (info.statusName) factPairs.push({ label: "Status", value: info.statusName });

  const factGrid = factPairs.length > 0 && (
    <Box
      sx={{
        display: "grid",
        gridTemplateColumns: "repeat(auto-fit, minmax(140px, 1fr))",
        gap: 1.5,
        mt: 2.5,
        pt: 2.5,
        borderTop: `1px solid ${theme.palette.divider}`,
      }}
    >
      {factPairs.map((fact) => (
        <Box key={fact.label}>
          <Typography
            variant="overline"
            sx={{ display: "block", color: theme.palette.text.disabled, letterSpacing: "0.08em" }}
          >
            {fact.label}
          </Typography>
          <Typography variant="body2" sx={{ color: theme.palette.text.primary, fontWeight: 600 }}>
            {fact.value}
          </Typography>
        </Box>
      ))}
    </Box>
  );

  const actions = (
    <DialogActions
      sx={{
        px: { xs: 2, sm: 3 },
        py: 2,
        // Keep the buttons above the home indicator when the dialog fills the screen
        pb: fullScreen ? "calc(16px + env(safe-area-inset-bottom, 0px))" : 2,
        borderTop: `1px solid ${theme.palette.divider}`,
      }}
    >
      {!isMovie && resolvedTvdbId && (
        <Box sx={{ mr: "auto" }}>
          <FollowButton media={{ id: resolvedTvdbId, mediaName, image_url: posterUrl }} size="small" />
        </Box>
      )}
      <Button onClick={onClose} variant="text" color="inherit">
        Close
      </Button>
      {showDownloadButton && onDownload && (
        <Button
          onClick={onDownload}
          variant="contained"
          color="primary"
          disabled={isDownloading}
          startIcon={
            isDownloading ? <CircularProgress size={16} color="inherit" /> : <DownloadRoundedIcon />
          }
        >
          {isDownloading ? "Requesting..." : downloadLabel}
        </Button>
      )}
    </DialogActions>
  );

  const closeButton = (
    <IconButton aria-label="close" onClick={onClose} sx={{ color: theme.palette.text.secondary }}>
      <CloseIcon />
    </IconButton>
  );

  const poster = (width: number, phoneWidth = width) => (
    <Box
      component="img"
      src={posterUrl}
      alt={mediaName}
      sx={{
        width: { xs: phoneWidth, sm: width },
        height: { xs: phoneWidth * 1.5, sm: width * 1.5 },
        flexShrink: 0,
        objectFit: "cover",
        borderRadius: 2,
        border: `1px solid ${theme.palette.divider}`,
      }}
    />
  );

  // Movies have no season/episode breakdown, so the detail record carries the
  // whole body: overview, facts and library state.
  if (isMovie) {
    return (
      <Dialog
        open={open}
        onClose={onClose}
        maxWidth="sm"
        fullWidth
        aria-labelledby="media-status-dialog-title"
      >
        <DialogTitle
          id="media-status-dialog-title"
          component="div"
          sx={{ display: "flex", alignItems: "flex-start", gap: { xs: 2, sm: 2.5 }, p: { xs: 2, sm: 3 }, pb: 2 }}
        >
          {poster(110, 84)}
          <Box sx={{ flex: 1, minWidth: 0 }}>
            <Typography variant="h4" component="h2" sx={{ color: theme.palette.text.primary }}>
              {mediaName}
            </Typography>
            {factsRow}
            {taglineRow}
            <Box sx={{ mt: 1.5 }}>{libraryPill}</Box>
            {genreChips}
          </Box>
          {closeButton}
        </DialogTitle>

        {(overviewBlock || factGrid) && (
          <DialogContent sx={{ px: { xs: 2, sm: 3 }, pb: 3, pt: 0 }}>
            {overviewBlock}
            {factGrid}
          </DialogContent>
        )}
        {actions}
      </Dialog>
    );
  }

  const handleSeasonChange = (_event: React.SyntheticEvent, newValue: number) => {
    setSelectedSeasonIndex(newValue);
  };

  // Land on the first real season rather than Specials, which sorts first
  const defaultSeasonIndex = Math.max(
    enrichedSeasons.findIndex((season) => season.seasonNum > 0),
    0,
  );
  const activeSeasonIndex = Math.min(
    selectedSeasonIndex ?? defaultSeasonIndex,
    Math.max(enrichedSeasons.length - 1, 0),
  );
  const currentSeason = enrichedSeasons[activeSeasonIndex];
  const percentComplete = totals.total > 0 ? (totals.downloaded / totals.total) * 100 : 0;

  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth="md"
      fullWidth
      fullScreen={fullScreen}
      aria-labelledby="media-status-dialog-title"
    >
      {/* Header with poster, title, facts and overall coverage */}
      <DialogTitle
        id="media-status-dialog-title"
        component="div"
        sx={{
          display: "flex",
          alignItems: "flex-start",
          gap: { xs: 2, sm: 2.5 },
          p: { xs: 2, sm: 3 },
          pb: { xs: 2, sm: 2.5 },
        }}
      >
        {poster(96, 72)}
        <Box sx={{ flex: 1, minWidth: 0 }}>
          <Typography variant="h4" component="h2" sx={{ color: theme.palette.text.primary }}>
            {mediaName}
          </Typography>
          {factsRow}
          {taglineRow}
          {enrichedSeasons.length > 0 && (
            <>
              <Typography variant="body2" sx={{ mt: 1, color: theme.palette.text.secondary }}>
                {totals.downloaded} of {totals.total} {totals.total === 1 ? "episode" : "episodes"}{" "}
                in your library
              </Typography>
              <LinearProgress
                variant="determinate"
                value={percentComplete}
                aria-label="Library coverage"
                sx={{
                  mt: 1,
                  maxWidth: 320,
                  backgroundColor: alpha(theme.palette.common.white, 0.08),
                  "& .MuiLinearProgress-bar": {
                    backgroundColor:
                      percentComplete === 100
                        ? theme.palette.success.main
                        : theme.palette.primary.main,
                  },
                }}
              />
            </>
          )}
          {genreChips}
        </Box>
        {closeButton}
      </DialogTitle>

      {/* A flex column so that, full screen, the episode list alone scrolls and
          the season tabs stay put above it */}
      <DialogContent sx={{ p: 0, display: "flex", flexDirection: "column", minHeight: 0 }}>
        {/* Overview and air dates */}
        {(overviewBlock || factGrid) && (
          <Box sx={{ px: { xs: 2, sm: 3 }, pb: 2.5, flexShrink: 0 }}>
            {overviewBlock}
            {factGrid}
          </Box>
        )}

        {/* TVDB Metadata Status Banner */}
        {enrichedMedia?.status.type === "series" && metadataStatus && (
          <Box sx={{ px: { xs: 2, sm: 3 }, pb: 2, flexShrink: 0 }}>
            {!metadataStatus.hasTvdbData && metadataStatus.plexEpisodeCount > 0 ? (
              <Alert severity="info">
                <AlertTitle>TVDB Sync in Progress</AlertTitle>
                Episode metadata is being synced from TVDB. Missing episode detection will be available shortly.
                Refresh this page in a few minutes to see complete episode status.
              </Alert>
            ) : metadataStatus.hasTvdbData && metadataStatus.missingCount > 0 ? (
              <Alert severity="warning">
                <AlertTitle>Missing Episodes Detected</AlertTitle>
                {metadataStatus.missingCount} episode{metadataStatus.missingCount !== 1 ? "s" : ""} not yet downloaded.
              </Alert>
            ) : metadataStatus.hasTvdbData && metadataStatus.plexEpisodeCount > 0 ? (
              <Alert severity="success">
                <AlertTitle>All Episodes Downloaded 🎉</AlertTitle>
                You have all {metadataStatus.tvdbEpisodeCount} aired episodes in your library!
              </Alert>
            ) : null}
          </Box>
        )}

        {/* Episode metadata is still loading — keep the dialog from looking empty */}
        {enrichedSeasons.length === 0 && isLoadingDetails && (
          <Box sx={{ px: { xs: 2, sm: 3 }, pb: 3 }}>
            <Skeleton variant="rounded" height={48} sx={{ mb: 1 }} />
            <Skeleton variant="rounded" height={120} />
          </Box>
        )}

        {enrichedSeasons.length === 0 && !isLoadingDetails && (
          <Typography variant="body2" color="text.secondary" sx={{ px: { xs: 2, sm: 3 }, pb: 3 }}>
            No episode information available for this title yet.
          </Typography>
        )}

        {currentSeason && (
          <>
            {/* Season Tabs — each label carries that season's coverage */}
            <Box sx={{ borderBottom: `1px solid ${theme.palette.divider}`, flexShrink: 0 }}>
              <Tabs
                value={activeSeasonIndex}
                onChange={handleSeasonChange}
                variant="scrollable"
                scrollButtons="auto"
                aria-label="season tabs"
                sx={{
                  px: { xs: 2, sm: 3 },
                  minHeight: 48,
                  "& .MuiTab-root": {
                    minHeight: 48,
                    textTransform: "none",
                    fontWeight: 600,
                    fontSize: "0.875rem",
                    color: theme.palette.text.secondary,
                    "&.Mui-selected": { color: theme.palette.text.primary },
                  },
                }}
              >
                {enrichedSeasons.map((season) => {
                  const have = season.episodes.filter((episode) => episode.downloaded).length;
                  return (
                    <Tab
                      key={season.seasonNum}
                      label={`${season.seasonNum === 0 ? "Specials" : `Season ${season.seasonNum}`} · ${have}/${season.episodes.length}`}
                      id={`season-tab-${season.seasonNum}`}
                      aria-controls={`season-panel-${season.seasonNum}`}
                    />
                  );
                })}
              </Tabs>
            </Box>

            {/* Episode List */}
            <Box
              role="tabpanel"
              id={`season-panel-${currentSeason.seasonNum}`}
              aria-labelledby={`season-tab-${currentSeason.seasonNum}`}
              sx={{
                px: { xs: 2, sm: 3 },
                py: 2,
                // Full screen: take what is left below the overview and tabs, but
                // never so little that the list disappears behind a long synopsis
                flex: fullScreen ? 1 : "none",
                minHeight: fullScreen ? 240 : 0,
                maxHeight: fullScreen ? "none" : "48vh",
                overflowY: "auto",
              }}
            >
              {currentSeason.episodes.length === 0 ? (
                <Typography variant="body2" color="text.secondary" align="center" sx={{ py: 6 }}>
                  No episodes found for this season
                </Typography>
              ) : (
                currentSeason.episodes.map((episode) => (
                  <Box
                    key={episode.episodeNum}
                    sx={{
                      display: "flex",
                      alignItems: "center",
                      gap: 2,
                      py: 1.25,
                      borderBottom: `1px solid ${theme.palette.divider}`,
                      "&:last-of-type": { borderBottom: 0 },
                    }}
                  >
                    {/* Episode number, fixed width so titles align down the column */}
                    <Typography
                      variant="body2"
                      sx={{
                        width: 34,
                        flexShrink: 0,
                        textAlign: "right",
                        fontVariantNumeric: "tabular-nums",
                        color: theme.palette.text.disabled,
                      }}
                    >
                      {episode.episodeNum}
                    </Typography>

                    <Box sx={{ flex: 1, minWidth: 0 }}>
                      <Typography
                        variant="body2"
                        sx={{
                          fontWeight: 600,
                          color: theme.palette.text.primary,
                          overflow: "hidden",
                          textOverflow: "ellipsis",
                          whiteSpace: "nowrap",
                        }}
                      >
                        {episode.name || `Episode ${episode.episodeNum}`}
                      </Typography>
                      {episode.aired && (
                        <Typography variant="caption" sx={{ color: theme.palette.text.secondary }}>
                          {formatAirDate(episode.aired) ?? episode.aired}
                        </Typography>
                      )}
                    </Box>

                    {episode.downloaded ? (
                      <Box
                        sx={{
                          display: "flex",
                          alignItems: "center",
                          gap: 0.5,
                          flexShrink: 0,
                          px: 1,
                          py: "3px",
                          borderRadius: 999,
                          fontSize: "0.6875rem",
                          fontWeight: 700,
                          color: theme.palette.success.main,
                          backgroundColor: alpha(theme.palette.success.main, 0.14),
                          border: `1px solid ${alpha(theme.palette.success.main, 0.35)}`,
                        }}
                      >
                        <CheckRoundedIcon sx={{ fontSize: 14 }} />
                        In library
                      </Box>
                    ) : (
                      <Typography
                        variant="caption"
                        sx={{ flexShrink: 0, color: theme.palette.text.disabled, fontWeight: 600 }}
                      >
                        Missing
                      </Typography>
                    )}
                  </Box>
                ))
              )}
            </Box>
          </>
        )}
      </DialogContent>

      {actions}
    </Dialog>
  );
};

export default MediaStatusDialog;
