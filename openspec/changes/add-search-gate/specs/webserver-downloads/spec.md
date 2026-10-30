## ADDED Requirements

### Requirement: Request sources
Download requests SHALL carry a source: `manual` for requests made from the UI (including Retry now, release listings and manual grabs), `scheduled` for an episode dispatched on its air date, `follow` for follow checks and `missing` for missing-episode fills. A retry SHALL keep the source of the request it came from.

#### Scenario: Air date beats back catalogue
- **WHEN** a followed show's new episode airs while its back catalogue is being filled
- **THEN** the new episode is dispatched as `scheduled` and searched before the `follow` back-catalogue episodes

### Requirement: Follow check budget
A scheduled follow check SHALL request at most `bulk_request_batch` episodes per run across all followed shows, visiting the least recently checked shows first and the oldest wanted episodes first, and SHALL report the episodes it deferred. A check a person starts (Check now, Follow) SHALL not be capped.

#### Scenario: Budget of ten
- **WHEN** two followed shows each have thirty missing episodes and the budget is ten
- **THEN** the run requests ten episodes of the show checked least recently and reports fifty deferred
