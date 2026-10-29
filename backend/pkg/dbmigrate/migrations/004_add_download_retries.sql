-- 004_add_download_retries.sql
-- Adds retry-engine columns to ScheduledDownloads and fixes the "queued trap":
-- pre-existing rows stranded in 'queued' are migrated to 'completed'.

-- The queued->completed fix-up is one-time: 'queued' is a live status now, so it
-- must only run on the pass that adds the columns (attempts missing), never on a
-- re-run, which would clobber in-flight dispatches.
DO $mig$
DECLARE
    first_run BOOLEAN;
BEGIN
    first_run := NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'scheduleddownloads' AND column_name = 'attempts'
    );

    ALTER TABLE ScheduledDownloads
      ADD COLUMN IF NOT EXISTS attempts INTEGER NOT NULL DEFAULT 0,
      ADD COLUMN IF NOT EXISTS next_attempt_at TIMESTAMP,
      ADD COLUMN IF NOT EXISTS queued_at TIMESTAMP,
      ADD COLUMN IF NOT EXISTS last_failure_code TEXT,
      ADD COLUMN IF NOT EXISTS last_failure_reason TEXT;

    CREATE INDEX IF NOT EXISTS idx_scheduled_downloads_next_attempt ON ScheduledDownloads(schedule_status, next_attempt_at);

    IF first_run THEN
        UPDATE ScheduledDownloads SET schedule_status='completed' WHERE schedule_status='queued';
    END IF;
END $mig$;
