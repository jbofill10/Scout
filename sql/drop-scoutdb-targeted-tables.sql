-- drop-scoutdb-targeted-tables.sql
--
-- Drops only the Scout application tables in database 'scoutdb'.
-- This targets the tables defined in sql/setup-postgres.sql so it won't indiscriminately
-- drop unrelated user tables.
-- WARNING: Destructive. Run only against a test copy or when you intend to wipe these tables.
-- Usage:
--   psql -d scoutdb -f sql/drop-scoutdb-targeted-tables.sql

-- Safety: ensure we're connected to the intended database
DO $$
BEGIN
  IF current_database() <> 'scoutdb' THEN
    RAISE EXCEPTION 'This script must be run against database "scoutdb". Current DB: %', current_database();
  END IF;
END$$;

BEGIN;

-- Drop child tables first to avoid FK issues (use IF EXISTS and CASCADE as a final safeguard)
-- Episode-related
DROP TABLE IF EXISTS EpisodeMedia CASCADE;
DROP TABLE IF EXISTS Episodes CASCADE;
DROP TABLE IF EXISTS Seasons CASCADE;

-- Show-related
DROP TABLE IF EXISTS ShowDownloadHistory CASCADE;
DROP TABLE IF EXISTS Shows CASCADE;

-- Movie-related
DROP TABLE IF EXISTS MovieMedia CASCADE;
DROP TABLE IF EXISTS MovieDownloadHistory CASCADE;
DROP TABLE IF EXISTS Movies CASCADE;

-- Libraries and uploads
DROP TABLE IF EXISTS Libraries CASCADE;
DROP TABLE IF EXISTS UploaderPreferences CASCADE;

-- Scheduled downloads
DROP TABLE IF EXISTS ScheduledDownloads CASCADE;

COMMIT;

-- End of file
