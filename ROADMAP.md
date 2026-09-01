# scheduler-cron — Remaining Work

### Events & Observability
- [x] Publish events on task lifecycle (`scheduler.task.*`)
- [x] Task timeout enforcement
- [x] Prometheus counters for fired/completed/failed/timeout

### Persistence & Advanced
- [x] Optional persistent schedule store via JSON file (`SCHEDULER_STORE_PATH`)
- [x] Missed schedule catch-up on restart
- [x] Configurable timezone (`SCHEDULER_TZ`)
- [x] One-shot tasks (non-recurring)
- [x] Integration test with running muxcored (`go test -tags=integration ./test`)
