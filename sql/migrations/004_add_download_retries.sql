-- 004_add_download_retries.sql
-- Adds retry-engine columns to ScheduledDownloads and fixes the "queued trap":
-- pre-existing rows stranded in 'queued' are migrated to 'completed'.

ALTER TABLE ScheduledDownloads
  ADD COLUMN IF NOT EXISTS attempts INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS next_attempt_at TIMESTAMP,
  ADD COLUMN IF NOT EXISTS queued_at TIMESTAMP,
  ADD COLUMN IF NOT EXISTS last_failure_code TEXT,
  ADD COLUMN IF NOT EXISTS last_failure_reason TEXT;

CREATE INDEX IF NOT EXISTS idx_scheduled_downloads_next_attempt ON ScheduledDownloads(schedule_status, next_attempt_at);

UPDATE ScheduledDownloads SET schedule_status='completed' WHERE schedule_status='queued';
