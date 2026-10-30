-- Migration: 003_simplify_notifications.sql
-- Description: Simplify notifications by removing stage and torrent_hash columns,
--              consolidating to single status field with 5 values
-- Date: 2025-11-16
-- Note: the runner wraps each file in a transaction, so no BEGIN/COMMIT here.

-- Step 1: Update existing status values to match the simplified 5-status model
-- Map old stage+status combination to new single status field.
-- Guarded: it reads the stage column, which only exists until this migration has run
-- (a repeat run, or a database built from setup-postgres.sql, skips it).
DO $mig$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'notifications' AND column_name = 'stage'
    ) THEN
        UPDATE Notifications
        SET status = CASE
            WHEN stage = 'scheduled' THEN 'scheduled'
            WHEN stage = 'searching' THEN 'searching'
            WHEN stage = 'downloading' THEN 'downloading'
            WHEN stage = 'completed' AND status = 'success' THEN 'completed'
            WHEN stage = 'failed' OR status = 'failure' THEN 'failed'
            ELSE 'failed' -- Default fallback for any unexpected states
        END;
    END IF;
END $mig$;

-- Step 2: Drop the index on stage column (before dropping column)
DROP INDEX IF EXISTS idx_notifications_stage;

-- Step 3: Drop the stage column (no longer needed)
ALTER TABLE Notifications DROP COLUMN IF EXISTS stage;

-- Step 4: Drop the torrent_hash column (no longer needed)
ALTER TABLE Notifications DROP COLUMN IF EXISTS torrent_hash;

-- Step 5: Add comment to clarify new status values
COMMENT ON COLUMN Notifications.status IS 'Download status: scheduled, searching, downloading, completed, failed';

-- Rollback script (uncomment to revert):
-- BEGIN;
-- ALTER TABLE Notifications ADD COLUMN stage TEXT NOT NULL DEFAULT 'searching';
-- ALTER TABLE Notifications ADD COLUMN torrent_hash TEXT;
-- CREATE INDEX idx_notifications_stage ON Notifications(stage);
-- -- Note: Reverting status values requires custom logic based on your data
-- COMMIT;
