-- 007_add_active_torrents_table.sql
-- Persists in-flight torrents so completion monitors survive a torrenter restart.
-- Before this, monitorTorrentCompletion lived only in memory: a restart silently
-- orphaned every download in progress, leaving the torrent finished in
-- qBittorrent but never linked into Plex and stuck "downloading" in the UI.
--
-- A row exists for exactly as long as a torrent is in flight and unaccounted for.
-- It is deleted once the completion event has been processed, or once the monitor
-- gives up (deadline reached / torrent no longer in qBittorrent).

CREATE TABLE IF NOT EXISTS ActiveTorrents (
    id SERIAL PRIMARY KEY,
    info_hash TEXT NOT NULL UNIQUE,
    tracking_uuid TEXT NOT NULL,
    torrent_title TEXT NOT NULL,
    -- Full models.SearchStrategy; ProcessDownloadedTorrent needs it to rebuild
    -- the target path, and it cannot be re-derived after a restart.
    strategy JSONB NOT NULL,
    -- Original monitor start, so a resumed monitor keeps the deadline it was born
    -- with rather than getting a fresh window on every restart.
    started_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    trace_id TEXT,
    span_id TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_active_torrents_info_hash ON ActiveTorrents(info_hash);
