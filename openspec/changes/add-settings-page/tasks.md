## 1. Storage and shared shapes
- [x] 1.1 Migration `009_add_settings_table.sql` (+ `setup-postgres.sql`) and a unique index on uploader preferences
- [x] 1.2 `pkg/settings`: Values, Update, View, Options, UploaderPreference

## 2. Torrenter
- [x] 2.1 `internal/torrenter/settings`: Store (key/value JSONB) and Resolver (env defaults + overrides, 15s cache, validation)
- [x] 2.2 Quality floor, subtitle languages / SDH and Plex sync delay / interval read through the resolver
- [x] 2.3 Repository: list / add / delete uploader preferences (+ mocks)
- [x] 2.4 Handler + routes: `GET|PUT /settings`, `GET|POST /settings/uploaders`, `DELETE /settings/uploaders/:id`
- [x] 2.5 Tests: resolver precedence and validation, dynamic sync interval, handler status codes

## 3. Webserver
- [x] 3.1 Torrenter client methods and a proxying `SettingsHandler`; routes under `/settings`
- [x] 3.2 Tests: handler passes validation errors through as 400

## 4. UI
- [x] 4.1 `useSettings`, `useUpdateSettings`, `useUploaderPreferences` hooks
- [x] 4.2 Settings page with Downloads, Preferred uploaders, Subtitles, Library sync sections; gear in the navbar
- [x] 4.3 Tests: hooks and the duration / language parsers

## 5. Verify
- [x] 5.1 `go vet`, `go test ./...`; `npm run lint`, `npx tsc -b`, `npm test`, `npm run build`
- [ ] 5.2 Deploy torrenter, webserver, ui; change the quality floor and confirm the next pick honours it
