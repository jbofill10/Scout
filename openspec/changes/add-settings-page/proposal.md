# Change: Add a settings page for the knobs that live in env vars and raw SQL

## Why
The quality floor, subtitle languages, Plex sync timing and preferred uploaders all exist, but changing any of them means editing `.env` and rebuilding a container, or inserting rows into `UploaderPreferences` with psql. The picker reads uploader preferences on every selection, yet nothing in the app can edit them.

## What Changes
- A `Settings` table (key/value JSONB) that overrides the environment defaults at runtime, read by the torrenter through a short-lived cache so a change applies to the next pick, the next subtitle fetch and the next sync tick without a restart.
- Torrenter endpoints `GET/PUT /settings` (quality floor, subtitle languages, prefer SDH, Plex sync delay and interval) and `GET/POST/DELETE /settings/uploaders`; the webserver proxies them under `/api/settings`.
- A Settings page in the UI, reached from a gear in the navbar, with Downloads, Preferred uploaders, Subtitles and Library sync sections, each showing the environment default and whether it is overridden.
- Env vars keep working as the defaults; a "reset to default" removes the override.

## Impact
- Affected specs: new `settings` capability
- Affected code:
  - shared: `pkg/settings` (API shapes), migration `009_add_settings_table.sql`
  - torrenter: `internal/torrenter/settings` (store + resolver), `service/torrent_score.go` (quality floor via resolver), `service/subtitles.go`, `service/plex_sync.go`, `handlers/settings_handler.go`, repository uploader CRUD
  - webserver: `clients/torrenter_settings.go`, `handlers/settings_handler.go`, routes
  - ui: `pages/Settings.tsx`, `hooks/useSettings.ts`, navbar gear
