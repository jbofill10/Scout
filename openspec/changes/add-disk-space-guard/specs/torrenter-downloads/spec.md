## ADDED Requirements

### Requirement: Disk space is checked before a torrent is added
The torrenter SHALL NOT add a torrent when the downloads volume's free space is less than the release size plus the configured reserve, and SHALL report the item with the transient code `insufficient_disk_space`.

#### Scenario: Release does not fit
- **WHEN** a 20 GB release is picked with 25 GB free and a 10 GB reserve
- **THEN** nothing is added to qBittorrent, the notification reads that the disk is full and a retry will follow, and the result code is `insufficient_disk_space`

#### Scenario: Unknown size
- **WHEN** the indexer reports no size
- **THEN** only the reserve is required

#### Scenario: Probe failure
- **WHEN** free space cannot be read
- **THEN** the download proceeds and the failure is logged
