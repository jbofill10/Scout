-- Migration: 001_add_notifications_table.sql
-- Description: Create Notifications table for tracking download progress and status
-- Date: 2025-11-15

-- Create Notifications table
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
    stage TEXT NOT NULL, -- 'scheduled', 'searching', 'downloading', 'completed', 'failed'
    status TEXT NOT NULL, -- 'searching', 'downloading', 'success', 'failure'
    reason TEXT,
    torrent_hash TEXT,
    is_read BOOLEAN DEFAULT FALSE,
    auto_dismissed BOOLEAN DEFAULT FALSE,
    trace_id TEXT,
    span_id TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tvdb_id, season, episode) -- Prevent duplicate notifications per episode
);

-- Create indexes for efficient queries
CREATE INDEX IF NOT EXISTS idx_notifications_tvdb_id ON Notifications(tvdb_id);
-- stage was dropped by 003; on a database that already ran 003 (or was built from
-- setup-postgres.sql) the column is gone, so only index it when it exists.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'notifications' AND column_name = 'stage'
    ) THEN
        CREATE INDEX IF NOT EXISTS idx_notifications_stage ON Notifications(stage);
    END IF;
END $$;
CREATE INDEX IF NOT EXISTS idx_notifications_status ON Notifications(status);
CREATE INDEX IF NOT EXISTS idx_notifications_created_at ON Notifications(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_notifications_is_read ON Notifications(is_read);

-- Rollback script (uncomment to revert):
-- DROP INDEX IF EXISTS idx_notifications_is_read;
-- DROP INDEX IF EXISTS idx_notifications_created_at;
-- DROP INDEX IF EXISTS idx_notifications_status;
-- DROP INDEX IF EXISTS idx_notifications_stage;
-- DROP INDEX IF EXISTS idx_notifications_tvdb_id;
-- DROP TABLE IF EXISTS Notifications;
