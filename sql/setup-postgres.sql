-- PostgreSQL setup script for Scout database
-- Run this script to initialize the database tables
-- This script includes all migrations and can be used to set up a fresh database

-- Webserver tables
-- Stores complete tvdb.Media objects ready to be sent to torrenter.Download()
-- This table stores TWO sets of trace IDs:
--   1. scheduled_trace_id/scheduled_span_id: The original trace when user scheduled the download
--   2. trace_id/span_id: The trace when the scheduled download actually executes
CREATE TABLE IF NOT EXISTS ScheduledDownloads (
    id SERIAL PRIMARY KEY,
    media JSONB NOT NULL,
    content_hash TEXT,
    release_time TIMESTAMP NOT NULL,
    schedule_status TEXT DEFAULT 'pending',
    scheduled_trace_id TEXT,
    scheduled_span_id TEXT,
    trace_id TEXT,
    span_id TEXT,
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMP,
    queued_at TIMESTAMP,
    last_failure_code TEXT,
    last_failure_reason TEXT
);

CREATE INDEX IF NOT EXISTS idx_scheduled_downloads_release ON ScheduledDownloads(release_time);
CREATE INDEX IF NOT EXISTS idx_scheduled_downloads_status ON ScheduledDownloads(schedule_status);
CREATE INDEX IF NOT EXISTS idx_scheduled_downloads_next_attempt ON ScheduledDownloads(schedule_status, next_attempt_at);

-- Index to ensure we can quickly detect duplicate scheduled content by its content hash
CREATE UNIQUE INDEX IF NOT EXISTS idx_scheduled_downloads_hash ON ScheduledDownloads(content_hash);

-- Index for querying all scheduled downloads from a specific user action
CREATE INDEX IF NOT EXISTS idx_scheduled_downloads_scheduled_trace
ON ScheduledDownloads(scheduled_trace_id)
WHERE scheduled_trace_id IS NOT NULL;

-- Index for querying execution traces
CREATE INDEX IF NOT EXISTS idx_scheduled_downloads_execution_trace
ON ScheduledDownloads(trace_id)
WHERE trace_id IS NOT NULL;

-- Torrenter tables
CREATE TABLE IF NOT EXISTS Libraries (
    id SERIAL PRIMARY KEY,
    type TEXT NOT NULL,
    path TEXT NOT NULL,
    preferred INTEGER DEFAULT 0,
    section INTEGER NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_unique_preferred_type
ON Libraries(type)
WHERE preferred = 1;

CREATE TABLE IF NOT EXISTS Movies (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    year INTEGER,
    thumb TEXT,
    art TEXT,
    tvdb_id TEXT,
    base_directory TEXT
);

CREATE INDEX IF NOT EXISTS idx_movies_tvdb_id ON Movies(tvdb_id);

COMMENT ON COLUMN Movies.tvdb_id IS 'TVDB series ID for matching';

CREATE TABLE IF NOT EXISTS MovieMedia (
    id SERIAL PRIMARY KEY,
    parentId TEXT REFERENCES Movies(id),
    video_resolution TEXT,
    file_path TEXT
);

-- The Plex sync replaces a movie's media rows on every run (DELETE ... WHERE parentId = ?),
-- so this index keeps that from degrading into a sequential scan per movie.
CREATE INDEX IF NOT EXISTS idx_moviemedia_parentid ON MovieMedia(parentId);

CREATE TABLE IF NOT EXISTS Shows (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    show_meta TEXT,
    thumb TEXT,
    tvdb_id TEXT,
    base_directory TEXT
);

CREATE INDEX IF NOT EXISTS idx_shows_tvdb_id ON Shows(tvdb_id);

CREATE TABLE IF NOT EXISTS Seasons (
    id TEXT PRIMARY KEY,
    parentId TEXT REFERENCES Shows(id),
    season_meta TEXT,
    season_number INTEGER,
    tvdb_id TEXT
);

CREATE INDEX IF NOT EXISTS idx_seasons_tvdb_id ON Seasons(tvdb_id);

COMMENT ON COLUMN Seasons.tvdb_id IS 'TVDB season ID for matching';

CREATE TABLE IF NOT EXISTS Episodes (
    id TEXT PRIMARY KEY,
    parentId TEXT REFERENCES Seasons(id),
    episode_meta TEXT,
    episode_number INTEGER,
    tvdb_id TEXT
);

CREATE INDEX IF NOT EXISTS idx_episodes_tvdb_id ON Episodes(tvdb_id);

COMMENT ON COLUMN Episodes.tvdb_id IS 'TVDB episode ID for matching';

-- TvdbEpisodes table stores TVDB episode metadata for shows in the library
-- This enables detection of "missing episodes" (aired but not downloaded)
-- Data is synced automatically during the download flow
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

CREATE TABLE IF NOT EXISTS EpisodeMedia (
    id SERIAL PRIMARY KEY,
    parentId TEXT REFERENCES Episodes(id),
    video_resolution TEXT,
    file_path TEXT
);

-- The Plex sync replaces an episode's media rows on every run (DELETE ... WHERE parentId = ?),
-- so this index keeps that from degrading into a sequential scan per episode.
CREATE INDEX IF NOT EXISTS idx_episodemedia_parentid ON EpisodeMedia(parentId);

CREATE TABLE IF NOT EXISTS ShowDownloadHistory (
    id SERIAL PRIMARY KEY,
    mediaTitle TEXT NOT NULL,
    season INTEGER,
    episode INTEGER,
    absoluteEpisode INTEGER,
    torrentHash TEXT,
    downloadDate TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    reason TEXT,
    status TEXT NOT NULL,
    trace_id TEXT,
    span_id TEXT
);

-- Index for querying all episodes from a specific download request
CREATE INDEX IF NOT EXISTS idx_show_download_history_trace
ON ShowDownloadHistory(trace_id)
WHERE trace_id IS NOT NULL;

-- Index for finding a specific episode's span in traces
CREATE INDEX IF NOT EXISTS idx_show_download_history_span
ON ShowDownloadHistory(span_id)
WHERE span_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS MovieDownloadHistory (
    id SERIAL PRIMARY KEY,
    mediaTitle TEXT NOT NULL,
    year INTEGER,
    downloadDate TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    reason TEXT,
    status TEXT NOT NULL,
    trace_id TEXT,
    span_id TEXT
);

-- Index for querying all movies from a specific download request
CREATE INDEX IF NOT EXISTS idx_movie_download_history_trace
ON MovieDownloadHistory(trace_id)
WHERE trace_id IS NOT NULL;

-- Index for finding a specific movie's span in traces
CREATE TABLE IF NOT EXISTS UploaderPreferences (
    id SERIAL PRIMARY KEY,
    mediaType TEXT NOT NULL,
    isAnime BOOLEAN NOT NULL,
    uploaderName TEXT NOT NULL
);

-- Notifications table for tracking download progress
CREATE TABLE IF NOT EXISTS Notifications (
    id SERIAL PRIMARY KEY,
    tvdb_id TEXT NOT NULL,
    media_title TEXT NOT NULL,
    category TEXT NOT NULL, -- 'series' or 'movie'
    season INTEGER,
    episode INTEGER,
    absolute_episode INTEGER,
    poster_url TEXT,
    is_anime BOOLEAN DEFAULT FALSE,
    status TEXT NOT NULL, -- 'scheduled', 'searching', 'downloading', 'completed', 'failed'
    reason TEXT,
    is_read BOOLEAN DEFAULT FALSE,
    auto_dismissed BOOLEAN DEFAULT FALSE,
    trace_id TEXT,
    span_id TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tvdb_id, season, episode) -- Prevent duplicate notifications per episode
);

CREATE INDEX IF NOT EXISTS idx_notifications_tvdb_id ON Notifications(tvdb_id);
CREATE INDEX IF NOT EXISTS idx_notifications_status ON Notifications(status);
CREATE INDEX IF NOT EXISTS idx_notifications_created_at ON Notifications(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_notifications_is_read ON Notifications(is_read);

-- Note: After running this script, restart the torrenter service to trigger
-- syncPlexLibrary() which will populate the tvdb_id values from Plex's external metadata.

-- Note: After running this script on a fresh database or applying migrations,
-- restart the torrenter service to trigger syncPlexLibrary() which will
-- populate the tvdb_id values from Plex's external metadata.
