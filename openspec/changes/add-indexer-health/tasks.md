## 1. Host relay agent
- [x] 1.1 `tools/relay-agent/relay_agent.py` (status, relays, rotate with pool/avoid/cooldown), unit tests, systemd user unit, README
- [x] 1.2 Installed on the host as `scout-relay-agent` with a generated token; token shared with Scout's `.env`

## 2. Torrenter
- [x] 2.1 `RelayAgentClient`; config `RELAY_AGENT_URL`, `RELAY_AGENT_TOKEN`, `RELAY_ROTATE_COOLDOWN`, `INDEXER_HEALTH_INTERVAL`
- [x] 2.2 `IndexerHealthMonitor`: Prowlarr poll, IP-block classification, rotation with cooldown, re-test, events, snapshot
- [x] 2.3 Setting `auto_rotate_relay`; `options.relay_agent_configured`
- [x] 2.4 Routes `GET /indexers/health`, `POST /indexers/rotate`, `POST /indexers/:id/test`; webserver relays them
- [x] 2.5 Tests: monitor decisions (healthy, blocked → rotate + re-test, cooldown, auto-rotate off, no agent, other causes), client

## 3. UI
- [x] 3.1 `useIndexerHealth` hooks; `IndexerHealthCard` on Activity; Indexers and relay section on Settings

## 4. Verify
- [x] 4.1 `go vet`, `go test ./...`; `npm run lint`, `npx tsc -b`, `npm test`, `npm run build`
- [ ] 4.2 Deploy torrenter, webserver, ui; health shows the relay; a manual rotation moves the exit IP and indexers stay enabled
