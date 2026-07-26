import React, { useState, useEffect, useRef, useCallback } from "react";
import Box from "@mui/material/Box";
import Container from "@mui/material/Container";
import InputAdornment from "@mui/material/InputAdornment";
import TextField from "@mui/material/TextField";
import ToggleButton from "@mui/material/ToggleButton";
import ToggleButtonGroup from "@mui/material/ToggleButtonGroup";
import Typography from "@mui/material/Typography";
import IconButton from "@mui/material/IconButton";
import CircularProgress from "@mui/material/CircularProgress";
import Slide from "@mui/material/Slide";
import CloseIcon from "@mui/icons-material/Close";
import SearchIcon from "@mui/icons-material/Search";
import SearchOffOutlinedIcon from "@mui/icons-material/SearchOffOutlined";
import Tv from "@mui/icons-material/Tv";
import Movie from "@mui/icons-material/Movie";
import { alpha, useTheme } from "@mui/material/styles";
import InfiniteScroll from "react-infinite-scroll-component/dist/index.js";
import MediaCard from "./MediaCard";
import MediaStatusDialog from "./MediaStatusDialog";
import type { EnrichedMedia, ShowStatus } from "../../types/MediaStatus";
import { buildStatusBadgeFromEnriched } from "../../utils/statusHelpers";
import { logError } from "../../lib/logger";

interface SearchDropdownProps {
  isOpen: boolean;
  onClose: () => void;
}

/**
 * SearchDropdown Component
 *
 * Full-width search overlay that appears from the top of the screen.
 * Features:
 * - Semi-transparent backdrop (opacity 0.85)
 * - Search input with auto-focus
 * - Media type toggle (TV Shows / Movies)
 * - Debounced search (300ms)
 * - Infinite scroll for results
 * - Opens SearchResultDialog on result click
 * - Closes on backdrop click or ESC key
 * - Reuses existing search logic from Search.tsx
 */
const SearchDropdown: React.FC<SearchDropdownProps> = ({ isOpen, onClose }) => {
  const theme = useTheme();
  const [searchTerm, setSearchTerm] = useState("");
  const [debouncedSearchTerm, setDebouncedSearchTerm] = useState("");
  const [searchResults, setSearchResults] = useState<EnrichedMedia[]>([]);
  const [isSearching, setIsSearching] = useState(false);
  const [hasMore, setHasMore] = useState(false);
  const [page, setPage] = useState(1);
  const [mediaType, setMediaType] = useState<"series" | "movie">("series");
  const [statusDialogOpen, setStatusDialogOpen] = useState(false);
  const [selectedShowStatus, setSelectedShowStatus] = useState<ShowStatus | null>(null);
  const [selectedShowInfo, setSelectedShowInfo] = useState<{
    name: string;
    posterUrl: string;
    tvdbId: string;
  } | null>(null);
  const [selectedEnrichedMedia, setSelectedEnrichedMedia] = useState<EnrichedMedia | null>(null);
  const searchInputRef = useRef<HTMLInputElement>(null);

  // Debounce search term (500ms)
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedSearchTerm(searchTerm);
    }, 500);

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
      setSearchResults([]);
      setPage(1);
      setHasMore(false);
    }
  }, [isOpen]);

  // Fetch results when debounced search term changes
  useEffect(() => {
    if (debouncedSearchTerm.trim().length > 0) {
      performSearch(debouncedSearchTerm, 1);
    } else {
      setSearchResults([]);
      setHasMore(false);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [debouncedSearchTerm, mediaType]);

  const performSearch = async (query: string, pageNum: number) => {
    setIsSearching(true);
    try {
      const params = new URLSearchParams({
        query,
        media_type: mediaType,
        page: pageNum.toString(),
      });
      const res = await fetch(`/api/search/enriched?${params}`);
      const data = await res.json();

      if (pageNum === 1) {
        setSearchResults(data);
        setPage(1);
      } else {
        setSearchResults((prev) => [...prev, ...data]);
      }

      setHasMore(data.length > 0);
    } catch (error) {
      console.error("Search error:", error);
      logError("Search request failed in SearchDropdown", error as Error, {
        component: "SearchDropdown",
        query: query,
        pageNum: pageNum,
        mediaType: mediaType,
      });
      setSearchResults([]);
      setHasMore(false);
    } finally {
      setIsSearching(false);
    }
  };

  const fetchMoreData = useCallback(() => {
    if (!isSearching && hasMore) {
      const nextPage = page + 1;
      performSearch(debouncedSearchTerm, nextPage);
      setPage(nextPage);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [page, hasMore, isSearching, debouncedSearchTerm]);

  const handleMediaClick = (enrichedMedia: EnrichedMedia) => {
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
  };

  const handleCloseStatusDialog = () => {
    setStatusDialogOpen(false);
    setSelectedShowStatus(null);
    setSelectedShowInfo(null);
    setSelectedEnrichedMedia(null);
  };

  const handleDownload = () => {
    if (!selectedEnrichedMedia) return;

    const endpoint = mediaType === "movie" ? "/api/movies" : "/api/shows";

    fetch(endpoint, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(selectedEnrichedMedia.media),
    })
      .then((response) => {
        if (!response.ok) {
          throw new Error("Network response was not ok");
        }
        return response.json();
      })
      .then(() => {
        // Download initiated successfully
        handleCloseStatusDialog();
      })
      .catch((error) => {
        console.error("Error initiating download:", error);
        logError(
          "Download initiation failed in SearchDropdown",
          error as Error,
          {
            component: "SearchDropdown",
            endpoint: endpoint,
            mediaType: mediaType,
            resultId: selectedEnrichedMedia.media.id,
          },
        );
      });
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

  // Centered message block reused by the three non-result states
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
          <Container maxWidth="xl" sx={{ py: 3, minHeight: 240 }}>
            {isSearching && searchResults.length === 0 && (
              <Box sx={{ display: "flex", justifyContent: "center", py: 10 }}>
                <CircularProgress size={28} />
              </Box>
            )}

            {!isSearching &&
              hasQuery &&
              searchResults.length === 0 &&
              message(
                <SearchOffOutlinedIcon sx={{ fontSize: 34, color: theme.palette.text.disabled }} />,
                `No results for "${searchTerm}"`,
                "Try a different spelling, or switch between TV Shows and Movies.",
              )}

            {searchResults.length > 0 && (
              <InfiniteScroll
                dataLength={searchResults.length}
                next={fetchMoreData}
                hasMore={hasMore}
                loader={
                  <Box sx={{ display: "flex", justifyContent: "center", py: 3 }}>
                    <CircularProgress size={24} />
                  </Box>
                }
                style={{ overflow: "visible" }}
              >
                <Box
                  sx={{
                    display: "grid",
                    gridTemplateColumns: "repeat(auto-fill, minmax(190px, 1fr))",
                    gap: 3,
                  }}
                >
                  {searchResults.map((enrichedMedia) => {
                    const statusBadge = buildStatusBadgeFromEnriched(enrichedMedia.status);

                    return (
                      <MediaCard
                        key={enrichedMedia.media.id}
                        media={{
                          id: enrichedMedia.media.id,
                          name: enrichedMedia.media.mediaName,
                          imageUrl: enrichedMedia.media.image_url,
                        }}
                        onClick={() => handleMediaClick(enrichedMedia)}
                        statusBadge={statusBadge}
                        isLoadingStatus={false}
                      />
                    );
                  })}
                </Box>
              </InfiniteScroll>
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
          mediaType={mediaType === "movie" ? "movie" : "series"}
        />
      )}
    </>
  );
};

export default SearchDropdown;
