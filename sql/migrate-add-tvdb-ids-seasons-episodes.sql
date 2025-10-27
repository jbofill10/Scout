-- Migration: Add tvdb_id columns to Seasons and Episodes tables
-- Date: 2025-10-27
-- Description: Extends TVDB ID tracking to include seasons and episodes for better matching

-- Add tvdb_id column to Seasons table
ALTER TABLE Seasons ADD COLUMN IF NOT EXISTS tvdb_id TEXT;

-- Add tvdb_id column to Episodes table
ALTER TABLE Episodes ADD COLUMN IF NOT EXISTS tvdb_id TEXT;

-- Create indexes for faster lookups
CREATE INDEX IF NOT EXISTS idx_seasons_tvdb_id ON Seasons(tvdb_id);
CREATE INDEX IF NOT EXISTS idx_episodes_tvdb_id ON Episodes(tvdb_id);

-- Note: After running this migration, restart the torrenter service
-- to trigger syncPlexLibrary() which will populate the tvdb_id values
-- from Plex's external metadata with includeGuids=1.
