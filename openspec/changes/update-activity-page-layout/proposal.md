# Change: Activity page that fits on one screen

## Why
The Activity page answered "what is Scout doing?" only after a lot of scrolling. Every tracked episode was a 110px row with a poster and a four-segment progress track, so a requested season was a screen and a half on its own. The sections were stacked (In flight, then up to ten Upcoming rows, then every Finished row with no cap: the endpoint returns up to 200), so Finished pushed to several thousand pixels. A future episode appeared twice, once as a `scheduled` notification under In flight and again as a schedule row under Upcoming. Above all that sat ~320px of header, indexer card and stage tiles. On a 1080p display four or five rows were visible at a time.

## What Changes
- **One tab at a time.** In flight, Upcoming and Finished become tabs with counts in their labels; only one list renders. Upcoming and Finished show twelve groups before a "Show all" control; Finished gains a "Clear finished" control.
- **One line per show.** Rows in a tab are grouped by show (category plus title). A group is a single 56px line: poster, title, season range, episode count, a per-stage count with a proportional bar, the most recent episode's code and status, and its time. Opening it lists the episodes as 40px rows with code, stage chip, status, time, trace prefix and the Cancel / Retry now / Pick release actions. A movie or a lone episode is a single 52px line. Expand all / Collapse all per tab.
- **Each item once.** A future episode shows only as its schedule row (under Upcoming, with its release time and Cancel); a retry wait shows only as its `searching` notification (under In flight, with "Retrying in …"). A `scheduled` notification with no row still shows under In flight.
- **Tiles are shortcuts.** The four stage tiles shrink to one row (count beside the label) and are buttons: Downloading and Searching open In flight filtered to that stage, Scheduled opens Upcoming, Failed opens Finished filtered to failed. The filter is a removable chip; the tile reads as pressed.
- **Indexer strip.** The indexer card becomes one line (indexer chips, a short relay summary, Rotate relay, a chevron); the full relay sentence, recent events and the per-indexer search queue from `add-search-gate` (only busy indexers) sit behind the chevron, with a "1 searching · 3 waiting" summary on the strip while anything is queued.
- **Status line.** A failed item the webserver has re-armed (PR #66) says "… · retrying in 1h" instead of reading as given up. Retry waits show the retry time, attempt number and last reason together.
- The stage progress track per row is dropped; the group bar and the stage chip carry that information.
- `useClearFinished` also invalidates the activity query so the Finished tab empties when the bell's or the page's Clear finished runs.
- No backend changes. The page asks `/api/activity` for 1000 notifications instead of the default 200 (grouping makes a long list cheap, and the default dropped the oldest in-flight episodes and most of Finished once a few seasons were queued), stops reading its `counts`, and counts its own rows so tiles and tabs always agree.

## Impact
- Affected specs: `ui-notifications` (Download Activity Page; Upcoming section from add-download-controls), `ui-layout` (Activity page on a phone), `indexer-health` (the card becomes a strip)
- Affected code: `ui/src/pages/Activity.tsx` (rewritten), new `ui/src/components/ui/ActivityList.tsx` and `ui/src/utils/activityRows.ts` (view model: flatten, dedupe, group), `ui/src/components/ui/IndexerHealthCard.tsx` (strip), `ui/src/utils/downloadStage.ts` (`formatEpisodeCode`), `ui/src/hooks/useNotifications.ts`, `ui/src/hooks/useActivity.ts` (limit)
- Builds on `add-download-controls` (actions, Upcoming) and `add-indexer-health` (card); both are merged, archive them first or together
