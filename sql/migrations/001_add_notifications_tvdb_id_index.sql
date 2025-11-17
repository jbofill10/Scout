-- Migration: Add index on tvdb_id for faster notification lookups
-- Date: 2025-11-16
-- Description: Since notifications now use episode-level tvdb_ids instead of show-level ids,
--              we need an index on tvdb_id for efficient Get() queries.
--              This index improves performance for torrenter's notification lookup operations.

-- Create index on tvdb_id for fast lookups
CREATE INDEX IF NOT EXISTS idx_notifications_tvdb_id ON Notifications(tvdb_id);

-- Optional: Add index on tvdb_id + auto_dismissed for even faster filtered queries
CREATE INDEX IF NOT EXISTS idx_notifications_tvdb_active ON Notifications(tvdb_id, auto_dismissed) WHERE auto_dismissed = false;
