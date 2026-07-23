# Changelog

## [0.1.0] — Unreleased

### Added

- Cron store backed by `robfig/cron/v3` (5-field expressions + descriptors).
- HTTP API: `/schedule`, `/cancel/{id}`, `/status/{id}`, `/list`, `/health`, `/metrics`.
- Sidecar module registration with capability `scheduler`.
- Unit tests for cronstore and HTTP server; integration test scaffold (`-tags=integration`).
