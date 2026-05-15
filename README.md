# scheduler-cron

Cron-based periodic task scheduler for MuxCore.

Uses `robfig/cron` for schedule parsing and execution. Provides reliable periodic task scheduling with standard cron expressions.

## Env Vars

- `MUXCORE_SCHEDULER_CRON_TIMEZONE` — Timezone for schedule evaluation (default: `UTC`)

## Capabilities

- `scheduler.cron` — Cron expression schedule parser and executor
- `scheduler.periodic` — Periodic task scheduling provider

## Usage

```go
import "github.com/Muxcore-Media/scheduler-cron"

mod := cron.NewModule()
mgr.Register(mod, nil)
```
