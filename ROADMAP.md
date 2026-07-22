# scheduler-cron — Remaining Work

### Events & Observability
- [ ] Publish events on task lifecycle (`scheduler.task.*`)
- [ ] Task timeout enforcement
- [ ] Prometheus metrics (scheduled count, executions)
- [ ] Health endpoint

### Persistence & Advanced
- [ ] Optional persistent schedule store via DatabaseProvider
- [ ] Missed schedule catch-up on restart
- [ ] One-shot tasks (non-recurring)
- [ ] Integration test with running muxcored
