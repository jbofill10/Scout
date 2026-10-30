## 1. Torrenter
- [x] 1.1 `searchgate` package: per-indexer token bucket, one in flight, priority queue, pause/resume, snapshot, request context
- [x] 1.2 `searchProwlarr` acquires through the gate; requests tagged with priority and label; paused indexers dropped from a search
- [x] 1.3 Settings `search_rate_per_minute`, `search_burst`, `indexer_rate_limits`, `bulk_request_batch`; env defaults
- [x] 1.4 Health monitor pauses and resumes indexers on the gate; `/indexers/health` carries the queue
- [x] 1.5 Tests: gate semantics, paused indexer falls through, gate accounting, resolver, health sync

## 2. Webserver
- [x] 2.1 `Media.Source` set for manual, scheduled, follow, missing requests; retries keep theirs
- [x] 2.2 Follow check budget from `bulk_request_batch`, least recently checked first, deferred count
- [x] 2.3 Tests: budget across shows, uncapped Check now

## 3. UI
- [x] 3.1 Settings: Search pacing section (rate, burst, per-indexer overrides, follow batch)
- [x] 3.2 Indexer health types carry the queue; `describeQueue` helper for the card

## 4. Verify
- [x] 4.1 `go vet ./... && go test ./...`; `npm run lint && npx tsc -b && npm test && npm run build`
- [ ] 4.2 Deploy torrenter + webserver + ui; settings show the new keys; health shows `queue`
