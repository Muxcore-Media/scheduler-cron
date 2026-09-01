# Compatibility

## Core Version

Requires MuxCore **v0.5.8** or later (`go.mod` replace pin).

## Capabilities

Registers with capabilities:

- `scheduler` — discovery + `contracts.Scheduler` ModuleMesh methods
- `scheduler.cron` — cron-specific implementation tag
- `settings` — live timezone / persistence / catch-up settings

## Contract Dependencies

- `github.com/Muxcore-Media/core/pkg/contracts` — `Scheduler`, `SettingsProvider`
- `github.com/Muxcore-Media/contracts-media/events` — `scheduler.task.*` event constants
- gRPC ModuleRegistration service for sidecar registration
- gRPC ModuleMesh for Schedule/Cancel/Status/List and settings
- gRPC DiscoveryService for module discovery

## Persistence

Task schedules persist to a **JSON file** when `SCHEDULER_STORE_PATH` is set.
There is no DatabaseProvider integration.
