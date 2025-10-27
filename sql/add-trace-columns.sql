-- Migration: Add trace_id and span_id columns to track OpenTelemetry trace context
-- This allows correlation of database records with distributed traces and logs

-- ========================================
-- ScheduledDownloads Table
-- ========================================
-- This table stores future episodes that haven't aired yet.
-- We store TWO sets of trace IDs:
--   1. scheduled_trace_id/scheduled_span_id: The original trace when user scheduled the download
--   2. trace_id/span_id: The trace when the scheduled download actually executes
-- This allows us to trace both the scheduling action AND the execution action.

ALTER TABLE ScheduledDownloads
ADD COLUMN IF NOT EXISTS scheduled_trace_id TEXT,
ADD COLUMN IF NOT EXISTS scheduled_span_id TEXT,
ADD COLUMN IF NOT EXISTS trace_id TEXT,
ADD COLUMN IF NOT EXISTS span_id TEXT;

COMMENT ON COLUMN ScheduledDownloads.scheduled_trace_id IS 'Trace ID from original scheduling request';
COMMENT ON COLUMN ScheduledDownloads.scheduled_span_id IS 'Span ID from original scheduling request';
COMMENT ON COLUMN ScheduledDownloads.trace_id IS 'Trace ID when scheduled download executes';
COMMENT ON COLUMN ScheduledDownloads.span_id IS 'Span ID when scheduled download executes';

-- Index for querying all scheduled downloads from a specific user action
CREATE INDEX IF NOT EXISTS idx_scheduled_downloads_scheduled_trace
ON ScheduledDownloads(scheduled_trace_id)
WHERE scheduled_trace_id IS NOT NULL;

-- Index for querying execution traces
CREATE INDEX IF NOT EXISTS idx_scheduled_downloads_execution_trace
ON ScheduledDownloads(trace_id)
WHERE trace_id IS NOT NULL;

-- ========================================
-- ShowDownloadHistory Table
-- ========================================
-- This table tracks individual episode download attempts.
-- Each episode gets its own trace span for detailed tracking.

ALTER TABLE ShowDownloadHistory
ADD COLUMN IF NOT EXISTS trace_id TEXT,
ADD COLUMN IF NOT EXISTS span_id TEXT;

COMMENT ON COLUMN ShowDownloadHistory.trace_id IS 'Trace ID from download request';
COMMENT ON COLUMN ShowDownloadHistory.span_id IS 'Span ID for this specific episode download';

-- Index for querying all episodes from a specific download request
CREATE INDEX IF NOT EXISTS idx_show_download_history_trace
ON ShowDownloadHistory(trace_id)
WHERE trace_id IS NOT NULL;

-- Index for finding a specific episode's span in traces
CREATE INDEX IF NOT EXISTS idx_show_download_history_span
ON ShowDownloadHistory(span_id)
WHERE span_id IS NOT NULL;

-- ========================================
-- MovieDownloadHistory Table
-- ========================================
-- This table tracks movie download attempts.

ALTER TABLE MovieDownloadHistory
ADD COLUMN IF NOT EXISTS trace_id TEXT,
ADD COLUMN IF NOT EXISTS span_id TEXT;

COMMENT ON COLUMN MovieDownloadHistory.trace_id IS 'Trace ID from download request';
COMMENT ON COLUMN MovieDownloadHistory.span_id IS 'Span ID for this specific movie download';

-- Index for querying all movies from a specific download request
CREATE INDEX IF NOT EXISTS idx_movie_download_history_trace
ON MovieDownloadHistory(trace_id)
WHERE trace_id IS NOT NULL;

-- Index for finding a specific movie's span in traces
CREATE INDEX IF NOT EXISTS idx_movie_download_history_span
ON MovieDownloadHistory(span_id)
WHERE span_id IS NOT NULL;

-- ========================================
-- Verification Queries
-- ========================================
-- After running this migration, you can verify the changes with:
--
-- \d+ ScheduledDownloads
-- \d+ ShowDownloadHistory
-- \d+ MovieDownloadHistory
--
-- Or query for traces:
-- SELECT * FROM ShowDownloadHistory WHERE trace_id = '<trace_id>';
-- SELECT * FROM ScheduledDownloads WHERE scheduled_trace_id = '<trace_id>';
