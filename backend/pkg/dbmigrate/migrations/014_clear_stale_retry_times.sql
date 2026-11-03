-- 014_clear_stale_retry_times.sql
-- A row that reached completed or failed used to keep the next_attempt_at of
-- the retry that got it there, and the activity view read that as a retry
-- still waiting ("Retrying any moment (attempt 4)" on a finished download).
-- Terminal rows now drop the timer when they get there; this clears the ones
-- already in the table. Cancelled rows are included for the same invariant:
-- next_attempt_at is only set while a row is pending or queued.

UPDATE ScheduledDownloads
SET next_attempt_at = NULL
WHERE schedule_status IN ('completed', 'failed', 'cancelled')
  AND next_attempt_at IS NOT NULL;
