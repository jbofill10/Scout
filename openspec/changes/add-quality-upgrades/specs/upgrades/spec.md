## ADDED Requirements

### Requirement: Upgrade candidates
The torrenter SHALL list library episodes and movies whose best copy is below `upgrade_cutoff` (Plex resolutions sd/480 → 480p, 720 → 720p, 1080 → 1080p, 4k → 2160p), excluding items attempted within the last seven days, items that have failed three times, and items with a torrent in flight.

#### Scenario: Mixed versions
- **WHEN** an episode has a 720p and a 1080p file and the cutoff is 1080p
- **THEN** it is not a candidate

### Requirement: Upgrade pass
When `upgrades_enabled` is on the torrenter SHALL, every `UPGRADE_CHECK_INTERVAL` and on `POST /upgrades/run`, take up to `upgrade_batch` candidates and search for each with a quality window above its current copy and at most the cutoff, recording an attempt per item and a notification with the reason "Upgrading from <quality>".

#### Scenario: Release found
- **WHEN** a 1080p release is picked for a 720p episode
- **THEN** it downloads through the normal pipeline and, when it lands, the old 720p file is removed from the library folder before the new file is linked and Plex is re-synced

#### Scenario: Nothing better
- **WHEN** no release above the current copy exists
- **THEN** the attempt is recorded as no_release and the item is retried after a week

#### Scenario: Off by default
- **WHEN** `upgrades_enabled` is false
- **THEN** the pass does nothing and the Settings page says so

### Requirement: Visibility
`GET /upgrades/candidates` SHALL return the current candidates, `GET /upgrades/attempts` the recent attempts, and the Settings page SHALL show the count, the recent attempts and a Run now button.

#### Scenario: Manual run
- **WHEN** the user presses Run now with upgrades enabled
- **THEN** a batch starts and the response says how many searches were started
