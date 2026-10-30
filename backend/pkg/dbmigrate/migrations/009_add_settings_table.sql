-- 009_add_settings_table.sql
-- Runtime settings edited from the Settings page. Each row overrides one
-- environment default (quality floor, subtitle languages, Plex sync timing);
-- a missing row means the environment value applies. Values are JSON so a key
-- can hold a string, a boolean or a list.
--
-- UploaderPreferences becomes editable from the UI at the same time, so stop
-- the same uploader being stored twice for one media type.

CREATE TABLE IF NOT EXISTS Settings (
    key TEXT PRIMARY KEY,
    value JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_uploader_preferences_unique
ON UploaderPreferences (mediaType, isAnime, lower(uploaderName));

-- Rollback:
-- DROP INDEX IF EXISTS idx_uploader_preferences_unique;
-- DROP TABLE IF EXISTS Settings;
