-- Migration: 002_backfill_notifications_from_history.sql
-- Description: Migrate existing ShowDownloadHistory entries to Notifications table
-- Date: 2025-11-15

-- Backfill notifications from ShowDownloadHistory
-- Maps old status values to new stage/status schema
-- Only runs while the legacy schema is still in place: Notifications still has the
-- stage column (dropped by 003) and ShowDownloadHistory still has the legacy
-- tvdb_id/media_title/downloaded_at columns (absent in setup-postgres.sql).
-- Once either is gone the backfill is skipped, so rows a user has since deleted
-- are never resurrected. The NOT EXISTS guard also keeps a repeat run duplicate-free.
DO $mig$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'notifications' AND column_name = 'stage'
    ) AND (
        SELECT count(*) FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'showdownloadhistory'
          AND column_name IN ('tvdb_id', 'media_title', 'season', 'episode', 'status', 'reason', 'downloaded_at')
    ) = 7 THEN
        INSERT INTO Notifications (
            tvdb_id,
            media_title,
            category,
            season,
            episode,
            poster_url,
            stage,
            status,
            reason,
            is_read,
            created_at,
            updated_at
        )
        SELECT
            tvdb_id,
            media_title,
            'series' as category, -- ShowDownloadHistory is only for series
            season,
            episode,
            '' as poster_url, -- Not stored in ShowDownloadHistory
            CASE
                WHEN status = 'success' THEN 'completed'
                WHEN status = 'failure' THEN 'failed'
                WHEN status = 'searching' THEN 'searching'
                ELSE 'failed'
            END as stage,
            status,
            reason,
            true as is_read, -- Mark historical notifications as read
            downloaded_at as created_at,
            downloaded_at as updated_at
        FROM ShowDownloadHistory
        WHERE NOT EXISTS (
            -- Avoid duplicates if migration is run multiple times
            SELECT 1 FROM Notifications n
            WHERE n.tvdb_id = ShowDownloadHistory.tvdb_id
            AND n.season = ShowDownloadHistory.season
            AND n.episode = ShowDownloadHistory.episode
        )
        ORDER BY downloaded_at ASC;
    END IF;
END $mig$;

-- Note: MovieDownloadHistory table exists but is currently unused
-- If it gets populated in the future, add similar migration for movies

-- Rollback script (uncomment to revert):
-- DELETE FROM Notifications WHERE category = 'series' AND is_read = true;
