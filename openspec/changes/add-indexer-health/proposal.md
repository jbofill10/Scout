# Change: Indexer health with automatic Mullvad relay rotation

## Why
When 1337x's Cloudflare front bans the exit IP, Prowlarr disables the indexer and every search silently falls through to the fallback indexer for hours. Nothing in Scout shows this, and the fix (switch the host's Mullvad relay, re-test the indexer) is manual.

## What Changes
- A host-side relay agent (`tools/relay-agent`, standard-library Python, systemd user unit) that exposes the Mullvad CLI over an authenticated HTTP API: status, relay pool, rotate (least recently used relay on a different /24, avoiding known-banned blocks).
- A torrenter monitor that polls Prowlarr's indexer status every minute, classifies a disabled indexer as IP-blocked unless Prowlarr names another cause, and, when the agent is configured and the `auto_rotate_relay` setting is on, rotates the relay (at most once per `RELAY_ROTATE_COOLDOWN`, default 30m) and re-tests the disabled indexers so Prowlarr re-enables them. Manual rotation and per-indexer tests are available too.
- Endpoints `GET /indexers/health`, `POST /indexers/rotate`, `POST /indexers/:id/test` on the torrenter, relayed by the webserver.
- UI: an Indexers card at the top of the Activity page (per-indexer state, exit relay and IP, last rotation, Rotate relay, recent events) and an "Indexers and relay" toggle on Settings.

## Impact
- Affected specs: new `indexer-health` capability; `settings`
- Affected code: `tools/relay-agent/*`; torrenter `clients/relay_agent.go`, `service/indexer_health.go`, `handlers/indexer_handler.go`, config, settings; webserver relay routes; UI `hooks/useIndexerHealth.ts`, `components/ui/IndexerHealthCard.tsx`, Settings; compose and env templates
