-- Migration script to add tvdb_id columns to existing Scout databases
-- Run this on existing PostgreSQL databases before deploying the updated code

-- Add tvdb_id column to Shows table
ALTER TABLE Shows ADD COLUMN IF NOT EXISTS tvdb_id TEXT;

-- Add tvdb_id column to Movies table
ALTER TABLE Movies ADD COLUMN IF NOT EXISTS tvdb_id TEXT;

-- Create indexes for faster lookups
CREATE INDEX IF NOT EXISTS idx_shows_tvdb_id ON Shows(tvdb_id);
CREATE INDEX IF NOT EXISTS idx_movies_tvdb_id ON Movies(tvdb_id);

-- Note: After running this migration, restart the torrenter service
-- to trigger syncPlexLibrary() which will populate the tvdb_id values
-- from Plex's external metadata.
