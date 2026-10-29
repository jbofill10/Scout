import React, { useState, useEffect, useRef, useCallback } from "react";
import Box from "@mui/material/Box";
import Chip from "@mui/material/Chip";
import Container from "@mui/material/Container";
import InputAdornment from "@mui/material/InputAdornment";
import LinearProgress from "@mui/material/LinearProgress";
import TextField from "@mui/material/TextField";
import ToggleButton from "@mui/material/ToggleButton";
import ToggleButtonGroup from "@mui/material/ToggleButtonGroup";
import Tooltip from "@mui/material/Tooltip";
import Typography from "@mui/material/Typography";
import IconButton from "@mui/material/IconButton";
import CircularProgress from "@mui/material/CircularProgress";
import Slide from "@mui/material/Slide";
import CloseIcon from "@mui/icons-material/Close";
import ErrorOutlineRoundedIcon from "@mui/icons-material/ErrorOutlineRounded";
import HelpOutlineRoundedIcon from "@mui/icons-material/HelpOutlineRounded";
import SearchIcon from "@mui/icons-material/Search";
import SearchOffOutlinedIcon from "@mui/icons-material/SearchOffOutlined";
import Tv from "@mui/icons-material/Tv";
import Movie from "@mui/icons-material/Movie";
import { alpha, useTheme } from "@mui/material/styles";
import MediaStatusDialog from "./MediaStatusDialog";
import SearchResultsGrid from "./SearchResultsGrid";
import type { EnrichedMedia, ShowStatus } from "../../types/MediaStatus";
import { logError } from "../../lib/logger";
import { useDownloadRequest } from "../../hooks/useDownloadRequest";
import { useEnrichedSearch } from "../../hooks/useEnrichedSearch";
import type { SearchMediaType } from "../../hooks/useEnrichedSearch";
import {
  SEARCH_EXAMPLES,
  filterLabel,
  needsTitle,
  withoutFilter,
} from "../../utils/searchQuery";
import type { SearchQuery } from "../../utils/searchQuery";

interface SearchDropdownProps {
  isOpen: boolean;
  onClose: () => void;
}

const SEARCH_DEBOUNCE_MS = 300;

const SYNTAX_HELP = [
  ["X (2022) or X 2022", "title and year"],
  ["actor:Mia Goth", "everything they have starred in"],
  ["director:Ti West", "a director's work"],
  ["person:Ti West", "any credit, in any role"],
  ["studio:A24 or network:HBO", "where it was made or aired"],
  ["country:deu lang:deu", "origin and language"],
  ["tt13560574 or an IMDb link", "one exact title"],
] as const;

/**
 * What to say when a search comes back empty, given how the backend read it.
 */
const noResultsCopy = (
  term: string,
  query: SearchQuery | null,
  mediaType: SearchMediaType,
): { title: string; detail: string } => {
  const kind = mediaType === "movie" ? "movie" : "TV show";
  const otherTab = mediaType === "movie" ? "TV Shows" : "Movies";

  if (query && needsTitle(query)) {
    const filters = query.filters
      .map((filter) => `${filterLabel(filter.key).toLowerCase()} ${filter.value}`)
      .join(", ");
    return {
      title: "Add a title to go with that",
      detail: `A ${filters} filter needs a title to search for, or a person to look up.`,
    };
  }
  if (query?.imdbId) {
    return {
      title: `No ${kind} has IMDb id ${query.imdbId}`,
      detail: `If it is a ${otherTab.toLowerCase().replace(/s$/, "")}, switch to the ${otherTab} tab.`,
    };
  }
  if (query?.person) {
    return {
      title: `No ${kind}s found for ${query.person}`,
      detail: `Check the spelling, or try the ${otherTab} tab.`,
    };
  }
  if (query?.director) {
    return {
      title: `No ${kind}s directed by ${query.director}`,
      detail: `Check the spelling, or try the ${otherTab} tab.`,
    };
  }
  return {
    title: `No results for "${term}"`,
    detail: `Try a different spelling, or switch between TV Shows and Movies.`,
  };
};

/**
 * SearchDropdown Component
 *
 * Full-width search overlay that appears from the top of the screen.
 * Features:
 * - Semi-transparent backdrop
 * - Search input with auto-focus
 * - Media type toggle (TV Shows / Movies)
 * - Debounced search backed by TanStack Query (cached, cancellable, race-free)
 * - Previous results stay visible while a refined term loads
 * - Filters typed into the box (a year, actor:, director:, studio:, an IMDb
 *   link) are shown back as removable chips once the backend has read them
 * - Example searches on the empty screen; syntax help behind the ? icon
 * - Opens MediaStatusDialog on result click
 * - Closes on backdrop click or ESC key
 * - Fills the screen on phones, with the media type toggle on its own row
 */
const SearchDropdown: React.FC<SearchDropdownProps> = ({ isOpen, onClose }) => {
  const theme = useTheme();
  const [searchTerm, setSearchTerm] = useState("");
  const [debouncedSearchTerm, setDebouncedSearchTerm] = useState("");
  const [mediaType, setMediaType] = useState<SearchMediaType>("series");
  const [statusDialogOpen, setStatusDialogOpen] = useState(false);
  const [selectedShowStatus, setSelectedShowStatus] = useState<ShowStatus | null>(null);
  const [selectedShowInfo, setSelectedShowInfo] = useState<{
    name: string;
    posterUrl: string;
    tvdbId: string;
  } | null>(null);
  const [selectedEnrichedMedia, setSelectedEnrichedMedia] = useState<EnrichedMedia | null>(null);
  const { requestDownload, isPending: isDownloading } = useDownloadRequest();
  const searchInputRef = useRef<HTMLInputElement>(null);

  const {
    results: searchResults,
    query: parsedQuery,
    isFetching,
    isPlaceholderData,
    error: searchError,
  } = useEnrichedSearch(isOpen ? debouncedSearchTerm : "", mediaType);

  // Debounce the typed term so we search once the user pauses
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedSearchTerm(searchTerm);
    }, SEARCH_DEBOUNCE_MS);

    return () => clearTimeout(timer);
  }, [searchTerm]);

  // Auto-focus search input when dropdown opens
  useEffect(() => {
    if (isOpen && searchInputRef.current) {
      searchInputRef.current.focus();
    }
  }, [isOpen]);

  // Reset search when dropdown closes
  useEffect(() => {
    if (!isOpen) {
      setSearchTerm("");
      setDebouncedSearchTerm("");
    }
  }, [isOpen]);

  useEffect(() => {
    if (searchError) {
      logError("Search request failed in SearchDropdown", searchError, {
        component: "SearchDropdown",
        query: debouncedSearchTerm,
        mediaType: mediaType,
      });
    }
  }, [searchError, debouncedSearchTerm, mediaType]);

  const handleMediaClick = useCallback((enrichedMedia: EnrichedMedia) => {
    setSelectedShowInfo({
      name: enrichedMedia.media.mediaName,
      posterUrl: enrichedMedia.media.image_url,
      tvdbId: enrichedMedia.media.id,
    });

    const showStatus: ShowStatus | null = enrichedMedia.status.seasons
      ? {
          tvdbId: enrichedMedia.media.id,
          seasons: enrichedMedia.status.seasons,
        }
      : null;

    setSelectedShowStatus(showStatus);
    setSelectedEnrichedMedia(enrichedMedia);
    setStatusDialogOpen(true);
  }, []);

  const handleCloseStatusDialog = () => {
    setStatusDialogOpen(false);
    setSelectedShowStatus(null);
    setSelectedShowInfo(null);
    setSelectedEnrichedMedia(null);
  };

  const handleDownload = () => {
    if (!selectedEnrichedMedia) return;

    requestDownload(
      {
        media: selectedEnrichedMedia.media,
        mediaType,
        title: selectedEnrichedMedia.media.mediaName,
        component: "SearchDropdown",
      },
      { onSuccess: () => handleCloseStatusDialog() },
    );
  };

  // Typing a new query right away, e.g. from an example or a removed chip
  const applyTerm = (term: string, nextMediaType?: SearchMediaType) => {
    if (nextMediaType) setMediaType(nextMediaType);
    setSearchTerm(term);
    setDebouncedSearchTerm(term);
    searchInputRef.current?.focus();
  };

  // Handle ESC key
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape" && isOpen) {
        onClose();
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [isOpen, onClose]);

  if (!isOpen) return null;

  const hasQuery = searchTerm.trim().length > 0;
  // The typed term has not reached the query yet: treat it as loading so the
  // "no results" message does not flash before the request even starts.
  const isDebouncing = searchTerm.trim() !== debouncedSearchTerm.trim();
  const isBusy = isFetching || isDebouncing;
  const hasResults = searchResults.length > 0;
  // Only describe the interpretation of the term currently in the box
  const interpretation =
    parsedQuery && !isDebouncing && !isPlaceholderData && parsedQuery.filters.length > 0
      ? parsedQuery
      : null;

  // Centered message block reused by the non-result states
  const message = (icon: React.ReactNode, title: string, detail?: React.ReactNode) => (
    <Box
      sx={{
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        gap: 1,
        py: { xs: 6, sm: 10 },
        textAlign: "center",
      }}
    >
      {icon}
      <Typography variant="subtitle1" sx={{ color: theme.palette.text.primary }}>
        {title}
      </Typography>
      {detail && (
        <Typography variant="body2" component="div" sx={{ color: theme.palette.text.secondary }}>
          {detail}
        </Typography>
      )}
    </Box>
  );

  const syntaxTooltip = (
    <Box component="dl" sx={{ m: 0, display: "grid", gridTemplateColumns: "auto 1fr", columnGap: 1.5, rowGap: 0.5 }}>
      {SYNTAX_HELP.map(([example, meaning]) => (
        <React.Fragment key={example}>
          <Box component="dt" sx={{ fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace", fontSize: "0.75rem" }}>
            {example}
          </Box>
          <Box component="dd" sx={{ m: 0, fontSize: "0.75rem", opacity: 0.85 }}>
            {meaning}
          </Box>
        </React.Fragment>
      ))}
    </Box>
  );

  const emptyCopy = noResultsCopy(searchTerm.trim(), parsedQuery, mediaType);

  return (
    <>
      {/* Backdrop */}
      <Box
        onClick={onClose}
        sx={{
          position: "fixed",
          inset: 0,
          backgroundColor: alpha("#020617", 0.7),
          backdropFilter: "blur(8px)",
          zIndex: 1200,
        }}
      />

      {/* Search Dropdown Content with Slide Animation */}
      <Slide
        direction="down"
        in={isOpen}
        mountOnEnter
        unmountOnExit
        timeout={300}
      >
        <Box
          sx={{
            position: "fixed",
            top: 0,
            left: 0,
            right: 0,
            // Phones: take the whole screen rather than a panel with a strip below it
            bottom: { xs: 0, sm: "auto" },
            maxHeight: { xs: "none", sm: "90vh" },
            pb: { xs: "env(safe-area-inset-bottom, 0px)", sm: 0 },
            backgroundColor: theme.palette.background.default,
            zIndex: 1300,
            borderBottom: { xs: 0, sm: `1px solid ${theme.palette.divider}` },
            borderBottomLeftRadius: { xs: 0, sm: 20 },
            borderBottomRightRadius: { xs: 0, sm: 20 },
            boxShadow: theme.shadows[16],
            overflowY: "auto",
          }}
        >
          {/* Search Header */}
          <Box sx={{ borderBottom: `1px solid ${theme.palette.divider}`, py: 2.5 }}>
            <Container maxWidth="xl">
              {/* Phones: field and close button on one row, the toggle full width below */}
              <Box
                sx={{
                  display: "flex",
                  alignItems: "center",
                  flexWrap: { xs: "wrap", sm: "nowrap" },
                  gap: { xs: 1.5, sm: 2 },
                }}
              >
                <TextField
                  inputRef={searchInputRef}
                  fullWidth
                  placeholder="Search titles, people, years… e.g. X 2022 or actor:Mia Goth"
                  variant="outlined"
                  value={searchTerm}
                  onChange={(e) => setSearchTerm(e.target.value)}
                  autoFocus
                  slotProps={{
                    input: {
                      startAdornment: (
                        <InputAdornment position="start">
                          <SearchIcon sx={{ color: theme.palette.text.secondary }} />
                        </InputAdornment>
                      ),
                      endAdornment: (
                        <InputAdornment position="end">
                          {hasQuery ? (
                            <IconButton
                              size="small"
                              onClick={() => setSearchTerm("")}
                              aria-label="Clear search"
                            >
                              <CloseIcon fontSize="small" />
                            </IconButton>
                          ) : (
                            <Tooltip title={syntaxTooltip} placement="bottom-end" enterTouchDelay={0} arrow>
                              <IconButton size="small" aria-label="Search syntax help">
                                <HelpOutlineRoundedIcon fontSize="small" />
                              </IconButton>
                            </Tooltip>
                          )}
                        </InputAdornment>
                      ),
                    },
                  }}
                  sx={{
                    flex: "1 1 200px",
                    minWidth: 0,
                    "& .MuiOutlinedInput-root": { fontSize: "1.0625rem" },
                  }}
                />

                {/* Media Type Toggle */}
                <ToggleButtonGroup
                  value={mediaType}
                  exclusive
                  onChange={(_, newValue) => newValue && setMediaType(newValue)}
                  aria-label="media type"
                  sx={{
                    flexShrink: 0,
                    order: { xs: 3, sm: 0 },
                    width: { xs: "100%", sm: "auto" },
                    "& .MuiToggleButton-root": { flex: { xs: 1, sm: "0 0 auto" } },
                  }}
                >
                  <ToggleButton value="series" aria-label="TV shows">
                    <Tv sx={{ mr: 1, fontSize: 20 }} />
                    TV Shows
                  </ToggleButton>
                  <ToggleButton value="movie" aria-label="Movies">
                    <Movie sx={{ mr: 1, fontSize: 20 }} />
                    Movies
                  </ToggleButton>
                </ToggleButtonGroup>

                <IconButton onClick={onClose} aria-label="Close search" sx={{ order: { xs: 2, sm: 0 } }}>
                  <CloseIcon />
                </IconButton>
              </Box>

              {/* How the backend read the term: title plus removable filter chips */}
              {interpretation && (
                <Box
                  role="status"
                  aria-label="Active search filters"
                  sx={{
                    display: "flex",
                    alignItems: "center",
                    flexWrap: "wrap",
                    gap: 1,
                    mt: 1.5,
                  }}
                >
                  <Typography variant="body2" sx={{ color: theme.palette.text.secondary }}>
                    {interpretation.title ? (
                      <>
                        Searching for{" "}
                        <Box component="span" sx={{ color: theme.palette.text.primary, fontWeight: 600 }}>
                          {interpretation.title}
                        </Box>
                      </>
                    ) : (
                      "Searching"
                    )}
                  </Typography>
                  {interpretation.filters.map((filter) => (
                    <Chip
                      key={`${filter.key}:${filter.value}`}
                      size="small"
                      variant="outlined"
                      color="primary"
                      label={
                        <>
                          <Box component="span" sx={{ opacity: 0.75 }}>
                            {filterLabel(filter.key)}
                          </Box>
                          {" "}
                          {filter.value}
                        </>
                      }
                      onDelete={() => applyTerm(withoutFilter(interpretation, filter))}
                      deleteIcon={<CloseIcon />}
                    />
                  ))}
                </Box>
              )}
            </Container>
          </Box>

          {/* Search Results */}
          <Container maxWidth="xl" sx={{ position: "relative", py: { xs: 2, sm: 3 }, minHeight: 240 }}>
            {/* Refining an existing result set: keep the grid, show a thin bar */}
            {isBusy && hasResults && (
              <LinearProgress
                aria-label="Updating results"
                sx={{ position: "absolute", top: 0, left: 0, right: 0, height: 2 }}
              />
            )}

            {isBusy && hasQuery && !hasResults && (
              <Box sx={{ display: "flex", justifyContent: "center", py: 10 }}>
                <CircularProgress size={28} />
              </Box>
            )}

            {!isBusy &&
              hasQuery &&
              !hasResults &&
              !searchError &&
              message(
                <SearchOffOutlinedIcon sx={{ fontSize: 34, color: theme.palette.text.disabled }} />,
                emptyCopy.title,
                emptyCopy.detail,
              )}

            {!isBusy &&
              hasQuery &&
              !hasResults &&
              searchError &&
              message(
                <ErrorOutlineRoundedIcon sx={{ fontSize: 34, color: theme.palette.error.main }} />,
                "Search didn't go through",
                "Scout couldn't reach the search service. Try again in a moment.",
              )}

            {hasResults && (
              <SearchResultsGrid
                results={searchResults}
                onSelect={handleMediaClick}
                minCardWidth={190}
                sx={{
                  opacity: isPlaceholderData ? 0.55 : 1,
                  transition: "opacity .2s ease",
                }}
              />
            )}

            {!hasQuery &&
              message(
                <SearchIcon sx={{ fontSize: 34, color: theme.palette.text.disabled }} />,
                "Start typing to search",
                <>
                  Titles, years, actors, directors, studios, networks or an IMDb link. Try one:
                  <Box
                    sx={{
                      display: "flex",
                      flexWrap: "wrap",
                      justifyContent: "center",
                      gap: 1,
                      mt: 2,
                      maxWidth: 640,
                      mx: "auto",
                    }}
                  >
                    {SEARCH_EXAMPLES.map((example) => (
                      <Tooltip key={example.text} title={example.hint} arrow>
                        <Chip
                          label={example.text}
                          variant="outlined"
                          onClick={() => applyTerm(example.text, example.mediaType)}
                          sx={{
                            fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace",
                            fontSize: "0.8125rem",
                            borderColor: alpha(theme.palette.primary.light, 0.35),
                            "&:hover": { borderColor: theme.palette.primary.light },
                          }}
                        />
                      </Tooltip>
                    ))}
                  </Box>
                </>,
              )}
          </Container>
        </Box>
      </Slide>

      {selectedShowInfo && (
        <MediaStatusDialog
          open={statusDialogOpen}
          onClose={handleCloseStatusDialog}
          status={selectedShowStatus}
          mediaName={selectedShowInfo.name}
          posterUrl={selectedShowInfo.posterUrl}
          tvdbId={selectedShowInfo.tvdbId}
          enrichedMedia={selectedEnrichedMedia}
          onDownload={handleDownload}
          showDownloadButton={true}
          isDownloading={isDownloading}
          mediaType={mediaType === "movie" ? "movie" : "series"}
        />
      )}
    </>
  );
};

export default SearchDropdown;
