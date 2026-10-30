-- Migration: Add TvdbEpisodes table
-- Date: 2026-07-26
-- Purpose:
--   Store TVDB episode metadata for shows in the Plex library so the library
--   page can detect "missing episodes" (aired on TVDB but not downloaded).
--   Populated by the torrenter's TVDB refresh service and the download flow.

CREATE TABLE IF NOT EXISTS TvdbEpisodes (
    tvdb_id TEXT PRIMARY KEY,
    series_tvdb_id TEXT NOT NULL,
    season_number INTEGER NOT NULL,
    episode_number INTEGER NOT NULL,
    absolute_number INTEGER,
    name TEXT,
    aired DATE,
    UNIQUE(series_tvdb_id, season_number, episode_number)
);

CREATE INDEX IF NOT EXISTS idx_tvdb_episodes_series ON TvdbEpisodes(series_tvdb_id);
CREATE INDEX IF NOT EXISTS idx_tvdb_episodes_aired ON TvdbEpisodes(aired);

COMMENT ON TABLE TvdbEpisodes IS 'TVDB episode metadata for library shows - enables missing episode detection';
COMMENT ON COLUMN TvdbEpisodes.series_tvdb_id IS 'Links to Shows.tvdb_id';
