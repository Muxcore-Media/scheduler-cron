# Changelog

## [0.1.1] — 2026-08-09

### Added

- Lifecycle events: `scheduler.task.fired`, `.completed`, `.failed`, `.timeout`
- Per-task webhook `timeout` (Go duration) on `/schedule`
- One-shot tasks via `"once": true` or `cron_expr: "@once"`
- `SCHEDULER_TZ` for cron evaluation timezone (default UTC)

## [0.1.0] — Unreleased

### Added

- Cron store backed by `robfig/cron/v3` (5-field expressions + descriptors).
- HTTP API: `/schedule`, `/cancel/{id}`, `/status/{id}`, `/list`, `/health`, `/metrics`.
- Sidecar module registration with capability `scheduler`.
- Unit tests for cronstore and HTTP server; integration test scaffold (`-tags=integration`).
