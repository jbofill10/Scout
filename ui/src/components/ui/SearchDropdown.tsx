import React, { useState, useEffect, useRef, useCallback } from "react";
import Box from "@mui/material/Box";
import Container from "@mui/material/Container";
import InputAdornment from "@mui/material/InputAdornment";
import LinearProgress from "@mui/material/LinearProgress";
import TextField from "@mui/material/TextField";
import ToggleButton from "@mui/material/ToggleButton";
import ToggleButtonGroup from "@mui/material/ToggleButtonGroup";
import Typography from "@mui/material/Typography";
import IconButton from "@mui/material/IconButton";
import CircularProgress from "@mui/material/CircularProgress";
import Slide from "@mui/material/Slide";
import CloseIcon from "@mui/icons-material/Close";
import ErrorOutlineRoundedIcon from "@mui/icons-material/ErrorOutlineRounded";
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

interface SearchDropdownProps {
  isOpen: boolean;
  onClose: () => void;
}

const SEARCH_DEBOUNCE_MS = 300;

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
 * - Opens MediaStatusDialog on result click
 * - Closes on backdrop click or ESC key
 */
const SearchDropdown: React.FC<SearchDropdownProps> = ({ isOpen, onClose }) => {
  const theme = useTheme();
  const [searchTerm, setSearchTerm] = useState("");
  const [debouncedSearchTerm, setDebouncedSearchTerm] = useState("");
  const [mediaType, setMediaType] = useState<"series" | "movie">("series");
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

  // Centered message block reused by the non-result states
  const message = (icon: React.ReactNode, title: string, detail?: string) => (
    <Box
      sx={{
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        gap: 1,
        py: 10,
        textAlign: "center",
      }}
    >
      {icon}
      <Typography variant="subtitle1" sx={{ color: theme.palette.text.primary }}>
        {title}
      </Typography>
      {detail && (
        <Typography variant="body2" sx={{ color: theme.palette.text.secondary }}>
          {detail}
        </Typography>
      )}
    </Box>
  );

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
            maxHeight: "90vh",
            backgroundColor: theme.palette.background.default,
            zIndex: 1300,
            borderBottom: `1px solid ${theme.palette.divider}`,
            borderBottomLeftRadius: 20,
            borderBottomRightRadius: 20,
            boxShadow: theme.shadows[16],
            overflowY: "auto",
          }}
        >
          {/* Search Header */}
          <Box sx={{ borderBottom: `1px solid ${theme.palette.divider}`, py: 2.5 }}>
            <Container maxWidth="xl">
              <Box sx={{ display: "flex", alignItems: "center", gap: 2 }}>
                <TextField
                  inputRef={searchInputRef}
                  fullWidth
                  placeholder="Search for shows or movies..."
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
                      endAdornment: hasQuery ? (
                        <InputAdornment position="end">
                          <IconButton
                            size="small"
                            onClick={() => setSearchTerm("")}
                            aria-label="Clear search"
                          >
                            <CloseIcon fontSize="small" />
                          </IconButton>
                        </InputAdornment>
                      ) : undefined,
                    },
                  }}
                  sx={{ "& .MuiOutlinedInput-root": { fontSize: "1.0625rem" } }}
                />

                {/* Media Type Toggle */}
                <ToggleButtonGroup
                  value={mediaType}
                  exclusive
                  onChange={(_, newValue) => newValue && setMediaType(newValue)}
                  aria-label="media type"
                  sx={{ flexShrink: 0 }}
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

                <IconButton onClick={onClose} aria-label="Close search">
                  <CloseIcon />
                </IconButton>
              </Box>
            </Container>
          </Box>

          {/* Search Results */}
          <Container maxWidth="xl" sx={{ position: "relative", py: 3, minHeight: 240 }}>
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
                `No results for "${searchTerm}"`,
                "Try a different spelling, or switch between TV Shows and Movies.",
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
                "Scout looks across TVDB for anything you want to add to your library.",
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
