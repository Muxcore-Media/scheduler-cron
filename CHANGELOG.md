# Changelog

## [0.1.10] - 2026-10-05


### Security
- NFR-SEC-009 / RULE-VAL-2: task `webhook_url` (from `meta` or `payload`) is validated at `/schedule` (http/https only; private, loopback, link-local, metadata targets and userinfo rejected) and fired through a netguard `UserURL` client (dial-time IP checks, redirect re-validation, DNS-rebinding safe), so restored tasks are guarded too. The webhook client no longer has an unbounded (zero) timeout. Built on sdk/go/module v0.6.6.

## [0.1.9] - 2026-10-05

### Changed
- Built on core v0.6.14 / sdk/go/module v0.6.4: unregisters on shutdown and re-registers after core restarts (ADR-0022).

## [0.1.8] - 2026-10-05


### Changed
- Reported version comes from muxcore.json (ADR-0021); built on core v0.6.12 / sdk/go/module v0.6.3 (mesh enrollment, ADR-0017).

## [0.1.7] - 2026-10-05


### Security
- NFR-SEC-011 / T-M3-07: default HTTP listen address is now `127.0.0.1:9200` (was `:9200`). A non-loopback `SCHEDULER_HTTP_ADDR` requires `SCHEDULER_HTTP_TOKEN` and the module refuses to start without it. With a token set, all endpoints except `GET /health` (and the multiplexed gRPC settings service) require `Authorization: Bearer <token>` (constant-time compare). No default token.

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
