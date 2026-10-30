## 1. View model
- [x] 1.1 `utils/activityRows.ts`: flatten notifications and schedule rows into one row shape with code, status line and tone; `buildViews` dedupes across In flight / Upcoming / Finished; `groupRows` folds rows by show with stage counts, season label and lead row
- [x] 1.2 `formatEpisodeCode` ("S03E01", "E12") in `utils/downloadStage.ts`
- [x] 1.3 Failed rows with a pending re-armed retry say "retrying in …"

## 2. Components
- [x] 2.1 `ActivityList`: group lines (counts, bar, lead status, disclosure), leaf lines, child rows, actions, Show all, per-tab empty state
- [x] 2.2 `IndexerHealthCard` as a one-line strip with the relay sentence and events behind a chevron
- [x] 2.3 `Activity` page: compact tiles that filter, tabs with counts, filter chip, Expand all / Collapse all, Clear finished, skeleton, error state
- [x] 2.4 `useClearFinished` invalidates `['activity']`
- [x] 2.5 `useActivity` requests `?limit=1000` (server default 200 dropped the oldest in-flight episodes and most of Finished once a few seasons were queued)

## 3. Tests and checks
- [x] 3.1 `activityRows.test.ts`: codes, status lines, dedupe rules, grouping order and counts
- [x] 3.2 `Activity.test.tsx`: grouping and disclosure, tab and tile counts, tile → filtered tab and back, Expand all, Show all cap, Upcoming groups, indexer strip; `useNotifications.test.tsx`: Clear finished refreshes activity
- [x] 3.3 `npm run lint`, `npx tsc -b`, `npm test`, `npm run build`
- [x] 3.4 Deploy ui (2026-10-30; live snapshot of 229 in-flight rows folds to a handful of show lines)
- [ ] 3.5 Eyeball on a phone: the tabs span the full width
