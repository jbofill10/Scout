-- 008_drop_movie_download_history.sql
-- MovieDownloadHistory was created alongside ShowDownloadHistory but nothing ever
-- read or wrote it: movie attempts are tracked through ScheduledDownloads and
-- Notifications like episodes are. Drop it so the schema stops implying a
-- feature that does not exist.

DROP INDEX IF EXISTS idx_movie_download_history_trace;
DROP INDEX IF EXISTS idx_movie_download_history_span;
DROP TABLE IF EXISTS MovieDownloadHistory;

-- Rollback:
-- CREATE TABLE IF NOT EXISTS MovieDownloadHistory (
--     id SERIAL PRIMARY KEY,
--     mediaTitle TEXT NOT NULL,
--     year INTEGER,
--     downloadDate TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
--     reason TEXT,
--     status TEXT NOT NULL,
--     trace_id TEXT,
--     span_id TEXT
-- );
