## ADDED Requirements

### Requirement: Notification targets
The webserver SHALL store notification targets (name, kind `webhook` | `ntfy` | `discord`, url, optional token, events, enabled) and expose `GET/POST /notification-targets`, `PUT/DELETE /notification-targets/:id` and `POST /notification-targets/:id/test`.

#### Scenario: Test delivery
- **WHEN** the user tests a target
- **THEN** a sample "completed" event is sent to it and the response reports success or the delivery error

### Requirement: Terminal outcomes are delivered once
The webserver SHALL deliver every transition of a notification to `completed`, `failed` or `cancelled` to each enabled target whose events include it, exactly once per transition, within fifteen seconds, and SHALL NOT replay notifications that were terminal before the feature was deployed.

#### Scenario: Download lands
- **WHEN** a notification becomes `completed`
- **THEN** each target wanting `completed` receives a message naming the show or movie and the episode, and the notification is marked delivered for that status

#### Scenario: Retry after failure
- **WHEN** a notification goes `failed`, then later `completed`
- **THEN** both transitions are delivered, each once

### Requirement: Payloads per kind
A `webhook` target SHALL receive a JSON body with event, status, title, message, tvdb id, media title, category, season, episode, poster and reason, with `Authorization: Bearer <token>` when a token is set; an `ntfy` target SHALL receive the message as the body with Title, Tags and Priority headers; a `discord` target SHALL receive `{"content": ...}`.

#### Scenario: Home Assistant
- **WHEN** a `webhook` target points at a Home Assistant webhook URL
- **THEN** the automation receives the JSON fields under `trigger.json`
