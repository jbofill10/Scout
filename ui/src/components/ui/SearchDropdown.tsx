import React, { useState, useEffect, useRef, useCallback } from "react";
import Box from "@mui/material/Box";
import TextField from "@mui/material/TextField";
import ToggleButton from "@mui/material/ToggleButton";
import ToggleButtonGroup from "@mui/material/ToggleButtonGroup";
import Typography from "@mui/material/Typography";
import IconButton from "@mui/material/IconButton";
import CircularProgress from "@mui/material/CircularProgress";
import Slide from "@mui/material/Slide";
import CloseIcon from "@mui/icons-material/Close";
import Tv from "@mui/icons-material/Tv";
import Movie from "@mui/icons-material/Movie";
import { useTheme } from "@mui/material/styles";
import InfiniteScroll from "react-infinite-scroll-component/dist/index.js";
import MediaCard from "./MediaCard";
import SearchResultDialog from "../SearchResultDialog";
import type { SearchResult } from "../SearchResultsList";
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
  const [searchResults, setSearchResults] = useState<SearchResult[]>([]);
  const [isSearching, setIsSearching] = useState(false);
  const [hasMore, setHasMore] = useState(false);
  const [page, setPage] = useState(1);
  const [mediaType, setMediaType] = useState<"series" | "movie">("series");
  const [dialogOpen, setDialogOpen] = useState(false);
  const [selectedResult, setSelectedResult] = useState<SearchResult | null>(
    null,
  );
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
      const res = await fetch(`/api/search?${params}`);
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

  const handleMediaClick = (result: SearchResult) => {
    setSelectedResult(result);
    setDialogOpen(true);
  };

  const handleCloseDialog = () => {
    setDialogOpen(false);
    setSelectedResult(null);
  };

  const handleDownload = (result: SearchResult) => {
    const endpoint = mediaType === "movie" ? "/api/movies" : "/api/shows";

    fetch(endpoint, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(result),
    })
      .then((response) => {
        if (!response.ok) {
          throw new Error("Network response was not ok");
        }
        return response.json();
      })
      .then(() => {
        // Download initiated successfully
        handleCloseDialog();
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
            resultId: result.id,
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

  return (
    <>
      {/* Backdrop */}
      <Box
        onClick={onClose}
        sx={{
          position: "fixed",
          top: 0,
          left: 0,
          right: 0,
          bottom: 0,
          backgroundColor: "rgba(0, 0, 0, 0.85)",
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
            overflowY: "auto",
          }}
        >
          {/* Search Header */}
          <Box
            sx={{ p: 3, borderBottom: `1px solid ${theme.palette.divider}` }}
          >
            <Box sx={{ display: "flex", alignItems: "center", gap: 2, mb: 2 }}>
              <TextField
                inputRef={searchInputRef}
                fullWidth
                placeholder="Search for shows or movies..."
                variant="outlined"
                value={searchTerm}
                onChange={(e) => setSearchTerm(e.target.value)}
                autoFocus
                sx={{
                  "& .MuiOutlinedInput-root": {
                    fontSize: "1.25rem",
                  },
                }}
              />
              <IconButton
                onClick={onClose}
                size="large"
                aria-label="Close search"
              >
                <CloseIcon />
              </IconButton>
            </Box>

            {/* Media Type Toggle */}
            <Box sx={{ display: "flex", justifyContent: "center" }}>
              <ToggleButtonGroup
                value={mediaType}
                exclusive
                onChange={(_, newValue) => newValue && setMediaType(newValue)}
                aria-label="media type"
              >
                <ToggleButton value="series" aria-label="TV shows">
                  <Tv sx={{ mr: 1 }} />
                  TV Shows
                </ToggleButton>
                <ToggleButton value="movie" aria-label="Movies">
                  <Movie sx={{ mr: 1 }} />
                  Movies
                </ToggleButton>
              </ToggleButtonGroup>
            </Box>
          </Box>

          {/* Search Results */}
          <Box sx={{ p: 3, minHeight: 200 }}>
            {isSearching && searchResults.length === 0 && (
              <Box
                sx={{
                  display: "flex",
                  justifyContent: "center",
                  alignItems: "center",
                  py: 8,
                }}
              >
                <CircularProgress />
              </Box>
            )}

            {!isSearching &&
              searchTerm.trim().length > 0 &&
              searchResults.length === 0 && (
                <Box
                  sx={{
                    display: "flex",
                    justifyContent: "center",
                    alignItems: "center",
                    py: 8,
                  }}
                >
                  <Typography
                    variant="body1"
                    sx={{ color: theme.palette.text.secondary }}
                  >
                    No results found for "{searchTerm}"
                  </Typography>
                </Box>
              )}

            {searchResults.length > 0 && (
              <InfiniteScroll
                dataLength={searchResults.length}
                next={fetchMoreData}
                hasMore={hasMore}
                loader={
                  <Box
                    sx={{ display: "flex", justifyContent: "center", py: 2 }}
                  >
                    <CircularProgress size={24} />
                  </Box>
                }
                style={{ overflow: "visible" }}
              >
                <Box
                  sx={{
                    display: "grid",
                    gridTemplateColumns:
                      "repeat(auto-fill, minmax(180px, 1fr))",
                    gap: 3,
                  }}
                >
                  {searchResults.map((result) => (
                    <MediaCard
                      key={result.id}
                      media={{
                        id: result.id,
                        name: result.mediaName,
                        imageUrl: result.image_url,
                      }}
                      onClick={() => handleMediaClick(result)}
                    />
                  ))}
                </Box>
              </InfiniteScroll>
            )}

            {searchTerm.trim().length === 0 && (
              <Box
                sx={{
                  display: "flex",
                  justifyContent: "center",
                  alignItems: "center",
                  py: 8,
                }}
              >
                <Typography
                  variant="body1"
                  sx={{ color: theme.palette.text.secondary }}
                >
                  Start typing to search for media
                </Typography>
              </Box>
            )}
          </Box>
        </Box>
      </Slide>

      <SearchResultDialog
        open={dialogOpen}
        onClose={handleCloseDialog}
        result={selectedResult}
        onDownload={handleDownload}
        mediaType={mediaType}
      />
    </>
  );
};

export default SearchDropdown;
