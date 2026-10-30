## ADDED Requirements

### Requirement: Cancel in-flight work for an item
The torrenter SHALL expose `POST /downloads/cancel` with `{"item_id"}` and SHALL stop any search or download for that item.

#### Scenario: Cancel during search
- **WHEN** a search for the item is running
- **THEN** the search context is cancelled and the item's result carries code `cancelled`

#### Scenario: Cancel during download
- **WHEN** a torrent for the item is being monitored
- **THEN** the monitor stops without reporting a failure, the torrent and its files are deleted from qBittorrent, the `ActiveTorrents` row is removed, and the notification becomes `cancelled`

#### Scenario: Monitor resumed after restart
- **WHEN** the monitored torrent was resumed from `ActiveTorrents` at startup
- **THEN** cancelling it behaves the same as for a monitor started in this process

### Requirement: List candidate releases
The torrenter SHALL expose `POST /releases` with `{"item_id", "media"?}` returning every candidate found for the item across all indexer tiers.

#### Scenario: Accepted and rejected candidates
- **WHEN** the search returns torrents
- **THEN** each candidate reports title, indexer, size, seeders, leechers, age, parsed quality, whether Scout accepts it, the rejection reason when it does not, its score when it does, whether it previously failed for this item, and the guid, download url, info hash and indexer id needed to grab it

#### Scenario: Media resolved from the active torrent
- **WHEN** no media is posted but the item has an `ActiveTorrents` row with a stored request
- **THEN** the search is built from that request

### Requirement: Download a chosen release
The torrenter SHALL expose `POST /downloads/manual` with `{"item_id", "media"?, "release"}` and SHALL download exactly that release.

#### Scenario: Manual grab
- **WHEN** a release is posted
- **THEN** any torrent already in flight for the item is cancelled first, the release is added to qBittorrent and monitored through the normal completion path, and the result reports `downloading`
