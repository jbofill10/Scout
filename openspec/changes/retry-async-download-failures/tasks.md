## 1. Torrenter
- [x] 1.1 Monitor: stall-based give-up (no progress for MONITOR_TIMEOUT), replacement through the dead-swarm path, progress in the poll log
- [x] 1.2 `failure_code` on notifications for every asynchronous failure; cleared on other transitions
- [x] 1.3 Tests: slow-but-moving download completes, stalled download is given up on with the code, resumed monitor measures from now

## 2. Webserver
- [x] 2.1 Completed row recorded for every accepted direct request (`RecordDispatched`)
- [x] 2.2 Reconciler re-arms lost items with backoff, fails them after the ladder, rebuilds a row for a lost movie
- [x] 2.3 Controls: Retry/Pick release from a completed row; Cancel skips completed rows; Activity flags
- [x] 2.4 Tests: reconciler cases, repository SQL, controls

## 3. Verify
- [x] 3.1 `go vet ./... && go test ./...`
- [ ] 3.2 Deploy torrenter + webserver; the X notification gets a retry row within a pass; lost items show Retry
