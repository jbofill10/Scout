## 1. Torrenter
- [x] 1.1 Repository `GetMissingEpisodes` (one query over Shows, TvdbEpisodes, Episodes) and `GET /library/missing`
- [x] 1.2 Tests: query mapping, handler

## 2. Webserver
- [x] 2.1 Client `GetMissingEpisodes`; `MissingInteractor` with requested-state marking and `FillMissing`
- [x] 2.2 Routes `GET /library/missing`, `POST /library/shows/:tvdbId/fill-missing`
- [x] 2.3 Tests: marking, fill excludes requested episodes, handler codes

## 3. UI
- [x] 3.1 `useMissing` hooks; Missing tab on Library with per-show expand and Download missing
- [x] 3.2 Tests: hooks

## 4. Verify
- [x] 4.1 `go vet`, `go test ./...`; `npm run lint`, `npx tsc -b`, `npm test`, `npm run build`
- [ ] 4.2 Deploy torrenter, webserver, ui; the Missing tab lists the known gaps
