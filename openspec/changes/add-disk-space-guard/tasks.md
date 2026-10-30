## 1. Backend
- [x] 1.1 `dlstatus.CodeInsufficientDiskSpace` (transient)
- [x] 1.2 Setting `min_free_space_gb` (env `MIN_FREE_SPACE_GB`, default 10) through the resolver; `options.free_space_bytes` in the settings view
- [x] 1.3 `service/diskspace.go`: free-space probe and the fit check; applied in `initiateDownloadAndMonitor`
- [x] 1.4 Tests: fit check, resolver validation, download path returns the code without touching qBittorrent

## 2. UI
- [x] 2.1 Downloads section: reserve field with the current free space

## 3. Verify
- [x] 3.1 `go vet`, `go test ./...`; `npm run lint`, `npx tsc -b`, `npm test`, `npm run build`
- [ ] 3.2 Deploy torrenter, webserver, ui; settings show the free space
