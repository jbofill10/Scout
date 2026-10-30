## ADDED Requirements

### Requirement: Indexer health is visible
The torrenter SHALL poll Prowlarr for indexers and their disabled state at least every minute and expose `GET /indexers/health` with each indexer's state, the host's exit relay as reported by the relay agent, recent events and whether automatic rotation is on. The Activity page SHALL show it.

#### Scenario: Disabled indexer
- **WHEN** Prowlarr has disabled an indexer until a future time
- **THEN** the health shows it as disabled with the time, and marks it IP-blocked unless Prowlarr's health text names another cause

### Requirement: Automatic relay rotation
When a relay agent is configured and `auto_rotate_relay` is on, the torrenter SHALL rotate the host's relay once an enabled indexer is disabled by an IP block, at most once per `RELAY_ROTATE_COOLDOWN`, and SHALL then re-test every disabled indexer so Prowlarr re-enables it.

#### Scenario: Cloudflare block
- **WHEN** 1337x is disabled after 1006 responses
- **THEN** the relay changes to a pool relay on a different /24, the indexer test is run, and the events record both

#### Scenario: Cooldown
- **WHEN** the indexer is disabled again within the cooldown
- **THEN** no second rotation happens

### Requirement: Manual control
`POST /indexers/rotate` SHALL rotate on request (409 when the agent refuses, 503 without an agent) and `POST /indexers/:id/test` SHALL run Prowlarr's test for one indexer; the Activity card SHALL offer both.

#### Scenario: Rotate from the UI
- **WHEN** the user confirms Rotate relay
- **THEN** a toast reports the old and new relay and the card shows the new exit IP

### Requirement: Host relay agent
`tools/relay-agent` SHALL expose `GET /status`, `GET /relays` and `POST /rotate` behind a bearer token, pick relays only from `RELAY_POOL`, never from `RELAY_AVOID`, prefer a different /24 and the least recently used relay, and refuse to rotate within `RELAY_COOLDOWN_S`.

#### Scenario: Pool exhausted
- **WHEN** every pool relay is excluded or avoided
- **THEN** `/rotate` answers 409 and nothing changes
