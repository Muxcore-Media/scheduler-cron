# scheduler-cron — Implementation Roadmap

**Priority:** P0 — Required for periodic/automated tasks.

## Phases

### Phase 1: Core Scheduling (minimum viable)
- [x] Project scaffold (this repo)
- [ ] `go mod init` with core dependency (+ `robfig/cron`)
- [ ] Cron expression parser (wrapping robfig/cron)
- [ ] In-memory schedule store
- [ ] `Scheduler.Schedule` — register a periodic task
- [ ] `Scheduler.Cancel` — remove a scheduled task
- [ ] `Scheduler.Status` — query next/last execution
- [ ] `Scheduler.List` — query all scheduled tasks
- [ ] Task execution loop (tick every 30s, fire due tasks)
- [ ] Sidecar entry point (`cmd/module/main.go`)
- [ ] Unit tests: cron parsing, schedule firing, edge cases (40+ tests)

### Phase 2: Events & Observability
- [ ] Publish events on task lifecycle (`scheduler.task.*`)
- [ ] Task timeout enforcement (from `SchedulerTask.Timeout`)
- [ ] Timezone support (`--timezone` flag)
- [ ] Prometheus metrics (scheduled task count, executions)
- [ ] Health endpoint
- [ ] Audit logging of schedule changes
- [ ] GitHub CI (build + lint + test)

### Phase 3: Persistence & Advanced
- [ ] Optional persistent schedule store via DatabaseProvider
- [ ] Missed schedule catch-up on restart (`--missed-startup`)
- [ ] One-shot tasks (non-recurring scheduled execution)
- [ ] Task execution history (last N results)
- [ ] Integration test with running muxcored

## Design Decisions

1. **robfig/cron** — Battle-tested, pure-Go cron library. No CGO.
2. **Event-driven execution** — Scheduler publishes events; executor modules
   subscribe. This decouples scheduling from execution.
3. **In-memory store for MVP** — Schedules are lost on restart. Optional
   DatabaseProvider support for persistence is Phase 3.
4. **30-second tick** — Adequate for cron tasks. One-minute resolution is the
   standard for cron. 30s tick gives 30s leeway for task dispatch.
