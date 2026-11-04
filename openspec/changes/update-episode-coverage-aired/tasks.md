# Tasks

## 1. Backend

- [x] 1.1 `MediaStatusInfo.Aired`; `calculateEpisodeCountsFromMetadata` returns downloaded/aired/total with a
  `now`; `episodeHasAired`; wired into the enriched search and popular interactors; tests
- [x] 1.2 `MetadataStatus.UpcomingCount`; `GetTvdbMetadataStatus` counts aired and upcoming in one query;
  `GetShowEpisodesWithStatus` lists unaired episodes too; mocks; interactor test

## 2. UI

- [x] 2.1 `utils/airDates.ts` and `utils/episodeCoverage.ts` with tests
- [x] 2.2 `EpisodeCoverage` badge type; `statusHelpers` builds it from `aired` (falling back to `total`)
- [x] 2.3 `MediaCard` pill `downloaded/aired`, complete once everything aired is in, sentence as tooltip
- [x] 2.4 `MediaStatusDialog` header sentence, aired-based progress, season tab labels, Upcoming/TBA rows,
  banner suffix; test
- [x] 2.5 Library page: card from `tvdbEpisodeCount`/`upcomingCount`, dialog status from the episode list
