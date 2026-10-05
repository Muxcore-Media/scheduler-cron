# Changelog


## [0.1.6] - 2026-10-05

### Changed
- CI runs on GitHub-hosted runners from the umbrella template; retired-origin workflows removed.
- Dependencies resolve from published GitHub tags (no filesystem `replace`); requires core v0.6.0.

## [0.1.5] — 2026-08-10

### Added

- SettingsProvider for `timezone` / `store_path` / `catch_up` (`SCHEDULER_TZ` / `SCHEDULER_STORE_PATH` / `SCHEDULER_CATCH_UP`)
- gRPC ModuleMesh settings multiplexed with HTTP via cmux on `SCHEDULER_HTTP_ADDR`
- Advertises `settings` capability for admin-ui discovery

## [0.1.4] — 2026-08-10

### Fixed
- Sync Info()/muxcore.json version to **0.1.4**.

## [0.1.3] — 2026-08-10

### Added

- Muxcored integration test (`go test -tags=integration ./test`): boots muxcored, registers scheduler, discovers by capability, fires `@once` webhook

## [0.1.2] — 2026-08-09

### Added

- Persistent JSON store via `SCHEDULER_STORE_PATH`
- Missed-fire catch-up on restore (disable with store `SetCatchUp(false)`)

## [0.1.1] — 2026-08-09

### Added

- Lifecycle events, task timeouts, `@once`, `SCHEDULER_TZ`

## [0.1.0]

### Added

- Initial cron HTTP scheduler
