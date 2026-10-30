-- 005_add_torrent_hash.sql
-- Adds torrentHash column to ShowDownloadHistory for tracking and status updates.
ALTER TABLE ShowDownloadHistory ADD COLUMN IF NOT EXISTS torrentHash TEXT;
