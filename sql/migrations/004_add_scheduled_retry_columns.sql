-- Migration: Add retry metadata columns to ScheduledDownloads
-- Date: 2026-02-07
-- Purpose:
--   Enable durable retry/backoff for scheduled download failures.

ALTER TABLE ScheduledDownloads
    ADD COLUMN IF NOT EXISTS retry_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS max_retries INTEGER NOT NULL DEFAULT 5,
    ADD COLUMN IF NOT EXISTS last_error TEXT,
    ADD COLUMN IF NOT EXISTS last_attempt_at TIMESTAMP,
    ADD COLUMN IF NOT EXISTS next_attempt_at TIMESTAMP;

CREATE INDEX IF NOT EXISTS idx_scheduled_downloads_next_attempt
ON ScheduledDownloads(next_attempt_at);
