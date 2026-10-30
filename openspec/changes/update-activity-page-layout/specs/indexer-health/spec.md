## MODIFIED Requirements

### Requirement: Indexer health is visible
The torrenter SHALL poll Prowlarr for indexers and their disabled state at least every minute and expose `GET /indexers/health` with each indexer's state, the host's exit relay as reported by the relay agent, recent events and whether automatic rotation is on. The Activity page SHALL show it as a single-line strip: one chip per indexer, a short relay summary (relay name, exit IP, "tunnel down" when disconnected), the Rotate relay control and a chevron; the full relay sentence (location, last rotation and its reason, whether automatic rotation is on) and the recent events SHALL sit behind the chevron, together with one line per indexer whose search queue is busy (a search running, searches waiting, or paused), as "<indexer>: searching … · N waiting · x/y per min"; when any queue is busy the strip SHALL add a short "N searching · M waiting" summary.

#### Scenario: Disabled indexer
- **WHEN** Prowlarr has disabled an indexer until a future time
- **THEN** the health shows it as disabled with the time, and marks it IP-blocked unless Prowlarr's health text names another cause

#### Scenario: Busy search queue
- **WHEN** an indexer has a search running or searches waiting
- **THEN** the strip shows the searching and waiting totals, and the details list that indexer's queue line; idle indexers get no line

#### Scenario: Healthy setup costs one line
- **WHEN** every indexer is healthy and the relay is connected
- **THEN** the strip renders as one line with the chips, the relay summary and the controls, and nothing else until the chevron is opened
