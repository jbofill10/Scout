## MODIFIED Requirements

### Requirement: Torrent monitor gives up on stalled downloads
The torrenter SHALL give up on a download only when it has made no progress for `MONITOR_TIMEOUT` (default 6h), or sooner when the swarm is dead. A download that keeps making progress SHALL be watched until it completes regardless of how long it has run. A monitor resumed after a restart SHALL measure the stall window from its own start. A download given up on SHALL be removed from qBittorrent, remembered so it is not picked again, and replaced through the same path as a dead swarm.

#### Scenario: Slow but moving
- **WHEN** a 61 GB torrent is still at 89% after 11 hours with seeds and transfer
- **THEN** the monitor keeps watching it and links it when it completes

#### Scenario: Stalled with seeds
- **WHEN** a torrent's progress has not changed for 6 hours
- **THEN** it is removed, recorded as failed for the item, and another copy is searched for; if none is found the item is marked failed with code `no_torrent_found`

### Requirement: Failure codes on notifications
Every failure the torrenter writes to a notification on its own (lost monitor, torrent removed from qBittorrent, no replacement found, processing error, episode missing from a pack) SHALL carry the `dlstatus` failure code in `failure_code`. Any transition to a non-failed status SHALL clear it.

#### Scenario: Lost download
- **WHEN** the monitor can no longer see a torrent in qBittorrent
- **THEN** the notification reads failed with `failure_code = monitor_lost`
