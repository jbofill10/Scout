## ADDED Requirements

### Requirement: Runtime settings override environment defaults
The torrenter SHALL resolve the quality floor, subtitle languages, subtitle SDH preference, Plex sync delay and Plex sync interval from the `Settings` table first and the environment second, and SHALL apply a change within fifteen seconds without a restart.

#### Scenario: Override applies to the next pick
- **WHEN** the quality floor is set to 720p through the API while the environment says 1080p
- **THEN** the next torrent selection accepts a 720p release

#### Scenario: Reset returns to the default
- **WHEN** a key is reset
- **THEN** its override row is removed and the environment value is in effect again

### Requirement: Settings API
The torrenter SHALL expose `GET /settings` returning effective values, defaults, which keys are overridden and the valid quality options, and `PUT /settings` accepting any subset of keys plus a `reset` list. The webserver SHALL proxy both under `/api/settings`.

#### Scenario: Validation
- **WHEN** a PUT carries an unknown quality label, a negative duration, or a language that is not a two or three letter code
- **THEN** the response is 400 naming the field and nothing is written

### Requirement: Preferred uploaders are editable
The torrenter SHALL expose `GET /settings/uploaders`, `POST /settings/uploaders` (`media_type`, `is_anime`, `uploader`) and `DELETE /settings/uploaders/:id`, backed by the `UploaderPreferences` table the picker already reads.

#### Scenario: Duplicate uploader
- **WHEN** an uploader is added twice for the same media type and anime flag
- **THEN** the second add responds 409 and the list still has one row

### Requirement: Settings page
The UI SHALL offer a Settings page from a gear in the navbar with Downloads (quality floor), Preferred uploaders, Subtitles (languages, prefer SDH, disabled notice when no API key) and Library sync (delay, interval) sections. Each value SHALL show its environment default and whether it is overridden, with a way to reset it.

#### Scenario: Save and reset
- **WHEN** the user changes the quality floor and saves
- **THEN** the page shows the new value as overridden and a toast confirms; resetting it shows the default again
