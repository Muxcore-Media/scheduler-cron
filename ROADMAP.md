# scheduler-cron — Implementation Roadmap

**Priority:** P0 — Required for periodic/automated tasks.

## Phases

### Phase 1: Core Scheduling (minimum viable) ✅
- [x] Project scaffold
- [x] `go mod init` with core + robfig/cron
- [x] In-memory cron schedule store (`internal/cronstore`)
- [x] HTTP API: schedule, cancel, status, list (`internal/server`)
- [x] Sidecar entry point (`cmd/module/main.go`)
- [x] Unit tests: schedule store (10+ tests)
- [x] Unit tests: HTTP API (6 test scenarios)
- [x] Timezone support

### Phase 2: Events & Observability
- [ ] Publish events on task lifecycle (`scheduler.task.*`)
- [ ] Task timeout enforcement
- [ ] Prometheus metrics (scheduled count, executions)
- [ ] Health endpoint

### Phase 3: Persistence & Advanced
- [ ] Optional persistent schedule store via DatabaseProvider
- [ ] Missed schedule catch-up on restart
- [ ] One-shot tasks (non-recurring)
- [ ] Integration test with running muxcored

## Design Decisions

1. **robfig/cron** — Battle-tested, pure-Go cron library. No CGO.
2. **Event-driven execution** — Scheduler publishes events; executor modules
   subscribe. This decouples scheduling from execution.
3. **In-memory store for MVP** — Schedules are lost on restart. Optional
   DatabaseProvider support for persistence is Phase 3.
4. **30-second tick** — Adequate for cron tasks. One-minute resolution is the
   standard for cron. 30s tick gives 30s leeway for task dispatch.
