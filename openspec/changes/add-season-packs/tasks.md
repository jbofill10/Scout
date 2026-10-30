## 1. Backend
- [x] 1.1 Settings `season_packs`, `season_pack_min_episodes` through the resolver; `PackInfo` on the strategy, `PackResults` on the completion event
- [x] 1.2 `torrent_pack.go`: pack title parser, pack strategies, validation and scoring, `searchForPack`
- [x] 1.3 `HandleDownload` tries packs per season when enabled and falls back per episode; in-flight detection and notifications cover pack episodes
- [x] 1.4 Media processor links every matching file of a pack; completion handler settles each episode's notification
- [x] 1.5 Tests: pack parser, validation, selection, grouping, processor linking, completion settlement

## 2. UI
- [x] 2.1 Downloads section: season packs toggle and minimum episodes

## 3. Verify
- [x] 3.1 `go vet`, `go test ./...`; `npm run lint`, `npx tsc -b`, `npm test`, `npm run build`
- [ ] 3.2 Deploy torrenter, webserver, ui; setting shows and stays off by default
