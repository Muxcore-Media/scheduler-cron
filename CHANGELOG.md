# Changelog


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
