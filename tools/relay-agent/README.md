# Scout relay agent

Indexers behind Cloudflare (1337x in particular) ban the exit IP after a burst
of searches. Scout's host egresses through the Mullvad app, so the fix is to
move to another relay. The Mullvad CLI is not reachable from inside a
container, so this tiny host-side service does it on the torrenter's behalf.

`relay_agent.py` is standard-library Python 3 and drives the Mullvad app's CLI
(2023 or later: `mullvad status --json` and `mullvad relay set location
<hostname>`; written against 2026.5). It listens on `RELAY_AGENT_BIND`
(default `0.0.0.0:18642`), requires `Authorization: Bearer $RELAY_AGENT_TOKEN`,
and offers:

| Method | Path | What it does |
| --- | --- | --- |
| GET | `/status` | tunnel state, current relay, exit IP, rotation history, relays skipped as dead |
| GET | `/relays` | WireGuard relays in the pool, minus avoided ones |
| POST | `/rotate` | `{"exclude": [...], "reason": "..."}`: picks the least recently used relay in the pool on a different /24 than the current one, sets it and waits for the tunnel; a relay that stays down is skipped for an hour and the next one is tried, up to `RELAY_ATTEMPTS`; 409 on cooldown or an empty pool, 502 when none came up (the previous relay is put back) |

Environment: `RELAY_AGENT_TOKEN` (required), `RELAY_POOL` (comma list of `cc`
or `cc/city`, default the current country), `RELAY_AVOID` (hostnames or IP
prefixes such as `146.70.198.` that are known to be banned), `RELAY_COOLDOWN_S`
(default 60), `RELAY_ATTEMPTS` (default 2), `RELAY_CONNECT_S` (seconds to wait
for each relay, default 15; two attempts must fit the torrenter's 45 s call),
`RELAY_DEAD_S` (default 3600).

Install with the steps at the top of `scout-relay-agent.service`. Run the unit
tests with `cd tools/relay-agent && python3 -m unittest test_relay_agent`.

The torrenter uses it when `RELAY_AGENT_URL` and `RELAY_AGENT_TOKEN` are set:
the indexer health monitor polls Prowlarr, and when an indexer is disabled by
an IP block it rotates the relay (at most once per `RELAY_ROTATE_COOLDOWN`,
default 30m) and re-tests the indexer so Prowlarr re-enables it. The Activity
page shows the state and offers a manual rotation.
