-- PostgreSQL setup script for Scout database
-- Run this script to initialize the database tables

-- Webserver tables
-- Stores complete tvdb.Media objects ready to be sent to torrenter.Download()
CREATE TABLE IF NOT EXISTS ScheduledDownloads (
    id SERIAL PRIMARY KEY,
    media JSONB NOT NULL,
    content_hash TEXT,
    release_time TIMESTAMP NOT NULL,
    schedule_status TEXT DEFAULT 'pending'
);

CREATE INDEX IF NOT EXISTS idx_scheduled_downloads_release ON ScheduledDownloads(release_time);
CREATE INDEX IF NOT EXISTS idx_scheduled_downloads_status ON ScheduledDownloads(schedule_status);

-- Index to ensure we can quickly detect duplicate scheduled content by its content hash
CREATE UNIQUE INDEX IF NOT EXISTS idx_scheduled_downloads_hash ON ScheduledDownloads(content_hash);

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
    tvdb_id TEXT
);

CREATE INDEX IF NOT EXISTS idx_movies_tvdb_id ON Movies(tvdb_id);

CREATE TABLE IF NOT EXISTS MovieMedia (
    id SERIAL PRIMARY KEY,
    parentId TEXT REFERENCES Movies(id),
    video_resolution TEXT,
    file_path TEXT
);

CREATE TABLE IF NOT EXISTS Shows (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    show_meta TEXT,
    thumb TEXT,
    tvdb_id TEXT
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

CREATE TABLE IF NOT EXISTS Episodes (
    id TEXT PRIMARY KEY,
    parentId TEXT REFERENCES Seasons(id),
    episode_meta TEXT,
    episode_number INTEGER,
    tvdb_id TEXT
);

CREATE INDEX IF NOT EXISTS idx_episodes_tvdb_id ON Episodes(tvdb_id);

CREATE TABLE IF NOT EXISTS EpisodeMedia (
    id SERIAL PRIMARY KEY,
    parentId TEXT REFERENCES Episodes(id),
    video_resolution TEXT,
    file_path TEXT
);

CREATE TABLE IF NOT EXISTS ShowDownloadHistory (
    id SERIAL PRIMARY KEY,
    mediaTitle TEXT NOT NULL,
    season INTEGER,
    episode INTEGER,
    downloadDate TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    reason TEXT,
    status TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS MovieDownloadHistory (
    id SERIAL PRIMARY KEY,
    mediaTitle TEXT NOT NULL,
    year INTEGER,
    downloadDate TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    reason TEXT,
    status TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS UploaderPreferences (
    id SERIAL PRIMARY KEY,
    mediaType TEXT NOT NULL,
    isAnime BOOLEAN NOT NULL,
    uploaderName TEXT NOT NULL
);